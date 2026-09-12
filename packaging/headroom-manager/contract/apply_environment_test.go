package contract

import (
	"strings"
	"testing"
)

func TestApplyHandoffDoesNotRetainPublicLauncherLifetime(t *testing.T) {
	environment := authoritativeApplyEnvironment([]string{"HEADROOM_PUBLIC_LAUNCHER_PID=123", "HEADROOM_PUBLIC_LAUNCHER_PATH=old", "headroom_public_launcher_token=old", "PATH=tools"}, map[string]string{"HEADROOM_INSTALL_ROOT": "new-root"})
	for _, item := range environment {
		if strings.HasPrefix(strings.ToUpper(item), "HEADROOM_PUBLIC_LAUNCHER_") {
			t.Fatal("replacement tied to old launcher", item)
		}
	}
	if len(environment) != 2 {
		t.Fatal(environment)
	}
}
