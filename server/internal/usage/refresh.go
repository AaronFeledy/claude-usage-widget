package usage

type RefreshResult int

const (
	RefreshSuccess RefreshResult = iota
	RefreshInvalidGrant
	RefreshFailed
)

func (r RefreshResult) String() string {
	switch r {
	case RefreshSuccess:
		return "success"
	case RefreshInvalidGrant:
		return "invalid_grant"
	case RefreshFailed:
		return "failed"
	default:
		return "unknown"
	}
}
