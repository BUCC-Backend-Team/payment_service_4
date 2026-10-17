-- name: CreatePayment :one
INSERT INTO payments (
    id, order_id, amount, currency, customer_email,
    return_url, status, reference, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING
    id, order_id, amount, currency, customer_email,
    return_url, status, channel, reference, late,
    checkout_url, created_at, updated_at, expires_at,
    succeeded_at, expired_at;


-- name: GetPaymentByID :one
SELECT
    id, order_id, amount, currency, customer_email,
    return_url, status, channel, reference, late,
    checkout_url, created_at, updated_at, expires_at,
    succeeded_at, expired_at
FROM payments
WHERE id = $1;


-- name: GetPaymentByReference :one
SELECT
    id, order_id, amount, currency, customer_email,
    return_url, status, channel, reference, late,
    checkout_url, created_at, updated_at, expires_at,
    succeeded_at, expired_at
FROM payments
WHERE reference = $1;


-- name: GetPaymentByIDForUpdate :one
SELECT
    id, order_id, amount, currency, customer_email,
    return_url, status, channel, reference, late,
    checkout_url, created_at, updated_at, expires_at,
    succeeded_at, expired_at
FROM payments
WHERE id = $1
FOR UPDATE;


-- name: GetPaymentByReferenceForUpdate :one
SELECT
    id, order_id, amount, currency, customer_email,
    return_url, status, channel, reference, late,
    checkout_url, created_at, updated_at, expires_at,
    succeeded_at, expired_at
FROM payments
WHERE reference = $1
FOR UPDATE;


-- name: UpdatePaymentCheckout :exec
UPDATE payments
SET checkout_url = $2, updated_at = NOW()
WHERE id = $1;


-- name: MarkPaymentSucceeded :exec
UPDATE payments
SET status = 'succeeded',
    channel = sqlc.narg(channel),
    late = $2,
    succeeded_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND status IN ('pending', 'expired');


-- name: MarkPaymentExpired :exec
UPDATE payments
SET status = 'expired',
    expired_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND status = 'pending';


-- name: SetPaymentRefundState :exec
UPDATE payments
SET status = CASE WHEN $2 >= amount THEN 'refunded'
                  ELSE 'partially_refunded' END,
    updated_at = NOW()
WHERE id = $1 AND status IN ('succeeded', 'partially_refunded');


-- name: ListPendingPayments :many
SELECT
    id, order_id, amount, currency, customer_email,
    return_url, status, channel, reference, late,
    checkout_url, created_at, updated_at, expires_at,
    succeeded_at, expired_at
FROM payments
WHERE status = 'pending'
ORDER BY created_at
LIMIT $1;