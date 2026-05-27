#[cfg(test)]
use std::collections::HashMap;
use std::collections::HashSet;

use anyhow::{Context, bail};
use chrono::Utc;
use sha2::{Digest, Sha256};
use std::ops::Range;
use uuid::Uuid;

use crate::proto::harness::{
    ComposeContextRequest, ComposeContextResponse, ContextMessage, ContextReport,
    DeleteMemoryRequest, DeleteMemoryResponse, GetContextReportRequest, GetMemoryRequest,
    GetMemoryResponse, MemoryRecord, PutMemoryRequest, PutMemoryResponse, SearchMemoryRequest,
    SearchMemoryResponse, StreamContextReportsRequest,
};
use crate::store::Store;

const CHARS_PER_TOKEN: i64 = 4;
const BUDGET_PERCENT: i64 = 80;
const MAX_MESSAGES: usize = 200;

pub struct Harness {
    store: Store,
}

impl Harness {
    pub const OWNER: &str = "harness";

    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn compose_context(
        &self,
        request: ComposeContextRequest,
    ) -> anyhow::Result<ComposeContextResponse> {
        require(&request.space, "space")?;
        require(&request.session_id, "session_id")?;

        let mut included = Vec::new();
        let mut included_memory_ids = Vec::new();
        let mut messages = Vec::new();
        let mut system = Vec::new();
        for material in request
            .system_materials
            .iter()
            .chain(&request.runtime_facts)
        {
            if material.content.trim().is_empty() {
                continue;
            }
            system.push(material.content.trim().to_owned());
            included.push(material.source_id.clone());
            if material.source_kind == "memory" {
                included_memory_ids.push(material.source_id.clone());
            }
        }
        if !system.is_empty() {
            messages.push(ContextMessage {
                role: "system".into(),
                content: system.join("\n\n"),
                source_id: "harness.system_materials".into(),
                tool_calls: Vec::new(),
                tool_call_id: String::new(),
            });
        }
        let protected_prefix_len = messages.len();
        messages.extend(request.history.clone());

        let mut omitted = Vec::new();
        let char_budget = if request.context_window > 0 {
            i64::from(request.context_window) * CHARS_PER_TOKEN * BUDGET_PERCENT / 100
        } else {
            0
        };
        while should_compact(&messages, char_budget) {
            let removable = oldest_removable_history_unit(&messages, protected_prefix_len)
                .context("context window is too small for required prompt material and the current history unit")?;
            for removed in messages.drain(removable) {
                if !removed.source_id.trim().is_empty() {
                    omitted.push(removed.source_id);
                }
            }
        }
        for message in &messages {
            if message.role != "system" && !message.source_id.trim().is_empty() {
                included.push(message.source_id.clone());
            }
        }
        included = deduplicate(included);
        omitted = deduplicate(omitted);
        let character_count = character_count(&messages);
        let report = ContextReport {
            id: format!("ctx-{}", Uuid::new_v4()),
            space: request.space,
            session_id: request.session_id,
            run_id: request.run_id,
            included_sources: included,
            omitted_sources: omitted,
            message_count: i32::try_from(messages.len()).unwrap_or(i32::MAX),
            character_count,
            estimated_input_tokens: divide_round_up(character_count, CHARS_PER_TOKEN),
            created_at: Utc::now().to_rfc3339(),
            included_memory_ids: deduplicate(included_memory_ids),
        };
        self.store.put_report(report.clone()).await?;
        Ok(ComposeContextResponse {
            messages,
            report: Some(report),
        })
    }

    pub async fn get_context_report(
        &self,
        request: GetContextReportRequest,
    ) -> anyhow::Result<ContextReport> {
        require(&request.space, "space")?;
        require(&request.id, "id")?;
        self.store.get_report(&request.space, &request.id).await
    }

    pub async fn stream_context_reports(
        &self,
        request: StreamContextReportsRequest,
    ) -> anyhow::Result<Vec<ContextReport>> {
        require(&request.space, "space")?;
        require(&request.session_id, "session_id")?;
        let limit = if request.limit <= 0 {
            50
        } else {
            usize::try_from(request.limit).unwrap_or(50).min(500)
        };
        Ok(self
            .store
            .reports(&request.space, &request.session_id, limit)
            .await)
    }

