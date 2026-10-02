package outbox

const insertQuery = `
INSERT INTO outbox_events (
    id,
    event_type,
    aggregate_type,
    aggregate_id,
    payload,
    status,
    attempts,
    available_at,
    published_at,
    last_error,
    created_at,
    updated_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12
);
`

const findByIDQuery = `
SELECT
    id,
    event_type,
    aggregate_type,
    aggregate_id,
    payload,
    status,
    attempts,
    available_at,
    published_at,
    last_error,
    created_at,
    updated_at
FROM outbox_events
WHERE id = $1;
`

const claimPendingQuery = `
WITH candidates AS (
    SELECT id
    FROM outbox_events
    WHERE (
            status = 'PENDING'
            AND available_at <= $1
          )
       OR (
            status = 'PROCESSING'
            AND (locked_until IS NULL OR locked_until <= $1)
          )
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT $2
)
UPDATE outbox_events AS event
SET
    status = 'PROCESSING',
    attempts = event.attempts + 1,
    locked_until = $1 + INTERVAL '30 seconds',
    updated_at = $1
FROM candidates
WHERE event.id = candidates.id
RETURNING
    event.id,
    event.event_type,
    event.aggregate_type,
    event.aggregate_id,
    event.payload,
    event.status,
    event.attempts,
    event.available_at,
    event.published_at,
    event.last_error,
    event.created_at,
    event.updated_at;
`

const markPublishedQuery = `
UPDATE outbox_events
SET
    status = 'PUBLISHED',
    published_at = $2,
    last_error = NULL,
    locked_until = NULL,
    updated_at = $2
WHERE id = $1
  AND status = 'PROCESSING';
`

const rescheduleQuery = `
UPDATE outbox_events
SET
    status = 'PENDING',
    available_at = $2,
    published_at = NULL,
    last_error = $3,
    locked_until = NULL,
    updated_at = $4
WHERE id = $1
  AND status = 'PROCESSING';
`
