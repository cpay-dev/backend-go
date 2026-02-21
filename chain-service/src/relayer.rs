use std::collections::HashMap;
use std::str::FromStr;

use alloy::network::EthereumWallet;
use alloy::primitives::{Address, U256};
use alloy::providers::{Provider, ProviderBuilder};
use alloy::signers::local::PrivateKeySigner;
use alloy::sol;
use alloy::sol_types::SolCall;
use alloy::transports::http::reqwest::Url;
use anyhow::{Context, Result};

sol! {
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
}

pub struct Relayer {
    address: Address,
    signer: PrivateKeySigner,
    rpc_urls: HashMap<u64, Url>,
}

impl Relayer {
    pub fn from_env() -> Result<Option<Self>> {
        let key = match std::env::var("RELAYER_PRIVATE_KEY") {
            Ok(k) if !k.is_empty() => k,
            _ => {
                tracing::warn!("RELAYER_PRIVATE_KEY not set, relayer disabled");
                return Ok(None);
            }
        };

        let signer: PrivateKeySigner = key.parse()
            .context("invalid RELAYER_PRIVATE_KEY")?;
        let address = signer.address();

        let mut rpc_urls = HashMap::new();
        let chain_configs = [
            (137u64, "RPC_URL_POLYGON"),
            (80002u64, "RPC_URL_POLYGON_AMOY"),
        ];
        for (chain_id, env_key) in chain_configs {
            if let Ok(url) = std::env::var(env_key) {
                if !url.is_empty() {
                    let parsed: Url = url.parse()
                        .with_context(|| format!("invalid URL for {env_key}"))?;
                    rpc_urls.insert(chain_id, parsed);
                }
            }
        }

        tracing::info!(%address, "relayer initialized");
        Ok(Some(Self { address, signer, rpc_urls }))
    }

    pub fn address(&self) -> Address {
        self.address
    }

    pub async fn execute_transfer_from(
        &self,
        chain_id: u64,
        token: Address,
        from: Address,
        to: Address,
        amount: U256,
    ) -> Result<String> {
        let rpc_url = self.rpc_urls.get(&chain_id)
            .ok_or_else(|| anyhow::anyhow!("relayer: unsupported chain {}", chain_id))?;

        let wallet = EthereumWallet::from(self.signer.clone());
        let provider = ProviderBuilder::new()
            .wallet(wallet)
            .on_http(rpc_url.clone());

        let call_data = transferFromCall { from, to, amount }.abi_encode();

        let tx = alloy::rpc::types::TransactionRequest::default()
            .to(token)
            .input(call_data.into());

        let pending = provider.send_transaction(tx).await
            .context("relayer: send transferFrom tx")?;

        let tx_hash = format!("{:#x}", pending.tx_hash());
        tracing::info!(%tx_hash, %from, %to, %amount, "relayer: transferFrom submitted");

        // Wait for receipt
        let receipt = pending.get_receipt().await
            .context("relayer: get receipt")?;

        if !receipt.status() {
            anyhow::bail!("relayer: transferFrom reverted, tx_hash={}", tx_hash);
        }

        Ok(tx_hash)
    }
}
