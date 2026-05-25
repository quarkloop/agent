use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, bail};
use async_nats::{Client, ConnectOptions};
use bytes::Bytes;
use futures::StreamExt;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sha2::{Digest, Sha256};

use crate::proto::harness::{
    ComposeContextRequest, DeleteMemoryRequest, GetContextReportRequest, GetMemoryRequest,
    PutMemoryRequest, SearchMemoryRequest, StreamContextReportsRequest,
};
use crate::service::Harness;

const VERSION: i32 = 1;
const QUEUE: &str = "q.service.v1.harness";

#[derive(Debug)]
pub struct TransportConfig {
    pub url: String,
    pub username: Option<String>,
    pub password: Option<String>,
    pub audit_prefix: Option<String>,
    pub telemetry_prefix: Option<String>,
}

#[derive(Clone, Copy)]
enum Operation {
    ComposeContext,
    GetContextReport,
    StreamContextReports,
    PutMemory,
    GetMemory,
    SearchMemory,
    DeleteMemory,
}

impl Operation {
    const fn subject(self) -> &'static str {
        match self {
            Self::ComposeContext => "svc.harness.v1.compose_context",
            Self::GetContextReport => "svc.harness.v1.get_context_report",
            Self::StreamContextReports => "svc.harness.v1.stream_context_reports",
            Self::PutMemory => "svc.harness.v1.put_memory",
            Self::GetMemory => "svc.harness.v1.get_memory",
            Self::SearchMemory => "svc.harness.v1.search_memory",
            Self::DeleteMemory => "svc.harness.v1.delete_memory",
        }
    }

    const fn function(self) -> &'static str {
        match self {
            Self::ComposeContext => "compose_context",
            Self::GetContextReport => "get_context_report",
            Self::StreamContextReports => "stream_context_reports",
            Self::PutMemory => "put_memory",
            Self::GetMemory => "get_memory",
            Self::SearchMemory => "search_memory",
            Self::DeleteMemory => "delete_memory",
        }
    }

    const fn streaming(self) -> bool {
        matches!(self, Self::StreamContextReports)
    }
}

const OPERATIONS: &[Operation] = &[
    Operation::ComposeContext,
    Operation::GetContextReport,
    Operation::StreamContextReports,
    Operation::PutMemory,
    Operation::GetMemory,
    Operation::SearchMemory,
    Operation::DeleteMemory,
];

#[derive(Clone, Deserialize, Serialize)]
struct RequestEnvelope {
    version: i32,
    service_call_id: String,
    space_id: String,
    #[serde(default)]
    session_id: String,
    #[serde(default)]
    agent_id: String,
    #[serde(default)]
    run_id: String,
    #[serde(default)]
    workflow_id: String,
    actor: String,
    payload: Value,
    #[serde(default)]
    traceparent: String,
}

#[derive(Serialize)]
struct ResponseEnvelope {
    version: i32,
    service_call_id: String,
    reference_id: String,
    audit_ref: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    trace_id: String,
    status: &'static str,
    #[serde(rename = "final", skip_serializing_if = "is_false")]
    final_: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    payload: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<ErrorPayload>,
}

#[derive(Serialize)]
struct ErrorPayload {
    boundary: &'static str,
    category: &'static str,
    operation: String,
    message: String,
}

#[derive(Serialize)]
struct ServiceCallEvent {
    #[serde(rename = "type")]
    kind: &'static str,
    service_call_id: String,
    reference_id: String,
    audit_ref: String,
    space_id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    session_id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    run_id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    workflow_id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    agent_id: String,
    service: &'static str,
    function: &'static str,
    subject: &'static str,
    status: &'static str,
    duration_millis: i64,
    #[serde(skip_serializing_if = "String::is_empty")]
    trace_id: String,
    recorded_at: String,
    retention_expires_at: String,
}

