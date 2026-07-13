package codex

import "errors"

var (
	ErrCredentialsMissing   = errors.New("codex: credentials missing")
	ErrCredentialsMalformed = errors.New("codex: credentials malformed")
	ErrRefreshInvalidGrant  = errors.New("codex: refresh invalid grant")
	ErrRefreshFailed        = errors.New("codex: refresh failed")
)
