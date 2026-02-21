use std::collections::HashMap;

use alloy::primitives::{Address, B256, U256};
use alloy::providers::{Provider, ProviderBuilder};
use alloy::sol;
use alloy::sol_types::SolCall;
use alloy::transports::http::reqwest::Url;
use anyhow::{Context, Result};

sol! {
    function balanceOf(address owner) external view returns (uint256);
    function allowance(address owner, address spender) external view returns (uint256);
}

pub struct TxReceiptInfo {
    pub success: bool,
    pub from: Address,
    pub to: Option<Address>,
    pub block_number: u64,
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

    pub async fn verify_transaction(
        &self,
        chain_id: u64,
        tx_hash: B256,
    ) -> Result<TxReceiptInfo> {
        let provider = self.providers.get(&chain_id)
            .ok_or_else(|| anyhow::anyhow!("unsupported chain: {}", chain_id))?;

        let receipt = provider.get_transaction_receipt(tx_hash).await
            .context("get_transaction_receipt")?
            .ok_or_else(|| anyhow::anyhow!("receipt not found for tx {}", tx_hash))?;

        let tx = provider.get_transaction_by_hash(tx_hash).await
            .context("get_transaction_by_hash")?
            .ok_or_else(|| anyhow::anyhow!("tx not found: {}", tx_hash))?;

        Ok(TxReceiptInfo {
            success: receipt.status(),
            from: tx.from,
            to: tx.to(),
            block_number: receipt.block_number.unwrap_or(0),
        })
    }

    pub async fn check_allowance(
        &self,
        chain_id: u64,
        token_address: Address,
        owner: Address,
        spender: Address,
    ) -> Result<U256> {
        let provider = self.providers.get(&chain_id)
            .ok_or_else(|| anyhow::anyhow!("unsupported chain: {}", chain_id))?;

        let call_data = allowanceCall { owner, spender }.abi_encode();

        let tx = alloy::rpc::types::TransactionRequest::default()
            .to(token_address)
            .input(call_data.into());

        let result = provider.call(&tx).await
            .with_context(|| "allowance call failed")?;

        let value = U256::from_be_slice(result.as_ref());
        Ok(value)
    }
}
