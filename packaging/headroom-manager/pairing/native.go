package pairing

import (
	"context"
	"errors"
	"os"

	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/contract"
)

const preparedName = "pairing/prepared-update.json"

type preparedRecord struct {
	Schema    int                   `json:"schema"`
	Product   string                `json:"product"`
	PairingID string                `json:"pairing_id"`
	Operation string                `json:"operation"`
	Result    contract.UpdateResult `json:"result"`
}

// Native delegates acquisition, verification, process identity, and apply to
// the existing native updater. The peer gets an opaque operation, never a path.
type Native struct {
	Root        string
	Config      Config
	InspectFunc func(context.Context) (Identity, error)
	ApplyFunc   func(context.Context, contract.UpdateResult) error
	Client      contract.UpdateClient
}

func LoadConfig(root string) (Config, error) {
	var config Config
	if err := contract.ReadPrivateJSON(root, RecordName, &config); err != nil {
		return Config{}, err
	}
	return config, config.Validate()
}

func SaveConfig(root string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	return contract.WritePrivateJSON(root, RecordName, config)
}

func (n *Native) Inspect(ctx context.Context) (Identity, error) {
	if n.InspectFunc == nil {
		return Identity{}, errors.New("native paired inspection is unavailable")
	}
	return n.InspectFunc(ctx)
}

func (n *Native) Prepare(ctx context.Context, version string) (Prepared, error) {
	if n.Config.Validate() != nil {
		return Prepared{}, errors.New("native pair identity is invalid")
	}
	result, err := n.Client.StageVersion(ctx, n.Root, version)
	if err != nil {
		return Prepared{}, err
	}
	if result.Status == "current" && result.Version == version {
		return Prepared{Version: version, Current: true}, nil
	}
	if result.Status != "staged" || result.Stage == nil || result.Version != version {
		return Prepared{}, errors.New("native paired update did not produce the exact stage")
	}
	operation, err := NewID()
	if err != nil {
		return Prepared{}, err
	}
	if err = contract.WritePrivateJSON(n.Root, preparedName, preparedRecord{Schema: 1, Product: "Headroom", PairingID: n.Config.ID, Operation: operation, Result: result}); err != nil {
		return Prepared{}, err
	}
	return Prepared{Version: version, Operation: operation}, nil
}

func (n *Native) Commit(ctx context.Context, prepared Prepared) error {
	if n.ApplyFunc == nil || n.Config.Validate() != nil || !validID(prepared.Operation) || prepared.Current {
		return errors.New("native paired apply is unavailable")
	}
	var record preparedRecord
	if err := contract.ReadPrivateJSON(n.Root, preparedName, &record); err != nil {
		return err
	}
	if record.Schema != 1 || record.Product != "Headroom" || record.PairingID != n.Config.ID || record.Operation != prepared.Operation || record.Result.Version != prepared.Version || record.Result.Stage == nil || record.Result.Status != "staged" {
		return errors.New("native prepared update identity changed")
	}
	// PrepareApply revalidates the stored package and stage before committing.
	return n.ApplyFunc(ctx, record.Result)
}

func (n *Native) Verify(ctx context.Context, version string) error {
	return WaitVerified(ctx, version, n.Inspect)
}

func (n *Native) Coordinator(peer Endpoint, platform string) Coordinator {
	return Coordinator{Config: n.Config, Platform: platform, Local: n, Peer: peer,
		Latest: func(ctx context.Context) (string, error) {
			result, err := n.Client.Check(ctx, n.Root)
			if err != nil {
				return "", err
			}
			if result.Status == "unavailable" {
				return "", errors.New("no compatible paired release is available")
			}
			return result.Version, nil
		},
		ReadProgress: func() (Progress, error) {
			var value Progress
			err := contract.ReadPrivateJSON(n.Root, ProgressName, &value)
			if errors.Is(err, os.ErrNotExist) {
				return Progress{}, nil
			}
			return value, err
		},
		SaveProgress: func(value Progress) error { return contract.WritePrivateJSON(n.Root, ProgressName, value) },
	}
}
