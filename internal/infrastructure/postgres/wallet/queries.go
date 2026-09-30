package wallet

import _ "embed"

var (
	//go:embed queries/insert.sql
	insertQuery string

	//go:embed queries/find_by_id.sql
	findByIDQuery string

	//go:embed queries/find_by_player_currency.sql
	findByPlayerCurrencyQuery string

	//go:embed queries/update.sql
	updateQuery string
)
