package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/cli"
	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/contract"
)

type desktopUpdateReply struct {
	OK           bool   `json:"ok"`
	Status       string `json:"status"`
	Version      string `json:"version"`
	RequestNonce string `json:"request_nonce"`
}

func fetchLocalDesktop(ctx context.Context) (cli.LocalSnapshot, error) {
	root := os.Getenv("HEADROOM_INSTALL_ROOT")
	if root == "" {
		return cli.LocalSnapshot{}, cli.ErrLocalUnavailable
	}
	inspection := contract.InspectInstall(root)
	if inspection.PackageKind == contract.PackageKindCLI {
		return cli.LocalSnapshot{}, cli.ErrLocalUnavailable
	}
	application, _, err := contract.ActiveExecutableForRole(root, contract.RoleApplication)
	if err != nil {
		return cli.LocalSnapshot{}, err
	}
	payload := []byte(`{"schema":1,"product":"Headroom","command":"usage"}`)
	command := exec.CommandContext(ctx, application, "--headroom-cli-request")
	command.Stdin = bytes.NewReader(payload)
	var output bytes.Buffer
	command.Stdout = &limitedWriter{writer: &output, remaining: 1 << 20}
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3 {
			return cli.LocalSnapshot{}, cli.ErrLocalUnavailable
		}
		return cli.LocalSnapshot{}, fmt.Errorf("desktop usage request failed: %w", err)
	}
	var reply struct {
		OK       bool            `json:"ok"`
		Result   json.RawMessage `json:"result"`
		Status   string          `json:"status"`
		LastGood int64           `json:"last_good"`
	}
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	var trailing any
	if decoder.Decode(&reply) != nil || !errors.Is(decoder.Decode(&trailing), io.EOF) || !reply.OK || len(reply.Result) == 0 ||
		(reply.Status != "ready" && reply.Status != "offline" && reply.Status != "connecting" && reply.Status != "setup") || reply.LastGood < 0 {
		return cli.LocalSnapshot{}, errors.New("desktop returned an invalid usage reply")
	}
	snapshot := cli.LocalSnapshot{Body: append([]byte(nil), reply.Result...), Status: reply.Status}
	if reply.LastGood > 0 {
		snapshot.LastGood = time.Unix(reply.LastGood, 0)
	}
	return snapshot, nil
}

func managedInstall() (string, contract.Inspection, string, error) {
	root := os.Getenv("HEADROOM_INSTALL_ROOT")
	if root == "" {
		return "", contract.Inspection{}, "", errors.New("updates require an official current-user Headroom installation")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", contract.Inspection{}, "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", contract.Inspection{}, "", err
	}
	expected, inspection, err := contract.ActiveExecutableForRole(root, contract.RoleCLI)
	if err != nil || !sameExecutable(expected, executable) {
		return "", inspection, "", errors.New("updates require the active verified Headroom CLI")
	}
	if inspection.CLIEntryPath == "" {
		return "", inspection, "", errors.New("this installation predates the managed CLI receipt; reinstall Headroom once")
	}
	return root, inspection, executable, nil
}

func updateCommand(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return errors.New("update does not accept arguments")
	}
	return updateLocal(ctx, "")
}

func updateLocal(ctx context.Context, exactVersion string) error {
	root, inspection, executable, err := managedInstall()
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if exactVersion != "" {
		staged, err := contract.DefaultUpdateClient().StageVersion(ctx, root, exactVersion)
		if err != nil {
			return err
		}
		if staged.Status == "current" {
			fmt.Printf("Headroom %s is current.\n", exactVersion)
			return nil
		}
		if err = applyStagedUpdate(ctx, staged); err != nil {
			return err
		}
		fmt.Printf("Headroom %s update accepted.\n", exactVersion)
		return nil
	}
	participants, err := updateParticipants(root, executable)
	if err != nil {
		return err
	}
	if inspection.PackageKind != contract.PackageKindCLI {
		reply, available, bridgeErr := requestDesktopUpdate(ctx, root, participants, nil)
		if bridgeErr != nil {
			return bridgeErr
		}
		if available {
			if !reply.OK {
				return fmt.Errorf("desktop declined update: %s", reply.Status)
			}
			fmt.Printf("Headroom update %s (%s).\n", reply.Status, reply.Version)
			return nil
		}
	}
	staged, err := contract.DefaultUpdateClient().StageLatest(ctx, root)
	if err != nil {
		return err
	}
	if staged.Status == "current" {
		fmt.Printf("Headroom %s is current.\n", inspection.Version)
		return nil
	}
	if staged.Status != "staged" || staged.Stage == nil {
		return fmt.Errorf("no compatible update is available (%s)", staged.Status)
	}
	if err = applyOfflineStage(ctx, root, inspection, executable, participants, staged); err != nil {
		return err
	}
	fmt.Printf("Headroom %s is staged and will finish installing now.\n", staged.Version)
	return nil
}