pub async fn run(config: TransportConfig, harness: Harness) -> anyhow::Result<()> {
    if config.url.trim().is_empty() {
        bail!("nats url is required");
    }
    let mut options = ConnectOptions::new().name("quark-harness");
    if let (Some(username), Some(password)) = (&config.username, &config.password) {
        options = options.user_and_password(username.clone(), password.clone());
    }
    let client = options
        .connect(&config.url)
        .await
        .context("connect harness NATS")?;
    let service = Arc::new(harness);
    let audit = Arc::new(Audit {
        client: client.clone(),
        durable_subject_prefix: config.audit_prefix.filter(|value| !value.trim().is_empty()),
        event_subject_prefix: config
            .telemetry_prefix
            .filter(|value| !value.trim().is_empty()),
    });
    let mut tasks = Vec::new();
    for operation in OPERATIONS {
        let subscription = client
            .queue_subscribe(operation.subject(), QUEUE.to_owned())
            .await
            .with_context(|| format!("subscribe {}", operation.subject()))?;
        tasks.push(tokio::spawn(serve(
            client.clone(),
            subscription,
            *operation,
            Arc::clone(&service),
            Arc::clone(&audit),
        )));
    }
    client
        .flush()
        .await
        .context("flush harness subscriptions")?;
    eprintln!("harness service ready queue={QUEUE}");
    tokio::signal::ctrl_c().await.context("wait for shutdown")?;
    for task in tasks {
        task.abort();
    }
    client.flush().await.context("flush harness shutdown")?;
    Ok(())
}

async fn serve(
    client: Client,
    mut subscription: async_nats::Subscriber,
    operation: Operation,
    harness: Arc<Harness>,
    audit: Arc<Audit>,
) {
    while let Some(message) = subscription.next().await {
        let Some(reply) = message.reply else {
            continue;
        };
        let started = Instant::now();
        let parsed = serde_json::from_slice::<RequestEnvelope>(&message.payload);
        let responses = match parsed {
            Ok(request) => {
                let responses = dispatch(operation, &harness, &request).await;
                if let Some(terminal) = responses.last()
                    && let Err(error) = audit
                        .record(&request, operation, terminal, started.elapsed())
                        .await
                {
                    eprintln!(
                        "harness audit error subject={} error={error}",
                        operation.subject()
                    );
                }
                responses
            }
            Err(error) => vec![failure(
                "",
                operation,
                format!("invalid request envelope: {error}"),
                true,
            )],
        };
        for response in responses {
            let data = match serde_json::to_vec(&response) {
                Ok(data) => data,
                Err(error) => {
                    eprintln!(
                        "harness encode error subject={} error={error}",
                        operation.subject()
                    );
                    break;
                }
            };
            if let Err(error) = client.publish(reply.clone(), Bytes::from(data)).await {
                eprintln!(
                    "harness reply error subject={} error={error}",
                    operation.subject()
                );
                break;
            }
        }
    }
}

