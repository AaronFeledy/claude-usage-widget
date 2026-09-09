package contract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var fixtureExecutable string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	root, err := os.MkdirTemp("", "headroom-native-fixture-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer os.RemoveAll(root)
	ext := ".exe"
	if runtime.GOOS != "windows" {
		ext = ""
	}
	fixtureExecutable = filepath.Join(root, "headroom-fixture"+ext)
	source := filepath.Join(root, "main.go")
	program := `package main
import ("encoding/json"; "os"; "path/filepath"; "strconv"; "strings"; "time")
func main() {
 var ready string
 for i:=1; i<len(os.Args); i++ { if os.Args[i]=="--headroom-ready-file" && i+1<len(os.Args) { ready=os.Args[i+1]; i++ } }
 executable,_:=os.Executable(); generation:=filepath.Base(filepath.Dir(filepath.Dir(executable))); version:=strings.Split(generation,".generation-")[0]
 if os.Getenv("HEADROOM_FIXTURE_CRASH_VERSION")==version { os.Exit(8) }
 if ready!="" && os.Getenv("HEADROOM_FIXTURE_NO_READY_VERSION")!=version { b,_:=json.Marshal(map[string]any{"nonce":os.Getenv("HEADROOM_READY_NONCE"),"pid":os.Getpid(),"version":version,"executable":executable}); _=os.WriteFile(ready,append(b,'\n'),0600) }
 if value:=os.Getenv("HEADROOM_FIXTURE_SLEEP_MS"); value!="" { if n,e:=strconv.Atoi(value); e==nil { time.Sleep(time.Duration(n)*time.Millisecond) } }
 if os.Getenv("HEADROOM_FIXTURE_FAIL")!="" { os.Exit(9) }
}`
	if err = os.WriteFile(source, []byte(program), 0o600); err == nil {
		command := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", fixtureExecutable, source)
		command.Env = append(os.Environ(), "GOTOOLCHAIN=local")
		output, buildErr := command.CombinedOutput()
		if buildErr != nil {
			err = fmt.Errorf("compile native fixture: %w: %s", buildErr, output)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return m.Run()
}
