-- Inbox identity is scoped by consumer so the same broker message identifier can
-- safely be consumed by independent consumers.
ALTER TABLE inbox_messages
    ADD COLUMN consumer_name TEXT NOT NULL DEFAULT 'wager-transactions';

ALTER TABLE inbox_messages
    DROP CONSTRAINT inbox_messages_pkey;

ALTER TABLE inbox_messages
    ADD CONSTRAINT inbox_messages_pkey
        PRIMARY KEY (consumer_name, message_id);

DROP INDEX IF EXISTS inbox_messages_status_idx;

CREATE INDEX inbox_messages_status_idx
    ON inbox_messages (
        consumer_name,
        status,
        updated_at,
        message_id
    );

-- A publisher claim is a lease, not permanent ownership. PROCESSING rows whose
-- lease expired can be reclaimed after a process crash.
ALTER TABLE outbox_events
    ADD COLUMN locked_until TIMESTAMPTZ;

DROP INDEX IF EXISTS outbox_events_pending_idx;

CREATE INDEX outbox_events_publishable_idx
    ON outbox_events (
        available_at,
        locked_until,
        created_at,
        id
    )
    WHERE status IN ('PENDING', 'PROCESSING');

-- Retry state for pending references must survive process restarts and be safe
-- with several worker instances.
ALTER TABLE wager_transactions
    ADD COLUMN reference_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN reference_next_attempt_at TIMESTAMPTZ,
    ADD COLUMN reference_lease_until TIMESTAMPTZ;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_reference_attempts_non_negative
        CHECK (reference_attempts >= 0);

DROP INDEX IF EXISTS wager_transactions_pending_reference_idx;

CREATE INDEX wager_transactions_pending_reference_idx
    ON wager_transactions (
        reference_next_attempt_at,
        reference_lease_until,
        created_at,
        id
    )
    WHERE status = 'PENDING_REFERENCE';
