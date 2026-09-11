#include "updateservice.h"
#include <QCoreApplication>
#include <QFile>
#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest>

class UpdateServiceTest : public QObject {
    Q_OBJECT
private:
    QTemporaryDir m_dir;
    QString m_record;
    UpdateServiceOptions options() const {
        UpdateServiceOptions value;
        const QString root = QFileInfo(m_dir.path()).canonicalFilePath();
        value.managerPath = QStringLiteral(UPDATE_FIXTURE_PATH);
        value.installRoot = root;
        value.launcherPath = QDir(root).filePath(QStringLiteral("headroom"));
        value.packageVersion = QStringLiteral("0.1.0");
        value.timeoutMs = 1000;
        value.cancelGraceMs = 100;
        value.fixtureIdentity = true;
        return value;
    }
    QByteArray record() const { QFile file(m_record); return file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray(); }
private slots:
    void initTestCase() { QVERIFY(m_dir.isValid()); m_record = m_dir.filePath(QStringLiteral("record")); QCoreApplication::setApplicationVersion(QStringLiteral("0.1.0")); }
    void init() {
        QFile::remove(m_record);
        QDir(m_dir.filePath(QStringLiteral("transactions"))).removeRecursively();
        QDir(m_dir.filePath(QStringLiteral("staging"))).removeRecursively();
        qputenv("HEADROOM_UPDATE_FIXTURE_RECORD", m_record.toUtf8());
		qunsetenv("HEADROOM_UPDATE_FIXTURE_MODE"); qunsetenv("HEADROOM_UPDATE_FIXTURE_MISSING"); qunsetenv("HEADROOM_UPDATE_FIXTURE_INTERNAL_LAUNCHER"); qunsetenv("HEADROOM_UPDATE_FIXTURE_BAD_STAGE"); qunsetenv("HEADROOM_UPDATE_FIXTURE_APPLY_STATUS");
    }
	void cleanup() { qunsetenv("HEADROOM_UPDATE_FIXTURE_RECORD"); qunsetenv("HEADROOM_UPDATE_FIXTURE_MODE"); qunsetenv("HEADROOM_UPDATE_FIXTURE_MISSING"); qunsetenv("HEADROOM_UPDATE_FIXTURE_INTERNAL_LAUNCHER"); qunsetenv("HEADROOM_UPDATE_FIXTURE_BAD_STAGE"); qunsetenv("HEADROOM_UPDATE_FIXTURE_APPLY_STATUS"); qunsetenv("USAGE_AUTH_TOKEN"); }
    void sourceAndIsolatedModesNeverStartManager() {
        UpdateServiceOptions source;
        source.managerPath = QStringLiteral(UPDATE_FIXTURE_PATH);
        UpdateService service(true, source);
        QCOMPARE(service.state(), QStringLiteral("unavailable")); QCOMPARE(service.updateMethod(), QStringLiteral("source"));
        QCOMPARE(service.statusText(), QStringLiteral("This source installation is updated from its source checkout."));
        service.startAutomaticCheck(); service.checkForUpdates(); QTest::qWait(50);
        QVERIFY(record().isEmpty());
        auto official = options(); UpdateService isolated(false, official);
        isolated.startAutomaticCheck(); QTest::qWait(50);
        QVERIFY(record().isEmpty()); QVERIFY(isolated.statusText().contains(QStringLiteral("disabled")));
        official.systemManaged = true; UpdateService system(true, official); system.startAutomaticCheck(); QTest::qWait(50);
        QVERIFY(record().isEmpty()); QCOMPARE(system.updateMethod(), QStringLiteral("system"));
        QCOMPARE(system.statusText(), QStringLiteral("This installation is managed by your system package manager."));
    }
    void externalStableEntryIsValidatedWithoutPathEquality() {
        const QString rootAlias = m_dir.filePath(QStringLiteral("native identity"));
        QVERIFY(QDir().mkpath(rootAlias));
        const QString root = QFileInfo(rootAlias).canonicalFilePath();
#ifdef Q_OS_WIN
        const QString appName = QStringLiteral("headroom.exe");
        const QString internalName = QStringLiteral("headroom.exe");
        const QString external = m_dir.filePath(QStringLiteral("custom entry.exe"));
#elif defined(Q_OS_MACOS)
        const QString appName = QStringLiteral("Headroom.app/Contents/MacOS/headroom");
        const QString internalName = QStringLiteral("headroom-launcher");
        const QString external = m_dir.filePath(QStringLiteral("custom entry"));
#else
        const QString appName = QStringLiteral("headroom");
        const QString internalName = QStringLiteral("headroom-launcher");
        const QString external = m_dir.filePath(QStringLiteral("custom entry"));
#endif
        const QString application = QDir(root).filePath(QStringLiteral("versions/0.1.0/")
#ifdef Q_OS_MACOS
            + appName);
#else
            + QStringLiteral("bin/") + appName);
#endif
        const QString internal = QDir(root).filePath(internalName);
        QVERIFY(QDir().mkpath(QFileInfo(application).absolutePath()));
        auto write = [](const QString &path, const QByteArray &data) { QFile file(path); return file.open(QIODevice::WriteOnly) && file.write(data) == data.size(); };
        QVERIFY(write(application, "application")); QVERIFY(write(internal, "stable launcher")); QVERIFY(write(external, "stable launcher"));
        QVERIFY(write(external + QStringLiteral(".root"), QDir::toNativeSeparators(root).toUtf8() + '\n'));
        qputenv("HEADROOM_UPDATE_FIXTURE_INTERNAL_LAUNCHER", QDir::toNativeSeparators(internal).toUtf8());
        auto value = options(); value.installRoot = QDir::toNativeSeparators(root); value.launcherPath = QDir::toNativeSeparators(external);
        value.applicationPath = QDir::toNativeSeparators(application); value.fixtureIdentity = false;
        UpdateService service(true, value);
        QTRY_COMPARE(service.state(), QStringLiteral("current")); QCOMPARE(service.updateMethod(), QStringLiteral("automatic"));
    }
    void damagedOfficialLauncherRequestsExternalInstallerRepair() {
        const QString rootAlias = m_dir.filePath(QStringLiteral("damaged official identity"));
        QVERIFY(QDir().mkpath(rootAlias));
        const QString root = QFileInfo(rootAlias).canonicalFilePath();
#ifdef Q_OS_WIN
        const QString appName = QStringLiteral("headroom.exe");
        const QString external = m_dir.filePath(QStringLiteral("damaged entry.exe"));
#elif defined(Q_OS_MACOS)
        const QString appName = QStringLiteral("Headroom.app/Contents/MacOS/headroom");
        const QString external = m_dir.filePath(QStringLiteral("damaged entry"));
#else
        const QString appName = QStringLiteral("headroom");
        const QString external = m_dir.filePath(QStringLiteral("damaged entry"));
#endif
        const QString application = QDir(root).filePath(QStringLiteral("versions/0.1.0/")
#ifdef Q_OS_MACOS
            + appName);
#else
            + QStringLiteral("bin/") + appName);
