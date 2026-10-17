-- name: CreateRefund :one
INSERT INTO refunds (id, payment_id, amount, status)
VALUES ($1, $2, $3, $4)
RETURNING
    id, payment_id, amount, status, paystack_refund_id,
    created_at, updated_at, processed_at;


-- name: GetRefundByID :one
SELECT
    id, payment_id, amount, status, paystack_refund_id,
    created_at, updated_at, processed_at
FROM refunds
WHERE id = $1;


-- name: GetRefundByIDForUpdate :one
SELECT
    id, payment_id, amount, status, paystack_refund_id,
    created_at, updated_at, processed_at
FROM refunds
WHERE id = $1
FOR UPDATE;


-- name: ListActiveRefundsByPaymentID :many
SELECT
    id, payment_id, amount, status, paystack_refund_id,
    created_at, updated_at, processed_at
FROM refunds
WHERE payment_id = $1 AND status IN ('pending', 'processed')
ORDER BY created_at;


-- name: ListRefundsByPaymentID :many
SELECT
    id, payment_id, amount, status, paystack_refund_id,
    created_at, updated_at, processed_at
FROM refunds
WHERE payment_id = $1
ORDER BY created_at;


-- name: ListStuckPendingRefunds :many
SELECT
    id, payment_id, amount, status, paystack_refund_id,
    created_at, updated_at, processed_at
FROM refunds
WHERE status = 'pending'
  AND updated_at <= NOW() - make_interval(secs => $1::double precision)
ORDER BY created_at
LIMIT $2;


-- name: MarkRefundProcessed :exec
UPDATE refunds
SET status = 'processed',
    paystack_refund_id = CASE WHEN $2 > 0 THEN $2 ELSE paystack_refund_id END,
    processed_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND status = 'pending';


-- name: MarkRefundFailed :exec
UPDATE refunds
SET status = 'failed', updated_at = NOW()
WHERE id = $1 AND status = 'pending';


-- name: SetRefundPaystackID :exec
UPDATE refunds
SET paystack_refund_id = $2, updated_at = NOW()
WHERE id = $1;


-- name: SumActiveRefunds :one
SELECT COALESCE(SUM(amount), 0)::bigint
FROM refunds
WHERE payment_id = $1 AND status IN ('pending', 'processed');


-- name: SumProcessedRefunds :one
SELECT COALESCE(SUM(amount), 0)::bigint
FROM refunds
WHERE payment_id = $1 AND status = 'processed';