#[cfg(test)]
use std::collections::HashMap;
use std::collections::HashSet;

use anyhow::{Context, bail};
use chrono::Utc;
use sha2::{Digest, Sha256};
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
            });
        }
        messages.extend(request.history.clone());

        let mut omitted = Vec::new();
        let char_budget = if request.context_window > 0 {
            i64::from(request.context_window) * CHARS_PER_TOKEN * BUDGET_PERCENT / 100
        } else {
            0
        };
        while should_compact(&messages, char_budget) {
            let removable = messages
                .iter()
                .position(|message| message.role != "system")
                .context("context window is too small for required system material")?;
            let removed = messages.remove(removable);
            if !removed.source_id.trim().is_empty() {
                omitted.push(removed.source_id);
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
        total.saturating_add(i64::try_from(message.content.len()).unwrap_or(i64::MAX))
    })
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
    use crate::proto::harness::PromptMaterial;

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
                    },
                    ContextMessage {
                        role: "user".into(),
                        content: "new".into(),
                        source_id: "session.2".into(),
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