    pub async fn put_memory(&self, request: PutMemoryRequest) -> anyhow::Result<PutMemoryResponse> {
        require(&request.space, "space")?;
        require(&request.scope, "scope")?;
        require(&request.key, "key")?;
        require(&request.value, "value")?;
        require(&request.provenance, "provenance")?;
        let id = memory_id(&request.space, &request.scope, &request.key);
        let record = MemoryRecord {
            id,
            space: request.space,
            scope: request.scope,
            key: request.key,
            value: request.value,
            metadata: request.metadata,
            provenance: request.provenance,
            updated_at: Utc::now().to_rfc3339(),
        };
        self.store.put_memory(record.clone()).await?;
        Ok(PutMemoryResponse {
            record: Some(record),
        })
    }

    pub async fn get_memory(&self, request: GetMemoryRequest) -> anyhow::Result<GetMemoryResponse> {
        require(&request.space, "space")?;
        require(&request.scope, "scope")?;
        require(&request.key, "key")?;
        let record = self
            .store
            .get_memory(&memory_id(&request.space, &request.scope, &request.key))
            .await?;
        Ok(GetMemoryResponse {
            record: Some(record),
        })
    }

    pub async fn search_memory(
        &self,
        request: SearchMemoryRequest,
    ) -> anyhow::Result<SearchMemoryResponse> {
        require(&request.space, "space")?;
        require(&request.scope, "scope")?;
        let limit = if request.limit <= 0 {
            20
        } else {
            usize::try_from(request.limit).unwrap_or(20).min(200)
        };
        let records = self
            .store
            .search_memory(
                &request.space,
                &request.scope,
                &request.query,
                &request.filters,
                limit,
            )
            .await;
        Ok(SearchMemoryResponse { records })
    }

    pub async fn delete_memory(
        &self,
        request: DeleteMemoryRequest,
    ) -> anyhow::Result<DeleteMemoryResponse> {
        require(&request.space, "space")?;
        require(&request.scope, "scope")?;
        require(&request.key, "key")?;
        let deleted = self
            .store
            .delete_memory(&memory_id(&request.space, &request.scope, &request.key))
            .await?;
        Ok(DeleteMemoryResponse { deleted })
    }
}

fn should_compact(messages: &[ContextMessage], char_budget: i64) -> bool {
    if messages.len() > MAX_MESSAGES {
        return true;
    }
    char_budget > 0 && character_count(messages) > char_budget
}

