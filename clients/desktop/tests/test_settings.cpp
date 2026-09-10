#include "settings.h"

#include <QDir>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QtTest>

namespace {
bool writeFile(const QString &path, const QByteArray &data)
{
    QDir().mkpath(QFileInfo(path).absolutePath());
    QFile file(path);
    return file.open(QIODevice::WriteOnly) && file.write(data) == data.size();
}
QByteArray readFile(const QString &path)
{
    QFile file(path);
    return file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray{};
}
QJsonObject readObject(const QString &path)
{
    return QJsonDocument::fromJson(readFile(path)).object();
}
}

class SettingsTest : public QObject {
    Q_OBJECT
private slots:
    void defaultsToLocalAndPreservesOverrides_data()
    {
        QTest::addColumn<int>("platform");
        QTest::newRow("linux") << int(SettingsService::Platform::Linux);
        QTest::newRow("windows") << int(SettingsService::Platform::Windows);
    }
    void defaultsToLocalAndPreservesOverrides()
    {
        QFETCH(int, platform);
        const auto targetPlatform = SettingsService::Platform(platform);
        QTemporaryDir dir; QVERIFY(dir.isValid());
        const QString path = dir.filePath("settings.json");
        SettingsService fresh(path, true, targetPlatform);
        QCOMPARE(fresh.value().connectionMode, QString("local"));
        QVERIFY(fresh.value().url.isEmpty());
        QVERIFY(fresh.value().token.isEmpty());
        QVERIFY(!QFileInfo::exists(path));

        // A pre-mode settings file with a saved URL is already an override.
        QVERIFY(writeFile(path, QByteArrayLiteral("{\"url\":\"https://usage.example.test/base\",\"token\":\"fixture-token\"}")));
        SettingsService existing(path, true, targetPlatform);
        QCOMPARE(existing.value().connectionMode, QString("remote"));
        QCOMPARE(existing.value().url, QString("https://usage.example.test/base"));
        QCOMPARE(existing.value().token, QString("fixture-token"));
        SettingsService reopened(path, true, targetPlatform);
        QCOMPARE(reopened.value().connectionMode, QString("remote"));
        QCOMPARE(reopened.value().url, existing.value().url);
        QCOMPARE(reopened.value().token, existing.value().token);
    }

    void importsLegacySchemas_data()
    {
        QTest::addColumn<int>("schema");
        for (int schema = 0; schema <= 3; ++schema) QTest::newRow(qPrintable(QString::number(schema))) << schema;
    }
    void importsLegacySchemas()
    {
        QFETCH(int, schema);
        QTemporaryDir dir; QVERIFY(dir.isValid());
        const QString target = dir.filePath("Headroom/Headroom/settings.json");
        const QString legacy = dir.filePath("ClaudeUsageWidget/settings.json");
        const QByteArray original = QString(
            "{\"SchemaVersion\":%1,\"ApiUrl\":\"https://example.test/base\",\"ApiToken\":\"fixture-token\",\"RefreshIntervalSeconds\":120,\"StartWithWindows\":true,\"NotificationsEnabled\":false,\"DebugMode\":true,\"PrimaryProvider\":\"Cursor\",\"ProviderOrder\":[\"Codex\",\"Cursor\",\"Claude\"],\"Unknown\":{\"nested\":1}}")
            .arg(schema).toUtf8();
        QVERIFY(writeFile(legacy, original));
        SettingsService service({}, true, SettingsService::Platform::Windows, legacy, target);
        QVERIFY2(service.loadError().isEmpty(), qPrintable(service.loadError()));
        QVERIFY(service.importedLegacy());
        QCOMPARE(service.value().connectionMode, QString("remote"));
        QCOMPARE(service.value().url, QString("https://example.test/base"));
        QCOMPARE(service.value().token, QString("fixture-token"));
        QCOMPARE(service.value().interval, 120);
        QCOMPARE(service.value().notifications, false);
        QCOMPARE(service.value().order, QStringList({"Codex", "Cursor", "Claude", "Grok"}));
        QCOMPARE(service.value().startup, true);
        QCOMPARE(service.value().startupMigrationPending, true);
        QCOMPARE(readFile(legacy), original);
        QCOMPARE(readFile(service.legacyBackupPath()), original);
#ifndef Q_OS_WIN
        QVERIFY(!(QFile::permissions(service.legacyBackupPath()) &
                  (QFileDevice::ReadGroup | QFileDevice::WriteGroup | QFileDevice::ReadOther | QFileDevice::WriteOther)));
#endif
        QVERIFY(!readFile(target).contains("DebugMode"));
    }

