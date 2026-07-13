using ClaudeUsageWidget.Services;

internal sealed partial class ServerProcessManagerTests
{
    public async Task Test_AcquisitionCleansTemp_when_DownloadFails()
    {
        using var http = new HttpClient(new FakeReleaseHandler(false));
        var dir = Directory.CreateTempSubdirectory();
        var target = Path.Combine(dir.FullName, "usage-server.exe");
        var acquirer = new UpdateServiceServerBinaryAcquirer(http, ArchitectureOverride.X64, 4);

        var result = await acquirer.AcquireAsync(new ServerBinaryRequest(target, "1.2.3"), CancellationToken.None);

        AssertEqual(false, result);
        AssertEqual(false, File.Exists(target));
        AssertEqual(false, Directory.EnumerateFiles(dir.FullName, "*.tmp").Any());
        dir.Delete(recursive: true);
    }

    public async Task Test_AcquisitionWritesVersionManifest_when_DownloadSucceeds()
    {
        using var http = new HttpClient(new FakeReleaseHandler(true));
        var dir = Directory.CreateTempSubdirectory();
        var target = Path.Combine(dir.FullName, "usage-server.exe");
        var acquirer = new UpdateServiceServerBinaryAcquirer(http, ArchitectureOverride.X64, 64);
        var reader = new SidecarServerBinaryVersionReader();

        var result = await acquirer.AcquireAsync(new ServerBinaryRequest(target, "1.2.3"), CancellationToken.None);

        AssertEqual(true, result);
        AssertEqual(true, File.Exists(target));
        AssertEqual(true, reader.Matches(target, "1.2.3"));
        AssertEqual(false, Directory.EnumerateFiles(dir.FullName, "*.tmp").Any());
        dir.Delete(recursive: true);
    }

    public async Task Test_HealthProbeAcceptsHealthyServer_when_VersionDiffers()
    {
        using var http = new HttpClient(new FakeHealthHandler("{\"status\":\"ok\",\"version\":\"9.9.9\",\"providers\":[]}"));
        http.BaseAddress = new Uri("http://127.0.0.1:7823/");
        var probe = new ServerHealthProbe(http);

        var healthy = await probe.IsHealthyAsync(http.BaseAddress, "1.2.3", CancellationToken.None);

        AssertEqual(true, healthy);
    }

    public async Task Test_HealthProbeAcceptsDegradedServer_when_ProviderErroring()
    {
        using var http = new HttpClient(new FakeHealthHandler("{\"status\":\"degraded\",\"version\":\"dev\",\"providers\":[{\"name\":\"Cursor\",\"ok\":false,\"fetched_at\":null}]}"));
        http.BaseAddress = new Uri("http://127.0.0.1:7823/");
        var probe = new ServerHealthProbe(http);

        var healthy = await probe.IsHealthyAsync(http.BaseAddress, "", CancellationToken.None);

        AssertEqual(true, healthy);
    }

    public async Task Test_AcquirerRethrowsCancellation_and_CleansTemp()
    {
        using var http = new HttpClient(new FakeReleaseHandler(true));
        var dir = Directory.CreateTempSubdirectory();
        var target = Path.Combine(dir.FullName, "usage-server.exe");
        await File.WriteAllBytesAsync(target + ".tmp", [1, 2, 3]);
        var acquirer = new UpdateServiceServerBinaryAcquirer(http, ArchitectureOverride.X64, 64);

        await AssertCanceled(() => acquirer.AcquireAsync(new ServerBinaryRequest(target, "1.2.3"), new CancellationToken(true)));

        AssertEqual(false, File.Exists(target));
        AssertEqual(false, Directory.EnumerateFiles(dir.FullName, "*.tmp").Any());
        dir.Delete(recursive: true);
    }
}
