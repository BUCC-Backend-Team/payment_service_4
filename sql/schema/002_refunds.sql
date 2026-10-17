-- +goose Up
CREATE TABLE IF NOT EXISTS refunds (
    id UUID PRIMARY KEY,
    payment_id UUID NOT NULL REFERENCES payments(id),
    amount BIGINT NOT NULL CHECK (amount > 0),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending',
        'processed',
        'failed'
    )),
    paystack_refund_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS refunds_payment_id_idx ON refunds (payment_id);
CREATE INDEX IF NOT EXISTS refunds_status_idx ON refunds (status, updated_at);

-- +goose Down
DROP TABLE IF EXISTS refunds;
DROP INDEX IF EXISTS refunds_payment_id_idx;
DROP INDEX IF EXISTS refunds_status_idx;