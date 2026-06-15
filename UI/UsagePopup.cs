using System.Drawing;
using System.Drawing.Drawing2D;
using ClaudeUsageWidget.Models;
using ClaudeUsageWidget.Services;
using ClaudeUsageWidget.TrayIcon;

namespace ClaudeUsageWidget.UI;

/// <summary>
/// Custom progress bar with rounded corners and color-coded fill.
/// </summary>
public class UsageProgressBar : Panel
{
    private float _value;
    private const int CornerRadius = 6;

    private float _burnRatePercent = -1;
    private int _notches;

    /// <summary>
    /// Number of internal divider notches to draw (e.g. 4 notches = 5 segments).
    /// Set to 0 to hide.
    /// </summary>
    public int Notches
    {
        get => _notches;
        set
        {
            _notches = Math.Max(0, value);
            Invalidate();
        }
    }

    public float Value
    {
        get => _value;
        set
        {
            _value = Math.Clamp(value, 0, 100);
            Invalidate();
        }
    }

    /// <summary>
    /// Position of the ideal burn-rate marker (0-100). Set to -1 to hide.
    /// </summary>
    public float BurnRatePercent
    {
        get => _burnRatePercent;
        set
        {
            _burnRatePercent = value;
            Invalidate();
        }
    }

    private readonly ToolTip _toolTip;
    private bool _tooltipVisible;

    public UsageProgressBar()
    {
        DoubleBuffered = true;
        Height = 16;

        _toolTip = new ToolTip
        {
            InitialDelay = 0,
            ReshowDelay = 0,
            AutoPopDelay = 5000,
            BackColor = Color.FromArgb(40, 40, 40),
            ForeColor = Color.White,
            OwnerDraw = false
        };

        MouseMove += OnBarMouseMove;
        MouseLeave += (_, _) =>
        {
            _toolTip.Hide(this);
            _tooltipVisible = false;
        };
    }

    private void OnBarMouseMove(object? sender, MouseEventArgs e)
    {
        if (_burnRatePercent < 0 || _burnRatePercent > 100) return;

        var markerX = (int)((Width - 1) * (_burnRatePercent / 100f));
        var distance = Math.Abs(e.X - markerX);

        if (distance <= 8)
        {
            if (!_tooltipVisible)
            {
                _toolTip.Show($"{_burnRatePercent:F0}%", this, markerX - 10, -20);
                _tooltipVisible = true;
            }
        }
        else if (_tooltipVisible)
        {
            _toolTip.Hide(this);
            _tooltipVisible = false;
        }
    }

