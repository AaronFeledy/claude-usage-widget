package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTerminalTextCannotControlTheScreen(t *testing.T) {
	attack := "provider\x1b]52;c;secret\a\r\n\u202ereordered\u009b" + strings.Repeat("x", 400)
	provider := Provider{ProviderName: attack, Subtitle: &attack, IsSuccess: true,
		Buckets: []Bucket{{ID: "weekly", Label: attack, StatusText: &attack}}}
	result := render([]Provider{provider, {ProviderName: "Error", Error: &attack}}, time.Now(), false, map[string]warningState{})
	if strings.ContainsAny(result, "\x1b\a\r\u202e\u009b") || strings.Contains(result, strings.Repeat("x", 121)) {
		t.Fatalf("unsafe terminal text: %q", result)
	}
	if !strings.Contains(result, "…") {
		t.Fatal("long labels were not bounded")
	}
}

func TestTerminalConventions(t *testing.T) {
	for _, test := range []struct {
		name      string
		args, env []string
		ansi      bool
	}{
		{"normal", []string{"--once"}, nil, true},
		{"no-color-empty", []string{"--once"}, []string{"NO_COLOR="}, false},
		{"plain", []string{"--plain"}, nil, false},
		{"dumb", nil, []string{"TERM=dumb"}, false},
		{"json", []string{"--json"}, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, stdout, _ := baseOptions(fixtureUsage(t))
			options.Args, options.Env = test.args, append([]string{}, test.env...)
			options.IsTerminal = func(io.Writer) bool { return true }
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := Run(ctx, options); err != nil {
				t.Fatal(err)
			}
			if ctx.Err() != nil {
				t.Fatal("one-shot mode unexpectedly watched")
			}
			if strings.Contains(stdout.String(), "\x1b[") != test.ansi {
				t.Fatalf("incorrect terminal controls: %q", stdout.String())
			}
		})
	}
}

func TestWatchRetainsReadingsAndRecovers(t *testing.T) {
	body := fixtureUsage(t)
	options, stdout, _ := baseOptions(body)
	options.Args = []string{"--watch", "--plain"}
	options.WatchInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	requests := 0
	options.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		if requests == 2 {
			return nil, errors.New("temporary outage")
		}
		if requests == 4 {
			cancel()
			return nil, context.Canceled
		}
		return response(body), nil
	})
	if err := Run(ctx, options); err != nil {
		t.Fatal(err)
	}
	if requests != 4 || strings.Count(stdout.String(), "ChatGPT") != 3 || !strings.Contains(stdout.String(), "last good readings") {
		t.Fatalf("watch did not recover: %s", stdout.String())
	}
}

func TestJSONWatchKeepsDiagnosticsOffStdout(t *testing.T) {
	body := fixtureUsage(t)
	options, stdout, stderr := baseOptions(body)
	options.Args = []string{"--watch", "--json"}
	options.WatchInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	options.FetchLocal = func(context.Context) (LocalSnapshot, error) {
		calls++
		if calls == 1 {
			return LocalSnapshot{}, errors.New("offline")
		}
		if calls == 3 {
			cancel()
			return LocalSnapshot{}, ctx.Err()
		}
		return LocalSnapshot{Body: body, Status: "ready"}, nil
	}
	if err := Run(ctx, options); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(stdout.Bytes()) || !strings.Contains(stderr.String(), "retrying") {
		t.Fatal("JSON output contained diagnostics or lost the valid snapshot")
	}
}

func TestLocalBridgeDoesNotFallBackOnFailure(t *testing.T) {
	for _, localErr := range []error{ErrLocalUnavailable, errors.New("private bridge failed")} {
		options, _, _ := baseOptions(fixtureUsage(t))
		requests := 0
		options.FetchLocal = func(context.Context) (LocalSnapshot, error) { return LocalSnapshot{}, localErr }
		options.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { requests++; return response([]byte("[]")), nil })
		err := Run(context.Background(), options)
		if errors.Is(localErr, ErrLocalUnavailable) {
			if err != nil || requests != 1 {
				t.Fatal("absent desktop did not allow local discovery")
			}
		} else if err == nil || requests != 0 {
			t.Fatal("failed bridge silently changed transport")
		}
	}
}

func TestCalendarPacingAndProviderOrder(t *testing.T) {
	for _, test := range []struct{ date, want string }{{"2026-03-31", "2026-02-28"}, {"2024-03-31", "2024-02-29"}, {"2026-05-31", "2026-04-30"}} {
		value, _ := time.Parse("2006-01-02", test.date)
		if got := previousMonth(value).Format("2006-01-02"); got != test.want {
			t.Fatalf("%s became %s", test.date, got)
		}
	}
	body := []byte(`[{"provider_name":"Grok","error":null,"is_success":true,"needs_reauth":false,"buckets":[]},{"provider_name":"Claude","error":null,"is_success":true,"needs_reauth":false,"buckets":[]}]`)
	providers, err := decodeUsage(body)
	if err != nil || providers[0].ProviderName != "Grok" {
		t.Fatal("saved provider order was changed")
	}
	for _, bad := range [][]byte{[]byte("null"), bytes.Replace(body, []byte(`"needs_reauth":false`), []byte(`"needs_reauth":null`), 1)} {
		if _, err := decodeUsage(bad); err == nil {
			t.Fatal("invalid API status accepted")
		}
	}
}

func TestWeeklyJSONPreservesCompatibleFields(t *testing.T) {
	body := []byte(`[{"future_field":{"enabled":true},"buckets":[{"id":"session"},{"id":"weekly","new_field":42}]}]`)
	filtered, err := filterWeeklyJSON(body)
	if err != nil || !bytes.Contains(filtered, []byte(`"future_field"`)) || !bytes.Contains(filtered, []byte(`"new_field":42`)) || bytes.Contains(filtered, []byte(`"session"`)) {
		t.Fatalf("filtered JSON lost compatible fields: %s %v", filtered, err)
	}
}
