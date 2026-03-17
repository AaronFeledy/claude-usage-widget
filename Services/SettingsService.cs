using System.Text.Json;
using Microsoft.Win32;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Application settings with JSON persistence.
/// </summary>
public class AppSettings
{
    public int RefreshIntervalSeconds { get; set; } = 60;
    public bool StartWithWindows { get; set; } = false;
    public bool NotificationsEnabled { get; set; } = true;
    public bool DebugMode { get; set; } = false;
    public string PrimaryProvider { get; set; } = "Claude";
}

/// <summary>
/// Manages application settings stored in %APPDATA%/ClaudeUsageWidget/settings.json.
/// </summary>
public class SettingsService
{
    private const string AppName = "ClaudeUsageWidget";
    private const string SettingsFileName = "settings.json";
    private const string RegistryKeyPath = @"Software\Microsoft\Windows\CurrentVersion\Run";

    private readonly string _settingsPath;
    private AppSettings _settings;

    public AppSettings Settings => _settings;

    public SettingsService()
    {
        var appDataPath = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
        var appFolder = Path.Combine(appDataPath, AppName);
        _settingsPath = Path.Combine(appFolder, SettingsFileName);
        _settings = new AppSettings();

        Load();
    }

    /// <summary>
    /// Loads settings from disk. Creates defaults if file doesn't exist.
    /// </summary>
    public void Load()
    {
        try
        {
            if (File.Exists(_settingsPath))
            {
                var json = File.ReadAllText(_settingsPath);
                var loaded = JsonSerializer.Deserialize<AppSettings>(json);
                if (loaded != null)
                {
                    _settings = loaded;
                    _settings.PrimaryProvider = NormalizeProviderName(_settings.PrimaryProvider);
                }
            }
        }
        catch
        {
            // Use defaults on any error
            _settings = new AppSettings();
        }
    }

    /// <summary>
    /// Saves current settings to disk.
    /// </summary>
    public void Save()
    {
        try
        {
            var directory = Path.GetDirectoryName(_settingsPath);
            if (!string.IsNullOrEmpty(directory) && !Directory.Exists(directory))
            {
                Directory.CreateDirectory(directory);
            }

            var options = new JsonSerializerOptions { WriteIndented = true };
            var json = JsonSerializer.Serialize(_settings, options);
            File.WriteAllText(_settingsPath, json);
        }
        catch
        {
            // Silently fail - settings are not critical
        }
    }

    /// <summary>
    /// Adds or removes the application from Windows startup.
    /// </summary>
    public void SetStartWithWindows(bool enabled)
    {
        _settings.StartWithWindows = enabled;

        try
        {
            using var key = Registry.CurrentUser.OpenSubKey(RegistryKeyPath, writable: true);
            if (key == null) return;

            if (enabled)
            {
                // Get the path to the current executable
                var exePath = Environment.ProcessPath;
                if (!string.IsNullOrEmpty(exePath))
                {
                    key.SetValue(AppName, $"\"{exePath}\"");
                }
            }
            else
            {
                key.DeleteValue(AppName, throwOnMissingValue: false);
            }
        }
        catch
        {
            // Registry access may fail in some environments
        }

        Save();
    }

    public void SetPrimaryProvider(string? providerName)
    {
        _settings.PrimaryProvider = NormalizeProviderName(providerName);
        Save();
    }

    public static string NormalizeProviderName(string? providerName)
    {
        return providerName?.Trim() switch
        {
            "Codex" => "Codex",
            "Cursor" => "Cursor",
            _ => "Claude"
        };
    }
}
