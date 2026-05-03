UPDATE checkout.payment_intents
SET required_confirmations = CASE
    WHEN lower(chain) = 'polygon' THEN 6
    WHEN lower(chain) IN ('bnb', 'bsc', 'bnb smart chain') THEN 6
    WHEN lower(chain) IN ('hyperevm', 'hyper evm', 'hyperliquid', 'hyperliquid evm') THEN 3
    WHEN lower(chain) = 'tron' THEN 21
    ELSE required_confirmations
END,
updated_at = NOW()
WHERE status IN ('created', 'awaiting_funds', 'partial', 'expired')
  AND lower(chain) IN (
    'polygon',
    'bnb', 'bsc', 'bnb smart chain',
    'hyperevm', 'hyper evm', 'hyperliquid', 'hyperliquid evm',
    'tron'
  );
