package usage

import (
	"context"
	"encoding/json"
	"time"
)

type Provider interface {
	Name() string
	Fetch(context.Context) (UsageData, error)
}

type UsageBucket struct {
	Utilization float64
	ResetsAt    *time.Time
}

func (b UsageBucket) MarshalJSON() ([]byte, error) {
	type bucketJSON struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    *string `json:"resets_at"`
	}
	var resetsAt *string
	if b.ResetsAt != nil {
		formatted := b.ResetsAt.UTC().Format(time.RFC3339)
		resetsAt = &formatted
	}
	return json.Marshal(bucketJSON{Utilization: b.Utilization, ResetsAt: resetsAt})
}

type UsageData struct {
	ProviderName        string
	PrimaryLabel        string
	SecondaryLabel      string
	ShowSecondary       bool
	Subtitle            *string
	PrimaryStatusText   *string
	SecondaryStatusText *string
	ReauthCommand       *string
	Current             UsageBucket
	Weekly              UsageBucket
	Error               *string
	NeedsReauth         bool
}

func (d UsageData) MarshalJSON() ([]byte, error) {
	type usageJSON struct {
		ProviderName        string      `json:"provider_name"`
		PrimaryLabel        string      `json:"primary_label"`
		SecondaryLabel      string      `json:"secondary_label"`
		ShowSecondary       bool        `json:"show_secondary"`
		Subtitle            *string     `json:"subtitle"`
		PrimaryStatusText   *string     `json:"primary_status_text"`
		SecondaryStatusText *string     `json:"secondary_status_text"`
		ReauthCommand       *string     `json:"reauth_command"`
		Current             UsageBucket `json:"current"`
		Weekly              UsageBucket `json:"weekly"`
		Error               *string     `json:"error"`
		NeedsReauth         bool        `json:"needs_reauth"`
		IsSuccess           bool        `json:"is_success"`
	}
	return json.Marshal(usageJSON{
		ProviderName:        d.ProviderName,
		PrimaryLabel:        d.PrimaryLabel,
		SecondaryLabel:      d.SecondaryLabel,
		ShowSecondary:       d.ShowSecondary,
		Subtitle:            d.Subtitle,
		PrimaryStatusText:   d.PrimaryStatusText,
		SecondaryStatusText: d.SecondaryStatusText,
		ReauthCommand:       d.ReauthCommand,
		Current:             d.Current,
		Weekly:              d.Weekly,
		Error:               d.Error,
		NeedsReauth:         d.NeedsReauth,
		IsSuccess:           d.Error == nil,
	})
}
