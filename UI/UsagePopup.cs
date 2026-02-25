using System.Drawing;
using System.Drawing.Drawing2D;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.UI;

/// <summary>
/// Custom progress bar with rounded corners and color-coded fill.
/// </summary>
public class UsageProgressBar : Panel
{
    private float _value;
    private const int CornerRadius = 6;

    public float Value
    {
        get => _value;
        set
        {
            _value = Math.Clamp(value, 0, 100);
            Invalidate();
        }
    }

    public UsageProgressBar()
    {
        DoubleBuffered = true;
        Height = 16;
    }

    /// <summary>
    /// Gets the fill color based on utilization percentage.
    /// </summary>
    private static Color GetFillColor(float utilization)
    {
        return utilization switch
        {
            >= 90 => Color.FromArgb(220, 53, 69),   // Red
            >= 75 => Color.FromArgb(255, 140, 0),   // Orange
            >= 50 => Color.FromArgb(255, 193, 7),   // Yellow
            _ => Color.FromArgb(40, 167, 69)         // Green
        };
    }

    protected override void OnPaint(PaintEventArgs e)
    {
        base.OnPaint(e);
        var g = e.Graphics;
        g.SmoothingMode = SmoothingMode.AntiAlias;

        var rect = new Rectangle(0, 0, Width - 1, Height - 1);

        // Draw background (dark gray)
        using (var bgBrush = new SolidBrush(Color.FromArgb(60, 60, 60)))
        using (var path = CreateRoundedRectPath(rect, CornerRadius))
        {
            g.FillPath(bgBrush, path);
        }

        // Draw filled portion
        if (_value > 0)
        {
            var fillWidth = (int)((Width - 1) * (_value / 100f));
            if (fillWidth > 0)
            {
                var fillRect = new Rectangle(0, 0, fillWidth, Height - 1);
                var fillColor = GetFillColor(_value);

                using var fillBrush = new SolidBrush(fillColor);
                using var fillPath = CreateRoundedRectPath(fillRect, CornerRadius, _value >= 100);
                g.FillPath(fillBrush, fillPath);
            }
        }

        // Draw border
        using (var borderPen = new Pen(Color.FromArgb(80, 80, 80), 1))
        using (var borderPath = CreateRoundedRectPath(rect, CornerRadius))
        {
            g.DrawPath(borderPen, borderPath);
        }
    }

    /// <summary>
    /// Creates a rounded rectangle path.
    /// </summary>
    private static GraphicsPath CreateRoundedRectPath(Rectangle rect, int radius, bool fullRound = true)
    {
        var path = new GraphicsPath();
        var diameter = radius * 2;

        // Top-left corner
        path.AddArc(rect.X, rect.Y, diameter, diameter, 180, 90);

        // Top-right corner
        if (fullRound)
            path.AddArc(rect.Right - diameter, rect.Y, diameter, diameter, 270, 90);
        else
            path.AddLine(rect.Right, rect.Y, rect.Right, rect.Y);

        // Bottom-right corner
        if (fullRound)
            path.AddArc(rect.Right - diameter, rect.Bottom - diameter, diameter, diameter, 0, 90);
        else
            path.AddLine(rect.Right, rect.Bottom, rect.Right, rect.Bottom);

        // Bottom-left corner
        path.AddArc(rect.X, rect.Bottom - diameter, diameter, diameter, 90, 90);

        path.CloseFigure();
        return path;
    }
}

/// <summary>
/// Popup panel showing detailed usage information.
/// </summary>
public class UsagePopup : Form
{
    // Colors
    private static readonly Color BackgroundColor = Color.FromArgb(30, 30, 30);
    private static readonly Color TextColor = Color.White;
    private static readonly Color SecondaryTextColor = Color.FromArgb(160, 160, 160);
    private static readonly Color SeparatorColor = Color.FromArgb(60, 60, 60);
    private static readonly Color ButtonBackColor = Color.FromArgb(50, 50, 50);
    private static readonly Color ButtonHoverColor = Color.FromArgb(70, 70, 70);

    // Controls
    private readonly Label _titleLabel;
    private readonly Button _closeButton;

    private readonly Label _fiveHourLabel;
    private readonly UsageProgressBar _fiveHourProgress;
    private readonly Label _fiveHourPercent;
    private readonly Label _fiveHourReset;

    private readonly Label _weeklyLabel;
    private readonly UsageProgressBar _weeklyProgress;
    private readonly Label _weeklyPercent;
    private readonly Label _weeklyReset;

