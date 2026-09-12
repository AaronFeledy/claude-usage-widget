package pairing

import "strings"

// peerEnvironment carries process-launch plumbing, never inherited account
// configuration. WSLENV can export arbitrary variables into another kernel or
// account; executable paths are translated explicitly by NativeRunner instead.
func peerEnvironment(environment []string, platform string) []string {
	allowed := map[string]bool{}
	switch platform {
	case "windows":
		for _, key := range []string{"SYSTEMROOT", "SYSTEMDRIVE", "WINDIR", "COMSPEC", "PATH", "PATHEXT", "TEMP", "TMP", "USERPROFILE", "LOCALAPPDATA", "APPDATA"} {
			allowed[key] = true
		}
	case "linux":
		for _, key := range []string{"PATH", "HOME", "USER", "LOGNAME", "TMPDIR", "WSL_INTEROP", "WSL_DISTRO_NAME"} {
			allowed[key] = true
		}
	}
	result := make([]string, 0, len(allowed))
	for _, item := range environment {
		key, _, found := strings.Cut(item, "=")
		if platform == "windows" {
			key = strings.ToUpper(key)
		}
		if found && allowed[key] {
			result = append(result, item)
		}
	}
	return result
}
