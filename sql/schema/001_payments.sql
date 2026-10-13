-- +goose Up
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    order_id TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'NGN' CHECK (currency = 'NGN'),
    customer_email TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending',
        'success',
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
    paystack_reference TEXT NOT NULL UNIQUE,
    checkout_url TEXT,
    access_code TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    succeeded_at TIMESTAMP,
    expired_at TIMESTAMP
);

CREATE UNIQUE INDEX payments_one_active_per_order
    ON payments (order_id)
    WHERE status = 'pending';

CREATE INDEX payments_status_created_at_idx
    ON payments (status, created_at);

-- +goose Down
DROP TABLE IF EXISTS payments;