// applyStagedUpdate applies an exact stage already acquired and verified for
// this installation. Pairing uses this boundary so peer data can select only a
// version; it can never provide an archive, origin, or staging path.
func applyStagedUpdate(ctx context.Context, staged contract.UpdateResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if staged.Status == "current" {
		return nil
	}
	if staged.Status != "staged" || staged.Stage == nil || strings.TrimSpace(staged.Version) == "" {
		return errors.New("the prepared Headroom update is invalid")
	}
	root, inspection, executable, err := managedInstall()
	if err != nil {
		return err
	}
	if err = verifyLocalStage(root, inspection, staged); err != nil {
		return err
	}
	participants, err := updateParticipants(root, executable)
	if err != nil {
		return err
	}
	if inspection.PackageKind != contract.PackageKindCLI {
		reply, available, bridgeErr := requestDesktopUpdate(ctx, root, participants, staged.Stage)
		if bridgeErr != nil {
			return bridgeErr
		}
		if available {
			if !reply.OK || (reply.Status != "accepted" && reply.Status != "current") || reply.Version != staged.Version {
				return errors.New("desktop declined the prepared update")
			}
			return nil
		}
	}
	return applyOfflineStage(ctx, root, inspection, executable, participants, staged)
}

func verifyLocalStage(root string, inspection contract.Inspection, staged contract.UpdateResult) error {
	record := filepath.Join(filepath.Dir(filepath.Dir(staged.Stage.PackageRoot)), "verified-stage.json")
	loaded, manifest, err := contract.LoadVerifiedStage(root, record)
	if err != nil {
		return err
	}
	if loaded != *staged.Stage || manifest.Version != staged.Version || staged.CurrentVersion != inspection.Version ||
		loaded.PackageKind != inspection.PackageKind || loaded.Platform != inspection.Platform || loaded.Architecture != inspection.Architecture {
		return errors.New("the prepared Headroom update does not match this installation")
	}
	return nil
}

func applyOfflineStage(ctx context.Context, root string, inspection contract.Inspection, executable string, participants []contract.ProcessClaim, staged contract.UpdateResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyLocalStage(root, inspection, staged); err != nil {
		return err
	}
	manager, _, err := contract.ImmutableManagerExecutable(root)
	if err != nil {
		return err
	}
	prepared, err := contract.PrepareApply(manager, contract.ApplyRequest{
		InstallRoot: root, EntryPath: inspection.CLIEntryPath,
		StageRecord: filepath.Join(filepath.Dir(filepath.Dir(staged.Stage.PackageRoot)), "verified-stage.json"),
		CurrentPID:  os.Getpid(), CurrentExecutable: executable, CurrentRole: contract.RoleCLI, CandidateRole: contract.RoleCLI,
		AdditionalProcesses: participants[1:], WaitTimeoutMS: 30000, ReadyTimeoutMS: 30000,
	})
	if err != nil {
		return err
	}
	if err = startDetachedApply(prepared.ManagerPath, prepared.RequestPath); err != nil {
		return err
	}
	if err = contract.WaitForApplyAcknowledgement(prepared, 10*time.Second); err != nil {
		return err
	}
	// Cancellation before commit leaves the acknowledged helper to expire
	// without changing the installation. After commit it owns the transaction.
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = contract.CommitPreparedApply(prepared); err != nil {
		return err
	}
	return nil
}

