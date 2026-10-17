-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'callback_outbox'
          AND column_name = 'type'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'callback_outbox'
          AND column_name = 'event_type'
    ) THEN
        ALTER TABLE callback_outbox RENAME COLUMN "type" TO event_type;
    ELSIF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'callback_outbox'
          AND column_name = 'event_type'
    ) THEN
        ALTER TABLE callback_outbox ADD COLUMN event_type TEXT NOT NULL DEFAULT 'unknown';
        ALTER TABLE callback_outbox ALTER COLUMN event_type DROP DEFAULT;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'callback_outbox'
          AND column_name = 'event_type'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'callback_outbox'
          AND column_name = 'type'
    ) THEN
        ALTER TABLE callback_outbox RENAME COLUMN event_type TO "type";
    END IF;
END
$$;
-- +goose StatementEnd
