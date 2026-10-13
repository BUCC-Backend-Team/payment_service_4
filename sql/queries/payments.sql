-- name: GetPaymentByIdempotencyKey :one
SELECT *
FROM payments
WHERE idempotency_key = $1
LIMIT 1;

-- name: CreatePayment :one
INSERT INTO payments (
    idempotency_key,
    order_id,
    amount,
    currency,
    customer_email,
    paystack_reference
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;

-- name: UpdatePaymentCheckout :one
UPDATE payments
SET
    checkout_url = $2,
    paystack_reference = $3,
    access_code = $4,
    updated_at = NOW()
WHERE id = $1
    AND status = 'pending'
RETURNING *;
