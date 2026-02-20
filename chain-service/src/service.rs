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

// PaymentWallet init bytecode (solc 0.8.20 --optimize --optimize-runs 1)
// constructor(address _owner) - owner is immutable
// withdraw(token) permissionless, withdrawNative() permissionless, execute(to,value,data) onlyOwner
const PAYMENT_WALLET_BYTECODE: &str = "60a060405234801561000f575f80fd5b5060405161070b38038061070b83398101604081905261002e9161003f565b6001600160a01b031660805261006c565b5f6020828403121561004f575f80fd5b81516001600160a01b0381168114610065575f80fd5b9392505050565b6080516106736100985f395f8181609201528181610120015281816102b101526103bc01526106735ff3fe608060405260043610610041575f3560e01c806350431ce41461004c57806351cff8d9146100625780638da5cb5b14610081578063b61d27f6146100ca575f80fd5b3661004857005b5f80fd5b348015610057575f80fd5b506100606100f6565b005b34801561006d575f80fd5b5061006061007c3660046104d6565b61020c565b34801561008c575f80fd5b506100b47f000000000000000000000000000000000000000000000000000000000000000081565b6040516100c191906104f6565b60405180910390f35b3480156100d5575f80fd5b506100e96100e436600461050a565b6103af565b6040516100c19190610589565b478061011d5760405162461bcd60e51b8152600401610114906105d4565b60405180910390fd5b5f7f00000000000000000000000000000000000000000000000000000000000000006001600160a01b0316826040515f6040518083038185875af1925050503d805f8114610186576040519150601f19603f3d011682016040523d82523d5f602084013e61018b565b606091505b50509050806101d55760405162461bcd60e51b81526020600482015260166024820152751b985d1a5d99481d1c985b9cd9995c8819985a5b195960521b6044820152606401610114565b6040518281527fe1abb8128ba4c3d7694c033a0a48ca4f62651a7e8a6356204348ea23841e6db69060200160405180910390a15050565b6040516370a0823160e01b81525f906001600160a01b038316906370a082319061023a9030906004016104f6565b602060405180830381865afa158015610255573d5f803e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061027991906105f8565b90505f811161029a5760405162461bcd60e51b8152600401610114906105d4565b60405163a9059cbb60e01b81526001600160a01b037f0000000000000000000000000000000000000000000000000000000000000000811660048301526024820183905283169063a9059cbb906044016020604051808303815f875af1158015610306573d5f803e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061032a919061060f565b6103685760405162461bcd60e51b815260206004820152600f60248201526e1d1c985b9cd9995c8819985a5b1959608a1b6044820152606401610114565b816001600160a01b03167f7084f5476618d8e60b11ef0d7d3f06914655adb8793e28ff7f018d4c76d505d5826040516103a391815260200190565b60405180910390a25050565b6060336001600160a01b037f000000000000000000000000000000000000000000000000000000000000000016146104155760405162461bcd60e51b81526020600482015260096024820152683737ba1037bbb732b960b91b6044820152606401610114565b5f80866001600160a01b031686868660405161043292919061062e565b5f6040518083038185875af1925050503d805f811461046c576040519150601f19603f3d011682016040523d82523d5f602084013e610471565b606091505b5091509150816104b15760405162461bcd60e51b815260206004820152600b60248201526a18d85b1b0819985a5b195960aa1b6044820152606401610114565b9695505050505050565b80356001600160a01b03811681146104d1575f80fd5b919050565b5f602082840312156104e6575f80fd5b6104ef826104bb565b9392505050565b6001600160a01b0391909116815260200190565b5f805f806060858703121561051d575f80fd5b610526856104bb565b93506020850135925060408501356001600160401b0380821115610548575f80fd5b818701915087601f83011261055b575f80fd5b813581811115610569575f80fd5b88602082850101111561057a575f80fd5b95989497505060200194505050565b5f6020808352835180828501525f5b818110156105b457858101830151858201604001528201610598565b505f604082860101526040601f19601f8301168501019250505092915050565b6020808252600a90820152696e6f2062616c616e636560b01b604082015260600190565b5f60208284031215610608575f80fd5b5051919050565b5f6020828403121561061f575f80fd5b815180151581146104ef575f80fd5b818382375f910190815291905056fea2646970667358221220f3431f894c4699a0abeaae1ac6b7a442c6f9bc4158bcf5b0308f4b6d2434e31164736f6c63430008140033";

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
    /// initCode = PAYMENT_WALLET_BYTECODE ++ abi.encode(merchant_wallet)
    pub fn derive_cf_address(&self, merchant: Address) -> Address {
        // salt = keccak256(merchant address bytes)
        let salt: B256 = keccak256(merchant.as_slice());

        // initCode = bytecode + abi.encode(address) (32-byte left-padded)
        let bytecode = hex::decode(PAYMENT_WALLET_BYTECODE).expect("valid hex");
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

        // Return info for the frontend to perform deploy+withdraw
        // The frontend will:
        // 1. If not deployed: call CREATE2 factory to deploy PaymentWallet
        // 2. Call withdraw(token) on the CF wallet (permissionless)
        Ok(Response::new(DeployAndWithdrawResponse {
            tx_hash: cf_address.to_checksum(None), // reuse field: cf address
            amount: format!("{}:{}", balance, if deployed { "deployed" } else { "not_deployed" }),
        }))
    }
}
