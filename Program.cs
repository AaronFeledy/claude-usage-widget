using ClaudeUsageWidget.Services;

namespace ClaudeUsageWidget;

static class Program
{
    static async Task Main(string[] args)
    {
        Console.WriteLine("Claude Usage Widget - API Test");
        Console.WriteLine("==============================");
        Console.WriteLine();

        try
        {
            using var httpClient = new HttpClient();
            httpClient.Timeout = TimeSpan.FromSeconds(30);

            var credentialService = new CredentialService(httpClient);
            var apiClient = new UsageApiClient(httpClient, credentialService);

            Console.WriteLine("Fetching usage data...");
            Console.WriteLine();

            var usage = await apiClient.FetchUsageAsync();

            if (!usage.IsSuccess)
            {
                Console.WriteLine($"Error: {usage.Error}");
                return;
            }

            Console.WriteLine("5-Hour Usage:");
            Console.WriteLine($"  Utilization: {usage.FiveHour.Utilization:F1}%");
            Console.WriteLine($"  Resets at:   {usage.FiveHour.ResetsAt?.ToLocalTime():g}");
            Console.WriteLine($"  Time left:   {usage.FiveHour.TimeUntilReset}");
            Console.WriteLine();

            Console.WriteLine("7-Day Usage:");
            Console.WriteLine($"  Utilization: {usage.SevenDay.Utilization:F1}%");
            Console.WriteLine($"  Resets at:   {usage.SevenDay.ResetsAt?.ToLocalTime():g}");
            Console.WriteLine($"  Time left:   {usage.SevenDay.TimeUntilReset}");
        }
        catch (FileNotFoundException ex)
        {
            Console.WriteLine($"Credentials not found: {ex.Message}");
            Console.WriteLine("Please authenticate with Claude CLI first.");
        }
        catch (Exception ex)
        {
            Console.WriteLine($"Error: {ex.Message}");
        }
    }
}
