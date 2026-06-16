using System.Drawing;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.UI;

public class ProviderUsagePanel : Panel
{
    private const int PanelWidth = 276;
    private const int ContentLeft = 10;
    private const int ContentWidth = 256;
    private const int PercentWidth = 48;
    private const int PrimaryLabelY = 36;
    private const int PrimaryPercentY = 34;
    private const int PrimaryProgressY = 54;
    private const int PrimaryStatusY = 71;
    private const int SecondaryLabelY = 88;
    private const int SecondaryPercentY = 86;
    private const int SecondaryProgressY = 106;
    private const int SecondaryStatusY = 123;

    private static readonly Color BackgroundColor = Color.FromArgb(36, 36, 36);
    private static readonly Color ProminentBackgroundColor = Color.FromArgb(44, 44, 44);
    private static readonly Color TextColor = Color.White;
    private static readonly Color SecondaryTextColor = Color.FromArgb(160, 160, 160);
    private static readonly Color SeparatorColor = Color.FromArgb(60, 60, 60);
    private static readonly Color AccentColor = Color.FromArgb(217, 119, 87);
    private static readonly Color PlanPillTextColor = Color.FromArgb(170, 170, 170);

    private readonly Label _titleLabel;
    private readonly Label _subtitleLabel;
    private readonly Label _primaryLabel;
    private readonly Label _primaryPercentLabel;
    private readonly UsageProgressBar _primaryProgress;
    private readonly Label _primaryStatusLabel;
    private readonly Label _secondaryLabel;
    private readonly Label _secondaryPercentLabel;
    private readonly UsageProgressBar _secondaryProgress;
    private readonly Label _secondaryStatusLabel;
    private bool _isProminent;

    public string ProviderName { get; }

    public ProviderUsagePanel(string providerName)
    {
        ProviderName = providerName;
        BackColor = BackgroundColor;
        Width = PanelWidth;
        Height = 116;
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

        _primaryLabel = CreateSectionLabel(new Point(10, PrimaryLabelY));
        Controls.Add(_primaryLabel);

        _primaryPercentLabel = CreatePercentLabel(new Point(ContentLeft + ContentWidth - PercentWidth, PrimaryPercentY));
        Controls.Add(_primaryPercentLabel);

        _primaryProgress = new UsageProgressBar
        {
            Location = new Point(10, PrimaryProgressY),
            Size = new Size(ContentWidth, 14)
        };
        Controls.Add(_primaryProgress);

        _primaryStatusLabel = CreateStatusLabel(new Point(10, PrimaryStatusY));
        Controls.Add(_primaryStatusLabel);

        _secondaryLabel = CreateSectionLabel(new Point(10, SecondaryLabelY));
        Controls.Add(_secondaryLabel);

        _secondaryPercentLabel = CreatePercentLabel(new Point(ContentLeft + ContentWidth - PercentWidth, SecondaryPercentY));
        Controls.Add(_secondaryPercentLabel);

        _secondaryProgress = new UsageProgressBar
        {
            Location = new Point(10, SecondaryProgressY),
            Size = new Size(ContentWidth, 14)
        };
        Controls.Add(_secondaryProgress);

        _secondaryStatusLabel = CreateStatusLabel(new Point(10, SecondaryStatusY));
        Controls.Add(_secondaryStatusLabel);

        SetSecondaryVisible(true);
    }

