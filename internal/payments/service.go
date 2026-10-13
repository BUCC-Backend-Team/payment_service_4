
package payments

import (
	"database/sql"
	"log/slog"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
)

type config struct {
	paystackSecretKey string
	logger            *slog.Logger
	db                *database.Queries
	DB                *sql.DB
	merchantApiKey string
}

var cfg *config

func New(paystackSecretKey string, logger *slog.Logger, db *database.Queries, DB *sql.DB, merchantApiKey string) {
	cfg = &config{
		paystackSecretKey: paystackSecretKey,
		logger:            logger,
		db:                db,
		DB:                DB,
		merchantApiKey: merchantApiKey,
	}
}