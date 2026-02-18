use std::sync::Arc;

use alloy::primitives::{keccak256, Address, B256};
use alloy::providers::Provider;
use tonic::{Request, Response, Status};

use crate::gen::chain::v1::chain_service_server::ChainService;
use crate::gen::chain::v1::{
    GetBalanceRequest, GetBalanceResponse,
    GetCounterfactualAddressRequest, GetCounterfactualAddressResponse,
    GetTokenBalanceRequest, GetTokenBalanceResponse,
    HealthCheckRequest, HealthCheckResponse,
};
use crate::provider::ChainProviders;

pub struct ChainServiceImpl {
    providers: Arc<ChainProviders>,
    default_factory: Address,
    default_init_code_hash: B256,
}

impl ChainServiceImpl {
    pub fn new(providers: ChainProviders) -> Self {
        let factory_str = std::env::var("CF_FACTORY_ADDRESS")
            .unwrap_or_else(|_| "0x0000000000000000000000000000000000000000".to_string());
        let init_code_hash_str = std::env::var("CF_INIT_CODE_HASH")
            .unwrap_or_else(|_| "0x0000000000000000000000000000000000000000000000000000000000000001".to_string());

        let default_factory: Address = factory_str.parse()
            .expect("invalid CF_FACTORY_ADDRESS");
        let default_init_code_hash: B256 = init_code_hash_str.parse()
            .expect("invalid CF_INIT_CODE_HASH");

        Self {
            providers: Arc::new(providers),
            default_factory,
            default_init_code_hash,
        }
    }
}

#[tonic::async_trait]
impl ChainService for ChainServiceImpl {
    async fn health_check(
        &self,
        _request: Request<HealthCheckRequest>,
    ) -> Result<Response<HealthCheckResponse>, Status> {
        Ok(Response::new(HealthCheckResponse {
            ok: true,
            supported_chains: self.providers.supported_chains(),
        }))
    }

    async fn get_balance(
        &self,
        request: Request<GetBalanceRequest>,
    ) -> Result<Response<GetBalanceResponse>, Status> {
        let req = request.into_inner();

        let provider = self
            .providers
            .get(req.chain_id)
            .ok_or_else(|| Status::invalid_argument(format!("unsupported chain: {}", req.chain_id)))?;

        let address: Address = req
            .address
            .parse()
            .map_err(|_| Status::invalid_argument("invalid address"))?;

        let balance = provider
            .get_balance(address)
            .await
            .map_err(|e| Status::internal(format!("failed to get balance: {e}")))?;

        Ok(Response::new(GetBalanceResponse {
            balance: balance.to_string(),
        }))
    }

    async fn get_counterfactual_address(
        &self,
        request: Request<GetCounterfactualAddressRequest>,
    ) -> Result<Response<GetCounterfactualAddressResponse>, Status> {
        let req = request.into_inner();

        let merchant: Address = req.merchant_wallet.parse()
            .map_err(|_| Status::invalid_argument("invalid merchant_wallet"))?;

        // salt = keccak256(merchant_wallet_bytes)
        let salt = keccak256(merchant.as_slice());

        // CREATE2 preimage: 0xff ++ factory(20) ++ salt(32) ++ initCodeHash(32) = 85 bytes
        let mut preimage = [0u8; 85];
        preimage[0] = 0xff;
        preimage[1..21].copy_from_slice(self.default_factory.as_slice());
        preimage[21..53].copy_from_slice(salt.as_slice());
        preimage[53..85].copy_from_slice(self.default_init_code_hash.as_slice());

        let hash = keccak256(&preimage);
        let addr = Address::from_slice(&hash[12..]);

        Ok(Response::new(GetCounterfactualAddressResponse {
            address: addr.to_checksum(None),
        }))
    }

    async fn get_token_balance(
        &self,
        request: Request<GetTokenBalanceRequest>,
    ) -> Result<Response<GetTokenBalanceResponse>, Status> {
        let req = request.into_inner();

        let token: Address = req.token_address.parse()
            .map_err(|_| Status::invalid_argument("invalid token_address"))?;
        let wallet: Address = req.wallet_address.parse()
            .map_err(|_| Status::invalid_argument("invalid wallet_address"))?;

        let balance = self.providers
            .get_token_balance(req.chain_id, token, wallet)
            .await
            .map_err(|e| Status::internal(format!("get_token_balance: {e}")))?;

        Ok(Response::new(GetTokenBalanceResponse {
            balance: balance.to_string(),
        }))
    }
}
