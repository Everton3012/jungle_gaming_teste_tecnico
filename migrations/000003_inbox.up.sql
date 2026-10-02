CREATE TABLE inbox_messages (
    message_id TEXT PRIMARY KEY,

    payload_hash TEXT NOT NULL,

    status TEXT NOT NULL DEFAULT 'PROCESSING',

    attempts INTEGER NOT NULL DEFAULT 1,

    last_error TEXT,

    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT inbox_messages_status_valid
        CHECK (
            status IN (
                'PROCESSING',
                'PROCESSED',
                'FAILED'
            )
        ),

    CONSTRAINT inbox_messages_attempts_positive
        CHECK (attempts > 0),

    CONSTRAINT inbox_messages_timestamps_valid
        CHECK (updated_at >= created_at),

    CONSTRAINT inbox_messages_processed_state_valid
        CHECK (
            (
                status = 'PROCESSED'
                AND processed_at IS NOT NULL
            )
            OR
            (
                status <> 'PROCESSED'
                AND processed_at IS NULL
            )
        )
);

CREATE INDEX inbox_messages_status_idx
    ON inbox_messages (
        status,
        updated_at,
        message_id
    );