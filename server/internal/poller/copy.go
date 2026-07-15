package poller

import (
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func copyEntry(entry Entry) Entry {
	return Entry{Data: copyUsageData(entry.Data), FetchedAt: entry.FetchedAt.UTC()}
}

func copyUsageData(data usage.UsageData) usage.UsageData {
	var buckets []usage.Bucket
	if data.Buckets != nil {
		buckets = make([]usage.Bucket, len(data.Buckets))
		for index, bucket := range data.Buckets {
			buckets[index] = usage.Bucket{
				ID:          bucket.ID,
				Label:       bucket.Label,
				Utilization: bucket.Utilization,
				ResetsAt:    copyTime(bucket.ResetsAt),
				StatusText:  copyString(bucket.StatusText),
			}
		}
	}
	return usage.UsageData{
		ProviderName:        data.ProviderName,
		PrimaryLabel:        data.PrimaryLabel,
		SecondaryLabel:      data.SecondaryLabel,
		ShowSecondary:       data.ShowSecondary,
		Subtitle:            copyString(data.Subtitle),
		PrimaryStatusText:   copyString(data.PrimaryStatusText),
		SecondaryStatusText: copyString(data.SecondaryStatusText),
		ReauthCommand:       copyString(data.ReauthCommand),
		Current:             copyBucket(data.Current),
		Weekly:              copyBucket(data.Weekly),
		Buckets:             buckets,
		Error:               copyString(data.Error),
		NeedsReauth:         data.NeedsReauth,
	}
}

func copyBucket(bucket usage.UsageBucket) usage.UsageBucket {
	return usage.UsageBucket{Utilization: bucket.Utilization, ResetsAt: copyTime(bucket.ResetsAt)}
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := value.UTC()
	return &copied
}
