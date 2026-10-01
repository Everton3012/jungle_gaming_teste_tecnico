CREATE TABLE outbox_events (
    id TEXT PRIMARY KEY,

    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,

    payload JSONB NOT NULL,

    status TEXT NOT NULL DEFAULT 'PENDING',

    attempts INTEGER NOT NULL DEFAULT 0,

    available_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,

    last_error TEXT,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT outbox_events_status_valid
        CHECK (
            status IN (
                'PENDING',
                'PROCESSING',
                'PUBLISHED'
            )
        ),

    CONSTRAINT outbox_events_attempts_non_negative
        CHECK (attempts >= 0),

    CONSTRAINT outbox_events_timestamps_valid
        CHECK (updated_at >= created_at),

    CONSTRAINT outbox_events_published_state_valid
        CHECK (
            (
                status = 'PUBLISHED'
                AND published_at IS NOT NULL
            )
            OR
            (
                status <> 'PUBLISHED'
                AND published_at IS NULL
            )
        )
);

CREATE INDEX outbox_events_pending_idx
    ON outbox_events (
        available_at,
        created_at,
        id
    )
    WHERE status = 'PENDING';

CREATE INDEX outbox_events_aggregate_idx
    ON outbox_events (
        aggregate_type,
        aggregate_id,
        created_at,
        id
    );