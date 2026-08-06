//! flowd binary entry point.
//!
//! Runs `flowd serve` — a gRPC server on 127.0.0.1 exposing the FlowService
//! read endpoints, backed by SQLite.

use std::net::SocketAddr;

use clap::Parser;
use tracing::info;

use flowd::db;
use flowd::generated::flow_v1::flow_service_server::FlowServiceServer as ServerTonic;
use flowd::grpc::FlowServiceServer;
use flowd::store::{DynStore, SqliteStore};

/// FlowControl core daemon.
#[derive(Parser, Debug)]
#[command(name = "flowd", version, about)]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(clap::Subcommand, Debug)]
enum Command {
    /// Run the gRPC server.
    Serve(ServeArgs),
}

#[derive(clap::Args, Debug)]
struct ServeArgs {
    /// Address to bind (default 127.0.0.1:50051).
    #[arg(long, default_value = "127.0.0.1:50051")]
    addr: String,

    /// SQLite URL. `:memory:` for a scratch DB, else a file path.
    #[arg(long, default_value = "flowd.db")]
    db: String,

    /// Seed fixture data on startup (dev/demo).
    #[arg(long)]
    seed: bool,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    init_tracing();
    let cli = Cli::parse();
    match cli.command {
        Command::Serve(args) => serve(args).await,
    }
}

fn init_tracing() {
    use tracing_subscriber::EnvFilter;
    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));
    tracing_subscriber::fmt().with_env_filter(filter).init();
}

async fn serve(args: ServeArgs) -> anyhow::Result<()> {
    let addr: SocketAddr = args.addr.parse()?;

    // Open the database and run migrations.
    info!(db = %args.db, "opening database");
    let pool = db::open(&args.db).await?;
    if args.seed {
        db::seed(&pool).await?;
        info!("seeded fixture data");
    }

    let store: DynStore = std::sync::Arc::new(SqliteStore::from_pool(pool));
    let svc = ServerTonic::new(FlowServiceServer::new(store));

    info!(%addr, "listening");
    let listener = tokio::net::TcpListener::bind(addr).await?;
    let (tx, rx) = tokio::sync::oneshot::channel();
    let mut sig = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::interrupt())?;
    tokio::spawn(async move {
        sig.recv().await;
        let _ = tx.send(());
    });

    tonic::transport::Server::builder()
        .add_service(svc)
        .serve_with_incoming_shutdown(
            tokio_stream::wrappers::TcpListenerStream::new(listener),
            async {
                let _ = rx.await;
            },
        )
        .await?;

    info!("shutting down");
    Ok(())
}
