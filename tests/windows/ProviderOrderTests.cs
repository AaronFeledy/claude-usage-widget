using ClaudeUsageWidget.Services;

internal sealed partial class TrayApiHarnessTests
{
    public Task Test_ProviderOrderMigrationAndPersistence()
    {
        var dir = Directory.CreateTempSubdirectory();
        try
        {
            var path = Path.Combine(dir.FullName, "settings.json");
            File.WriteAllText(path, "{\"PrimaryProvider\":\"Cursor\",\"SchemaVersion\":2,\"ApiUrl\":\"https://server.example\",\"ApiToken\":\"secret\"}");
            var service = new SettingsService(path);
            AssertEqual("Cursor,Claude,Codex,Grok", string.Join(",", service.Settings.ProviderOrder));
            service.SetProviderOrder(new[] { "Grok", "Codex", "Cursor", "Claude" });
            AssertEqual("Grok", service.Settings.PrimaryProvider);
            var restored = new SettingsService(path);
            AssertEqual("Grok,Codex,Cursor,Claude", string.Join(",", restored.Settings.ProviderOrder));
            AssertEqual("Grok", restored.Settings.PrimaryProvider);
            AssertEqual("secret", restored.Settings.ApiToken);
            restored.SetPrimaryProvider("Cursor");
            AssertEqual("Cursor,Grok,Codex,Claude", string.Join(",", restored.Settings.ProviderOrder));
            AssertEqual("Cursor", new SettingsService(path).Settings.PrimaryProvider);
            AssertEqual("Grok,Claude,Codex,Cursor", string.Join(",", SettingsService.NormalizeProviderOrder(new[] { "grok", "Grok", "bad", "claude" })));
            AssertEqual("Codex,Claude,Cursor,Grok", string.Join(",", SettingsService.NormalizeProviderOrder(null, "Codex")));
            AssertEqual(true, File.Exists(path + ".bak"));
        }
        finally { dir.Delete(recursive: true); }
        return Task.CompletedTask;
    }
}
