internal static class TrayApiHarnessRunner
{
    public static async Task RunAsync()
    {
        var trayTests = new TrayApiHarnessTests();
        await trayTests.Test_SettingsMigration_preservesValuesAndBackup();
        await trayTests.Test_SettingsWhitespaceApiUrlRemainsInvalidRemote();
        await trayTests.Test_SettingsEmptyApiUrlRemainsLocalSentinel();
        await trayTests.Test_SettingsValidApiUrlRemainsRemote();
await trayTests.Test_OfflineRetainsLastGoodAndRetries();
await trayTests.Test_OfflineSnapshotPreservesIndependentBucketCopies();
await trayTests.Test_UnauthorizedDistinctFromOffline();
        await trayTests.Test_LocalOwnedOfflineDoesNotDoubleSpawnHealthyChild();
        await trayTests.Test_RestartFailedOfflineRequestsRecovery();
        await trayTests.Test_RemoteModeSkipsLocalProcessWork();
        await trayTests.Test_CursorCookiePutDoesNotLogSecret();
        await trayTests.Test_CursorMissingSessionPushesCookieOnce();
        await trayTests.Test_CursorExpiredSessionPushesCookieOnce();
        await trayTests.Test_CursorGenericErrorDoesNotPushCookie();
        await trayTests.Test_CursorFailedPushRetainsOriginalState();
        await trayTests.Test_GetHealth_returns_server_version();
        await trayTests.Test_GetHealth_malformed_when_version_missing();
        await trayTests.Test_GetHealth_offline_when_unreachable();
        await trayTests.Test_TooltipTextCapsEveryBranchSafely();
        await trayTests.Test_IconKindMapsNonReadyStatesBeforeStaleSuccessData();
        await trayTests.Test_IconKindPreservesReadyAndProviderAuthSemantics();
    }
}