async fn dispatch(
    operation: Operation,
    harness: &Harness,
    request: &RequestEnvelope,
) -> Vec<ResponseEnvelope> {
    if let Err(error) = validate_request(request) {
        return vec![failure(
            &request.service_call_id,
            operation,
            error,
            operation.streaming(),
        )];
    }
    let trace_id = trace_id(&request.traceparent);
    let result: anyhow::Result<Vec<Value>> = match operation {
        Operation::ComposeContext => parse(&request.payload)
            .and_then_async(|payload: ComposeContextRequest| harness.compose_context(payload))
            .await
            .and_then(values),
        Operation::GetContextReport => parse(&request.payload)
            .and_then_async(|payload: GetContextReportRequest| harness.get_context_report(payload))
            .await
            .and_then(values),
        Operation::StreamContextReports => {
            match parse::<StreamContextReportsRequest>(&request.payload) {
                Ok(payload) => harness
                    .stream_context_reports(payload)
                    .await
                    .and_then(|items| items.into_iter().map(to_value).collect()),
                Err(error) => Err(error),
            }
        }
        Operation::PutMemory => parse(&request.payload)
            .and_then_async(|payload: PutMemoryRequest| harness.put_memory(payload))
            .await
            .and_then(values),
        Operation::GetMemory => parse(&request.payload)
            .and_then_async(|payload: GetMemoryRequest| harness.get_memory(payload))
            .await
            .and_then(values),
        Operation::SearchMemory => parse(&request.payload)
            .and_then_async(|payload: SearchMemoryRequest| harness.search_memory(payload))
            .await
            .and_then(values),
        Operation::DeleteMemory => parse(&request.payload)
            .and_then_async(|payload: DeleteMemoryRequest| harness.delete_memory(payload))
            .await
            .and_then(values),
    };
    match result {
        Ok(values) if operation.streaming() => {
            let mut output: Vec<_> = values
                .into_iter()
                .map(|value| success(request, value, false, trace_id.clone()))
                .collect();
            output.push(success(request, Value::Null, true, trace_id));
            output
        }
        Ok(mut values) => vec![success(
            request,
            values.pop().unwrap_or(Value::Null),
            false,
            trace_id,
        )],
        Err(error) => vec![failure(
            &request.service_call_id,
            operation,
            error.to_string(),
            operation.streaming(),
        )],
    }
}

trait AsyncResultExt<T> {
    async fn and_then_async<U, F, Fut>(self, function: F) -> anyhow::Result<U>
    where
        F: FnOnce(T) -> Fut,
        Fut: std::future::Future<Output = anyhow::Result<U>>;
}

impl<T> AsyncResultExt<T> for anyhow::Result<T> {
    async fn and_then_async<U, F, Fut>(self, function: F) -> anyhow::Result<U>
    where
        F: FnOnce(T) -> Fut,
        Fut: std::future::Future<Output = anyhow::Result<U>>,
    {
        function(self?).await
    }
}

fn parse<T: serde::de::DeserializeOwned>(payload: &Value) -> anyhow::Result<T> {
    serde_json::from_value(payload.clone()).context("decode protobuf JSON payload")
}

fn values<T: Serialize>(value: T) -> anyhow::Result<Vec<Value>> {
    Ok(vec![to_value(value)?])
}

fn to_value<T: Serialize>(value: T) -> anyhow::Result<Value> {
    serde_json::to_value(value).context("encode protobuf JSON payload")
}

fn validate_request(request: &RequestEnvelope) -> Result<(), String> {
    if request.version != VERSION {
        return Err(format!(
            "unsupported nats request envelope version {}",
            request.version
        ));
    }
    if request.service_call_id.trim().is_empty() {
        return Err("service_call_id is required".into());
    }
    if request.space_id.trim().is_empty() {
        return Err("space_id is required".into());
    }
    if !matches!(
        request.actor.as_str(),
        "user" | "agent" | "runtime" | "workflow" | "supervisor"
    ) {
        return Err(format!("invalid actor {:?}", request.actor));
    }
    Ok(())
}

fn success(
    request: &RequestEnvelope,
    payload: Value,
    final_: bool,
    trace_id: String,
) -> ResponseEnvelope {
    response(
        &request.service_call_id,
        "ok",
        Some(payload),
        None,
        final_,
        trace_id,
    )
}

fn failure(
    service_call_id: &str,
    operation: Operation,
    message: String,
    final_: bool,
) -> ResponseEnvelope {
    let service_call_id = if service_call_id.trim().is_empty() {
        "call-invalid-request"
    } else {
        service_call_id
    };
    response(
        service_call_id,
        "error",
        None,
        Some(ErrorPayload {
            boundary: "service",
            category: "invalid_argument",
            operation: operation.subject().into(),
            message,
        }),
        final_,
        String::new(),
    )
}