    void localLegacyAndLinuxEmptyMode()
    {
        QTemporaryDir dir;
        const QString legacy = dir.filePath("old.json"), target = dir.filePath("new/settings.json");
        QVERIFY(writeFile(legacy, QByteArrayLiteral("{\"ApiUrl\":\"\",\"ApiToken\":\"keep\",\"PrimaryProvider\":\"Grok\"}")));
        SettingsService windows({}, true, SettingsService::Platform::Windows, legacy, target);
        QCOMPARE(windows.value().connectionMode, QString("local"));
        QCOMPARE(windows.value().token, QString("keep"));
        QCOMPARE(windows.value().order.first(), QString("Grok"));

        const QString linuxPath = dir.filePath("linux/settings.json");
        QVERIFY(writeFile(linuxPath, QByteArrayLiteral("{\"url\":\"\",\"token\":\"\",\"interval\":60}")));
        SettingsService linuxSettings(linuxPath, true, SettingsService::Platform::Linux);
        QCOMPARE(linuxSettings.value().connectionMode, QString("local"));
        QCOMPARE(readObject(linuxPath).value("connectionMode").toString(), QString("local"));
        QCOMPARE(readFile(linuxPath + ".bak"), QByteArrayLiteral("{\"url\":\"\",\"token\":\"\",\"interval\":60}"));
#ifndef Q_OS_WIN
        QVERIFY(!(QFile::permissions(linuxPath + ".bak") &
                  (QFileDevice::ReadGroup | QFileDevice::WriteGroup | QFileDevice::ReadOther | QFileDevice::WriteOther)));
#endif
    }

    void existingHeadroomWinsAndPreservesUnknown()
    {
        QTemporaryDir dir;
        const QString target = dir.filePath("new.json"), legacy = dir.filePath("old.json");
        QVERIFY(writeFile(target, QByteArrayLiteral("{\"connectionMode\":\"remote\",\"url\":\"https://new.test\",\"token\":\"new-token\",\"order\":[\"Claude\"],\"custom\":{\"x\":2}}")));
        QVERIFY(writeFile(legacy, QByteArrayLiteral("{\"ApiUrl\":\"https://old.test\",\"ApiToken\":\"old-token\"}")));
        SettingsService service({}, true, SettingsService::Platform::Windows, legacy, target);
        QCOMPARE(service.value().url, QString("https://new.test"));
        QVERIFY(!service.importedLegacy());
        auto changed = service.value(); changed.interval = 120;
        QVERIFY(service.save(changed, true).isEmpty());
        QCOMPARE(readObject(target).value("custom").toObject().value("x").toInt(), 2);
        QVERIFY(!QFileInfo::exists(service.legacyBackupPath()));
    }

    void explicitConfigIsIsolated()
    {
        QTemporaryDir dir;
        const QString explicitPath = dir.filePath("isolated/settings.json");
        const QString defaultPath = dir.filePath("default/settings.json");
        const QString legacy = dir.filePath("old.json");
        QVERIFY(writeFile(legacy, QByteArrayLiteral("{\"ApiUrl\":\"https://legacy.test\"}")));
        SettingsService service(explicitPath, true, SettingsService::Platform::Windows, legacy, defaultPath);
        QVERIFY(!service.importedLegacy());
        QVERIFY(!QFileInfo::exists(explicitPath));
        QVERIFY(!QFileInfo::exists(defaultPath));
        QVERIFY(!QFileInfo::exists(defaultPath + ".legacy.bak"));
    }

