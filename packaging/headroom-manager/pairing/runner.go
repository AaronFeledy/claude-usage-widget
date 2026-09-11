package pairing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/contract"
)

type ExecuteFunc func(context.Context, string, []string, []byte) ([]byte, error)

// Runner invokes only an explicit native peer, never a remote network address.
type Runner struct {
	Config           Config
	Platform         string
	WSLExecutable    string
	TranslateWindows func(context.Context, string) (string, error)
	Execute          ExecuteFunc
	identity         Identity
}

func (r *Runner) command(ctx context.Context, executable string, args []string) (string, []string, error) {
	if r.Config.Validate() != nil {
		return "", nil, errors.New("invalid peer configuration")
	}
	if r.Platform == "windows" {
		if r.WSLExecutable == "" {
			return "", nil, errors.New("Windows system WSL executable is unavailable")
		}
		return r.WSLExecutable, append([]string{"--distribution", r.Config.Distribution, "--user", r.Config.User, "--exec", executable}, args...), nil
	}
	if r.Platform != "linux" || r.TranslateWindows == nil {
		return "", nil, errors.New("native Windows interop is unavailable")
	}
	program, err := r.TranslateWindows(ctx, executable)
	return program, args, err
}

func (r *Runner) invoke(ctx context.Context, executable string, args []string, input []byte) ([]byte, error) {
	program, arguments, err := r.command(ctx, executable, args)
	if err != nil {
		return nil, err
	}
	run := r.Execute
	if run == nil {
		run = execute
	}
	return run(ctx, program, arguments, input)
}

func (r *Runner) Call(ctx context.Context, request Request) (Response, error) {
	nonce, err := NewID()
	if err != nil {
		return Response{}, err
	}
	request.Schema, request.Product, request.Nonce, request.PairingID = 1, "Headroom", nonce, r.Config.ID
	if err = request.Validate(); err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(request)
	if err != nil || len(body) >= MaximumMessage {
		return Response{}, errors.New("paired request is too large")
	}
	entry := r.Config.WindowsEntry
	if r.Platform == "windows" {
		entry = r.Config.LinuxEntry
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	output, err := r.invoke(ctx, entry, []string{RPCArgument}, append(body, '\n'))
	if err != nil {
		return Response{}, fmt.Errorf("native paired command failed: %w", err)
	}
	var response Response
	if Decode(bytes.NewReader(output), &response) != nil || response.Schema != 1 || response.Product != "Headroom" || response.Nonce != nonce || response.PairingID != r.Config.ID {
		return Response{}, errors.New("paired reply identity is invalid")
	}
	if err = response.ValidateFor(request.Command); err != nil {
		return Response{}, err
	}
	if !response.OK {
		return response, errors.New("paired installation rejected the request; check its installation and pairing")
	}
	return response, nil
}

func (r *Runner) Inspect(ctx context.Context) (Identity, error) {
	response, err := r.Call(ctx, Request{Command: "inspect"})
	if err != nil {
		return Identity{}, err
	}
	if response.Identity == nil {
		return Identity{}, errors.New("paired inspection is missing")
	}
	platform := "windows"
	if r.Platform == "windows" {
		platform = "linux"
	}
	if err = r.Config.ValidateIdentity(*response.Identity, platform); err != nil {
		return Identity{}, err
	}
	r.identity = *response.Identity
	return r.identity, nil
}

func (r *Runner) RestoreProbe(identity Identity) error {
	platform := "windows"
	if r.Platform == "windows" {
		platform = "linux"
	}
	if err := r.Config.ValidateIdentity(identity, platform); err != nil {
		return err
	}
	r.identity = identity
	return nil
}

func (r *Runner) Prepare(ctx context.Context, version string) (Prepared, error) {
	response, err := r.Call(ctx, Request{Command: "prepare", Version: version})
	if err != nil {
		return Prepared{}, err
	}
	if response.Prepared == nil {
		return Prepared{}, errors.New("paired preparation is missing")
	}
	return *response.Prepared, nil
}

func (r *Runner) Commit(ctx context.Context, prepared Prepared) error {
	_, err := r.Call(ctx, Request{Command: "commit", Version: prepared.Version, Operation: prepared.Operation})
	return err
}

func (r *Runner) Verify(ctx context.Context, version string) error {
	if !r.identity.Trusted {
		return errors.New("paired installation has not been identified")
	}
	return WaitVerified(ctx, version, func(ctx context.Context) (Identity, error) {
		// Probe the verified immutable manager directly. Running the mutable
		// Windows console launcher here would prevent its own replacement.
		output, err := r.invoke(ctx, r.identity.Manager, []string{"inspect", "--install-root", r.identity.Root}, nil)
		if err != nil {
			return Identity{}, err
		}
		var reply struct {
			OK      bool                `json:"ok"`
			Command string              `json:"command"`
			Result  contract.Inspection `json:"result"`
			Error   string              `json:"error,omitempty"`
		}
		if Decode(bytes.NewReader(output), &reply) != nil || !reply.OK || reply.Command != "inspect" {
			return Identity{}, errors.New("paired progress inspection failed")
		}
		identity := r.identity
		identity.Version = reply.Result.Version
		identity.Trusted = reply.Result.TrustedIdentity && reply.Result.Complete && reply.Result.Platform == identity.Platform && reply.Result.Architecture == identity.Architecture && reply.Result.PackageKind == identity.Kind
		if version != r.identity.Version && reply.Result.ApplyStatus != "applied" {
			// Install state switches before candidate/service readiness. Only the
			// terminal apply result proves that the new generation is running.
			identity.Trusted = false
		}
		if reply.Result.ApplyStatus == "recovery_required" || reply.Result.ApplyStatus == "rolled_back" {
			identity.Trusted = false
		}
		if err := r.Config.ValidateIdentity(identity, identity.Platform); err != nil {
			return Identity{}, err
		}
		return identity, nil
	})
}

func execute(ctx context.Context, program string, args []string, input []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Stdin = bytes.NewReader(input)
	command.Stderr = io.Discard
	// Native interop carries only update metadata, not saved configuration,
	// bearer tokens, provider cookies, or inherited install associations.
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "HEADROOM_") || strings.HasPrefix(upper, "USAGE_") {
			continue
		}
		command.Env = append(command.Env, item)
	}
	var output messageBuffer
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

type messageBuffer struct{ bytes.Buffer }

func (b *messageBuffer) Write(value []byte) (int, error) {
	if b.Len()+len(value) > MaximumMessage {
		return 0, errors.New("native paired output exceeded its bound")
	}
	return b.Buffer.Write(value)
}
