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

        using var httpClient = new HttpClient();
        httpClient.Timeout = TimeSpan.FromSeconds(30);

        var settingsService = new SettingsService();
        var credentialService = new CredentialService(httpClient);
        var usageApiClient = new UsageApiClient(httpClient, credentialService);

        Application.Run(new TrayApplicationContext(usageApiClient, credentialService, settingsService, httpClient));
    }
}
