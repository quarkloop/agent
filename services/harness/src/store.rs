use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use anyhow::{Context, bail};
use serde::{Deserialize, Serialize};
use tokio::sync::Mutex;

use crate::proto::harness::{ContextReport, MemoryRecord};

#[derive(Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct State {
    reports: BTreeMap<String, ContextReport>,
    memories: BTreeMap<String, MemoryRecord>,
}

pub struct Store {
    path: PathBuf,
    state: Mutex<State>,
}

impl Store {
    pub async fn open(root: &Path) -> anyhow::Result<Self> {
        tokio::fs::create_dir_all(root).await?;
        let path = root.join("harness-state.json");
        let state = match tokio::fs::read(&path).await {
            Ok(data) => serde_json::from_slice(&data).context("decode harness state")?,
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => State::default(),
            Err(err) => return Err(err.into()),
        };
        Ok(Self {
            path,
            state: Mutex::new(state),
        })
    }

    pub async fn put_report(&self, report: ContextReport) -> anyhow::Result<()> {
        let mut state = self.state.lock().await;
        state.reports.insert(report.id.clone(), report);
        self.flush(&state).await
    }

    pub async fn get_report(&self, space: &str, id: &str) -> anyhow::Result<ContextReport> {
        let state = self.state.lock().await;
        match state.reports.get(id).filter(|value| value.space == space) {
            Some(report) => Ok(report.clone()),
            None => bail!("context report not found"),
        }
    }

    pub async fn reports(&self, space: &str, session_id: &str, limit: usize) -> Vec<ContextReport> {
        let state = self.state.lock().await;
        let mut reports: Vec<_> = state
            .reports
            .values()
            .filter(|value| value.space == space && value.session_id == session_id)
            .cloned()
            .collect();
        reports.sort_by(|left, right| right.created_at.cmp(&left.created_at));
        reports.truncate(limit);
        reports
    }

    pub async fn put_memory(&self, record: MemoryRecord) -> anyhow::Result<()> {
        let mut state = self.state.lock().await;
        state.memories.insert(record.id.clone(), record);
        self.flush(&state).await
    }

    pub async fn get_memory(&self, id: &str) -> anyhow::Result<MemoryRecord> {
        let state = self.state.lock().await;
        state
            .memories
            .get(id)
            .cloned()
            .ok_or_else(|| anyhow::anyhow!("memory record not found"))
    }

    pub async fn search_memory(
        &self,
        space: &str,
        scope: &str,
        query: &str,
        filters: &std::collections::HashMap<String, String>,
        limit: usize,
    ) -> Vec<MemoryRecord> {
        let query = query.to_lowercase();
        let state = self.state.lock().await;
        state
            .memories
            .values()
            .filter(|record| record.space == space && record.scope == scope)
            .filter(|record| {
                query.is_empty()
                    || record.key.to_lowercase().contains(&query)
                    || record.value.to_lowercase().contains(&query)
            })
            .filter(|record| {
                filters
                    .iter()
                    .all(|(key, value)| record.metadata.get(key) == Some(value))
            })
            .take(limit)
            .cloned()
            .collect()
    }

    pub async fn delete_memory(&self, id: &str) -> anyhow::Result<bool> {
        let mut state = self.state.lock().await;
        let deleted = state.memories.remove(id).is_some();
        if deleted {
            self.flush(&state).await?;
        }
        Ok(deleted)
    }

    async fn flush(&self, state: &State) -> anyhow::Result<()> {
        let data = serde_json::to_vec_pretty(state)?;
        let temporary = self.path.with_extension("json.tmp");
        tokio::fs::write(&temporary, data).await?;
        tokio::fs::rename(&temporary, &self.path).await?;
        Ok(())
    }
}
