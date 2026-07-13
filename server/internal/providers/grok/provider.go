package grok

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

const (
	providerName       = "Grok"
	billingURL         = "https://cli-chat-proxy.grok.com/v1/billing"
	settingsURL        = "https://cli-chat-proxy.grok.com/v1/settings"
	tokenURL           = "https://auth.x.ai/oauth2/token"
	oauthClientID      = "b1a00492-073a-47ea-816f-4c329264a828"
	tokenAuthHeader    = "xai-grok-cli"
	userAgent          = "ClaudeUsageWidget"
	defaultHTTPTimeout = 10 * time.Second
	refreshBuffer      = 5 * time.Minute
)

var (
	errInvalidGrant     = errors.New("grok: invalid grant")
	errMissingAuthEntry = errors.New("grok: missing auth.x.ai entry")
	errRefreshFailed    = errors.New("grok: refresh failed")
)

type Options struct {
	CredentialsPath string
	HTTPClient      *http.Client
	BillingURL      string
	SettingsURL     string
	TokenURL        string
	Now             func() time.Time
}

type Provider struct {
	credentialsPath string
	httpClient      *http.Client
	billingURL      string
	settingsURL     string
	tokenURL        string
	now             func() time.Time
	store           credentialStore
}

func NewProvider(opts Options) (*Provider, error) {
	path, err := resolveCredentialsPath(opts.CredentialsPath)
	if err != nil {
		return nil, err
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	provider := &Provider{
		credentialsPath: path,
		httpClient:      client,
		billingURL:      defaultString(opts.BillingURL, billingURL),
		settingsURL:     defaultString(opts.SettingsURL, settingsURL),
		tokenURL:        defaultString(opts.TokenURL, tokenURL),
		now:             opts.Now,
	}
	if provider.now == nil {
		provider.now = func() time.Time { return time.Now().UTC() }
	}
	return provider, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Fetch(ctx context.Context) (usage.UsageData, error) {
	data := baseUsageData()
	creds, err := p.credentials(ctx)
	if err != nil {
		return p.authError(data, err)
	}
	if creds.needsRefresh(p.now()) {
		creds, err = p.refresh(ctx, creds, refreshReasonProactive)
		if err != nil {
			return p.refreshError(data, err)
		}
	}
	return p.fetchWithToken(ctx, data, creds)
}

func (p *Provider) fetchWithToken(ctx context.Context, data usage.UsageData, creds credentials) (usage.UsageData, error) {
	billingResp, err := p.sendGet(ctx, p.billingURL, creds.accessToken)
	if err != nil {
		return data, err
	}

	if billingResp.StatusCode == http.StatusUnauthorized || billingResp.StatusCode == http.StatusForbidden {
		if err := drainAndClose(billingResp); err != nil {
			return data, err
		}
		refreshed, err := p.refresh(ctx, creds, refreshReasonUnauthorized)
		if err != nil {
			return p.refreshError(data, err)
		}
		creds = refreshed
		billingResp, err = p.sendGet(ctx, p.billingURL, creds.accessToken)
		if err != nil {
			return data, err
		}
	}

	if billingResp.StatusCode == http.StatusUnauthorized || billingResp.StatusCode == http.StatusForbidden {
		if err := drainAndClose(billingResp); err != nil {
			return data, err
		}
		return reauth(data), nil
	}
	if billingResp.StatusCode < http.StatusOK || billingResp.StatusCode >= http.StatusMultipleChoices {
		if err := drainAndClose(billingResp); err != nil {
			return data, err
		}
		data.Error = strPtr(fmt.Sprintf("Grok billing request failed (%d). Try again later.", billingResp.StatusCode))
		return data, nil
	}
	if err := mapBilling(billingResp.Body, &data, p.now()); err != nil {
		if closeErr := drainAndClose(billingResp); closeErr != nil {
			return data, closeErr
		}
		data.Error = strPtr("Grok billing response changed.")
		return data, nil
	}
	if err := drainAndClose(billingResp); err != nil {
		return data, err
	}
	p.populateSettings(ctx, creds.accessToken, &data)
	return data, nil
}

func (p *Provider) authError(data usage.UsageData, err error) (usage.UsageData, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return data, err
	}
	if errors.Is(err, errMissingAuthEntry) {
		return reauth(data), nil
	}
	data.Error = strPtr("Grok auth error. Run `grok login`.")
	return data, nil
}

func (p *Provider) refreshError(data usage.UsageData, err error) (usage.UsageData, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return data, err
	}
	if errors.Is(err, errInvalidGrant) {
		return reauth(data), nil
	}
	data.Error = strPtr("Token refresh failed. Will retry.")
	return data, nil
}

func baseUsageData() usage.UsageData {
	return usage.UsageData{
		ProviderName:   providerName,
		PrimaryLabel:   "Credits",
		SecondaryLabel: "Pay as you go",
		ShowSecondary:  false,
		ReauthCommand:  strPtr("grok login"),
	}
}

func reauth(data usage.UsageData) usage.UsageData {
	data.Error = strPtr("Grok auth expired. Run `grok login` again.")
	data.NeedsReauth = true
	return data
}

func defaultString(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func strPtr(value string) *string { return &value }

var _ usage.Provider = (*Provider)(nil)
