using ClaudeUsageWidget.Models;

internal sealed partial class TrayApiHarnessTests
{
    public Task Test_BucketPresentation_StatusOnlyForEnabledOrCapOnlyOnDemand()
    {
        var capOnly = new UsageBucketDetail { Id = "on_demand", Label = "Pay as you go", Utilization = 0, StatusText = "40 pay-as-you-go cap" };
        var enabledOnly = new UsageBucketDetail { Id = "on_demand", Label = "Pay as you go", Utilization = 0, StatusText = "Pay as you go enabled" };
        var usedOnly = new UsageBucketDetail { Id = "on_demand", Label = "Pay as you go", Utilization = 0, StatusText = "15 pay-as-you-go used" };

        AssertEqual(true, BucketPresentation.IsStatusOnly(capOnly));
        AssertEqual(true, BucketPresentation.IsStatusOnly(enabledOnly));
        AssertEqual(true, BucketPresentation.IsStatusOnly(usedOnly));
        return Task.CompletedTask;
    }

    public Task Test_BucketPresentation_MeasuredForUtilizedOnDemandAndOtherMeters()
    {
        var measured = new UsageBucketDetail { Id = "on_demand", Label = "Pay as you go", Utilization = 25, StatusText = "10 / 40 pay-as-you-go" };
        var onDemandNoStatus = new UsageBucketDetail { Id = "on_demand", Label = "Pay as you go", Utilization = 0, StatusText = null };
        var creditsZero = new UsageBucketDetail { Id = "credits", Label = "Credits", Utilization = 0, StatusText = "0 / 100 credits" };

        AssertEqual(false, BucketPresentation.IsStatusOnly(measured));
        AssertEqual(false, BucketPresentation.IsStatusOnly(onDemandNoStatus));
        AssertEqual(false, BucketPresentation.IsStatusOnly(creditsZero));
        return Task.CompletedTask;
    }

    public Task Test_BucketPresentation_MeasuredForTruthfulZeroRatioOnDemand()
    {
        var zeroRatio = new UsageBucketDetail { Id = "on_demand", Label = "Pay as you go", Utilization = 0, StatusText = "0 / 40 pay-as-you-go" };
        var capOnly = new UsageBucketDetail { Id = "on_demand", Label = "Pay as you go", Utilization = 0, StatusText = "40 pay-as-you-go cap" };

        AssertEqual(false, BucketPresentation.IsStatusOnly(zeroRatio));
        AssertEqual(true, BucketPresentation.IsStatusOnly(capOnly));
        return Task.CompletedTask;
    }
}
