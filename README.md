# Marketly Payment Service

payment service the Marketly calls to start, check and refund payments.

- Money is in kobo (integers). The only currency is NGN.
- Outcomes arrive on `/webhooks/paystack` from Paystack, and leave on `CALLBACK_URL` to Marketly as signed callbacks.

## API

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | /health | none | `{ "status": "ok" }` |
| POST | /payments | Bearer | Requires `Idempotency-Key`. 201 on create, replays with `Idempotent-Replayed: true`. 422 when the same key is reused with a different body, 409 `payment_exists` when the order already has a pending or succeeded payment. |
| GET | /payments/:id | Bearer | status, channel, amountRefunded. 404 when unknown. |
| POST | /payments/:id/refunds | Bearer | Requires `Idempotency-Key`. Optional `amount`, defaults to the remaining refundable balance. 202 `{ refundId, amount, status: "pending" }`, 409 `not_refundable`. |
| POST | /webhooks/paystack | none | HMAC-SHA512 signed Paystack events. Handles `charge.success`, `refund.processed`, `refund.failed`; unknown events are acknowledged 200. |
| POST | /admin/reset | none | Only with `TEST_MODE=true`. Truncates all tables, returns 204. |

States: `pending → succeeded | expired`, `expired → succeeded` (verified late transfer only), `succeeded → partially_refunded → refunded`. Refunds go `pending → processed | failed`. Invalid transitions are rejected in code, and enforced again at the SQL level with conditional updates.

## Configuration
```
cp .env.example .env
```

| Variable | Meaning |
|---|---|
| `PORT` | HTTP port, default 4000. |
| `DATABASE_URL` | PostgreSQL connection string. |
| `PAYSTACK_SECRET_KEY` | Paystack test secret key (`sk_test_...`). |
| `MERCHANT_API_KEY` | Key Marketly sends as `Authorization: Bearer ...`. |
| `CALLBACK_URL` | Where signed callbacks are POSTed to Marketly. |
| `CALLBACK_SECRET` | Secret used to sign callbacks. |
| `PAYMENT_TIMEOUT_SECONDS` | How long a payment stays pending before the expiry worker acts. |
| `PAYMENT_EXPIRY_WORKER_INTERVAL_SECONDS` | How often the expiry worker runs. |
| `CALLBACK_WORKER_INTERVAL_SECONDS` | How often the callback outbox worker runs. |
| `TEST_MODE` | Enables `POST /admin/reset`. |
| `PAYSTACK_BASE_URL` | useful for local tests against a fake. |


## Running locally

1. Start PostgreSQL and point `DATABASE_URL` at an empty database.
2. Apply the migrations and generate code.

```
./migrate.sh up        # goose applies sql/schema migrations
./migrate.sh generate  # sqlc regenerates internal/database from sql/queries
```

3. Run the service.

```
go run .
```

4. Expose Paystack webhooks with a tunnel when testing against real Paystack:

```
ngrok http 4000
# set the https://...ngrok-free.app/webhooks/paystack URL in the Paystack
# dashboard under Settings > API Keys & Webhooks > Test Webhook URL, leaving
# Test Callback URL empty because the service passes callback_url per transaction
```

## How the pieces fit

```
internal/
  api/         HTTP wiring: health, auth guard, admin reset, routes
  callbacks/   event envelope builder, HMAC signer, outbox worker
  config/      env loading and validation
  database/    sqlc generated queries plus a small hand written Store
  errors/      typed application errors mapped to HTTP statuses
  jobs/        expiry and reconciliation worker
  payments/    payment service, validation, lifecycle transitions
  providers/   Paystack client behind the PaymentProvider interface
  refunds/     refund creation, submission, idempotent replay
  webhooks/    signature verification, event dispatch, dedupe
```

Guarantees worth knowing about:

- **Idempotent writes.** Every request fills `idempotency_keys(key, request_hash, response_body)` under a primary key that rejects duplicates. The payment id doubles as the Paystack reference, so a retried initialize after a timeout hits Paystack duplicate-reference rejection instead of creating a second transaction.
- **Concurrency.** Database guarantees, not read-then-write checks in Go: a partial unique index blocks two active (`pending` or `succeeded`) payments for one order; refunds sum synchronously against a row lock so pending refunds count against the balance until they fail; webhook state changes ride the same transaction that records the dedupe key.
- **Callbacks.** Written to an outbox in the same transaction as the state change, then delivered by a worker with exponential backoff (`1s, 2s, 4s, …`, many attempts, same id across retries). Undelivered callbacks survive a restart because they stay in Postgres.
- **Expiry and reconciliation.** One worker queries Paystack for payments that are still pending past their expiry, flips them to `succeeded` if Paystack shows success (a missed webhook) or to `expired` otherwise. The same pass re-checks stuck refunds, so the database and Paystack agree even if webhooks were lost while the service was down. A verified late `charge.success` after expiry marks the payment succeeded and the callback carries `late: true`.

## Tests

```
go test ./...
```

Covers config validation, request validation, HTTP handler shapes, callback signing, webhook parsing and dedupe keys, and provider call classification against an httptest server. The schemas in `sql/schema` are the source of truth for state transitions and are verified against those tests by `goose` migrations.