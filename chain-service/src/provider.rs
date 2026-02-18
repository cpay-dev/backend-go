use std::collections::HashMap;

use alloy::primitives::{Address, U256};
use alloy::providers::{Provider, ProviderBuilder};
use alloy::sol;
use alloy::sol_types::SolCall;
use alloy::transports::http::reqwest::Url;
use anyhow::{Context, Result};

sol! {
    function balanceOf(address owner) external view returns (uint256);
}

pub struct ChainProviders {
    providers: HashMap<u64, alloy::providers::RootProvider<alloy::transports::http::Http<alloy::transports::http::reqwest::Client>>>,
}

impl ChainProviders {
    pub fn from_env() -> Result<Self> {
        let mut providers = HashMap::new();

        let chain_configs = [
            (137u64, "RPC_URL_POLYGON"),
            (80002u64, "RPC_URL_POLYGON_AMOY"),
        ];

        for (chain_id, env_key) in chain_configs {
            if let Ok(url) = std::env::var(env_key) {
                if !url.is_empty() {
                    let parsed_url: Url = url.parse()
                        .with_context(|| format!("invalid URL for {env_key}"))?;
                    let provider = ProviderBuilder::new().on_http(parsed_url);
                    providers.insert(chain_id, provider);
                    tracing::info!(chain_id, env_key, "configured RPC provider");
                }
            }
        }

        if providers.is_empty() {
            tracing::warn!("no RPC providers configured");
        }

        Ok(Self { providers })
    }

    pub fn get(&self, chain_id: u64) -> Option<&alloy::providers::RootProvider<alloy::transports::http::Http<alloy::transports::http::reqwest::Client>>> {
        self.providers.get(&chain_id)
    }

    pub fn supported_chains(&self) -> Vec<u64> {
        self.providers.keys().copied().collect()
    }

    pub async fn get_token_balance(
        &self,
        chain_id: u64,
        token_address: Address,
        wallet_address: Address,
    ) -> Result<U256> {
        let provider = self.providers.get(&chain_id)
            .ok_or_else(|| anyhow::anyhow!("unsupported chain: {}", chain_id))?;

        let call_data = balanceOfCall { owner: wallet_address }.abi_encode();

        let tx = alloy::rpc::types::TransactionRequest::default()
            .to(token_address)
            .input(call_data.into());

        let result = provider.call(&tx).await
            .with_context(|| "balanceOf call failed")?;

        // ABI decode: 32-byte big-endian uint256
        let balance = U256::from_be_slice(result.as_ref());
        Ok(balance)
    }
}
