using System.Text.Json;
using System.Text.Json.Serialization;
using System.Runtime.Versioning;
using Microsoft.Win32;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Application settings with JSON persistence.
/// </summary>
public class AppSettings
{
    public const int CurrentSchemaVersion = 2;

    public int RefreshIntervalSeconds { get; set; } = 60;
    public bool StartWithWindows { get; set; } = false;
    public bool NotificationsEnabled { get; set; } = true;
    public bool DebugMode { get; set; } = false;
    public string PrimaryProvider { get; set; } = "Claude";
    public string ApiUrl { get; set; } = string.Empty;
    public string ApiToken { get; set; } = string.Empty;
    public int SchemaVersion { get; set; } = CurrentSchemaVersion;

    [JsonExtensionData]
    public Dictionary<string, JsonElement>? ExtraFields { get; set; }
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
    private string? _loadError;

    public AppSettings Settings => _settings;
    public string? LoadError => _loadError;

    public SettingsService()
    {
        var appDataPath = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
        var appFolder = Path.Combine(appDataPath, AppName);
        _settingsPath = Path.Combine(appFolder, SettingsFileName);
        _settings = new AppSettings();

        Load();
    }

    public SettingsService(string settingsPath)
    {
        _settingsPath = settingsPath;
        _settings = new AppSettings();
        Load();
    }

    /// <summary>
    /// Loads settings from disk. Creates defaults if file doesn't exist.
    /// </summary>
    public void Load()
    {
        _loadError = null;
        try
        {
            if (File.Exists(_settingsPath))
            {
                var original = File.ReadAllText(_settingsPath);
                using var document = JsonDocument.Parse(original);
                var loaded = JsonSerializer.Deserialize<AppSettings>(original);
                if (loaded != null)
                {
                    _settings = loaded;
                    NormalizeLoadedSettings();
                    if (NeedsMigration(document.RootElement))
                    {
                        BackupOriginalSettings();
                        SaveAtomic();
                    }
                }
            }
        }
        catch (JsonException ex)
        {
            _loadError = $"Settings file is malformed and was not overwritten: {ex.Message}";
            _settings = new AppSettings();
        }
        catch (IOException ex)
        {
            _loadError = $"Settings file could not be read: {ex.Message}";
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
            NormalizeLoadedSettings();
            SaveAtomic();
        }
        catch
        {
            // Silently fail - settings are not critical
        }
    }

    /// <summary>
    /// Adds or removes the application from Windows startup.
    /// </summary>
    [SupportedOSPlatform("windows")]
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
            "Grok" => "Grok",
            _ => "Claude"
        };
    }

    private void NormalizeLoadedSettings()
    {
        _settings.PrimaryProvider = NormalizeProviderName(_settings.PrimaryProvider);
        _settings.ApiUrl = _settings.ApiUrl ?? string.Empty;
        _settings.ApiToken = _settings.ApiToken?.Trim() ?? string.Empty;
        _settings.SchemaVersion = AppSettings.CurrentSchemaVersion;
    }

    private static bool NeedsMigration(JsonElement root)
    {
        return !root.TryGetProperty(nameof(AppSettings.SchemaVersion), out var schema)
            || schema.ValueKind != JsonValueKind.Number
            || schema.GetInt32() < AppSettings.CurrentSchemaVersion
            || !root.TryGetProperty(nameof(AppSettings.ApiUrl), out _)
            || !root.TryGetProperty(nameof(AppSettings.ApiToken), out _);
    }

    private void BackupOriginalSettings()
    {
        var backupPath = _settingsPath + ".bak";
        File.Copy(_settingsPath, backupPath, overwrite: true);
    }

    private void SaveAtomic()
    {
        var directory = Path.GetDirectoryName(_settingsPath);
        if (!string.IsNullOrEmpty(directory))
        {
            Directory.CreateDirectory(directory);
        }

        var options = new JsonSerializerOptions { WriteIndented = true };
        var json = JsonSerializer.Serialize(_settings, options);
        var tempPath = _settingsPath + ".tmp";
        File.WriteAllText(tempPath, json);
        File.Move(tempPath, _settingsPath, overwrite: true);
    }
}
