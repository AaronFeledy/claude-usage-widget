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

type Bucket struct {
	ID          string
	Label       string
	Utilization float64
	ResetsAt    *time.Time
	StatusText  *string
}

func (b Bucket) MarshalJSON() ([]byte, error) {
	type bucketJSON struct {
		ID          string  `json:"id"`
		Label       string  `json:"label"`
		Utilization float64 `json:"utilization"`
		ResetsAt    *string `json:"resets_at"`
		StatusText  *string `json:"status_text"`
	}
	var resetsAt *string
	if b.ResetsAt != nil {
		formatted := b.ResetsAt.UTC().Format(time.RFC3339)
		resetsAt = &formatted
	}
	return json.Marshal(bucketJSON{ID: b.ID, Label: b.Label, Utilization: b.Utilization, ResetsAt: resetsAt, StatusText: b.StatusText})
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
	Buckets             []Bucket
	Error               *string
	NeedsReauth         bool
}

type Header struct {
	Current        UsageBucket
	Weekly         UsageBucket
	ShowSecondary  bool
	PrimaryLabel   string
	SecondaryLabel string
}

func DeriveHeader(buckets []Bucket) Header {
	header := Header{SecondaryLabel: "Weekly"}
	if len(buckets) == 0 {
		return header
	}

	first := buckets[0]
	header.Current = UsageBucket{Utilization: first.Utilization, ResetsAt: first.ResetsAt}
	header.PrimaryLabel = first.Label
	header.ShowSecondary = len(buckets) >= 2

	weekly := Bucket{}
	weeklyFound := false
	for _, bucket := range buckets {
		if bucket.ID == "weekly" {
			weekly = bucket
			weeklyFound = true
			break
		}
	}
	if !weeklyFound && len(buckets) >= 2 {
		weekly = buckets[1]
		weeklyFound = true
	}
	if weeklyFound {
		header.Weekly = UsageBucket{Utilization: weekly.Utilization, ResetsAt: weekly.ResetsAt}
		if len(buckets) >= 2 {
			header.SecondaryLabel = weekly.Label
		}
	}
	return header
}

func (d UsageData) WithBuckets(buckets []Bucket) UsageData {
	buckets = NormalizeBuckets(buckets)
	header := DeriveHeader(buckets)
	d.Buckets = buckets
	d.Current = header.Current
	d.Weekly = header.Weekly
	d.ShowSecondary = header.ShowSecondary
	d.PrimaryLabel = header.PrimaryLabel
	d.SecondaryLabel = header.SecondaryLabel
	return d
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
		Buckets             []Bucket    `json:"buckets"`
		Error               *string     `json:"error"`
		NeedsReauth         bool        `json:"needs_reauth"`
		IsSuccess           bool        `json:"is_success"`
	}
	buckets := d.Buckets
	if d.Error != nil || buckets == nil {
		buckets = []Bucket{}
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
		Buckets:             buckets,
		Error:               d.Error,
		NeedsReauth:         d.NeedsReauth,
		IsSuccess:           d.Error == nil,
	})
}