    void automaticMigrationCanBeDisabledForDemoAndScreenshot()
    {
        QTemporaryDir dir;
        const QString target = dir.filePath("default/settings.json"), legacy = dir.filePath("old.json");
        QVERIFY(writeFile(legacy, QByteArrayLiteral("{\"ApiUrl\":\"https://legacy.test\"}")));
        SettingsService absent({}, false, SettingsService::Platform::Windows, legacy, target);
        QVERIFY(!absent.importedLegacy());
        QVERIFY(!QFileInfo::exists(target));
        QVERIFY(!QFileInfo::exists(target + ".legacy.bak"));

        QVERIFY(writeFile(target, QByteArrayLiteral("{\"url\":\"\",\"token\":\"\"}")));
        const QByteArray original = readFile(target);
        SettingsService existing({}, false, SettingsService::Platform::Windows, legacy, target);
        QCOMPARE(existing.value().connectionMode, QString("local"));
        QCOMPARE(readFile(target), original);
        QVERIFY(!QFileInfo::exists(target + ".bak"));
    }

    void malformedAndInvalidFilesStayUntouched()
    {
        QTemporaryDir dir;
        for (const QByteArray original : {QByteArray("{broken"),
             QByteArray("{\"connectionMode\":\"remote\",\"url\":\"https://user:pass@example.test\",\"token\":\"x\"}"),
             QByteArray("{\"connectionMode\":\"remote\",\"url\":\"https://example.test\",\"token\":\"line\\nbreak\"}")}) {
            const QString path = dir.filePath(QString::number(qHash(original)) + ".json");
            QVERIFY(writeFile(path, original));
            SettingsService service(path, true, SettingsService::Platform::Linux);
            QVERIFY(!service.loadError().isEmpty());
            QCOMPARE(readFile(path), original);
            QVERIFY(!service.saveOrder({"Grok"}, "Grok").isEmpty());
            QCOMPARE(readFile(path), original);
            auto replacement = service.value(); replacement.connectionMode = "remote";
            replacement.url = "https://replacement.test"; replacement.token.clear();
            QVERIFY(service.save(replacement, true).isEmpty());
            QVERIFY(readFile(path) != original);
        }
    }

    void failedImportIsRetryableAndBackupIsNeverOverwritten()
    {
        QTemporaryDir dir;
        const QString target = dir.filePath("new/settings.json"), legacy = dir.filePath("old.json");
        const QByteArray malformed = "{bad"; QVERIFY(writeFile(legacy, malformed));
        SettingsService failed({}, true, SettingsService::Platform::Windows, legacy, target);
        QVERIFY(!failed.loadError().isEmpty());
        QVERIFY(!failed.saveOrder({"Grok"}, "Grok").isEmpty());
        QVERIFY(!QFileInfo::exists(target));
        QVERIFY(writeFile(legacy, QByteArrayLiteral("{\"ApiUrl\":\"\",\"ApiToken\":\"token\"}")));
        SettingsService imported({}, true, SettingsService::Platform::Windows, legacy, target);
        QVERIFY(imported.importedLegacy());
        const QByteArray backup = readFile(imported.legacyBackupPath());
        QVERIFY(writeFile(legacy, QByteArrayLiteral("{\"ApiUrl\":\"https://changed.test\"}")));
        QFile::remove(target);
        SettingsService retry({}, true, SettingsService::Platform::Windows, legacy, target);
        QVERIFY(retry.importedLegacy());
        QCOMPARE(readFile(retry.legacyBackupPath()), backup);
    }
};

QTEST_GUILESS_MAIN(SettingsTest)
#include "test_settings.moc"
