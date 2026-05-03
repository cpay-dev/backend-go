CREATE TABLE IF NOT EXISTS platform.email_notifications (
    id ulid PRIMARY KEY,
    event_id ulid NOT NULL,
    event_type TEXT NOT NULL,
    merchant_id ulid,
    template TEXT NOT NULL,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    provider_message_id TEXT,
    last_error TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, template, recipient),
    CHECK (status IN ('pending', 'sent', 'failed'))
);
