use std::sync::Arc;

use alloy::primitives::{keccak256, Address, B256, U256};
use alloy::providers::Provider;
use tonic::{Request, Response, Status};

use crate::gen::chain::v1::chain_service_server::ChainService;
use crate::gen::chain::v1::{
    DeployAndWithdrawRequest, DeployAndWithdrawResponse,
    GetBalanceRequest, GetBalanceResponse,
    GetCounterfactualAddressRequest, GetCounterfactualAddressResponse,
    GetTokenBalanceRequest, GetTokenBalanceResponse,
    HealthCheckRequest, HealthCheckResponse,
};
use crate::provider::ChainProviders;

// Universal CREATE2 factory (deployed on all EVM chains including Amoy)
const CREATE2_FACTORY: &str = "0x4e59b44847b379578588920cA78FbF26c0B4956C";

// MinimalWallet init bytecode (solc 0.8.20 --optimize --optimize-runs 1)
// constructor(address _owner) - owner is immutable, execute(to,value,data) onlyOwner
const MINIMAL_WALLET_BYTECODE: &str = "60a060405234801561000f575f80fd5b5060405161036883038061036883398101604081905261002e9161003f565b6001600160a01b031660805261006c565b5f6020828403121561004f575f80fd5b81516001600160a01b0381168114610065575f80fd5b9392505050565b6080516102df6100895f395f81816047015260bf01526102df5ff3fe60806040526004361061002b575f3560e01c80638da5cb5b14610036578063b61d27f614610086575f80fd5b3661003257005b5f80fd5b348015610041575f80fd5b506100697f000000000000000000000000000000000000000000000000000000000000000081565b6040516001600160a01b0390911681526020015b60405180910390f35b348015610091575f80fd5b506100a56100a03660046101c3565b6100b2565b60405161007d919061024f565b6060336001600160a01b037f0000000000000000000000000000000000000000000000000000000000000000161461011d5760405162461bcd60e51b81526020600482015260096024820152683737ba1037bbb732b960b91b60448201526064015b60405180910390fd5b5f80866001600160a01b031686868660405161013a92919061029a565b5f6040518083038185875af1925050503d805f8114610174576040519150601f19603f3d011682016040523d82523d5f602084013e610179565b606091505b5091509150816101b95760405162461bcd60e51b815260206004820152600b60248201526a18d85b1b0819985a5b195960aa1b6044820152606401610114565b9695505050505050565b5f805f80606085870312156101d6575f80fd5b84356001600160a01b03811681146101ec575f80fd5b93506020850135925060408501356001600160401b038082111561020e575f80fd5b818701915087601f830112610221575f80fd5b81358181111561022f575f80fd5b886020828501011115610240575f80fd5b95989497505060200194505050565b5f6020808352835180828501525f5b8181101561027a5785810183015185820160400152820161025e565b505f604082860101526040601f19601f8301168501019250505092915050565b818382375f910190815291905056fea2646970667358221220a9640b5094688010a419fc9d0f8507ab64efb2167ad7daa81f98994eefcf3ab464736f6c63430008140033";

pub struct ChainServiceImpl {
    providers: Arc<ChainProviders>,
}

impl ChainServiceImpl {
    pub fn new(providers: ChainProviders) -> Self {
        Self {
            providers: Arc::new(providers),
        }
    }

    /// Derive the CREATE2 counterfactual address for a merchant's MinimalWallet.
    /// Uses universal CREATE2 factory at 0x4e59b44847b379578588920cA78FbF26c0B4956C
    /// salt = keccak256(abi.encodePacked(merchant_wallet))
    /// initCode = MINIMAL_WALLET_BYTECODE ++ abi.encode(merchant_wallet)
    pub fn derive_cf_address(&self, merchant: Address) -> Address {
        // salt = keccak256(merchant address bytes)
        let salt: B256 = keccak256(merchant.as_slice());

        // initCode = bytecode + abi.encode(address) (32-byte left-padded)
        let bytecode = hex::decode(MINIMAL_WALLET_BYTECODE).expect("valid hex");
        let mut init_code = bytecode;
        let mut abi_owner = [0u8; 32];
        abi_owner[12..].copy_from_slice(merchant.as_slice());
        init_code.extend_from_slice(&abi_owner);

        let init_code_hash: B256 = keccak256(&init_code);

        // CREATE2 preimage: 0xff ++ factory(20) ++ salt(32) ++ initCodeHash(32) = 85 bytes
        let factory: Address = CREATE2_FACTORY.parse().expect("valid factory address");
        let mut preimage = [0u8; 85];
        preimage[0] = 0xff;
        preimage[1..21].copy_from_slice(factory.as_slice());
        preimage[21..53].copy_from_slice(salt.as_slice());
        preimage[53..85].copy_from_slice(init_code_hash.as_slice());

        let hash = keccak256(&preimage);
        Address::from_slice(&hash[12..])
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

        let addr = self.derive_cf_address(merchant);

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

    async fn deploy_and_withdraw(
        &self,
        request: Request<DeployAndWithdrawRequest>,
    ) -> Result<Response<DeployAndWithdrawResponse>, Status> {
        let req = request.into_inner();

        let merchant: Address = req.merchant_wallet.parse()
            .map_err(|_| Status::invalid_argument("invalid merchant_wallet"))?;
        let token: Address = req.token_address.parse()
            .map_err(|_| Status::invalid_argument("invalid token_address"))?;

        let cf_address = self.derive_cf_address(merchant);

        let provider = self.providers.get(req.chain_id)
            .ok_or_else(|| Status::invalid_argument(format!("unsupported chain: {}", req.chain_id)))?;

        // Check token balance
        let balance = self.providers
            .get_token_balance(req.chain_id, token, cf_address)
            .await
            .map_err(|e| Status::internal(format!("get_token_balance: {e}")))?;

        // Check if CF wallet is already deployed
        let code = provider.get_code_at(cf_address).await
            .map_err(|e| Status::internal(format!("get_code: {e}")))?;

        let deployed = !code.is_empty();

        // Return info for the frontend to perform deploy+withdraw via wallet signature
        // The frontend will:
        // 1. If not deployed: call CREATE2 factory to deploy MinimalWallet
        // 2. Call execute(token, 0, transfer(merchant, balance)) on the CF wallet
        Ok(Response::new(DeployAndWithdrawResponse {
            tx_hash: cf_address.to_checksum(None), // reuse field: cf address
            amount: format!("{}:{}", balance, if deployed { "deployed" } else { "not_deployed" }),
        }))
    }
}
