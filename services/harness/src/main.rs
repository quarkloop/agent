mod contract;
mod proto;
mod service;
mod store;

use anyhow::Context;
use clap::Parser;
use contract::TransportConfig;
use service::Harness;
use store::Store;

#[derive(Debug, Parser)]
#[command(name = "harness-service")]
struct Config {
    #[arg(long, env = "QUARK_HARNESS_ROOT")]
    root: std::path::PathBuf,
    #[arg(long = "nats-url", env = "QUARK_NATS_URL")]
    nats_url: String,
    #[arg(long = "nats-user", env = "QUARK_NATS_SERVICE_USER")]
    nats_user: Option<String>,
    #[arg(long = "nats-password", env = "QUARK_NATS_SERVICE_PASSWORD")]
    nats_password: Option<String>,
    #[arg(long = "audit-prefix", env = "QUARK_NATS_AUDIT_PREFIX")]
    audit_prefix: Option<String>,
    #[arg(long = "telemetry-prefix", env = "QUARK_NATS_TELEMETRY_PREFIX")]
    telemetry_prefix: Option<String>,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let config = Config::parse();
    let state = Store::open(&config.root)
        .await
        .with_context(|| format!("open harness state at {}", config.root.display()))?;
    let harness = Harness::new(state);
    let transport = TransportConfig {
        url: config.nats_url,
        username: config.nats_user,
        password: config.nats_password,
        audit_prefix: config.audit_prefix,
        telemetry_prefix: config.telemetry_prefix,
    };
    contract::run(transport, harness).await
}
