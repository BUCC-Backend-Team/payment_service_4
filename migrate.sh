#!/usr/bin/env bash

set -euo pipefail

# Load environment variables
if [[ -f .env ]]; then
    set -a
    source .env
    set +a
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
    echo "Error: DATABASE_URL is not set in .env"
    exit 1
fi

MIGRATIONS_DIR="./sql/schema"
SQLC_CONFIG="./sqlc.yaml"

case "${1:-}" in
    generate)
        echo "Generating Go code with sqlc..."
        sqlc generate
        echo "Code generation complete."
        ;;

    up)
        echo "Applying database migrations..."
        goose -dir "$MIGRATIONS_DIR" postgres "$DATABASE_URL" up
        echo "Migrations applied."
        ;;

    down)
        echo "Rolling back the last database migration..."
        goose -dir "$MIGRATIONS_DIR" postgres "$DATABASE_URL" down
        echo "Migration rolled back."
        ;;

    *)
        echo "Usage: $0 {generate|up|down}"
        exit 1
        ;;
esac