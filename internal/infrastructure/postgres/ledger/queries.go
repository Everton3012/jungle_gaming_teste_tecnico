package ledger

import _ "embed"

var (
	//go:embed queries/insert.sql
	insertQuery string

	//go:embed queries/find_by_id.sql
	findByIDQuery string

	//go:embed queries/find_by_wallet.sql
	findByWalletQuery string

	//go:embed queries/find_by_transaction.sql
	findByTransactionQuery string
)
