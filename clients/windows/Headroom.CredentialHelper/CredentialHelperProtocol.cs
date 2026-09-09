using System.Text.Json;
using ClaudeUsageWidget.Services;

namespace Headroom.CredentialHelper;

public static class CredentialHelperProtocol
{
    public const int MaximumOutputBytes = 64 * 1024;

    public static bool IsKnownInvocation(IReadOnlyList<string> arguments) =>
        arguments.Count == 1 && arguments[0].Trim().ToLowerInvariant() is "cursor" or "grok";

    public static int Run(IReadOnlyList<string> arguments, IProviderCookieReader reader, TextWriter output)
    {
        if (!IsKnownInvocation(arguments)) return 2;
        var provider = arguments[0].Trim().ToLowerInvariant();
        string? cookie = provider switch
        {
            "cursor" => reader.ReadCursorCookieHeader(),
            "grok" => reader.ReadGrokCookieHeader(),
            _ => null
        };
        var json = JsonSerializer.Serialize(new { provider, cookie });
        if (System.Text.Encoding.UTF8.GetByteCount(json) > MaximumOutputBytes) return 3;
        output.Write(json);
        return 0;
    }
}