    private readonly Button _refreshButton;
    private readonly Button _settingsButton;
    private readonly Button _quitButton;

    // Events
    public event EventHandler? OnRefreshClicked;
    public event EventHandler? OnQuitClicked;

    public UsagePopup()
    {
        // Form settings
        FormBorderStyle = FormBorderStyle.None;
        StartPosition = FormStartPosition.Manual;
        ShowInTaskbar = false;
        BackColor = BackgroundColor;
        Size = new Size(280, 260);
        TopMost = true;

        // Title bar
        _titleLabel = new Label
        {
            Text = "Claude Usage",
            Font = new Font("Segoe UI", 11, FontStyle.Bold),
            ForeColor = TextColor,
            Location = new Point(12, 10),
            AutoSize = true
        };
        Controls.Add(_titleLabel);

        _closeButton = CreateFlatButton("X", 8);
        _closeButton.Size = new Size(24, 24);
        _closeButton.Location = new Point(Width - 32, 6);
        _closeButton.Click += (_, _) => Hide();
        Controls.Add(_closeButton);

        // First separator
        var sep1 = CreateSeparator(40);
        Controls.Add(sep1);

        // 5-Hour section
        _fiveHourLabel = new Label
        {
            Text = "5-Hour Window",
            Font = new Font("Segoe UI", 9, FontStyle.Bold),
            ForeColor = TextColor,
            Location = new Point(12, 52),
            AutoSize = true
        };
        Controls.Add(_fiveHourLabel);

        _fiveHourPercent = new Label
        {
            Text = "0%",
            Font = new Font("Segoe UI", 9, FontStyle.Bold),
            ForeColor = TextColor,
            TextAlign = ContentAlignment.MiddleRight,
            Size = new Size(40, 20),
            Location = new Point(Width - 52, 52)
        };
        Controls.Add(_fiveHourPercent);

        _fiveHourProgress = new UsageProgressBar
        {
            Location = new Point(12, 74),
            Size = new Size(Width - 24, 16)
        };
        Controls.Add(_fiveHourProgress);

        _fiveHourReset = new Label
        {
            Text = "Resets in --",
            Font = new Font("Segoe UI", 8),
            ForeColor = SecondaryTextColor,
            Location = new Point(12, 94),
            AutoSize = true
        };
        Controls.Add(_fiveHourReset);

        // Second separator
        var sep2 = CreateSeparator(118);
        Controls.Add(sep2);

        // Weekly section
        _weeklyLabel = new Label
        {
            Text = "Weekly",
            Font = new Font("Segoe UI", 9, FontStyle.Bold),
            ForeColor = TextColor,
            Location = new Point(12, 130),
            AutoSize = true
        };
        Controls.Add(_weeklyLabel);

        _weeklyPercent = new Label
        {
            Text = "0%",
            Font = new Font("Segoe UI", 9, FontStyle.Bold),
            ForeColor = TextColor,
            TextAlign = ContentAlignment.MiddleRight,
            Size = new Size(40, 20),
            Location = new Point(Width - 52, 130)
        };
        Controls.Add(_weeklyPercent);

        _weeklyProgress = new UsageProgressBar
        {
            Location = new Point(12, 152),
            Size = new Size(Width - 24, 16)
        };
        Controls.Add(_weeklyProgress);

        _weeklyReset = new Label
        {
            Text = "Resets --",
            Font = new Font("Segoe UI", 8),
            ForeColor = SecondaryTextColor,
            Location = new Point(12, 172),
            AutoSize = true
        };
        Controls.Add(_weeklyReset);

        // Third separator
        var sep3 = CreateSeparator(196);
        Controls.Add(sep3);

        // Bottom buttons
        const int buttonWidth = 76;
        const int buttonHeight = 28;
        const int buttonSpacing = 10;
        const int totalButtonsWidth = buttonWidth * 3 + buttonSpacing * 2;
        var startX = (Width - totalButtonsWidth) / 2;
        const int buttonY = 210;

        _refreshButton = CreateFlatButton("Refresh", 9);
        _refreshButton.Size = new Size(buttonWidth, buttonHeight);
        _refreshButton.Location = new Point(startX, buttonY);
        _refreshButton.Click += (s, e) => OnRefreshClicked?.Invoke(this, EventArgs.Empty);
        Controls.Add(_refreshButton);

        _settingsButton = CreateFlatButton("Settings", 9);
        _settingsButton.Size = new Size(buttonWidth, buttonHeight);
        _settingsButton.Location = new Point(startX + buttonWidth + buttonSpacing, buttonY);
        _settingsButton.Click += (_, _) => MessageBox.Show("Settings coming soon", "Claude Usage", MessageBoxButtons.OK, MessageBoxIcon.Information);
        Controls.Add(_settingsButton);

        _quitButton = CreateFlatButton("Quit", 9);
        _quitButton.Size = new Size(buttonWidth, buttonHeight);
        _quitButton.Location = new Point(startX + (buttonWidth + buttonSpacing) * 2, buttonY);
        _quitButton.Click += (s, e) => OnQuitClicked?.Invoke(this, EventArgs.Empty);
        Controls.Add(_quitButton);

        // Handle losing focus
        Deactivate += (_, _) => Hide();
    }

