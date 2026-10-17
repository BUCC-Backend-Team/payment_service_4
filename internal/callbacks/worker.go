package callbacks

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
)

const (
	maxOutboxAttempts = 1_000_000
	maxDeliveryBatch  = 25
	deliveryTimeout   = 10 * time.Second
)

type Worker struct {
	URL      string
	Secret   string
	Store    *database.Store
	Interval time.Duration
	Log      *slog.Logger
	HTTP     *http.Client

	BackoffBase time.Duration
}

func NewWorker(url, secret string, store *database.Store, interval time.Duration, log *slog.Logger) *Worker {
	if interval <= 0 {
		interval = time.Second
	}
	return &Worker{
		URL:         url,
		Secret:      secret,
		Store:       store,
		Interval:    interval,
		Log:         log,
		HTTP:        &http.Client{Timeout: deliveryTimeout},
		BackoffBase: time.Second,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.deliverOnce(ctx)
		}
	}
}

func (w *Worker) deliverOnce(ctx context.Context) {
	tx, err := w.Store.BeginTx(ctx, nil)
	if err != nil {
		w.Log.Error("callbacks: begin outbox tx", "error", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	txStore := w.Store.WithTx(tx)

	due, err := txStore.ListDueCallbacks(ctx, database.ListDueCallbacksParams{
		Attempts: maxOutboxAttempts,
		Limit:    maxDeliveryBatch,
	})
	if err != nil {
		w.Log.Error("callbacks: list due callbacks", "error", err)
		return
	}

	for _, callback := range due {
		delivered, err2 := w.deliver(ctx, callback.Payload)
		if err2 != nil {
			w.Log.Error("callbacks: delivery error", "id", callback.ID, "error", err2)
		}
		if delivered {
			if err := txStore.MarkCallbackDelivered(ctx, callback.ID); err != nil {
				w.Log.Error("callbacks: mark delivered", "id", callback.ID, "error", err)
			}
			continue
		}
		if err := txStore.MarkCallbackFailed(ctx, database.MarkCallbackFailedParams{
			ID:      callback.ID,
			Column2: w.retryDelay(int(callback.Attempts)).Seconds(),
		}); err != nil {
			w.Log.Error("callbacks: schedule retry", "id", callback.ID, "error", err)
		}
	}

	if err := tx.Commit(); err != nil {
		w.Log.Error("callbacks: commit outbox tx", "error", err)
	}
}

func (w *Worker) retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 3 {
		attempt = 3
	}
	base := w.BackoffBase
	if base <= 0 {
		base = time.Second
	}
	return (1 << attempt) * base
}

func (w *Worker) deliver(ctx context.Context, payload []byte) (bool, error) {
	deliverCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(deliverCtx, http.MethodPost, w.URL, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MarketlySignature", Sign(w.Secret, time.Now(), payload))

	resp, err := w.HTTP.Do(req)
	if err != nil {
		w.Log.Warn("callbacks: delivery failed", "id", req.URL, "error", err)
		return false, nil
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode >= 200 && resp.StatusCode <= 299, nil
}
