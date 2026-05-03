UPDATE checkout.payment_intents
SET required_confirmations = CASE
    WHEN lower(chain) IN ('arbitrum', 'arbitrum one') THEN 20
    WHEN lower(chain) IN ('base', 'hyperevm', 'hyper evm', 'hyperliquid', 'hyperliquid evm', 'optimism') THEN 12
    ELSE required_confirmations
END,
updated_at = NOW()
WHERE status IN ('created', 'awaiting_funds', 'partial', 'expired')
  AND required_confirmations < 12;
