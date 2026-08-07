//! flowd binary entry point.
//!
//! Runs `flowd serve` — a gRPC server on 127.0.0.1 exposing the FlowService
//! read endpoints, backed by SQLite.

use std::net::SocketAddr;

use axum::Router;
use clap::Parser;
use tower_http::cors::{Any, CorsLayer};
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
    let grpc_service = ServerTonic::new(FlowServiceServer::new(store));
    // tonic-web decodes HTTP/1.1 grpc-web (browser) while raw gRPC over HTTP/2
    // (TUI) passes straight through; into_router() exposes the gRPC routes as an
    // axum Router, which axum::serve hosts over both HTTP/1.1 and HTTP/2.
    let wrapped_grpc = tonic_web::enable(grpc_service);
    // into_router is deprecated in 0.12 (Routes::into_axum_router), but tonic
    // 0.12.x exposes no non-deprecated public path from the builder to the axum
    // router; keep the working API until the next tonic major.
    #[allow(deprecated)]
    let grpc_router = tonic::transport::Server::builder()
        .accept_http1(true)
        .add_service(wrapped_grpc)
        .into_router();

    // Dev CORS so the browser's flowui origin can reach the core at 127.0.0.1.
    // Any is fine while bound to loopback; lock to the exact origin in M4.
    let cors = CorsLayer::new()
        .allow_origin(Any)
        .allow_headers(Any)
        .allow_methods(Any);
    let app = Router::new().fallback_service(grpc_router).layer(cors);

    info!(%addr, "listening (gRPC + grpc-web)");
    let listener = tokio::net::TcpListener::bind(addr).await?;

    let (tx, rx) = tokio::sync::oneshot::channel();
    let mut sig = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::interrupt())?;
    tokio::spawn(async move {
        sig.recv().await;
        let _ = tx.send(());
    });

    axum::serve(listener, app)
        .with_graceful_shutdown(async {
            let _ = rx.await;
        })
        .await?;

    info!("shutting down");
    Ok(())
}
