#include "startup.h"

#include <QDir>
#include <QCoreApplication>
#include <QFile>
#include <QProcess>
#include <QProcessEnvironment>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QSettings>
#include <QTemporaryDir>
#include <QUuid>
#include <QtTest>

namespace {
bool writeFile(const QString &path, const QByteArray &contents, bool executable = false)
{
    if (!QDir().mkpath(QFileInfo(path).absolutePath())) return false;
    QFile file(path);
    if (!file.open(QIODevice::WriteOnly) || file.write(contents) != contents.size()) return false;
    file.close();
    return !executable || file.setPermissions(QFile::ReadOwner | QFile::WriteOwner | QFile::ExeOwner);
}
QByteArray readFile(const QString &path)
{
    QFile file(path);
    return file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray{};
}
QString currentExecutable()
{
    return QCoreApplication::applicationFilePath();
}
}

class StartupTest : public QObject {
    Q_OBJECT
private slots:
    void packagedLauncherPathIsUsedOnlyForMatchingInstall()
    {
        QTemporaryDir dir;
        QVERIFY(dir.isValid());
        const QString launcher = dir.filePath("external/headroom");
        QVERIFY(writeFile(launcher, "fixture", true));
        QVERIFY(writeFile(launcher + QStringLiteral(".root"), QDir::toNativeSeparators(dir.path()).toUtf8() + '\n'));
        qputenv("HEADROOM_INSTALL_ROOT", dir.path().toUtf8());
        qputenv("HEADROOM_LAUNCHER_PATH", launcher.toUtf8());
        qputenv("HEADROOM_PACKAGE_VERSION", "9.8.7");
        QCOMPARE(StartupService::defaultExecutablePath(), QCoreApplication::applicationFilePath());
        const QString packaged = dir.filePath(QStringLiteral("versions/9.8.7/bin/%1")
#ifdef Q_OS_WIN
            .arg("headroom.exe"));
#else
            .arg("headroom"));
#endif
        QVERIFY(writeFile(packaged, "fixture", true));
        QCOMPARE(StartupService::packagedExecutablePath(packaged), QFileInfo(launcher).absoluteFilePath());
        const QString generation = dir.filePath(QStringLiteral("versions/9.8.7.generation-0123456789abcdef-fedcba9876543210/bin/%1")
#ifdef Q_OS_WIN
            .arg("headroom.exe"));
#else
            .arg("headroom"));