    /// <summary>
    /// Gets the fill color based on utilization percentage.
    /// </summary>
    private static Color GetFillColor(float utilization, float burnRatePercent = -1)
    {
        // If we have a burn rate and usage is below it, use tighter thresholds
        if (burnRatePercent >= 0 && utilization < burnRatePercent)
        {
            return utilization switch
            {
                >= 95 => Color.FromArgb(244, 67, 54),    // Red    (#F44336)
                >= 90 => Color.FromArgb(255, 152, 0),    // Orange (#FF9800)
                >= 85 => Color.FromArgb(255, 193, 7),    // Yellow (#FFC107)
                _ => Color.FromArgb(76, 175, 80)          // Green  (#4CAF50)
            };
        }

        // At or above burn target — standard thresholds
        return utilization switch
        {
            >= 90 => Color.FromArgb(244, 67, 54),    // Red    (#F44336)
            >= 75 => Color.FromArgb(255, 152, 0),    // Orange (#FF9800)
            >= 50 => Color.FromArgb(255, 193, 7),    // Yellow (#FFC107)
            _ => Color.FromArgb(76, 175, 80)          // Green  (#4CAF50)
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

        // Draw time-span notches (faint vertical dividers)
        if (_notches > 0)
        {
            using var notchPen = new Pen(Color.FromArgb(40, 255, 255, 255), 1);
            for (var i = 1; i <= _notches; i++)
            {
                var notchX = (int)((Width - 1) * ((float)i / (_notches + 1)));
                g.DrawLine(notchPen, notchX, 2, notchX, Height - 3);
            }
        }

        // Draw filled portion
        if (_value > 0)
        {
            var fillWidth = (int)((Width - 1) * (_value / 100f));
            if (fillWidth > 0)
            {
                var fillRect = new Rectangle(0, 0, fillWidth, Height - 1);
                var fillColor = GetFillColor(_value, _burnRatePercent);

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

        // Draw burn-rate marker line
        if (_burnRatePercent >= 0 && _burnRatePercent <= 100)
        {
            var markerX = (int)((Width - 1) * (_burnRatePercent / 100f));
            markerX = Math.Clamp(markerX, 2, Width - 3);
            var markerColor = _value > _burnRatePercent
                ? Color.FromArgb(200, 244, 67, 54)    // Red — usage exceeded burn rate
                : Color.FromArgb(200, 80, 160, 255);  // Blue — on track
            using var markerPen = new Pen(markerColor, 2);
            g.DrawLine(markerPen, markerX, 1, markerX, Height - 2);
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
    private static readonly Color BackgroundColor = Color.FromArgb(30, 30, 30);
    private static readonly Color TextColor = Color.White;
    private static readonly Color SeparatorColor = Color.FromArgb(60, 60, 60);
    private static readonly Color ButtonBackColor = Color.FromArgb(50, 50, 50);

    private readonly Label _titleLabel;
    private readonly FlowLayoutPanel _providerList;
    private readonly ProviderUsagePanel _claudePanel;
    private readonly ProviderUsagePanel _codexPanel;
    private readonly ProviderUsagePanel _cursorPanel;
    private readonly Panel _settingsPanel;
    private readonly CheckBox _startWithWindowsCheckbox;
    private readonly CheckBox _notificationsCheckbox;
    private readonly CheckBox _debugModeCheckbox;
    private readonly Label _primaryProviderLabel;
    private readonly ComboBox _primaryProviderCombo;
    private readonly Label _refreshIntervalLabel;
    private readonly ComboBox _refreshIntervalCombo;

    // State
    private SettingsService? _settingsService;
    private bool _settingsExpanded;
    private int _collapsedHeight;

    // Events
    public event EventHandler? OnSettingsChanged;

    public UsagePopup()
    {
        // Form settings
        FormBorderStyle = FormBorderStyle.None;
        StartPosition = FormStartPosition.Manual;
        ShowInTaskbar = false;
        BackColor = BackgroundColor;
        Size = new Size(300, 420);
        TopMost = true;

        var titleIcon = new PictureBox
        {
            Size = new Size(20, 20),
            Location = new Point(10, 10),
            SizeMode = PictureBoxSizeMode.StretchImage,
            Image = IconGenerator.GenerateAppIcon(includeBadge: false).ToBitmap()
        };
        Controls.Add(titleIcon);

        _titleLabel = new Label
        {
            Text = "Usage Overview",
            Font = new Font("Segoe UI", 11, FontStyle.Bold),
            ForeColor = TextColor,
            Location = new Point(34, 10),
            AutoSize = true
        };
        Controls.Add(_titleLabel);

        _providerList = new FlowLayoutPanel
        {
            Location = new Point(12, 44),
            Size = new Size(276, 320),
            FlowDirection = FlowDirection.TopDown,
            WrapContents = false,
            AutoSize = true,
            AutoSizeMode = AutoSizeMode.GrowAndShrink
        };

        _claudePanel = new ProviderUsagePanel("Claude");
        _codexPanel = new ProviderUsagePanel("Codex");
        _cursorPanel = new ProviderUsagePanel("Cursor");
        _providerList.Controls.Add(_claudePanel);
        _providerList.Controls.Add(_codexPanel);
        _providerList.Controls.Add(_cursorPanel);
        Controls.Add(_providerList);

        _settingsPanel = new Panel
        {
            BackColor = BackgroundColor,
            Location = new Point(0, _providerList.Bottom + 8),
            Size = new Size(Width, 178),
            Visible = false
        };

        var settingsSep = new Panel
        {
            BackColor = SeparatorColor,
            Location = new Point(12, 0),
            Size = new Size(Width - 24, 1)
        };
        _settingsPanel.Controls.Add(settingsSep);

        var settingsTitle = new Label
        {
            Text = "Settings",
            Font = new Font("Segoe UI", 9, FontStyle.Bold),
            ForeColor = TextColor,
            Location = new Point(12, 12),
            AutoSize = true
        };
        _settingsPanel.Controls.Add(settingsTitle);

        _startWithWindowsCheckbox = new CheckBox
        {
            Text = "Start with Windows",
            Font = new Font("Segoe UI", 9),
            ForeColor = TextColor,
            Location = new Point(12, 38),
            AutoSize = true,
            FlatStyle = FlatStyle.Flat
        };
        _startWithWindowsCheckbox.CheckedChanged += OnStartWithWindowsChanged;
        _settingsPanel.Controls.Add(_startWithWindowsCheckbox);

        _notificationsCheckbox = new CheckBox
        {
            Text = "Enable notifications",
            Font = new Font("Segoe UI", 9),
            ForeColor = TextColor,
            Location = new Point(12, 62),
            AutoSize = true,
            FlatStyle = FlatStyle.Flat
        };
        _notificationsCheckbox.CheckedChanged += OnNotificationsChanged;
        _settingsPanel.Controls.Add(_notificationsCheckbox);

        _debugModeCheckbox = new CheckBox
        {
            Text = "Debug mode (restart required)",
            Font = new Font("Segoe UI", 9),
            ForeColor = TextColor,
            Location = new Point(12, 86),
            AutoSize = true,
            FlatStyle = FlatStyle.Flat
        };
        _debugModeCheckbox.CheckedChanged += OnDebugModeChanged;
        _settingsPanel.Controls.Add(_debugModeCheckbox);

        _primaryProviderLabel = new Label
        {
            Text = "Primary provider:",
            Font = new Font("Segoe UI", 9),
            ForeColor = TextColor,
            Location = new Point(12, 114),
            AutoSize = true
        };
        _settingsPanel.Controls.Add(_primaryProviderLabel);

        _primaryProviderCombo = new ComboBox
        {
            Font = new Font("Segoe UI", 9),
            Location = new Point(115, 111),
            Size = new Size(110, 24),
            DropDownStyle = ComboBoxStyle.DropDownList,
            BackColor = ButtonBackColor,
            ForeColor = TextColor,
            FlatStyle = FlatStyle.Flat
        };
        _primaryProviderCombo.Items.AddRange(new object[] { "Claude", "Codex", "Cursor" });
        _primaryProviderCombo.SelectedIndexChanged += OnPrimaryProviderChanged;
        _settingsPanel.Controls.Add(_primaryProviderCombo);

        _refreshIntervalLabel = new Label
        {
            Text = "Refresh interval:",
            Font = new Font("Segoe UI", 9),
            ForeColor = TextColor,
            Location = new Point(12, 142),
            AutoSize = true
        };
        _settingsPanel.Controls.Add(_refreshIntervalLabel);

        _refreshIntervalCombo = new ComboBox
        {
            Font = new Font("Segoe UI", 9),
            Location = new Point(115, 139),
            Size = new Size(80, 24),
            DropDownStyle = ComboBoxStyle.DropDownList,
            BackColor = ButtonBackColor,
            ForeColor = TextColor,
            FlatStyle = FlatStyle.Flat
        };
        _refreshIntervalCombo.Items.AddRange(new object[] { "30s", "60s", "120s", "300s" });
        _refreshIntervalCombo.SelectedIndexChanged += OnRefreshIntervalChanged;
        _settingsPanel.Controls.Add(_refreshIntervalCombo);

        var version = System.Reflection.Assembly.GetExecutingAssembly().GetName().Version;
        var versionLabel = new Label
        {
            Text = $"v{version?.ToString(3) ?? "?"}",
            Font = new Font("Segoe UI", 8),
            ForeColor = Color.FromArgb(100, 100, 100),
            Location = new Point(Width - 60, 12),
            AutoSize = true
        };
        _settingsPanel.Controls.Add(versionLabel);

        Controls.Add(_settingsPanel);
        RecalculateLayout();

        Deactivate += (_, _) =>
        {
            HideSettings();
            Hide();
        };
    }

    /// <summary>
    /// Sets the settings service for the popup to use.
    /// </summary>
    public void SetSettingsService(SettingsService settingsService)
    {
        _settingsService = settingsService;
        LoadSettingsToControls();
        ApplyProviderOrder();
    }

    /// <summary>
    /// Loads current settings into the UI controls.
    /// </summary>
    private void LoadSettingsToControls()
    {
        if (_settingsService == null) return;

        var settings = _settingsService.Settings;

        // Temporarily unhook events to avoid triggering saves
        _startWithWindowsCheckbox.CheckedChanged -= OnStartWithWindowsChanged;
        _notificationsCheckbox.CheckedChanged -= OnNotificationsChanged;
        _debugModeCheckbox.CheckedChanged -= OnDebugModeChanged;
        _primaryProviderCombo.SelectedIndexChanged -= OnPrimaryProviderChanged;
        _refreshIntervalCombo.SelectedIndexChanged -= OnRefreshIntervalChanged;

        _startWithWindowsCheckbox.Checked = settings.StartWithWindows;
        _notificationsCheckbox.Checked = settings.NotificationsEnabled;
        _debugModeCheckbox.Checked = settings.DebugMode;
        _primaryProviderCombo.SelectedItem = SettingsService.NormalizeProviderName(settings.PrimaryProvider);

        // Map interval to combo index
        _refreshIntervalCombo.SelectedIndex = settings.RefreshIntervalSeconds switch
        {
            30 => 0,
            60 => 1,
            120 => 2,
            300 => 3,
            _ => 1 // Default to 60s
        };

        // Re-hook events
        _startWithWindowsCheckbox.CheckedChanged += OnStartWithWindowsChanged;
        _notificationsCheckbox.CheckedChanged += OnNotificationsChanged;
        _debugModeCheckbox.CheckedChanged += OnDebugModeChanged;
        _primaryProviderCombo.SelectedIndexChanged += OnPrimaryProviderChanged;
        _refreshIntervalCombo.SelectedIndexChanged += OnRefreshIntervalChanged;
    }

    /// <summary>
    /// Shows the settings panel (called from tray context menu).
    /// </summary>
    public void ShowSettings()
    {
        if (!_settingsExpanded)
        {
            _settingsExpanded = true;
            LoadSettingsToControls();
            _settingsPanel.Visible = true;
            RecalculateLayout();
            PositionNearTray();
        }
    }

    /// <summary>
    /// Hides the settings panel.
    /// </summary>
    private void HideSettings()
    {
        if (_settingsExpanded)
        {
            _settingsExpanded = false;
            _settingsPanel.Visible = false;
            RecalculateLayout();
            PositionNearTray();
        }
    }

    private void OnStartWithWindowsChanged(object? sender, EventArgs e)
    {
        if (_settingsService == null) return;
        _settingsService.SetStartWithWindows(_startWithWindowsCheckbox.Checked);
    }

    private void OnNotificationsChanged(object? sender, EventArgs e)
    {
        if (_settingsService == null) return;
        _settingsService.Settings.NotificationsEnabled = _notificationsCheckbox.Checked;
        _settingsService.Save();
    }

    private void OnDebugModeChanged(object? sender, EventArgs e)
    {
        if (_settingsService == null) return;
        _settingsService.Settings.DebugMode = _debugModeCheckbox.Checked;
        _settingsService.Save();
    }

    private void OnRefreshIntervalChanged(object? sender, EventArgs e)
    {
        if (_settingsService == null) return;

        var interval = _refreshIntervalCombo.SelectedIndex switch
        {
            0 => 30,
            1 => 60,
            2 => 120,
            3 => 300,
            _ => 60
        };

        _settingsService.Settings.RefreshIntervalSeconds = interval;
        _settingsService.Save();
        OnSettingsChanged?.Invoke(this, EventArgs.Empty);
    }

    private void OnPrimaryProviderChanged(object? sender, EventArgs e)
    {
        if (_settingsService == null)
            return;

        _settingsService.SetPrimaryProvider(_primaryProviderCombo.SelectedItem?.ToString());
        OnSettingsChanged?.Invoke(this, EventArgs.Empty);
    }

    /// <summary>
    /// Updates the popup with new usage data.
    /// </summary>
    public void UpdateData(IReadOnlyList<UsageData> providerData)
    {
        var providerMap = providerData.ToDictionary(x => x.ProviderName, StringComparer.OrdinalIgnoreCase);
        _claudePanel.UpdateData(providerMap.TryGetValue("Claude", out var claude) ? claude : null);
        _codexPanel.UpdateData(providerMap.TryGetValue("Codex", out var codex) ? codex : null);
        _cursorPanel.UpdateData(providerMap.TryGetValue("Cursor", out var cursor) ? cursor : null);
        ApplyProviderOrder();
        RecalculateLayout();
    }

    private void ApplyProviderOrder()
    {
        var primaryProvider = SettingsService.NormalizeProviderName(_settingsService?.Settings.PrimaryProvider);
        var panels = new[] { _claudePanel, _codexPanel, _cursorPanel };

        foreach (var panel in panels)
        {
            panel.SetProminent(panel.ProviderName == primaryProvider);
        }

        var orderedPanels = panels
            .OrderByDescending(panel => panel.ProviderName == primaryProvider)
            .ThenBy(panel => panel.ProviderName switch
            {
                "Claude" => 0,
                "Codex" => 1,
                _ => 2
            })
            .ToArray();

        for (var i = 0; i < orderedPanels.Length; i++)
        {
            _providerList.Controls.SetChildIndex(orderedPanels[i], i);
        }
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

    private void RecalculateLayout()
    {
        _providerList.Location = new Point(12, 44);
        _settingsPanel.Location = new Point(0, _providerList.Bottom + 8);
        _collapsedHeight = _providerList.Bottom + 12;
        Height = _settingsExpanded ? _collapsedHeight + _settingsPanel.Height : _collapsedHeight;
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
