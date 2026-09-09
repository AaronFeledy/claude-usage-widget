#include "startup.h"

#include <QDir>
#include <QFile>
#include <QProcess>
#include <QProcessEnvironment>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTemporaryDir>
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
}

class StartupTest : public QObject {
    Q_OBJECT
private slots:
    void optInPersistenceAndRemoval()
    {
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
        StartupService service(dir.filePath("config"), "/bin/true", false);
        QVERIFY(!service.available());
        QVERIFY(!service.setEnabled(true));
        QVERIFY(!service.error().isEmpty());
        QVERIFY(!QFileInfo::exists(dir.filePath("config")));
        service.setAllowChanges(true);
        QVERIFY(service.available());
        QVERIFY(service.error().isEmpty());
        QVERIFY(service.setEnabled(true));
        const QByteArray original = readFile(service.entryPath());
        service.setAllowChanges(false);
        QVERIFY(!service.setEnabled(false));
        QCOMPARE(readFile(service.entryPath()), original);
        QVERIFY(service.enabled());
    }

    void disabledEntryAndWriteFailure()
    {
        QTemporaryDir dir;
        StartupService service(dir.path(), "/bin/true");
        QVERIFY(writeFile(service.entryPath(), "[Desktop Entry]\nType=Application\nExec=/bin/true\nHidden=true\n"));
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
        QTemporaryDir dir;
        for (const QString path : {QString("relative/headroom"), QString("/missing/headroom"),
                                   QString("/tmp/a=b"), QString("/tmp/percent%F"), QString("/tmp/headroom\nHidden=true")}) {
            StartupService service(dir.path(), path);
            const QByteArray entry = "[Desktop Entry]\nType=Application\nExec=/bin/true\n";
            QVERIFY(writeFile(service.entryPath(), entry));
            QVERIFY(!service.setEnabled(true));
            QCOMPARE(readFile(service.entryPath()), entry);
            QVERIFY(service.enabled());
        }
    }

    void respectsXdgConfigHome()
    {
        QTemporaryDir dir;
        const bool existed = qEnvironmentVariableIsSet("XDG_CONFIG_HOME");
        const QByteArray previous = qgetenv("XDG_CONFIG_HOME");
        qputenv("XDG_CONFIG_HOME", dir.path().toUtf8());
        StartupService service({}, "/bin/true");
        qputenv("XDG_CONFIG_HOME", "relative/ignored");
        StartupService relative({}, "/bin/true");
        if (existed) qputenv("XDG_CONFIG_HOME", previous); else qunsetenv("XDG_CONFIG_HOME");
        QCOMPARE(service.entryPath(), dir.filePath("autostart/headroom.desktop"));
        QCOMPARE(relative.entryPath(), QDir::homePath() + "/.config/autostart/headroom.desktop");
        QVERIFY(!QFileInfo::exists(service.entryPath()));
    }

    void desktopLauncherPreservesSpecialCharacters()
    {
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
