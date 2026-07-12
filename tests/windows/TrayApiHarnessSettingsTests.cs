using ClaudeUsageWidget.Services;

internal sealed partial class TrayApiHarnessTests
{
    public async Task Test_SettingsWhitespaceApiUrlRemainsInvalidRemote()
    {
        var dir = Directory.CreateTempSubdirectory();
        try
        {
            var path = Path.Combine(dir.FullName, "settings.json");
            File.WriteAllText(path, "{\"ApiUrl\":\"   \",\"ApiToken\":\" token \"}");
            var settings = new SettingsService(path);
            var deps = FakeDependencies.Create();

            await using var manager = new ServerProcessManager(new ServerProcessOptions(ApiUrl: settings.Settings.ApiUrl), deps.Value);
            var result = await manager.EnsureStartedAsync();

            AssertEqual("   ", settings.Settings.ApiUrl);
            AssertEqual("token", settings.Settings.ApiToken);
            AssertEqual(ServerProcessState.Failed, result.State);
            AssertEqual(ServerProcessError.InvalidApiUrl, result.Error);
            AssertEqual(0, deps.Health.Calls);
            AssertEqual(0, deps.Launcher.Starts);
            AssertEqual(0, deps.Acquirer.Calls);
        }
        finally
        {
            dir.Delete(recursive: true);
        }
    }

    public async Task Test_SettingsEmptyApiUrlRemainsLocalSentinel()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(true);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(ApiUrl: string.Empty), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Attached, result.State);
        AssertEqual(1, deps.Health.Calls);
        AssertEqual(0, deps.Launcher.Starts);
    }

    public async Task Test_SettingsValidApiUrlRemainsRemote()
    {
        var dir = Directory.CreateTempSubdirectory();
        try
        {
            var path = Path.Combine(dir.FullName, "settings.json");
            File.WriteAllText(path, "{\"ApiUrl\":\"https://example.test/api\",\"ApiToken\":\" token \"}");
            var settings = new SettingsService(path);
            var deps = FakeDependencies.Create();

            await using var manager = new ServerProcessManager(new ServerProcessOptions(ApiUrl: settings.Settings.ApiUrl), deps.Value);
            var result = await manager.EnsureStartedAsync();

            AssertEqual(ServerProcessState.Remote, result.State);
            AssertEqual("https://example.test/api/", result.EffectiveBaseUrl.ToString());
            AssertEqual("token", settings.Settings.ApiToken);
            AssertEqual(0, deps.Health.Calls);
            AssertEqual(0, deps.Launcher.Starts);
        }
        finally
        {
            dir.Delete(recursive: true);
        }
    }
}
