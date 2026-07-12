using System.Globalization;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

public enum TrayIconKind
{
    Loading,
    Idle,
    Usage,
    Offline,
    ApiUnauthorized,
    ApiMalformed,
    ApiError,
    ProviderError
}

public static class TrayVisualState
{
    public const int MaxTooltipLength = 127;

    public static string TooltipText(string text)
    {
        if (text.Length <= MaxTooltipLength)
        {
            return text;
        }

        var info = StringInfo.GetTextElementEnumerator(text);
        var result = new System.Text.StringBuilder(MaxTooltipLength);
        while (info.MoveNext())
        {
            var element = info.GetTextElement();
            if (result.Length + element.Length > MaxTooltipLength)
            {
                break;
            }
            result.Append(element);
        }
        return result.ToString();
    }

    public static TrayIconKind IconKind(TrayApiState state, UsageData? data) => state switch
    {
        TrayApiState.Loading => TrayIconKind.Loading,
        TrayApiState.Offline => TrayIconKind.Offline,
        TrayApiState.Unauthorized => TrayIconKind.ApiUnauthorized,
        TrayApiState.Malformed => TrayIconKind.ApiMalformed,
        TrayApiState.ApiError => TrayIconKind.ApiError,
        _ => ReadyIconKind(data)
    };

    private static TrayIconKind ReadyIconKind(UsageData? data)
    {
        if (data == null)
        {
            return TrayIconKind.Loading;
        }
        if (!data.IsSuccess)
        {
            return TrayIconKind.ProviderError;
        }
        return data.Current.Utilization < 1f ? TrayIconKind.Idle : TrayIconKind.Usage;
    }
}
