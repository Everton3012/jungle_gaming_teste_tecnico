package wagertransaction

type Status string

const (
	StatusPending          Status = "PENDING"
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
	StatusFailed           Status = "FAILED"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPending,
		StatusPendingReference,
		StatusProcessed,
		StatusRejected,
		StatusFailed:
		return true
	default:
		return false
	}
}

func (s Status) IsTerminal() bool {
	switch s {
	case StatusProcessed, StatusRejected, StatusFailed:
		return true
	default:
		return false
	}
}
