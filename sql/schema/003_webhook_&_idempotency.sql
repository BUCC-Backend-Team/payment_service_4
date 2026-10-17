-- +goose Up
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    response_body TEXT NOT NULL DEFAULT '',
    response_status INT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS processed_webhook_events (
    key TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS callback_outbox (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered')),
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS callback_outbox_due_idx
    ON callback_outbox (status, next_attempt_at);


-- +goose Down
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS processed_webhook_events;
DROP TABLE IF EXISTS callback_outbox;
DROP INDEX IF EXISTS callback_outbox_due_idx;