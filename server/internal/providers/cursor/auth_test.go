package cursor

import (
	"errors"
	"testing"
)

func Test_cookieFromAccessToken_rejects_invalid_last_subject_segment(t *testing.T) {
	token := jwtWithSub("auth0|user/bad", "payload")

	_, err := cookieFromAccessToken(token)

	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("error = %v, want ErrInvalidToken", err)
	}
}
