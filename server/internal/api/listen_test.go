package api_test

import (
	"errors"
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/api"
)

func Test_ValidateStartup_allows_loopback_without_auth_and_off_loopback_with_auth(t *testing.T) {
	// Given
	addresses := []string{"127.0.0.1:7823", "localhost:7823", "[::1]:7823"}
	for _, address := range addresses {
		// When
		err := api.ValidateStartup(address, "")

		// Then
		if err != nil {
			t.Fatalf("ValidateStartup(%q) returned error: %v", address, err)
		}
	}
	if err := api.ValidateStartup("0.0.0.0:7823", "secret"); err != nil {
		t.Fatalf("ValidateStartup with auth returned error: %v", err)
	}
}

func Test_ValidateStartup_rejects_unsafe_empty_auth_binds(t *testing.T) {
	// Given
	addresses := []string{"0.0.0.0:7823", ":7823", "[::]:7823", "192.168.1.5:7823", "example.com:7823", "not an addr"}
	for _, address := range addresses {
		// When
		err := api.ValidateStartup(address, "")

		// Then
		if err == nil {
			t.Fatalf("ValidateStartup(%q) succeeded, want error", address)
		}
		if address != "not an addr" && !errors.Is(err, api.ErrUnsafeBind) {
			t.Fatalf("ValidateStartup(%q) error = %v, want ErrUnsafeBind", address, err)
		}
	}
}