#endif
        QVERIFY(writeFile(generation, "fixture", true));
        QCOMPARE(StartupService::packagedExecutablePath(generation), QFileInfo(launcher).absoluteFilePath());

        for (const QString &unsafe : {
                 dir.filePath(QStringLiteral("versions/9.8.6.generation-0123456789abcdef-fedcba9876543210/bin/") + QFileInfo(packaged).fileName()),
                 dir.filePath(QStringLiteral("versions/9.8.7.generation-0123456789ABCDEf-fedcba9876543210/bin/") + QFileInfo(packaged).fileName()),
                 dir.filePath(QStringLiteral("versions/9.8.7.generation-0123456789abcdef-short/bin/") + QFileInfo(packaged).fileName()),
                 dir.filePath(QStringLiteral("foreign/bin/") + QFileInfo(packaged).fileName())}) {
            QVERIFY(writeFile(unsafe, "fixture", true));
            QCOMPARE(StartupService::packagedExecutablePath(unsafe), QFileInfo(unsafe).absoluteFilePath());
        }
        QVERIFY(writeFile(launcher + QStringLiteral(".root"), QByteArrayLiteral("/wrong/root\n")));
        QCOMPARE(StartupService::packagedExecutablePath(generation), QFileInfo(generation).absoluteFilePath());
        qunsetenv("HEADROOM_INSTALL_ROOT"); qunsetenv("HEADROOM_LAUNCHER_PATH"); qunsetenv("HEADROOM_PACKAGE_VERSION");
    }

    void repairsPriorGenerationStartupEntryToStableLauncher()
    {
        QTemporaryDir dir; QVERIFY(dir.isValid());
        const QString root = dir.filePath(QStringLiteral("install"));
        const QString launcher = dir.filePath(QStringLiteral("bin/headroom"));
#ifdef Q_OS_WIN
        const QString appName = QStringLiteral("headroom.exe");
#else
        const QString appName = QStringLiteral("headroom");
#endif
        const QString current = QDir(root).filePath(QStringLiteral("versions/2.0.0.generation-0123456789abcdef-fedcba9876543210/bin/") + appName);
        const QString previous = QDir(root).filePath(QStringLiteral("versions/1.8.0.generation-1111111111111111-2222222222222222/bin/") + appName);
        QVERIFY(writeFile(current, "current", true)); QVERIFY(writeFile(previous, "previous", true));
        QVERIFY(writeFile(launcher, "launcher", true));
        QVERIFY(writeFile(launcher + QStringLiteral(".root"), QDir::toNativeSeparators(root).toUtf8() + '\n'));
        qputenv("HEADROOM_INSTALL_ROOT", root.toUtf8()); qputenv("HEADROOM_LAUNCHER_PATH", launcher.toUtf8());
        qputenv("HEADROOM_PACKAGE_VERSION", "2.0.0");

#ifdef Q_OS_WIN
        const QString registryPath = QStringLiteral("HKEY_CURRENT_USER\\Software\\HeadroomTests\\")
            + QUuid::createUuid().toString(QUuid::WithoutBraces);
        QSettings registry(registryPath, QSettings::NativeFormat);
        const QString oldCommand = QStringLiteral("\"") + QDir::toNativeSeparators(previous) + QStringLiteral("\" --background");
        const QString stableCommand = QStringLiteral("\"") + QDir::toNativeSeparators(launcher) + QStringLiteral("\" --background");
        registry.setValue(QStringLiteral("Headroom"), oldCommand); registry.sync();
        StartupService repaired({}, current, true, nullptr, StartupService::Platform::Windows, registryPath);
        QCOMPARE(registry.value(QStringLiteral("Headroom")).toString(), stableCommand);
        registry.setValue(QStringLiteral("Headroom"), oldCommand + QStringLiteral(" --custom")); registry.sync();
        StartupService custom({}, current, true, nullptr, StartupService::Platform::Windows, registryPath);
        QCOMPARE(registry.value(QStringLiteral("Headroom")).toString(), oldCommand + QStringLiteral(" --custom"));
        registry.setValue(QStringLiteral("Headroom"), oldCommand); registry.sync();
        StartupService preview({}, current, false, nullptr, StartupService::Platform::Windows, registryPath);
        QCOMPARE(registry.value(QStringLiteral("Headroom")).toString(), oldCommand);
        registry.remove(QStringLiteral("Headroom"));
        registry.setValue(QStringLiteral("ClaudeUsageWidget"), QStringLiteral("legacy")); registry.sync();
        StartupService migration({}, current, true, nullptr, StartupService::Platform::Windows, registryPath);
        migration.setPreferenceWriter([](bool) { return QString(); });
        QVERIFY(migration.migrateLegacyRegistration(true));
        QCOMPARE(registry.value(QStringLiteral("Headroom")).toString(), stableCommand);
        QVERIFY(!registry.contains(QStringLiteral("ClaudeUsageWidget")));
        registry.clear(); registry.sync();
#else
        const QString config = dir.filePath(QStringLiteral("config"));
        const QString entry = QDir(config).filePath(QStringLiteral("autostart/headroom.desktop"));
        const QByteArray oldEntry = QStringLiteral("[Desktop Entry]\nType=Application\nExec=\"%1\" --background\nX-GNOME-Autostart-enabled=true\n")
            .arg(previous).toUtf8();
        QVERIFY(writeFile(entry, oldEntry));
        StartupService repaired(config, current, true);
        QVERIFY(repaired.enabled());
        const QByteArray repairedEntry = readFile(entry);
        QVERIFY(repairedEntry.contains((QStringLiteral("Exec=\"") + launcher + QStringLiteral("\" --background")).toUtf8()));
        QVERIFY(!repairedEntry.contains(previous.toUtf8()));

        QVERIFY(writeFile(entry, QByteArray(oldEntry).replace("X-GNOME-Autostart-enabled=true", "X-GNOME-Autostart-enabled=false")));
        StartupService disabled(config, current, true);
        QVERIFY(!disabled.enabled()); QCOMPARE(readFile(entry), QByteArray(oldEntry).replace("X-GNOME-Autostart-enabled=true", "X-GNOME-Autostart-enabled=false"));

        const QByteArray hiddenEntry = oldEntry + QByteArrayLiteral("Hidden = true\n");
        QVERIFY(writeFile(entry, hiddenEntry));
        StartupService hidden(config, current, true);
        QVERIFY(!hidden.enabled()); QCOMPARE(readFile(entry), hiddenEntry);

        const QByteArray duplicateEntry = oldEntry + QByteArrayLiteral("Type=Application\n");
        QVERIFY(writeFile(entry, duplicateEntry));
        StartupService duplicate(config, current, true);
        QCOMPARE(readFile(entry), duplicateEntry);

        const QByteArray custom = QStringLiteral("[Desktop Entry]\nType=Application\nExec=\"%1\" --background --custom\n").arg(previous).toUtf8();
        QVERIFY(writeFile(entry, custom));
        StartupService unrelated(config, current, true);
        QCOMPARE(readFile(entry), custom);

        QVERIFY(writeFile(entry, oldEntry));
        StartupService preview(config, current, false);
        QCOMPARE(readFile(entry), oldEntry);
#endif
        qunsetenv("HEADROOM_INSTALL_ROOT"); qunsetenv("HEADROOM_LAUNCHER_PATH"); qunsetenv("HEADROOM_PACKAGE_VERSION");
    }
    void preferenceWriterTracksBothToggles()
    {
        QTemporaryDir dir;
        QList<bool> writes;
        StartupService service(dir.filePath("config"), currentExecutable(), true, nullptr,
            StartupService::Platform::Linux);
        service.setPreferenceWriter([&](bool enabled) { writes.append(enabled); return QString(); });
        QVERIFY(service.setEnabled(true));
        QVERIFY(service.setEnabled(false));
        QCOMPARE(writes, QList<bool>({true, false}));
        QVERIFY(!QFileInfo::exists(service.entryPath()));

        service.setPreferenceWriter([](bool) { return QString("Settings could not be saved."); });
        QVERIFY(!service.setEnabled(true));
        QVERIFY(!QFileInfo::exists(service.entryPath()));
        QVERIFY(!service.error().isEmpty());
    }
