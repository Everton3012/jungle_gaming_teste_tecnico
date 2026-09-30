package ledger

type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

func (d Direction) IsValid() bool {
	switch d {
	case DirectionDebit, DirectionCredit:
		return true
	default:
		return false
	}
}
