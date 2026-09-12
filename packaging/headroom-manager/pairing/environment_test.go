package pairing

import (
	"reflect"
	"testing"
)

func TestPeerEnvironmentDoesNotExportAccountSecrets(t *testing.T) {
	environment := []string{"PATH=tools", "SystemRoot=system", "TEMP=temp", "HOME=home", "WSL_INTEROP=socket", "WSL_DISTRO_NAME=Ubuntu", "WSLENV=PRIVATE_TOKEN/p", "wSlEnV=PRIVATE_TOKEN", "PRIVATE_TOKEN=synthetic", "HEADROOM_AUTH_TOKEN=synthetic", "USAGE_CONFIG=private", "SSH_AUTH_SOCK=private", "BASH_ENV=private", "NODE_OPTIONS=private", "malformed"}
	for _, test := range []struct {
		platform string
		want     []string
	}{
		{"windows", []string{"PATH=tools", "SystemRoot=system", "TEMP=temp"}},
		{"linux", []string{"PATH=tools", "HOME=home", "WSL_INTEROP=socket", "WSL_DISTRO_NAME=Ubuntu"}},
		{"other", []string{}},
	} {
		t.Run(test.platform, func(t *testing.T) {
			if got := peerEnvironment(environment, test.platform); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("peer environment = %v, want %v", got, test.want)
			}
		})
	}
}