    /// <summary>
    /// Updates the popup with new usage data.
    /// </summary>
    public void UpdateData(UsageData? data)
    {
        if (data == null || !data.IsSuccess)
        {
            _fiveHourProgress.Value = 0;
            _fiveHourPercent.Text = "--%";
            _fiveHourReset.Text = data?.Error ?? "No data";

            _weeklyProgress.Value = 0;
            _weeklyPercent.Text = "--%";
            _weeklyReset.Text = "";
            return;
        }

        // 5-hour data
        _fiveHourProgress.Value = data.FiveHour.Utilization;
        _fiveHourPercent.Text = $"{data.FiveHour.Utilization:F0}%";
        _fiveHourReset.Text = $"Resets in {data.FiveHour.TimeUntilReset}";

        // Weekly data
        _weeklyProgress.Value = data.SevenDay.Utilization;
        _weeklyPercent.Text = $"{data.SevenDay.Utilization:F0}%";
        _weeklyReset.Text = GetWeeklyResetText(data.SevenDay.ResetsAt);
    }

    /// <summary>
    /// Gets the reset text for weekly usage showing local time.
    /// </summary>
    private static string GetWeeklyResetText(DateTime? resetsAt)
    {
        if (resetsAt == null)
            return "Resets --";

        var localReset = resetsAt.Value.ToLocalTime();
        return $"Resets {localReset:ddd h:mm tt}";
    }

    /// <summary>
    /// Creates a dark themed flat button.
    /// </summary>
    private static Button CreateFlatButton(string text, float fontSize)
    {
        var button = new Button
        {
            Text = text,
            FlatStyle = FlatStyle.Flat,
            BackColor = ButtonBackColor,
            ForeColor = TextColor,
            Font = new Font("Segoe UI", fontSize),
            Cursor = Cursors.Hand
        };

        button.FlatAppearance.BorderColor = SeparatorColor;
        button.FlatAppearance.BorderSize = 1;
        button.FlatAppearance.MouseOverBackColor = ButtonHoverColor;
        button.FlatAppearance.MouseDownBackColor = ButtonHoverColor;

        return button;
    }

    /// <summary>
    /// Creates a horizontal separator line.
    /// </summary>
    private Panel CreateSeparator(int y)
    {
        return new Panel
        {
            BackColor = SeparatorColor,
            Location = new Point(12, y),
            Size = new Size(Width - 24, 1)
        };
    }

    /// <summary>
    /// Positions the popup near the system tray.
    /// </summary>
    public void PositionNearTray()
    {
        var cursorPos = Cursor.Position;
        var workingArea = Screen.FromPoint(cursorPos).WorkingArea;

        int x, y;

        // Position horizontally: prefer to the left of cursor, but stay on screen
        x = cursorPos.X - Width / 2;
        if (x + Width > workingArea.Right)
            x = workingArea.Right - Width - 8;
        if (x < workingArea.Left)
            x = workingArea.Left + 8;

        // Position vertically: above the taskbar if at bottom, below if at top
        if (cursorPos.Y > workingArea.Top + workingArea.Height / 2)
        {
            // Taskbar at bottom - show popup above
            y = workingArea.Bottom - Height - 8;
        }
        else
        {
            // Taskbar at top - show popup below
            y = workingArea.Top + 8;
        }

        Location = new Point(x, y);
    }

    protected override void OnPaint(PaintEventArgs e)
    {
        base.OnPaint(e);

        // Draw border
        using var pen = new Pen(SeparatorColor, 1);
        e.Graphics.DrawRectangle(pen, 0, 0, Width - 1, Height - 1);
    }

    protected override CreateParams CreateParams
    {
        get
        {
            // Add drop shadow
            var cp = base.CreateParams;
            cp.ClassStyle |= 0x00020000; // CS_DROPSHADOW
            return cp;
        }
    }
}
