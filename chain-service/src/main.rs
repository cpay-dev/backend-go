mod gen;
mod provider;
mod service;

use anyhow::Result;
use tonic::transport::Server;
use tracing_subscriber::EnvFilter;

use gen::chain::v1::chain_service_server::ChainServiceServer;
use provider::ChainProviders;
use service::ChainServiceImpl;

#[tokio::main]
async fn main() -> Result<()> {
    dotenvy::dotenv().ok();

    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env().add_directive("info".parse()?))
        .json()
        .init();

    let providers = ChainProviders::from_env()?;
    let service = ChainServiceImpl::new(providers);

    let port: u16 = std::env::var("GRPC_PORT")
        .unwrap_or_else(|_| "50051".to_string())
        .parse()?;

    let addr = ([0, 0, 0, 0], port).into();
    tracing::info!(%addr, "starting gRPC server");

    Server::builder()
        .add_service(ChainServiceServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
