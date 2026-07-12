using ClaudeUsageWidget.Services;

namespace ClaudeUsageWidget.UI;

public partial class UsagePopup
{
    public void SetSettingsService(SettingsService settingsService)
    {
        _settingsService = settingsService;
        LoadSettingsToControls();
        ApplyProviderOrder();
    }

    private void LoadSettingsToControls()
    {
        if (_settingsService == null) return;

        var settings = _settingsService.Settings;

        _startWithWindowsCheckbox.CheckedChanged -= OnStartWithWindowsChanged;
        _notificationsCheckbox.CheckedChanged -= OnNotificationsChanged;
        _debugModeCheckbox.CheckedChanged -= OnDebugModeChanged;
        _primaryProviderCombo.SelectedIndexChanged -= OnPrimaryProviderChanged;
        _refreshIntervalCombo.SelectedIndexChanged -= OnRefreshIntervalChanged;

        _startWithWindowsCheckbox.Checked = settings.StartWithWindows;
        _notificationsCheckbox.Checked = settings.NotificationsEnabled;
        _debugModeCheckbox.Checked = settings.DebugMode;
        _primaryProviderCombo.SelectedItem = SettingsService.NormalizeProviderName(settings.PrimaryProvider);
        _refreshIntervalCombo.SelectedIndex = settings.RefreshIntervalSeconds switch
        {
            30 => 0,
            60 => 1,
            120 => 2,
            300 => 3,
            _ => 1
        };

        _startWithWindowsCheckbox.CheckedChanged += OnStartWithWindowsChanged;
        _notificationsCheckbox.CheckedChanged += OnNotificationsChanged;
        _debugModeCheckbox.CheckedChanged += OnDebugModeChanged;
        _primaryProviderCombo.SelectedIndexChanged += OnPrimaryProviderChanged;
        _refreshIntervalCombo.SelectedIndexChanged += OnRefreshIntervalChanged;
    }

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

    private void ApplyProviderOrder()
    {
        var primaryProvider = SettingsService.NormalizeProviderName(_settingsService?.Settings.PrimaryProvider);
        var panels = new[] { _claudePanel, _codexPanel, _cursorPanel, _grokPanel };

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
                "Cursor" => 2,
                "Grok" => 3,
                _ => 4
            })
            .ToArray();

        for (var i = 0; i < orderedPanels.Length; i++)
        {
            _providerList.Controls.SetChildIndex(orderedPanels[i], i);
        }
    }
}