fn character_count(messages: &[ContextMessage]) -> i64 {
    messages.iter().fold(0_i64, |total, message| {
        let content = i64::try_from(message.content.len()).unwrap_or(i64::MAX);
        let tool_call_id = i64::try_from(message.tool_call_id.len()).unwrap_or(i64::MAX);
        let tool_calls = message.tool_calls.iter().fold(0_i64, |subtotal, call| {
            subtotal
                .saturating_add(i64::try_from(call.id.len()).unwrap_or(i64::MAX))
                .saturating_add(i64::try_from(call.r#type.len()).unwrap_or(i64::MAX))
                .saturating_add(i64::try_from(call.name.len()).unwrap_or(i64::MAX))
                .saturating_add(i64::try_from(call.arguments_json.len()).unwrap_or(i64::MAX))
        });
        total
            .saturating_add(content)
            .saturating_add(tool_call_id)
            .saturating_add(tool_calls)
    })
}

// Model APIs require assistant tool calls and their matching tool responses to
// remain adjacent. Context compaction therefore removes history units rather
// than individual messages and keeps the newest unit as the active turn.
fn oldest_removable_history_unit(
    messages: &[ContextMessage],
    protected_prefix_len: usize,
) -> Option<Range<usize>> {
    let units = history_units(&messages[protected_prefix_len..]);
    if units.len() <= 1 {
        return None;
    }
    let first = units.first()?;
    Some((protected_prefix_len + first.start)..(protected_prefix_len + first.end))
}

fn history_units(history: &[ContextMessage]) -> Vec<Range<usize>> {
    let mut units = Vec::new();
    let mut index = 0;
    while index < history.len() {
        let start = index;
        index += 1;
        if history[start].role == "assistant" && !history[start].tool_calls.is_empty() {
            let call_ids: HashSet<&str> = history[start]
                .tool_calls
                .iter()
                .map(|call| call.id.as_str())
                .filter(|id| !id.is_empty())
                .collect();
            while index < history.len()
                && history[index].role == "tool"
                && call_ids.contains(history[index].tool_call_id.as_str())
            {
                index += 1;
            }
        }
        units.push(start..index);
    }
    units
}

fn require(value: &str, name: &str) -> anyhow::Result<()> {
    if value.trim().is_empty() {
        bail!("{name} is required");
    }
    Ok(())
}

fn memory_id(space: &str, scope: &str, key: &str) -> String {
    let mut digest = Sha256::new();
    digest.update(space.trim().as_bytes());
    digest.update([0]);
    digest.update(scope.trim().as_bytes());
    digest.update([0]);
    digest.update(key.trim().as_bytes());
    format!("mem-{}", hex::encode(&digest.finalize()[..12]))
}

fn divide_round_up(value: i64, divisor: i64) -> i64 {
    if value <= 0 {
        0
    } else {
        (value + divisor - 1) / divisor
    }
}

fn deduplicate(values: Vec<String>) -> Vec<String> {
    let mut seen = HashSet::new();
    values
        .into_iter()
        .filter(|value| !value.trim().is_empty() && seen.insert(value.clone()))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::harness::{ContextToolCall, PromptMaterial};

    #[tokio::test]
    async fn composes_and_reports_context_without_authoring_prompt_material() {
        let root = tempfile::tempdir().unwrap();
        let harness = Harness::new(Store::open(root.path()).await.unwrap());
        let result = harness
            .compose_context(ComposeContextRequest {
                space: "docs".into(),
                session_id: "s1".into(),
                run_id: "r1".into(),
                context_window: 10,
                system_materials: vec![PromptMaterial {
                    source_id: "plugin.agent.main.system".into(),
                    source_kind: "agent".into(),
                    content: "Use verified evidence.".into(),
                    required: true,
                }],
                runtime_facts: Vec::new(),
                history: vec![
                    ContextMessage {
                        role: "user".into(),
                        content: "old content that is removed due to budget".into(),
                        source_id: "session.1".into(),
                        ..ContextMessage::default()
                    },
                    ContextMessage {
                        role: "user".into(),
                        content: "new".into(),
                        source_id: "session.2".into(),
                        ..ContextMessage::default()
                    },
                ],
            })
            .await
            .unwrap();
        assert_eq!(result.messages[0].content, "Use verified evidence.");
        let report = result.report.unwrap();
        assert!(
            report
                .included_sources
                .contains(&"plugin.agent.main.system".into())
        );
        assert!(report.omitted_sources.contains(&"session.1".into()));
        assert!(report.included_memory_ids.is_empty());
        assert_eq!(
            harness
                .get_context_report(GetContextReportRequest {
                    space: "docs".into(),
                    id: report.id.clone()
                })
                .await
                .unwrap()
                .id,
            report.id
        );
    }

    #[tokio::test]
    async fn preserves_structured_tool_execution_history() {
        let root = tempfile::tempdir().unwrap();
        let harness = Harness::new(Store::open(root.path()).await.unwrap());
        let result = harness
            .compose_context(ComposeContextRequest {
                space: "docs".into(),
                session_id: "s1".into(),
                context_window: 100,
                history: vec![
                    ContextMessage {
                        role: "assistant".into(),
                        source_id: "session.1".into(),
                        tool_calls: vec![ContextToolCall {
                            index: 0,
                            id: "call-index".into(),
                            r#type: "function".into(),
                            name: "indexer_UpsertChunk".into(),
                            arguments_json: "{\"document\":{\"sourceUri\":\"a.pdf\"}}".into(),
                        }],
                        ..ContextMessage::default()
                    },
                    ContextMessage {
                        role: "tool".into(),
                        content: "{\"referenceId\":\"ref-1\"}".into(),
                        source_id: "session.2".into(),
                        tool_call_id: "call-index".into(),
                        ..ContextMessage::default()
                    },
                ],
                ..ComposeContextRequest::default()
            })
            .await
            .unwrap();

        assert_eq!(result.messages[0].tool_calls[0].id, "call-index");
        assert_eq!(result.messages[1].tool_call_id, "call-index");
    }

    #[tokio::test]
    async fn compaction_removes_tool_call_and_results_as_one_history_unit() {
        let root = tempfile::tempdir().unwrap();
        let harness = Harness::new(Store::open(root.path()).await.unwrap());
        let result = harness
            .compose_context(ComposeContextRequest {
                space: "docs".into(),
                session_id: "s1".into(),
                context_window: 16,
                history: vec![
                    ContextMessage {
                        role: "assistant".into(),
                        source_id: "session.tool-call".into(),
                        tool_calls: vec![ContextToolCall {
                            id: "call-index".into(),
                            name: "indexer_UpsertChunk".into(),
                            arguments_json: "{\"large\":\"this older tool argument must be budgeted and removed atomically\"}".into(),
                            ..ContextToolCall::default()
                        }],
                        ..ContextMessage::default()
                    },
                    ContextMessage {
                        role: "tool".into(),
                        content: "{\"referenceId\":\"ref-1\"}".into(),
                        source_id: "session.tool-result".into(),
                        tool_call_id: "call-index".into(),
                        ..ContextMessage::default()
                    },
                    ContextMessage {
                        role: "user".into(),
                        content: "latest request".into(),
                        source_id: "session.current".into(),
                        ..ContextMessage::default()
                    },
                ],
                ..ComposeContextRequest::default()
            })
            .await
            .unwrap();

        assert_eq!(result.messages.len(), 1);
        assert_eq!(result.messages[0].content, "latest request");
        let report = result.report.unwrap();
        assert!(report.omitted_sources.contains(&"session.tool-call".into()));
        assert!(
            report
                .omitted_sources
                .contains(&"session.tool-result".into())
        );
        assert!(report.included_sources.contains(&"session.current".into()));
    }

    #[tokio::test]
    async fn reports_explicit_memory_material_contribution() {
        let root = tempfile::tempdir().unwrap();
        let harness = Harness::new(Store::open(root.path()).await.unwrap());
        let result = harness
            .compose_context(ComposeContextRequest {
                space: "docs".into(),
                session_id: "s1".into(),
                context_window: 100,
                system_materials: vec![PromptMaterial {
                    source_id: "mem-preference".into(),
                    source_kind: "memory".into(),
                    content: "Use concise answers.".into(),
                    required: false,
                }],
                ..ComposeContextRequest::default()
            })
            .await
            .unwrap();

        assert_eq!(
            result.report.unwrap().included_memory_ids,
            vec!["mem-preference".to_owned()]
        );
    }

    #[tokio::test]
    async fn explicit_memory_requires_provenance_and_is_searchable() {
        let root = tempfile::tempdir().unwrap();
        let harness = Harness::new(Store::open(root.path()).await.unwrap());
        let record = harness
            .put_memory(PutMemoryRequest {
                space: "docs".into(),
                scope: "session".into(),
                key: "preference".into(),
                value: "Use concise answers".into(),
                metadata: HashMap::default(),
                provenance: "user request".into(),
            })
            .await
            .unwrap()
            .record
            .unwrap();
        let found = harness
            .search_memory(SearchMemoryRequest {
                space: "docs".into(),
                scope: "session".into(),
                query: "concise".into(),
                limit: 10,
                filters: HashMap::default(),
            })
            .await
            .unwrap();
        assert_eq!(found.records[0].id, record.id);
    }
}
