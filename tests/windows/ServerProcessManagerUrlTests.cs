using ClaudeUsageWidget.Services;

internal sealed partial class ServerProcessManagerTests
{
    public async Task Test_InvalidApiUrl_when_ConfiguredDoesNotThrowOrSpawn()
    {
        foreach (var apiUrl in new[] { "relative/path", "http://localhost:bad", "ftp://localhost:7823", "file:///tmp/server", "   ", "http://user:pass@localhost:7823" })
        {
            var deps = FakeDependencies.Create();
            await using var manager = new ServerProcessManager(new ServerProcessOptions(ApiUrl: apiUrl), deps.Value);

            var result = await manager.EnsureStartedAsync();

            AssertEqual(ServerProcessState.Failed, result.State);
            AssertEqual(ServerProcessError.InvalidApiUrl, result.Error);
            AssertEqual(0, deps.Health.Calls);
            AssertEqual(0, deps.Launcher.Starts);
            AssertEqual(0, deps.Acquirer.Calls);
        }
    }

    public async Task Test_ValidApiUrl_when_ConfiguredNormalizes()
    {
        foreach (var apiUrl in new[] { "http://127.0.0.1:9000", "https://example.test/base", "http://[::1]:7823" })
        {
            var deps = FakeDependencies.Create();
            await using var manager = new ServerProcessManager(new ServerProcessOptions(ApiUrl: apiUrl), deps.Value);

            var result = await manager.EnsureStartedAsync();

            AssertEqual(ServerProcessState.Remote, result.State);
            AssertEqual(true, result.EffectiveBaseUrl.ToString().EndsWith('/'));
            AssertEqual(0, deps.Health.Calls);
            AssertEqual(0, deps.Launcher.Starts);
        }
    }
}
