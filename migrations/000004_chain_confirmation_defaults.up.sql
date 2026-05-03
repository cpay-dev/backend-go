UPDATE checkout.payment_intents
SET required_confirmations = CASE
    WHEN lower(chain) IN ('ethereum', 'ethereum mainnet', 'mainnet') THEN 64
    WHEN lower(chain) = 'polygon' THEN 3
    WHEN lower(chain) IN ('arbitrum', 'arbitrum one') THEN 4800
    WHEN lower(chain) = 'base' THEN 600
    WHEN lower(chain) IN ('hyperevm', 'hyper evm', 'hyperliquid', 'hyperliquid evm') THEN 1
    WHEN lower(chain) IN ('bnb', 'bsc', 'bnb smart chain') THEN 2
    WHEN lower(chain) = 'optimism' THEN 600
    WHEN lower(chain) = 'solana' THEN 32
    WHEN lower(chain) = 'tron' THEN 19
    ELSE required_confirmations
END,
updated_at = NOW()
WHERE status IN ('created', 'awaiting_funds', 'partial', 'expired')
  AND required_confirmations = 12;
