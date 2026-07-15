using ClaudeUsageWidget.Services;
using ClaudeUsageWidget.TrayIcon;

namespace ClaudeUsageWidget;

static class Program
{
    private static Mutex? _mutex;

    [STAThread]
    static void Main()
    {
        // Apply pending update BEFORE mutex check
        // This handles the swap: .update.exe -> current exe, current -> .old.exe
        if (UpdateService.ApplyPendingUpdate())
        {
            // Update applied, new process started - exit this one
            return;
        }

        // Single-instance enforcement
        const string mutexName = "Global\\ClaudeUsageWidget_SingleInstance";
        _mutex = new Mutex(true, mutexName, out bool createdNew);
        if (!createdNew)
        {
            // Another instance is already running
            MessageBox.Show("Claude Usage Widget is already running.", "Claude Usage Widget",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }

        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);

        using var apiHttpClient = new HttpClient { Timeout = Timeout.InfiniteTimeSpan };
        using var updateHttpClient = new HttpClient { Timeout = TimeSpan.FromSeconds(30) };

        var settingsService = new SettingsService();
        var debugService = new DebugService();
        if (!string.IsNullOrWhiteSpace(settingsService.LoadError))
        {
            debugService.LogWarning("Settings", settingsService.LoadError);
        }

        var serverManager = new ServerProcessManager(
            new ServerProcessOptions(ApiUrl: settingsService.Settings.ApiUrl),
            new ServerProcessDependencies(
                new ServerHealthProbe(updateHttpClient),
                new SidecarServerBinaryVersionReader(),
                new UpdateServiceServerBinaryAcquirer(updateHttpClient),
                new ServerProcessLauncher(),
                new WindowsServerJobAssigner(),
                new RealServerDelay()));

        ServerProcessResult serverStartup;
        try
        {
            serverStartup = serverManager.EnsureStartedAsync().GetAwaiter().GetResult();
            if (serverStartup.Error == ServerProcessError.InvalidApiUrl)
            {
                debugService.LogWarning("Settings", "Invalid ApiUrl setting; tray will remain offline until corrected.");
            }
        }
        catch (Exception ex)
        {
            serverStartup = new ServerProcessResult(ServerProcessState.Failed, serverManager.EffectiveBaseUrl, false, ServerProcessError.LaunchFailed, ex.Message);
            debugService.LogWarning("Server", "Usage server startup failed; tray will retry in offline mode", ex.Message);
        }

        var apiSettings = new TrayApiClientSettingsAdapter(settingsService, serverStartup.EffectiveBaseUrl);
        var apiClient = new ApiClient(apiHttpClient, apiSettings);
        var browserCookieReader = new WindowsBrowserCookieReader(debugService);
        var usagePoller = new TrayUsagePoller(apiClient, serverManager, browserCookieReader, debugService);

        try
        {
            Application.Run(new TrayApplicationContext(usagePoller, apiClient, settingsService, debugService, updateHttpClient));
        }
        finally
        {
            serverManager.DisposeAsync().AsTask().GetAwaiter().GetResult();
        }
    }
}
