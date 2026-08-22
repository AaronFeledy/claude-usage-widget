using System.Drawing;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.UI;

public class ProviderUsagePanel : Panel
{
    private const int PanelWidth = 276;
    private const int ContentLeft = 10;
    private const int ContentWidth = 256;
    private const int PercentWidth = 48;

    // Vertical layout. First bar row's section label sits at RowTopStart; each
    // subsequent row is offset by RowPitch. Height = 92 + (N-1)*RowPitch, which
    // reproduces the original anchors exactly: 1 bar -> 92, 2 bars -> 152.
    private const int RowTopStart = 36;
    private const int RowPitch = 60;
    private const int SingleRowHeight = 92;

    // Offsets of each control within a row, relative to the row's top (section label).
    private const int PercentDeltaY = -2;
    private const int ProgressDeltaY = 18;
    private const int StatusDeltaY = 35;
    private const int ProgressHeight = 14;

    private static readonly Color BackgroundColor = Color.FromArgb(36, 36, 36);
    private static readonly Color ProminentBackgroundColor = Color.FromArgb(44, 44, 44);
    private static readonly Color TextColor = Color.White;
    private static readonly Color SecondaryTextColor = Color.FromArgb(160, 160, 160);
    private static readonly Color SeparatorColor = Color.FromArgb(60, 60, 60);
    private static readonly Color AccentColor = Color.FromArgb(217, 119, 87);
    private static readonly Color PlanPillTextColor = Color.FromArgb(170, 170, 170);
    private static readonly Color ReauthColor = Color.FromArgb(220, 53, 69);

    private readonly Label _titleLabel;
    private readonly Label _subtitleLabel;
    private readonly List<BarRow> _rows = new();
    private bool _isProminent;

    public string ProviderName { get; }

    public ProviderUsagePanel(string providerName)
    {
        ProviderName = providerName;
        BackColor = BackgroundColor;
        Width = PanelWidth;
        Height = SingleRowHeight;
        Margin = new Padding(0, 0, 0, 10);
        Padding = new Padding(10);

        _titleLabel = new Label
        {
            Text = providerName,
            Font = new Font("Segoe UI", 9.5f, FontStyle.Bold),
            ForeColor = TextColor,
            Location = new Point(10, 10),
            AutoSize = true
        };
        Controls.Add(_titleLabel);

        _subtitleLabel = new Label
        {
            Font = new Font("Segoe UI", 7.5f, FontStyle.Regular),
            ForeColor = PlanPillTextColor,
            BackColor = Color.Transparent,
            Location = new Point(ContentLeft + ContentWidth - 64, 12),
            Size = new Size(64, 14),
            TextAlign = ContentAlignment.TopRight,
            Visible = false
        };
        Controls.Add(_subtitleLabel);
    }

    public void UpdateData(UsageData? data)
    {
        if (data == null)
        {
            _titleLabel.Text = "Unavailable";
            _subtitleLabel.Visible = false;
            RenderStatusOnly("Status", "No data", SecondaryTextColor);
            return;
        }

        _titleLabel.Text = data.ProviderName;
        _subtitleLabel.Text = data.Subtitle ?? string.Empty;
        _subtitleLabel.Visible = !string.IsNullOrWhiteSpace(data.Subtitle);

        if (!data.IsSuccess)
        {
            var statusColor = data.NeedsReauth ? ReauthColor : SecondaryTextColor;
            var statusText = data.NeedsReauth ? BuildReauthText(data) : data.Error ?? "Unavailable";
            RenderStatusOnly("Status", statusText, statusColor);
            return;
        }

        var buckets = ResolveBuckets(data);
        RenderBuckets(data, buckets);
    }

    public void SetProminent(bool isProminent)
    {
        if (_isProminent == isProminent)
            return;

        _isProminent = isProminent;
        BackColor = isProminent ? ProminentBackgroundColor : BackgroundColor;
        var oldFont = _titleLabel.Font;
        _titleLabel.Font = new Font("Segoe UI", isProminent ? 10.5f : 9.5f, FontStyle.Bold);
        oldFont.Dispose();
        Invalidate();
    }