func updateParticipants(root, executable string) ([]contract.ProcessClaim, error) {
	participants := []contract.ProcessClaim{{Role: contract.RoleCLI, PID: os.Getpid(), Executable: executable}}
	if runtime.GOOS == "windows" {
		pidValue, path := os.Getenv("HEADROOM_PUBLIC_LAUNCHER_PID"), os.Getenv("HEADROOM_PUBLIC_LAUNCHER_PATH")
		if pidValue == "" && path == "" {
			return participants, nil
		}
		pid, parseErr := strconv.Atoi(pidValue)
		expected, _, expectedErr := contract.ActiveExecutableForRole(root, contract.RolePublicLauncher)
		if parseErr != nil || pid <= 0 || !filepath.IsAbs(path) || expectedErr != nil || !sameExecutable(expected, path) {
			return nil, errors.New("Windows updates require the verified public Headroom launcher")
		}
		participants = append(participants, contract.ProcessClaim{Role: contract.RolePublicLauncher, PID: pid, Executable: filepath.Clean(path)})
	}
	return participants, nil
}

func requestDesktopUpdate(ctx context.Context, root string, participants []contract.ProcessClaim, preparedStage *contract.StageResult) (desktopUpdateReply, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	application, _, err := contract.ActiveExecutableForRole(root, contract.RoleApplication)
	if err != nil {
		return desktopUpdateReply{}, false, nil
	}
	nonceBytes := make([]byte, 24)
	if _, err = rand.Read(nonceBytes); err != nil {
		return desktopUpdateReply{}, false, err
	}
	nonce := hex.EncodeToString(nonceBytes)
	claims := make([]map[string]any, 0, len(participants))
	for _, item := range participants {
		claims = append(claims, map[string]any{"role": item.Role, "pid": item.PID, "executable": item.Executable})
	}
	request := map[string]any{"schema": 1, "product": "Headroom", "command": "update", "install_root": root, "request_nonce": nonce, "additional_processes": claims}
	if preparedStage != nil {
		request["prepared_stage"] = preparedStage
	}
	payload, _ := json.Marshal(request)
	command := exec.CommandContext(ctx, application, "--headroom-cli-request")
	command.Stdin = bytes.NewReader(payload)
	var output bytes.Buffer
	command.Stdout = &limitedWriter{writer: &output, remaining: 4096}
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3 {
			return desktopUpdateReply{}, false, nil
		}
		return desktopUpdateReply{}, false, fmt.Errorf("desktop update request failed: %w", err)
	}
	var reply desktopUpdateReply
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	var trailing any
	if decoder.Decode(&reply) != nil || !errors.Is(decoder.Decode(&trailing), io.EOF) || reply.RequestNonce != nonce || strings.TrimSpace(reply.Status) == "" ||
		(reply.OK && (reply.Status != "accepted" && reply.Status != "current" || reply.Version == "")) {
		return reply, true, errors.New("desktop returned an invalid update reply")
	}
	return reply, true, nil
}

func desktopCommand(_ context.Context, args []string) error {
	root, inspection, _, err := managedInstall()
	if err != nil {
		return err
	}
	if inspection.PackageKind == contract.PackageKindCLI {
		return errors.New("the desktop app is not installed")
	}
	application, _, err := contract.ActiveExecutableForRole(root, contract.RoleApplication)
	if err != nil {
		return err
	}
	process, err := os.StartProcess(application, append([]string{application}, args...), &os.ProcAttr{Env: os.Environ(), Files: []*os.File{nil, os.Stdout, os.Stderr}})
	if err != nil {
		return err
	}
	return process.Release()
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(value []byte) (int, error) {
	if len(value) > w.remaining {
		return 0, errors.New("response is too large")
	}
	n, err := w.writer.Write(value)
	w.remaining -= n
	return n, err
}

func sameExecutable(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func writePrivateReady(path string, data []byte, root string) error {
	root, err := contract.ValidateInstallRoot(root)
	if err != nil {
		return errors.New("update readiness root is invalid")
	}
	path = filepath.Clean(path)
	directory := filepath.Dir(path)
	if filepath.Base(path) != "ready.json" || filepath.Dir(directory) != filepath.Join(root, "transactions") || !strings.HasPrefix(filepath.Base(directory), "apply-") {
		return errors.New("update readiness path is invalid")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("update readiness directory is invalid")
	}
	temporary := fmt.Sprintf("%s.new-%d", path, os.Getpid())
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	failed := true
	defer func() {
		_ = file.Close()
		if failed {
			_ = os.Remove(temporary)
		}
	}()
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temporary, path); err != nil {
		return err
	}
	failed = false
	return nil
}