#endif
        QVERIFY(QDir().mkpath(QFileInfo(application).absolutePath()));
        QFile app(application); QVERIFY(app.open(QIODevice::WriteOnly)); QVERIFY(app.write("application") > 0); app.close();
        auto value = options(); value.installRoot = QDir::toNativeSeparators(root); value.launcherPath = QDir::toNativeSeparators(external);
        value.applicationPath = QDir::toNativeSeparators(application); value.fixtureIdentity = false;
        UpdateService service(true, value);
        QTRY_COMPARE(service.state(), QStringLiteral("unavailable"));
        QVERIFY(service.statusText().contains(QStringLiteral("official installer")));
        QVERIFY(!service.canRepair());
    }
    void manualCheckAndStageUseExplicitStatesAndNoBearerEnvironment() {
        qputenv("USAGE_AUTH_TOKEN", "must-not-reach-public-updater");
        UpdateService service(true, options());
        QTRY_COMPARE(service.state(), QStringLiteral("current"));
        QVERIFY(service.canCheck()); service.checkForUpdates();
        QTRY_COMPARE(service.state(), QStringLiteral("available"));
        QCOMPARE(service.latestVersion(), QStringLiteral("9.1.0")); QVERIFY(service.canStage());
        service.stageUpdate(); QTRY_COMPARE(service.state(), QStringLiteral("staged"));
        QVERIFY(service.restartAvailable()); QVERIFY(service.statusText().contains(QStringLiteral("Restart")));
        const auto calls = record(); QVERIFY(calls.contains("inspect token=absent")); QVERIFY(calls.contains("check-update token=absent")); QVERIFY(calls.contains("stage-update token=absent"));
    }
    void automaticCheckStagesOnce() {
        UpdateService service(true, options());
        service.startAutomaticCheck();
        QTRY_COMPARE(service.state(), QStringLiteral("staged"));
        QCOMPARE(record().count("check-update"), 1); QCOMPARE(record().count("stage-update"), 1);
    }
    void previewBeforeStartupTimerConsumesAutomaticCheckWithoutTraffic() {
        UpdateService service(true, options());
        QTRY_COMPARE(service.state(), QStringLiteral("current"));
        service.setPublicTrafficAllowed(false);
        QVERIFY(!service.canCheck()); QVERIFY(!service.canRepair()); QVERIFY(!service.restartAvailable());
        service.startAutomaticCheck(); QTest::qWait(100);
        QVERIFY(!record().contains("check-update")); QVERIFY(!record().contains("stage-update"));
        service.setPublicTrafficAllowed(true); QTRY_VERIFY(service.canCheck()); QTest::qWait(100);
        QVERIFY(!record().contains("check-update"));
        service.startAutomaticCheck(); QTest::qWait(100); QVERIFY(!record().contains("check-update"));
        service.checkForUpdates(); QTRY_COMPARE(service.state(), QStringLiteral("available"));
        service.setPublicTrafficAllowed(false); QVERIFY(!service.canStage()); QCOMPARE(service.state(), QStringLiteral("unavailable"));
        service.setPublicTrafficAllowed(true); QCOMPARE(service.state(), QStringLiteral("available")); QVERIFY(service.canStage());
    }
    void rapidPreviewOffOnWaitsForCancelledOperationThenRestoresManualOnly() {
        UpdateService service(true, options()); QTRY_COMPARE(service.state(), QStringLiteral("current"));
        qputenv("HEADROOM_UPDATE_FIXTURE_MODE", "hang"); service.checkForUpdates(); QTRY_VERIFY(service.busy());
        service.setPublicTrafficAllowed(false); service.setPublicTrafficAllowed(true);
        QTRY_COMPARE(service.state(), QStringLiteral("current")); QVERIFY(service.canCheck());
        QCOMPARE(record().count("check-update"), 1); QCOMPARE(record().count("stage-update"), 0);
    }
    void matchingRepairStagesCurrentVersion() {
        qputenv("HEADROOM_UPDATE_FIXTURE_MISSING", "1");
        UpdateService service(true, options());
        QTRY_VERIFY(service.canRepair()); QVERIFY(service.statusText().contains(QStringLiteral("needs repair")));
        service.repairInstallation(); QTRY_COMPARE(service.state(), QStringLiteral("staged"));
        QCOMPARE(service.latestVersion(), QStringLiteral("0.1.0")); QVERIFY(record().contains("stage-repair"));
    }
    void bootstrapDamageIsRepairableForVerifiedIdentity() {
        qputenv("HEADROOM_UPDATE_FIXTURE_MISSING", "bootstrap/headroom-package");
        UpdateService service(true, options());
        QTRY_VERIFY(service.canRepair()); QVERIFY(service.statusText().contains(QStringLiteral("installation needs repair")));
    }
    void cancellationNeverAdvertisesRestart() {
        UpdateService service(true, options()); QTRY_COMPARE(service.state(), QStringLiteral("current"));
        qputenv("HEADROOM_UPDATE_FIXTURE_MODE", "hang");
        service.checkForUpdates(); QTRY_VERIFY(service.busy()); service.cancel();
        QTRY_COMPARE(service.state(), QStringLiteral("failed")); QVERIFY(!service.restartAvailable());
    }
    void malformedToolOutputFailsClosed() {
        UpdateService service(true, options()); QTRY_COMPARE(service.state(), QStringLiteral("current"));
        qputenv("HEADROOM_UPDATE_FIXTURE_MODE", "malformed"); service.checkForUpdates();
        QTRY_COMPARE(service.state(), QStringLiteral("failed")); QVERIFY(!service.restartAvailable());
    }
    void timeoutAndOversizedStderrFailClosed() {
        auto shortOptions = options(); shortOptions.timeoutMs = 500;
        UpdateService timed(true, shortOptions); QTRY_COMPARE(timed.state(), QStringLiteral("current"));
        qputenv("HEADROOM_UPDATE_FIXTURE_MODE", "hang"); timed.checkForUpdates();
        QTRY_COMPARE(timed.state(), QStringLiteral("failed")); QVERIFY(timed.statusText().contains(QStringLiteral("timed out")));
        qunsetenv("HEADROOM_UPDATE_FIXTURE_MODE");
        UpdateService noisy(true, options()); QTRY_COMPARE(noisy.state(), QStringLiteral("current"));
        qputenv("HEADROOM_UPDATE_FIXTURE_MODE", "stderr"); noisy.checkForUpdates();
        QTRY_COMPARE(noisy.state(), QStringLiteral("failed")); QVERIFY(!noisy.restartAvailable());
    }
    void inconsistentVerifiedStageIsNeverAdvertised() {
        UpdateService service(true, options()); QTRY_COMPARE(service.state(), QStringLiteral("current"));
        service.checkForUpdates(); QTRY_COMPARE(service.state(), QStringLiteral("available"));
        qputenv("HEADROOM_UPDATE_FIXTURE_BAD_STAGE", "1"); service.stageUpdate();
        QTRY_COMPARE(service.state(), QStringLiteral("failed")); QVERIFY(!service.restartAvailable()); QVERIFY(service.verifiedStage().isEmpty());
    }
    void restartRequiresValidatedCommitAndCannotBeCancelled() {
        UpdateService service(true, options()); QTRY_COMPARE(service.state(), QStringLiteral("current"));
        service.setRelaunchArguments({QStringLiteral("--background")});
        service.checkForUpdates(); QTRY_COMPARE(service.state(), QStringLiteral("available"));
        service.stageUpdate(); QTRY_COMPARE(service.state(), QStringLiteral("staged"));
        QSignalSpy prepared(&service, &UpdateService::applyPrepared);
        service.restartToApply();
		QVERIFY(service.busy()); QVERIFY(!service.canCancel());
        service.cancel(); service.setPublicTrafficAllowed(false);
		QTRY_VERIFY2(prepared.count() == 1, qPrintable(service.statusText()));
        QCOMPARE(service.state(), QStringLiteral("applying"));
        const QString commit = m_dir.filePath(QStringLiteral("transactions/apply-0123456789abcdef0123456789abcdef/commit.json"));
        QFile commitFile(commit); QVERIFY(commitFile.open(QIODevice::ReadOnly));
        const auto committed = QJsonDocument::fromJson(commitFile.readAll()).object();
        QVERIFY(committed.value(QStringLiteral("commit")).toBool());
        QCOMPARE(committed.value(QStringLiteral("nonce")).toString(), QString(48, QLatin1Char('a')));
        QVERIFY(record().contains("--relaunch-arg|--background"));
    }
    void mismatchedPreparedApplyDoesNotCommitOrClose() {
        UpdateService service(true, options()); QTRY_COMPARE(service.state(), QStringLiteral("current"));
        service.checkForUpdates(); QTRY_COMPARE(service.state(), QStringLiteral("available"));
        service.stageUpdate(); QTRY_COMPARE(service.state(), QStringLiteral("staged"));
        QSignalSpy prepared(&service, &UpdateService::applyPrepared);
        qputenv("HEADROOM_UPDATE_FIXTURE_MODE", "mismatched-prepare");
        service.restartToApply();
        QTRY_COMPARE(service.state(), QStringLiteral("failed"));
        QCOMPARE(prepared.count(), 0);
        const QString commit = m_dir.filePath(QStringLiteral("transactions/apply-0123456789abcdef0123456789abcdef/commit.json"));
        QVERIFY(!QFileInfo::exists(commit));
    }
    void rollbackOutcomeSuppressesAutomaticRestageUntilManualCheck() {
        qputenv("HEADROOM_UPDATE_FIXTURE_APPLY_STATUS", "rolled_back");
        UpdateService service(true, options()); service.startAutomaticCheck(); QTRY_COMPARE(service.state(), QStringLiteral("failed"));
        QVERIFY(service.statusText().contains(QStringLiteral("restored")));
        QTest::qWait(100);
        QCOMPARE(record().count("check-update"), 0);
        service.checkForUpdates(); QTRY_COMPARE(service.state(), QStringLiteral("available"));
        QCOMPARE(record().count("check-update"), 1);
    }
    void recoveryRequiredSuppressesAutomaticRestage() {
        qputenv("HEADROOM_UPDATE_FIXTURE_APPLY_STATUS", "recovery_required");
        UpdateService service(true, options()); service.startAutomaticCheck();
        QTRY_COMPARE(service.state(), QStringLiteral("failed"));
        QVERIFY(service.statusText().contains(QStringLiteral("retry recovery")));
        QTest::qWait(100); QCOMPARE(record().count("check-update"), 0);
    }
    void failedApplyPreparationAppliesDeferredPreviewState() {
        auto value = options(); value.timeoutMs = 1000;
        for (const auto &mode : {QByteArray("malformed"), QByteArray("hang")}) {
            qunsetenv("HEADROOM_UPDATE_FIXTURE_MODE");
            UpdateService service(true, value); QTRY_COMPARE(service.state(), QStringLiteral("current"));
            service.checkForUpdates(); QTRY_COMPARE(service.state(), QStringLiteral("available"));
            service.stageUpdate(); QTRY_COMPARE(service.state(), QStringLiteral("staged"));
            qputenv("HEADROOM_UPDATE_FIXTURE_MODE", mode);
            service.restartToApply(); QVERIFY(service.busy()); QVERIFY(!service.canCancel());
            service.setPublicTrafficAllowed(false);
            QTRY_COMPARE(service.state(), QStringLiteral("unavailable"));
            QVERIFY(!service.canCheck()); QVERIFY(service.statusText().contains(QStringLiteral("paused")));
        }
    }
};

QTEST_GUILESS_MAIN(UpdateServiceTest)
#include "test_update.moc"
