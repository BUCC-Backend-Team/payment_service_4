-- +goose Up
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY,
    order_id TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'NGN' CHECK (currency = 'NGN'),
    customer_email TEXT NOT NULL,
    return_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending',
        'succeeded',
        'expired',
        'partially_refunded',
        'refunded'
    )),
    channel TEXT CHECK (channel IS NULL OR channel IN (
        'card',
        'bank',
        'bank_transfer',
        'ussd'
    )),
    reference UUID NOT NULL UNIQUE,
    late BOOLEAN NOT NULL DEFAULT FALSE,
    checkout_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    succeeded_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS payments_one_active_per_order
    ON payments (order_id)
    WHERE status IN ('pending', 'succeeded');

CREATE INDEX IF NOT EXISTS payments_status_created_at_idx
    ON payments (status, created_at);

-- +goose Down
DROP TABLE IF EXISTS payments;
DROP INDEX IF EXISTS payments_one_active_per_order;
DROP INDEX IF EXISTS payments_status_created_at_idx;