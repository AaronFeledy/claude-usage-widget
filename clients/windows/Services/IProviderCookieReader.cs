namespace ClaudeUsageWidget.Services;

public interface IProviderCookieReader
{
    string? ReadCursorCookieHeader();
    string? ReadGrokCookieHeader();
}