fn response(
    service_call_id: &str,
    status: &'static str,
    payload: Option<Value>,
    error: Option<ErrorPayload>,
    final_: bool,
    trace_id: String,
) -> ResponseEnvelope {
    let reference_id = reference_id(service_call_id);
    ResponseEnvelope {
        version: VERSION,
        service_call_id: service_call_id.into(),
        audit_ref: format!("service-call/{reference_id}"),
        reference_id,
        trace_id,
        status,
        final_,
        payload,
        error,
    }
}

fn reference_id(service_call_id: &str) -> String {
    let digest = Sha256::digest(service_call_id.trim().as_bytes());
    format!("ref-{}", hex::encode(&digest[..12]))
}

fn trace_id(value: &str) -> String {
    let parts: Vec<_> = value.trim().split('-').collect();
    if parts.len() == 4
        && parts[0] == "00"
        && parts[1].len() == 32
        && parts[1] != "00000000000000000000000000000000"
        && parts[1]
            .chars()
            .all(|character| character.is_ascii_hexdigit() && !character.is_ascii_uppercase())
    {
        parts[1].into()
    } else {
        String::new()
    }
}

#[allow(clippy::trivially_copy_pass_by_ref)] // serde callback receives a reference.
const fn is_false(value: &bool) -> bool {
    !*value
}

struct Audit {
    client: Client,
    durable_subject_prefix: Option<String>,
    event_subject_prefix: Option<String>,
}

impl Audit {
    async fn record(
        &self,
        request: &RequestEnvelope,
        operation: Operation,
        response: &ResponseEnvelope,
        duration: Duration,
    ) -> anyhow::Result<()> {
        let now = chrono::Utc::now();
        let event = ServiceCallEvent {
            kind: "service_call",
            service_call_id: response.service_call_id.clone(),
            reference_id: response.reference_id.clone(),
            audit_ref: response.audit_ref.clone(),
            space_id: request.space_id.clone(),
            session_id: request.session_id.clone(),
            run_id: request.run_id.clone(),
            workflow_id: request.workflow_id.clone(),
            agent_id: request.agent_id.clone(),
            service: Harness::OWNER,
            function: operation.function(),
            subject: operation.subject(),
            status: response.status,
            duration_millis: i64::try_from(duration.as_millis()).unwrap_or(i64::MAX),
            trace_id: response.trace_id.clone(),
            recorded_at: now.to_rfc3339(),
            retention_expires_at: (now + chrono::Duration::days(90)).to_rfc3339(),
        };
        let data = Bytes::from(serde_json::to_vec(&event)?);
        if let Some(prefix) = &self.durable_subject_prefix {
            let subject = audit_subject(prefix, &request.space_id, &response.reference_id);
            async_nats::jetstream::new(self.client.clone())
                .publish(subject, data.clone())
                .await?
                .await?;
        }
        if let Some(prefix) = &self.event_subject_prefix {
            self.client
                .publish(
                    audit_subject(prefix, &request.space_id, &response.reference_id),
                    data,
                )
                .await?;
        }
        Ok(())
    }
}

fn audit_subject(prefix: &str, space: &str, reference: &str) -> String {
    format!(
        "{}.{}.service_calls.{}",
        prefix.trim_matches('.'),
        stable_token(space),
        stable_token(reference)
    )
}

fn stable_token(value: &str) -> String {
    let mut output = String::new();
    let mut underscore = false;
    for character in value.trim().chars() {
        if character.is_ascii_alphanumeric() {
            output.push(character.to_ascii_lowercase());
            underscore = false;
        } else if !underscore && !output.is_empty() {
            output.push('_');
            underscore = true;
        }
    }
    output.trim_matches('_').to_owned()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn matches_go_reference_and_subject_contract() {
        assert_eq!(QUEUE, "q.service.v1.harness");
        assert_eq!(
            Operation::ComposeContext.subject(),
            "svc.harness.v1.compose_context"
        );
        assert_eq!(reference_id("call-example"), "ref-cc66dbadcb5b79f4556efba4");
    }
}
