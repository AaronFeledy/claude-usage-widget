using ClaudeUsageWidget.Services;

namespace Headroom.CredentialHelper;

internal static class Program
{
    private static int Main(string[] args)
    {
        if (!CredentialHelperProtocol.IsKnownInvocation(args)) return 2;
        try
        {
            var snapshotRoot = Environment.GetEnvironmentVariable("HEADROOM_CREDENTIAL_SNAPSHOT_ROOT");
            var options = WindowsBrowserCookieReaderOptions.ForCurrentUser(
                string.IsNullOrWhiteSpace(snapshotRoot) ? null : Path.GetFullPath(snapshotRoot));
            return CredentialHelperProtocol.Run(args, new WindowsBrowserCookieReader(options), Console.Out);
        }
        catch
        {
            // The parent only receives a status code. Paths, browser errors, and
            // credential material never go to stdout or stderr.
            return 1;
        }
    }
}