#ifdef Q_OS_WIN
    void windowsRegistrationAndLegacyMigration()
    {
        const QString registryPath = "HKEY_CURRENT_USER\\Software\\HeadroomTests\\" +
                                     QUuid::createUuid().toString(QUuid::WithoutBraces);
        QSettings registry(registryPath, QSettings::NativeFormat);
        registry.setValue("ClaudeUsageWidget", "\"C:\\Legacy App\\ClaudeUsageWidget.exe\"");
        registry.sync();
        bool preference = false;
        StartupService service({}, currentExecutable(), true, nullptr,
            StartupService::Platform::Windows, registryPath);
        service.setPreferenceWriter([&](bool enabled) { preference = enabled; return QString(); });
        QVERIFY(service.migrateLegacyRegistration(true));
        QCOMPARE(preference, true);
        const QString expected = "\"" + QDir::toNativeSeparators(currentExecutable()) + "\" --background";
        QCOMPARE(registry.value("Headroom").toString(), expected);
        QVERIFY(!registry.contains("ClaudeUsageWidget"));
        QVERIFY(service.enabled());
        QVERIFY(service.setEnabled(false));
        QCOMPARE(preference, false);
        QVERIFY(!registry.contains("Headroom"));
        registry.clear(); registry.sync();
    }

    void windowsPersistenceFailureRollsBackAndRemainsRetryable()
    {
        const QString registryPath = "HKEY_CURRENT_USER\\Software\\HeadroomTests\\" +
                                     QUuid::createUuid().toString(QUuid::WithoutBraces);
        QSettings registry(registryPath, QSettings::NativeFormat);
        registry.setValue("ClaudeUsageWidget", "legacy"); registry.sync();
        StartupService service({}, currentExecutable(), true, nullptr,
            StartupService::Platform::Windows, registryPath);
        service.setPreferenceWriter([](bool) { return QString("Could not persist the startup preference."); });
        QVERIFY(!service.migrateLegacyRegistration(true));
        QVERIFY(!registry.contains("Headroom"));
        QVERIFY(registry.contains("ClaudeUsageWidget"));
        QVERIFY(!service.error().isEmpty());
        service.setPreferenceWriter([](bool) { return QString(); });
        QVERIFY(service.migrateLegacyRegistration(true));
        QVERIFY(registry.contains("Headroom"));
        QVERIFY(!registry.contains("ClaudeUsageWidget"));
        registry.clear(); registry.sync();
    }
