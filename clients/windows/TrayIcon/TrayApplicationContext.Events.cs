using ClaudeUsageWidget.UI;

namespace ClaudeUsageWidget.TrayIcon;

public partial class TrayApplicationContext
{
    private void OnNotifyIconClick(object? sender, MouseEventArgs e)
    {
        if (e.Button != MouseButtons.Left) return;
        if (_popup != null && _popup.Visible) _popup.Hide();
        else EnsurePopupVisible();
    }

    private void OnSettingsChanged(object? sender, EventArgs e)
    {
        _pollTimer.Interval = _settingsService.Settings.RefreshIntervalSeconds * 1000;
        UpdateTooltip();
        UpdateIcon();
        UpdatePopup();
    }

    private void EnsurePopupVisible()
    {
        if (_popup == null)
        {
            _popup = new UsagePopup();
            _popup.SetSettingsService(_settingsService);
            _popup.OnSettingsChanged += OnSettingsChanged;
        }
        _popup.UpdateData(GetOrderedUsageDataForDisplay());
        _popup.PositionNearTray();
        _popup.Show();
        _popup.Activate();
    }

    private void OnSettingsClicked(object? sender, EventArgs e)
    {
        EnsurePopupVisible();
        _popup!.ShowSettings();
    }

    private async void OnRefreshNow(object? sender, EventArgs e) => await PollUsageAsync();

    private async void OnCheckForUpdates(object? sender, EventArgs e) =>
        await _updateService.CheckForUpdatesAsync(showNotificationIfNoUpdate: true);

    private void OnDebugConsoleClicked(object? sender, EventArgs e)
    {
        _debugConsole ??= new DebugConsole(_debugService);
        if (_debugConsole.Visible)
        {
            _debugConsole.BringToFront();
            _debugConsole.Activate();
        }
        else
        {
            _debugConsole.Show();
        }
    }

    private void OnQuit(object? sender, EventArgs e)
    {
        _pollTimer.Stop();
        _pollTimer.Dispose();
        _notifyIcon.Visible = false;
        _notifyIcon.Dispose();
        Application.Exit();
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _pollTimer.Stop();
            _pollTimer.Dispose();
            _notifyIcon.Visible = false;
            _notifyIcon.Dispose();
            _popup?.Close();
            _popup?.Dispose();
            _popup = null;
            _debugConsole?.Close();
            _debugConsole?.Dispose();
            _debugConsole = null;
            if (_currentIcon != null)
            {
                DestroyIcon(_currentIcon.Handle);
                _currentIcon.Dispose();
                _currentIcon = null;
            }
        }
        base.Dispose(disposing);
    }
}
