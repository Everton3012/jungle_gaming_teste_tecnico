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
