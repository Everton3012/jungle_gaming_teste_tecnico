package wagertransaction

import _ "embed"

var (
	//go:embed queries/insert.sql
	insertQuery string

	//go:embed queries/update.sql
	updateQuery string

	//go:embed queries/find_by_id.sql
	findByIDQuery string

	//go:embed queries/find_by_provider_external_id.sql
	findByProviderExternalIDQuery string

	//go:embed queries/find_by_provider_idempotency_key.sql
	findByProviderIdempotencyKeyQuery string

	//go:embed queries/find_pending_references.sql
	findPendingReferencesQuery string
)

const claimPendingReferenceIDsQuery = `
WITH candidates AS (
    SELECT id
    FROM wager_transactions
    WHERE status = 'PENDING_REFERENCE'
      AND COALESCE(reference_next_attempt_at, created_at) <= $1
      AND (reference_lease_until IS NULL OR reference_lease_until <= $1)
    ORDER BY COALESCE(reference_next_attempt_at, created_at), created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT $2
)
UPDATE wager_transactions AS transaction
SET
    reference_attempts = transaction.reference_attempts + 1,
    reference_lease_until = $3
FROM candidates
WHERE transaction.id = candidates.id
RETURNING transaction.id, transaction.reference_attempts;
`

const schedulePendingReferenceQuery = `
UPDATE wager_transactions
SET
    reference_next_attempt_at = $2,
    reference_lease_until = NULL
WHERE id = $1
  AND status = 'PENDING_REFERENCE';
`

const releasePendingReferenceQuery = `
UPDATE wager_transactions
SET reference_lease_until = NULL
WHERE id = $1
  AND status = 'PENDING_REFERENCE';
`