    protected override void OnPaint(PaintEventArgs e)
    {
        base.OnPaint(e);
        using var pen = new Pen(_isProminent ? AccentColor : SeparatorColor, _isProminent ? 2 : 1);
        e.Graphics.DrawRectangle(pen, 0, 0, Width - 1, Height - 1);

        if (_isProminent)
        {
            using var brush = new SolidBrush(AccentColor);
            e.Graphics.FillRectangle(brush, 0, 0, 5, Height);
        }
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            foreach (var row in _rows)
                row.Dispose();
            _rows.Clear();
        }
        base.Dispose(disposing);
    }

    /// <summary>
    /// Resolves the list of bar rows to render. Prefers the server-provided
    /// <see cref="UsageData.Buckets"/>; if empty, synthesizes from Current
    /// (and Weekly when <see cref="UsageData.ShowSecondary"/>) as a fallback.
    /// </summary>
    private static IReadOnlyList<UsageBucketDetail> ResolveBuckets(UsageData data)
    {
        if (data.Buckets.Count > 0)
            return data.Buckets;

        var synthesized = new List<UsageBucketDetail>
        {
            new()
            {
                Id = "session",
                Label = data.PrimaryLabel,
                Utilization = data.Current.Utilization,
                ResetsAt = data.Current.ResetsAt,
                StatusText = data.PrimaryStatusText
            }
        };
        if (data.ShowSecondary)
        {
            synthesized.Add(new UsageBucketDetail
            {
                Id = "weekly",
                Label = data.SecondaryLabel,
                Utilization = data.Weekly.Utilization,
                ResetsAt = data.Weekly.ResetsAt,
                StatusText = data.SecondaryStatusText
            });
        }
        return synthesized;
    }

    private void RenderBuckets(UsageData data, IReadOnlyList<UsageBucketDetail> buckets)
    {
        EnsureRowCount(buckets.Count);

        for (var i = 0; i < buckets.Count; i++)
        {
            var bucket = buckets[i];
            var row = _rows[i];

            row.Section.Text = bucket.Label;
            if (BucketPresentation.IsStatusOnly(bucket))
            {
                row.Percent.Visible = false;
                row.Progress.Visible = false;
                row.Percent.Text = string.Empty;
                row.Progress.Value = 0;
            }
            else
            {
                row.Percent.Visible = true;
                row.Progress.Visible = true;
                row.Percent.Text = $"{bucket.Utilization:F0}%";
                row.Progress.Value = bucket.Utilization;
            }
            row.Status.ForeColor = SecondaryTextColor;
            row.Status.Text = ResolveStatusText(data, i, bucket);

            ApplyBucketStyle(row.Progress, data.ProviderName, bucket);
        }

        ApplyHeight(buckets.Count);
        Invalidate();
    }

    /// <summary>
    /// Collapses to a single row showing a status message (no-data / error / reauth paths).
    /// </summary>
    private void RenderStatusOnly(string label, string status, Color statusColor)
    {
        EnsureRowCount(1);
        var row = _rows[0];
        row.Section.Text = label;
        row.Percent.Visible = true;
        row.Progress.Visible = true;
        row.Percent.Text = "--";
        row.Progress.Value = 0;
        row.Progress.Notches = 0;
        row.Progress.BurnRatePercent = -1;
        row.Status.ForeColor = statusColor;
        row.Status.Text = status;
        ApplyHeight(1);
        Invalidate();
    }

    private static string ResolveStatusText(UsageData data, int index, UsageBucketDetail bucket)
    {
        return BucketPresentation.ResolveStatusOverride(data, index, bucket)
            ?? BuildResetText(bucket.ResetsAt, index == 0 ? "Resets in" : "Resets");
    }

    /// <summary>
    /// Grows or shrinks the pool of bar rows to exactly <paramref name="count"/>,
    /// disposing controls of any rows removed to avoid GDI handle leaks.
    /// </summary>
    private void EnsureRowCount(int count)
    {
        while (_rows.Count < count)
        {
            var row = BarRow.Create();
            row.AddTo(Controls);
            _rows.Add(row);
        }
        while (_rows.Count > count)
        {
            var row = _rows[^1];
            _rows.RemoveAt(_rows.Count - 1);
            row.RemoveFrom(Controls);
            row.Dispose();
        }
        for (var i = 0; i < _rows.Count; i++)
            _rows[i].Position(RowTopStart + i * RowPitch);
    }

    private void ApplyHeight(int count)
    {
        Height = SingleRowHeight + Math.Max(0, count - 1) * RowPitch;
    }

    private static void ApplyBucketStyle(UsageProgressBar bar, string providerName, UsageBucketDetail bucket)
    {
        switch (providerName)
        {
            case "Claude":
            case "Codex":
                if (IsCreditBucket(bucket.Id))
                {
                    bar.Notches = 3;
                    bar.BurnRatePercent = -1;
                }
                else if (IsWeeklyBucket(bucket.Id))
                {
                    bar.Notches = 6;
                    bar.BurnRatePercent = CalculateElapsedPercent(bucket.ResetsAt, TimeSpan.FromDays(7));
                }
                else
                {
                    bar.Notches = 4;
                    bar.BurnRatePercent = CalculateElapsedPercent(bucket.ResetsAt, TimeSpan.FromHours(5));
                }
                break;
            case "Cursor":
                if (IsWeeklyBucket(bucket.Id))
                {
                    bar.Notches = 6;
                    bar.BurnRatePercent = CalculateElapsedPercent(bucket.ResetsAt, TimeSpan.FromDays(7));
                }
                else
                {
                    bar.Notches = 3;
                    bar.BurnRatePercent = CalculateElapsedPercent(bucket.ResetsAt, TimeSpan.FromDays(30));
                }
                break;
            case "Grok":
                if (IsWeeklyBucket(bucket.Id))
                {
                    bar.Notches = 6;
                    bar.BurnRatePercent = CalculateElapsedPercent(bucket.ResetsAt, TimeSpan.FromDays(7));
                }
                else
                {
                    bar.Notches = 3;
                    bar.BurnRatePercent = CalculateElapsedPercent(bucket.ResetsAt, ResolveMonthEndWindowDuration(bucket.ResetsAt));
                }
                break;
            default:
                bar.Notches = 0;
                bar.BurnRatePercent = -1;
                break;
        }
    }

    private static bool IsWeeklyBucket(string id)
    {
        return id.Equals("weekly", StringComparison.OrdinalIgnoreCase)
            || id.StartsWith("weekly_", StringComparison.OrdinalIgnoreCase);
    }

    private static bool IsCreditBucket(string id)
    {
        return id.Equals("extra", StringComparison.OrdinalIgnoreCase)
            || id.Equals("credits", StringComparison.OrdinalIgnoreCase)
            || id.Equals("on_demand", StringComparison.OrdinalIgnoreCase);
    }

    private static string BuildReauthText(UsageData data)
    {
        return string.IsNullOrWhiteSpace(data.ReauthCommand)
            ? "Re-authentication required"
            : $"Run '{data.ReauthCommand}' to re-authenticate.";
    }

    private static float CalculateElapsedPercent(DateTime? resetsAt, TimeSpan windowDuration)
    {
        if (resetsAt == null)
            return -1;

        var remaining = resetsAt.Value - DateTime.UtcNow;
        if (remaining <= TimeSpan.Zero)
            return 100;

        var elapsed = windowDuration - remaining;
        if (elapsed <= TimeSpan.Zero)
            return 0;

        return (float)(elapsed / windowDuration * 100);
    }

    private static TimeSpan ResolveMonthEndWindowDuration(DateTime? resetsAt)
    {
        if (resetsAt == null)
            return TimeSpan.FromDays(30);

        var reset = resetsAt.Value;
        var periodStart = reset.AddMonths(-1);
        var duration = reset - periodStart;
        return duration > TimeSpan.Zero ? duration : TimeSpan.FromDays(30);
    }

    private static string BuildResetText(DateTime? resetsAt, string prefix)
    {
        if (resetsAt == null)
            return prefix == "Resets in" ? "Reset unknown" : "Resets --";

        var remaining = resetsAt.Value - DateTime.UtcNow;
        if (remaining <= TimeSpan.Zero)
            return "Resets now";

        if (remaining.TotalDays >= 1)
            return $"{prefix} {(int)remaining.TotalDays}d {remaining.Hours}h";

        if (remaining.TotalHours >= 1)
            return $"{prefix} {(int)remaining.TotalHours}h {remaining.Minutes}m";

        return $"{prefix} {Math.Max(remaining.Minutes, 1)}m";
    }

    /// <summary>
    /// One bar row: section label, right-aligned percent, progress bar, and status line.
    /// Controls are parented to the owning panel; <see cref="Position"/> lays them out
    /// relative to the row's top y-coordinate.
    /// </summary>
    private sealed class BarRow
    {
        public required Label Section { get; init; }
        public required Label Percent { get; init; }
        public required UsageProgressBar Progress { get; init; }
        public required Label Status { get; init; }

        public static BarRow Create()
        {
            return new BarRow
            {
                Section = new Label
                {
                    Font = new Font("Segoe UI", 8.5f, FontStyle.Bold),
                    ForeColor = TextColor,
                    AutoSize = true
                },
                Percent = new Label
                {
                    Font = new Font("Segoe UI", 8.5f, FontStyle.Bold),
                    ForeColor = TextColor,
                    Size = new Size(PercentWidth, 18),
                    TextAlign = ContentAlignment.MiddleRight
                },
                Progress = new UsageProgressBar
                {
                    Size = new Size(ContentWidth, ProgressHeight)
                },
                Status = new Label
                {
                    Font = new Font("Segoe UI", 8),
                    ForeColor = SecondaryTextColor,
                    Size = new Size(ContentWidth, 16)
                }
            };
        }

        public void AddTo(Control.ControlCollection controls)
        {
            controls.Add(Section);
            controls.Add(Percent);
            controls.Add(Progress);
            controls.Add(Status);
        }

        public void RemoveFrom(Control.ControlCollection controls)
        {
            controls.Remove(Section);
            controls.Remove(Percent);
            controls.Remove(Progress);
            controls.Remove(Status);
        }

        public void Position(int top)
        {
            Section.Location = new Point(ContentLeft, top);
            Percent.Location = new Point(ContentLeft + ContentWidth - PercentWidth, top + PercentDeltaY);
            Progress.Location = new Point(ContentLeft, top + ProgressDeltaY);
            Status.Location = new Point(ContentLeft, top + StatusDeltaY);
        }

        public void Dispose()
        {
            Section.Dispose();
            Percent.Dispose();
            Progress.Dispose();
            Status.Dispose();
        }
    }
}