    public void UpdateData(UsageData? data)
    {
        if (data == null)
        {
            _titleLabel.Text = "Unavailable";
            _subtitleLabel.Visible = false;
            _primaryLabel.Text = "Status";
            _primaryPercentLabel.Text = "--";
            _primaryProgress.Value = 0;
            _primaryStatusLabel.Text = "No data";
            SetSecondaryVisible(false);
            return;
        }

        _titleLabel.Text = data.ProviderName;
        _subtitleLabel.Text = data.Subtitle ?? string.Empty;
        _subtitleLabel.Visible = !string.IsNullOrWhiteSpace(data.Subtitle);
        ApplyProviderStyles(data);

        if (!data.IsSuccess)
        {
            _primaryLabel.Text = "Status";
            _primaryPercentLabel.Text = "--";
            _primaryProgress.Value = 0;
            _primaryStatusLabel.ForeColor = data.NeedsReauth ? Color.FromArgb(220, 53, 69) : SecondaryTextColor;
            _primaryStatusLabel.Text = data.NeedsReauth
                ? BuildReauthText(data)
                : data.Error ?? "Unavailable";
            Invalidate();
            SetSecondaryVisible(false);
            return;
        }

        _primaryStatusLabel.ForeColor = SecondaryTextColor;
        _primaryLabel.Text = data.PrimaryLabel;
        _primaryPercentLabel.Text = $"{data.Current.Utilization:F0}%";
        _primaryProgress.Value = data.Current.Utilization;
        _primaryStatusLabel.Text = data.PrimaryStatusText ?? BuildResetText(data.Current.ResetsAt, "Resets in");

        SetSecondaryVisible(data.ShowSecondary);
        if (data.ShowSecondary)
        {
            _secondaryLabel.Text = data.SecondaryLabel;
            _secondaryPercentLabel.Text = $"{data.Weekly.Utilization:F0}%";
            _secondaryProgress.Value = data.Weekly.Utilization;
            _secondaryStatusLabel.Text = data.SecondaryStatusText ?? BuildResetText(data.Weekly.ResetsAt, "Resets");
        }
        else
        {
            _secondaryStatusLabel.Text = data.SecondaryStatusText ?? string.Empty;
        }

        Invalidate();
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

    private void SetSecondaryVisible(bool visible)
    {
        _secondaryLabel.Visible = visible;
        _secondaryPercentLabel.Visible = visible;
        _secondaryProgress.Visible = visible;
        _secondaryStatusLabel.Visible = visible;
        Height = visible ? 152 : 92;
    }

    private void ApplyProviderStyles(UsageData data)
    {
        switch (data.ProviderName)
        {
            case "Claude":
            case "Codex":
                _primaryProgress.Notches = 4;
                _secondaryProgress.Notches = 6;
                _primaryProgress.BurnRatePercent = CalculateElapsedPercent(data.Current.ResetsAt, TimeSpan.FromHours(5));
                _secondaryProgress.BurnRatePercent = CalculateElapsedPercent(data.Weekly.ResetsAt, TimeSpan.FromDays(7));
                break;
            case "Cursor":
                _primaryProgress.Notches = 3;
                _secondaryProgress.Notches = 3;
                _primaryProgress.BurnRatePercent = CalculateElapsedPercent(data.Current.ResetsAt, TimeSpan.FromDays(30));
                _secondaryProgress.BurnRatePercent = CalculateElapsedPercent(data.Weekly.ResetsAt, TimeSpan.FromDays(30));
                break;
            case "Grok":
                _primaryProgress.Notches = 3;
                _secondaryProgress.Notches = 3;
                _primaryProgress.BurnRatePercent = CalculateElapsedPercent(data.Current.ResetsAt, ResolveMonthEndWindowDuration(data.Current.ResetsAt));
                _secondaryProgress.BurnRatePercent = CalculateElapsedPercent(data.Weekly.ResetsAt, ResolveMonthEndWindowDuration(data.Weekly.ResetsAt));
                break;
            default:
                _primaryProgress.Notches = 0;
                _secondaryProgress.Notches = 0;
                _primaryProgress.BurnRatePercent = -1;
                _secondaryProgress.BurnRatePercent = -1;
                break;
        }
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
        var firstDayOfCurrentMonth = new DateTime(reset.Year, reset.Month, 1, 0, 0, 0, reset.Kind);
        var firstDayOfPreviousMonth = firstDayOfCurrentMonth.AddMonths(-1);
        var duration = reset - firstDayOfPreviousMonth;
        return duration > TimeSpan.Zero ? duration : TimeSpan.FromDays(30);
    }

    private static string BuildResetText(DateTime? resetsAt, string prefix)
    {
        if (resetsAt == null)
            return prefix == "Resets in" ? "Reset unknown" : "Resets --";

        var remaining = resetsAt.Value - DateTime.UtcNow;
        if (remaining <= TimeSpan.Zero)
            return prefix == "Resets in" ? "Resets now" : "Resets now";

        if (remaining.TotalDays >= 1)
            return $"{prefix} {(int)remaining.TotalDays}d {remaining.Hours}h";

        if (remaining.TotalHours >= 1)
            return $"{prefix} {(int)remaining.TotalHours}h {remaining.Minutes}m";

        return $"{prefix} {Math.Max(remaining.Minutes, 1)}m";
    }

    private static Label CreateSectionLabel(Point location)
    {
        return new Label
        {
            Font = new Font("Segoe UI", 8.5f, FontStyle.Bold),
            ForeColor = TextColor,
            Location = location,
            AutoSize = true
        };
    }

    private static Label CreatePercentLabel(Point location)
    {
        return new Label
        {
            Font = new Font("Segoe UI", 8.5f, FontStyle.Bold),
            ForeColor = TextColor,
            Location = location,
            Size = new Size(48, 18),
            TextAlign = ContentAlignment.MiddleRight
        };
    }

    private static Label CreateStatusLabel(Point location)
    {
        return new Label
        {
            Font = new Font("Segoe UI", 8),
            ForeColor = SecondaryTextColor,
            Location = location,
            Size = new Size(ContentWidth, 16)
        };
    }
}
