using ClaudeUsageWidget.Services;
using ClaudeUsageWidget.TrayIcon;

namespace ClaudeUsageWidget;

static class Program
{
    [STAThread]
    static void Main()
    {
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);

        using var httpClient = new HttpClient();
        httpClient.Timeout = TimeSpan.FromSeconds(30);

        var credentialService = new CredentialService(httpClient);
        var usageApiClient = new UsageApiClient(httpClient, credentialService);

        Application.Run(new TrayApplicationContext(usageApiClient));
    }
}
