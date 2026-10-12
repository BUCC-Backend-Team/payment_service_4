package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func connectPostgres(uri string) (*sql.DB, error) {
	db, err := sql.Open("postgres", uri)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL database: %v", err)
	}

	logger.Info("successfully connected to PostgreSQL database")
	return db, nil
}