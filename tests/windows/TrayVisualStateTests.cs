using ClaudeUsageWidget.Models;
using ClaudeUsageWidget.Services;

internal sealed partial class TrayApiHarnessTests
{
    public Task Test_TooltipTextCapsEveryBranchSafely()
    {
        var longText = new string('x', 150) + char.ConvertFromUtf32(0x1F680);
        AssertEqual(true, TrayVisualState.TooltipText(longText).Length <= 127);
        AssertEqual(false, char.IsHighSurrogate(TrayVisualState.TooltipText(longText)[^1]));
        AssertEqual(true, TrayVisualState.TooltipText("Usage: No data").Length <= 127);
        AssertEqual(true, TrayVisualState.TooltipText("Claude: run '" + new string('a', 150) + "' to re-auth").Length <= 127);
        AssertEqual(true, TrayVisualState.TooltipText("Claude: " + new string('e', 200)).Length <= 127);
        AssertEqual(true, TrayVisualState.TooltipText("Usage API error: " + new string('m', 200)).Length <= 127);
        return Task.CompletedTask;
    }

    public Task Test_IconKindMapsNonReadyStatesBeforeStaleSuccessData()
    {
        var success = new UsageData { ProviderName = "Claude", Current = new UsageBucket { Utilization = 25 }, Weekly = new UsageBucket() };
        AssertEqual(TrayIconKind.Loading, TrayVisualState.IconKind(TrayApiState.Loading, success));
        AssertEqual(TrayIconKind.Offline, TrayVisualState.IconKind(TrayApiState.Offline, success));
        AssertEqual(TrayIconKind.ApiUnauthorized, TrayVisualState.IconKind(TrayApiState.Unauthorized, success));
        AssertEqual(TrayIconKind.ApiMalformed, TrayVisualState.IconKind(TrayApiState.Malformed, success));
        AssertEqual(TrayIconKind.ApiError, TrayVisualState.IconKind(TrayApiState.ApiError, success));
        return Task.CompletedTask;
    }

    public Task Test_IconKindPreservesReadyAndProviderAuthSemantics()
    {
        var authFailure = new UsageData { ProviderName = "Cursor", Error = "needs auth", NeedsReauth = true };
        var idle = new UsageData { ProviderName = "Claude", Current = new UsageBucket { Utilization = 0.5f } };
        var usage = new UsageData { ProviderName = "Claude", Current = new UsageBucket { Utilization = 50 } };
        AssertEqual(TrayIconKind.ProviderError, TrayVisualState.IconKind(TrayApiState.Ready, authFailure));
        AssertEqual(TrayIconKind.Idle, TrayVisualState.IconKind(TrayApiState.Ready, idle));
        AssertEqual(TrayIconKind.Usage, TrayVisualState.IconKind(TrayApiState.Ready, usage));
        return Task.CompletedTask;
    }
}
