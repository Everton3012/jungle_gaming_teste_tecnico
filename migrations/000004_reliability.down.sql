DROP INDEX IF EXISTS wager_transactions_pending_reference_idx;

ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_transactions_reference_attempts_non_negative,
    DROP COLUMN IF EXISTS reference_lease_until,
    DROP COLUMN IF EXISTS reference_next_attempt_at,
    DROP COLUMN IF EXISTS reference_attempts;

CREATE INDEX wager_transactions_pending_reference_idx
    ON wager_transactions (created_at, id)
    WHERE status = 'PENDING_REFERENCE';

DROP INDEX IF EXISTS outbox_events_publishable_idx;

ALTER TABLE outbox_events
    DROP COLUMN IF EXISTS locked_until;

CREATE INDEX outbox_events_pending_idx
    ON outbox_events (available_at, created_at, id)
    WHERE status = 'PENDING';

DROP INDEX IF EXISTS inbox_messages_status_idx;

ALTER TABLE inbox_messages
    DROP CONSTRAINT inbox_messages_pkey;

ALTER TABLE inbox_messages
    ADD CONSTRAINT inbox_messages_pkey PRIMARY KEY (message_id);

ALTER TABLE inbox_messages
    DROP COLUMN consumer_name;

CREATE INDEX inbox_messages_status_idx
    ON inbox_messages (status, updated_at, message_id);
