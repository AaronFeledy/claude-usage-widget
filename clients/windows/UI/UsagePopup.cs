using System.Drawing;
using ClaudeUsageWidget.Models;
using ClaudeUsageWidget.Services;
using ClaudeUsageWidget.TrayIcon;

namespace ClaudeUsageWidget.UI;

/// <summary>
/// Popup panel showing detailed usage information.
/// </summary>
public partial class UsagePopup : Form
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
    private readonly ProviderUsagePanel _grokPanel;
    private readonly Panel _settingsPanel;
    private readonly CheckBox _startWithWindowsCheckbox;
    private readonly CheckBox _notificationsCheckbox;
    private readonly CheckBox _debugModeCheckbox;
    private readonly Label _primaryProviderLabel;
    private readonly ComboBox _primaryProviderCombo;
    private readonly Label _refreshIntervalLabel;
    private readonly ComboBox _refreshIntervalCombo;
    private readonly OwnedGeneratedIcon _titleIcon;
    private readonly Bitmap _titleIconBitmap;

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

        _titleIcon = new OwnedGeneratedIcon(IconGenerator.GenerateAppIcon(includeBadge: false));
        _titleIconBitmap = _titleIcon.Icon.ToBitmap();
        var titleIcon = new PictureBox
        {
            Size = new Size(20, 20),
            Location = new Point(10, 10),
            SizeMode = PictureBoxSizeMode.StretchImage,
            Image = _titleIconBitmap
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
        _grokPanel = new ProviderUsagePanel("Grok");
        _providerList.Controls.Add(_claudePanel);
        _providerList.Controls.Add(_codexPanel);
        _providerList.Controls.Add(_cursorPanel);
        _providerList.Controls.Add(_grokPanel);
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
        _primaryProviderCombo.Items.AddRange(new object[] { "Claude", "Codex", "Cursor", "Grok" });
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
    /// Updates the popup with new usage data.
    /// </summary>
    public void UpdateData(IReadOnlyList<UsageData> providerData)
    {
        var providerMap = providerData.ToDictionary(x => x.ProviderName, StringComparer.OrdinalIgnoreCase);
        _claudePanel.UpdateData(providerMap.TryGetValue("Claude", out var claude) ? claude : null);
        _codexPanel.UpdateData(providerMap.TryGetValue("Codex", out var codex) ? codex : null);
        _cursorPanel.UpdateData(providerMap.TryGetValue("Cursor", out var cursor) ? cursor : null);
        _grokPanel.UpdateData(providerMap.TryGetValue("Grok", out var grok) ? grok : null);
        ApplyProviderOrder();
        RecalculateLayout();
    }

}
