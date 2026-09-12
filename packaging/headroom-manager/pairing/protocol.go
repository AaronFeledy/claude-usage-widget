package pairing

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/contract"
	"io"
)

const MaximumMessage = 32 << 10

type Request struct {
	Schema    int     `json:"schema"`
	Product   string  `json:"product"`
	Nonce     string  `json:"nonce"`
	PairingID string  `json:"pairing_id"`
	Command   string  `json:"command"`
	Config    *Config `json:"config,omitempty"`
	Version   string  `json:"version,omitempty"`
	Operation string  `json:"operation,omitempty"`
}

type Response struct {
	Schema    int       `json:"schema"`
	Product   string    `json:"product"`
	Nonce     string    `json:"nonce"`
	PairingID string    `json:"pairing_id"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	Identity  *Identity `json:"identity,omitempty"`
	Prepared  *Prepared `json:"prepared,omitempty"`
}

func Decode(reader io.Reader, value any) error {
	body, err := io.ReadAll(io.LimitReader(reader, MaximumMessage+1))
	if err != nil || len(body) == 0 || len(body) > MaximumMessage {
		return errors.New("invalid paired message size")
	}
	// Reject duplicate object keys, including nested identity/config objects.
	tokens := json.NewDecoder(bytes.NewReader(body))
	if err = uniqueValue(tokens, 0); err != nil {
		return err
	}
	if _, err = tokens.Token(); err != io.EOF {
		return errors.New("paired message has trailing data")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil {
		return errors.New("invalid paired message")
	}
	return nil
}

func uniqueValue(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("paired message is too deep")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return errors.New("paired message contains duplicate or invalid keys")
			}
			seen[name] = true
			if err = uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid paired object")
		}
	case '[':
		for decoder.More() {
			if err := uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("invalid paired array")
		}
	default:
		return errors.New("invalid paired message token")
	}
	return nil
}

func (r Request) Validate() error {
	if r.Schema != 1 || r.Product != "Headroom" || !validID(r.Nonce) || !validID(r.PairingID) {
		return errors.New("invalid paired request identity")
	}
	if r.Version != "" {
		if _, err := contract.CompareVersions(r.Version, r.Version); err != nil {
			return err
		}
	}
	switch r.Command {
	case "offer", "probe":
		if r.Config == nil || r.Config.Validate() != nil || r.Config.ID != r.PairingID || r.Version != "" || r.Operation != "" {
			return errors.New("invalid pairing handshake")
		}
	case "inspect":
		if r.Config != nil || r.Version != "" || r.Operation != "" {
			return errors.New("invalid paired inspection")
		}
	case "prepare":
		if r.Config != nil || r.Version == "" || r.Operation != "" {
			return errors.New("invalid paired preparation")
		}
	case "commit":
		if r.Config != nil || r.Version == "" || !validID(r.Operation) {
			return errors.New("invalid paired commit")
		}
	default:
		return errors.New("unsupported paired command")
	}
	return nil
}

func (r Response) ValidateFor(command string) error {
	if !r.OK {
		if r.Identity != nil || r.Prepared != nil || !safeText(r.Error, 160) {
			return errors.New("invalid paired failure response")
		}
		return nil
	}
	if r.Error != "" {
		return errors.New("paired success contains an error")
	}
	switch command {
	case "inspect", "offer", "probe":
		if r.Identity == nil || r.Prepared != nil {
			return errors.New("invalid paired identity response")
		}
	case "prepare":
		if r.Prepared == nil || r.Identity != nil {
			return errors.New("invalid paired preparation response")
		}
	case "commit":
		if r.Prepared != nil || r.Identity != nil {
			return errors.New("invalid paired commit response")
		}
	default:
		return errors.New("unknown paired reply command")
	}
	return nil
}