#endif
    void optInPersistenceAndRemoval()
    {
#ifdef Q_OS_WIN
        QSKIP("XDG autostart entries are Linux-specific");
#endif
        QTemporaryDir dir;
        QVERIFY(dir.isValid());
        const QString executable = dir.filePath("bin/headroom");
        QVERIFY(writeFile(executable, "#!/bin/sh\nexit 0\n", true));
        StartupService service(dir.filePath("config"), executable);
        QVERIFY(!service.enabled());
        QVERIFY(!QFileInfo::exists(dir.filePath("config")));
        QSignalSpy changed(&service, &StartupService::enabledChanged);
        QVERIFY(service.setEnabled(true));
        QVERIFY(service.enabled());
        QCOMPARE(changed.size(), 1);
        const QByteArray original = readFile(service.entryPath());
        QVERIFY(original.contains(" --background\n"));
        StartupService reloaded(dir.filePath("config"), executable);
        QVERIFY(reloaded.enabled());
        QVERIFY(service.setEnabled(true));
        QCOMPARE(readFile(service.entryPath()), original);
        QCOMPARE(changed.size(), 1);
        const QString other = dir.filePath("config/autostart/another.desktop");
        QVERIFY(writeFile(other, "keep me"));
        QVERIFY(service.setEnabled(false));
        QVERIFY(!service.enabled());
        QVERIFY(!QFileInfo::exists(service.entryPath()));
        QCOMPARE(readFile(other), QByteArray("keep me"));
        QCOMPARE(changed.size(), 2);
        QVERIFY(service.setEnabled(false));
    }

    void previewNeverMutates()
    {
        QTemporaryDir dir;
#ifdef Q_OS_WIN
        const QString registryPath = "HKEY_CURRENT_USER\\Software\\HeadroomTests\\" +
                                     QUuid::createUuid().toString(QUuid::WithoutBraces);
        QSettings registry(registryPath, QSettings::NativeFormat);
        StartupService service(dir.filePath("config"), currentExecutable(), false, nullptr,
            StartupService::Platform::Windows, registryPath);
#else
        StartupService service(dir.filePath("config"), currentExecutable(), false);
#endif
        QVERIFY(!service.available());
        QVERIFY(!service.setEnabled(true));
        QVERIFY(!service.error().isEmpty());
#ifdef Q_OS_WIN
        QVERIFY(!registry.contains("Headroom"));
#else
        QVERIFY(!QFileInfo::exists(dir.filePath("config")));
#endif
        service.setAllowChanges(true);
#ifdef Q_OS_WIN
        QVERIFY(service.available());
        QVERIFY(service.error().isEmpty());
        QVERIFY(service.setEnabled(true));
        const QString registered = registry.value("Headroom").toString();
        QVERIFY(!registered.isEmpty());
        service.setAllowChanges(false);
        QVERIFY(!service.setEnabled(false));
        QCOMPARE(registry.value("Headroom").toString(), registered);
        QVERIFY(service.enabled());
        registry.clear(); registry.sync();
        return;
#else
        QVERIFY(service.available());
        QVERIFY(service.error().isEmpty());
        QVERIFY(service.setEnabled(true));
        const QByteArray original = readFile(service.entryPath());
        service.setAllowChanges(false);
        QVERIFY(!service.setEnabled(false));
        QCOMPARE(readFile(service.entryPath()), original);
        QVERIFY(service.enabled());
#endif
    }

    void disabledEntryAndWriteFailure()
    {
#ifdef Q_OS_WIN
        QSKIP("XDG autostart entries are Linux-specific");
#endif
        QTemporaryDir dir;
        StartupService service(dir.path(), currentExecutable());
        QVERIFY(writeFile(service.entryPath(), "[Desktop Entry]\nType=Application\nExec=headroom\nHidden=true\n"));
        service.refresh();
        QVERIFY(!service.enabled());
        QVERIFY(service.setEnabled(true));
        QVERIFY(service.enabled());
        QVERIFY(service.setEnabled(false));
        QVERIFY(QDir().mkdir(service.entryPath()));
        QVERIFY(!service.setEnabled(true));
        QVERIFY(!service.enabled());
        QVERIFY(!service.error().isEmpty());
        QVERIFY(QFileInfo(service.entryPath()).isDir());
    }

    void invalidExecutablePreservesExistingEntry()
    {
#ifdef Q_OS_WIN
        QSKIP("Desktop Entry executable validation is Linux-specific");
#endif
        QTemporaryDir dir;
        for (const QString path : {QString("relative/headroom"), dir.filePath("missing/headroom"),
                                   dir.filePath("a=b"), dir.filePath("percent%F"), dir.filePath("headroom\nHidden=true")}) {
            StartupService service(dir.path(), path);
            const QByteArray entry = "[Desktop Entry]\nType=Application\nExec=headroom\n";
            QVERIFY(writeFile(service.entryPath(), entry));
            QVERIFY(!service.setEnabled(true));
            QCOMPARE(readFile(service.entryPath()), entry);
            QVERIFY(service.enabled());
        }
    }

    void respectsXdgConfigHome()
    {
#ifdef Q_OS_WIN
        QSKIP("XDG_CONFIG_HOME is Linux-specific");
#endif
        QTemporaryDir dir;
        const bool existed = qEnvironmentVariableIsSet("XDG_CONFIG_HOME");
        const QByteArray previous = qgetenv("XDG_CONFIG_HOME");
        qputenv("XDG_CONFIG_HOME", dir.path().toUtf8());
        StartupService service({}, currentExecutable());
        qputenv("XDG_CONFIG_HOME", "relative/ignored");
        StartupService relative({}, currentExecutable());
        if (existed) qputenv("XDG_CONFIG_HOME", previous); else qunsetenv("XDG_CONFIG_HOME");
        QCOMPARE(service.entryPath(), dir.filePath("autostart/headroom.desktop"));
        QCOMPARE(relative.entryPath(), QDir::homePath() + "/.config/autostart/headroom.desktop");
        QVERIFY(!QFileInfo::exists(service.entryPath()));
    }

    void desktopLauncherPreservesSpecialCharacters()
    {
#ifdef Q_OS_WIN
        QSKIP("GIO desktop entry launching is Linux-specific");
#endif
        const QString gio = QStandardPaths::findExecutable("gio");
        if (gio.isEmpty()) QSKIP("GIO is unavailable for desktop entry launch verification");
        QTemporaryDir dir;
        const QString executable = dir.filePath("Head room '$`\\;special\"/headroom");
        QVERIFY(writeFile(executable, "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" > \"$HEADROOM_STARTUP_TEST_CAPTURE\"\n", true));
        StartupService service(dir.filePath("config"), executable);
        QVERIFY(service.setEnabled(true));
        const QString validator = QStandardPaths::findExecutable("desktop-file-validate");
        if (!validator.isEmpty()) QCOMPARE(QProcess::execute(validator, {service.entryPath()}), 0);
        QProcess launch;
        auto environment = QProcessEnvironment::systemEnvironment();
        const QString capture = dir.filePath("arguments.txt");
        environment.insert("HEADROOM_STARTUP_TEST_CAPTURE", capture);
        launch.setProcessEnvironment(environment);
        launch.start(gio, {"launch", service.entryPath()});
        QVERIFY(launch.waitForFinished());
        QVERIFY2(launch.exitCode() == 0, launch.readAllStandardError().constData());
        QTRY_COMPARE(readFile(capture), executable.toUtf8() + "\n--background\n");
    }
};

QTEST_GUILESS_MAIN(StartupTest)
#include "test_startup.moc"
