CREATE TABLE IF NOT EXISTS checkout.conversion_events (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    payment_link_id ulid REFERENCES catalog.payment_links(id) ON DELETE SET NULL,
    checkout_session_id ulid REFERENCES checkout.checkout_sessions(id) ON DELETE SET NULL,
    payment_intent_id ulid REFERENCES checkout.payment_intents(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    visitor_id TEXT,
    ip_hash TEXT,
    user_agent TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (event_type IN (
        'checkout_page_opened',
        'pay_clicked',
        'wallet_connected',
        'payment_made',
        'payment_incomplete',
        'checkout_canceled'
    ))
);

CREATE INDEX IF NOT EXISTS idx_conversion_events_merchant_created
    ON checkout.conversion_events (merchant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversion_events_merchant_type_created
    ON checkout.conversion_events (merchant_id, event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversion_events_session_type
    ON checkout.conversion_events (checkout_session_id, event_type);
