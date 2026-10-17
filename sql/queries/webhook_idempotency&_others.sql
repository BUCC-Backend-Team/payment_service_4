-- name: ReserveIdempotencyKey :execrows
INSERT INTO idempotency_keys (key, request_hash)
VALUES ($1, $2)
ON CONFLICT (key) DO NOTHING;


-- name: GetIdempotencyRecord :one
SELECT key, request_hash, response_body, response_status
FROM idempotency_keys
WHERE key = $1;


-- name: StoreIdempotencyResponse :exec
UPDATE idempotency_keys
SET response_body = $2,
    response_status = $3,
    updated_at = NOW()
WHERE key = $1;


-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_keys
WHERE key = $1;


-- name: RecordProcessedEvent :execrows
INSERT INTO processed_webhook_events (key)
VALUES ($1)
ON CONFLICT (key) DO NOTHING;


-- name: EnqueueCallback :exec
INSERT INTO callback_outbox (id, event_type, payload)
VALUES ($1, $2, $3);


-- name: ListDueCallbacks :many
SELECT id, event_type, payload, status, attempts
FROM callback_outbox
WHERE status = 'pending'
  AND attempts < $1
  AND next_attempt_at <= NOW()
ORDER BY next_attempt_at, created_at
LIMIT $2
FOR UPDATE SKIP LOCKED;


-- name: MarkCallbackDelivered :exec
UPDATE callback_outbox
SET status = 'delivered', delivered_at = NOW()
WHERE id = $1;


-- name: MarkCallbackFailed :exec
UPDATE callback_outbox
SET attempts = attempts + 1,
    next_attempt_at = NOW() + make_interval(secs => $2::double precision)
WHERE id = $1;

-- name: ResetAll :exec
TRUNCATE callback_outbox, processed_webhook_events, idempotency_keys, refunds, payments CASCADE;