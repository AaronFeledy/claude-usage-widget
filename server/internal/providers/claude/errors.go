package claude

import (
	"errors"
	"fmt"
)

var (
	ErrCredentialsMissing   = errors.New("claude: credentials missing")
	ErrCredentialsMalformed = errors.New("claude: credentials malformed")
	ErrUpstream             = errors.New("claude: upstream error")
	ErrInvalidGrant         = errors.New("claude: invalid grant")
)

type credentialsError struct {
	path string
	err  error
}

func (e credentialsError) Error() string {
	return fmt.Sprintf("claude credentials %s: %v", e.path, e.err)
}

func (e credentialsError) Unwrap() error { return e.err }
