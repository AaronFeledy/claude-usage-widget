#include "instance.h"

#include <QDir>
#include <QFile>
#include <QLocalSocket>
#include <QProcess>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QTimer>
#include <QtTest>

#include <memory>

#ifndef Q_OS_WIN
#include <sys/stat.h>
#endif

class InstanceTest : public QObject {
    Q_OBJECT
private:
    QByteArray previousRuntime;
    std::unique_ptr<QTemporaryDir> runtime;

    QString helperPath() const
    {
        QString path = QCoreApplication::applicationDirPath() + "/headroom-instance-helper";
#ifdef Q_OS_WIN
        path += ".exe";
#endif
        return path;
    }
private slots:
    void initTestCase()
    {
        QCoreApplication::setOrganizationName("HeadroomTests");
        QCoreApplication::setApplicationName("InstanceFixture");
#ifndef Q_OS_WIN
        previousRuntime = qgetenv("XDG_RUNTIME_DIR");
#endif
    }
    void init()
    {
#ifndef Q_OS_WIN
        runtime = std::make_unique<QTemporaryDir>(QDir(QDir::tempPath()).filePath("hr-instance-XXXXXX"));
        QVERIFY(runtime->isValid());
        QVERIFY(QFile::setPermissions(runtime->path(), QFileDevice::ReadOwner |
                                                       QFileDevice::WriteOwner |
                                                       QFileDevice::ExeOwner));
        qputenv("XDG_RUNTIME_DIR", QFile::encodeName(runtime->path()));
#endif
    }
    void cleanup()
    {
#ifndef Q_OS_WIN
        if (previousRuntime.isNull()) qunsetenv("XDG_RUNTIME_DIR");
        else qputenv("XDG_RUNTIME_DIR", previousRuntime);
        runtime.reset();
#endif
    }
    void scopeStableAcrossSettingsCreation()
    {
        QTemporaryDir dir;
        const QString path = dir.filePath("config/settings.json");
        QDir().mkpath(QFileInfo(path).absolutePath());
        InstanceService before(path);
        QFile file(path); QVERIFY(file.open(QIODevice::WriteOnly)); file.write("{}"); file.close();
        InstanceService after(path);
        QCOMPARE(before.scopeName(), after.scopeName());
        InstanceService other(dir.filePath("other/settings.json"));
        QVERIFY(before.scopeName() != other.scopeName());
    }

    void secondLaunchActivatesPrimary()
    {
        QTemporaryDir dir;
        const QString path = dir.filePath("settings.json");
        const QString ready = dir.filePath("ready"), activated = dir.filePath("activated");
        QProcess primary;
        primary.start(helperPath(), {path, ready, activated});
        QVERIFY2(primary.waitForStarted(2000), qPrintable(primary.errorString()));
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(ready), 3000);
        InstanceService secondary(path);
        QVERIFY2(secondary.start() == InstanceService::Result::Secondary, qPrintable(secondary.error()));
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(activated), 3000);
        QCOMPARE(primary.state(), QProcess::Running);
        QFile marker(activated); QVERIFY(marker.open(QIODevice::ReadOnly));
        QCOMPARE(marker.readAll(), QByteArray("activated"));
        primary.kill();
        QVERIFY2(primary.waitForFinished(3000), qPrintable(primary.errorString()));
    }

    void fragmentedMessageActivatesOnce()
    {
        QTemporaryDir dir;
        InstanceService primary(dir.filePath("settings.json"));
        QVERIFY2(primary.start() == InstanceService::Result::Primary, qPrintable(primary.error()));
        QSignalSpy activated(&primary, &InstanceService::activationRequested);
        QLocalSocket socket; socket.connectToServer(primary.scopeName());
        QVERIFY(socket.waitForConnected(1000));
        QCOMPARE(socket.write("acti"), qint64(4));
        qint64 secondWrite = -1;
        QTimer::singleShot(10, &socket, [&] { secondWrite = socket.write("vate\n"); });
        QTRY_COMPARE(secondWrite, qint64(5));
        QTRY_COMPARE(activated.size(), 1);
        QTRY_VERIFY(socket.bytesAvailable() > 0);
        QCOMPARE(socket.readAll(), QByteArray("ok\n"));
        QTest::qWait(30);
        QCOMPARE(activated.size(), 1);
    }

    void malformedAndOversizedMessagesAreRejected()
    {
        QTemporaryDir dir;
        InstanceService primary(dir.filePath("settings.json"));
        QVERIFY2(primary.start() == InstanceService::Result::Primary, qPrintable(primary.error()));
        QSignalSpy activated(&primary, &InstanceService::activationRequested);
        for (const QByteArray message : {QByteArray("not-activate\n"), QByteArray(33, 'x')}) {
            QLocalSocket socket; socket.connectToServer(primary.scopeName());
            QVERIFY(socket.waitForConnected(1000));
            QCOMPARE(socket.write(message), message.size());
            QTRY_VERIFY(socket.state() == QLocalSocket::UnconnectedState);
            QCOMPARE(activated.size(), 0);
            QCOMPARE(socket.bytesAvailable(), qint64(0));
        }
    }

    void missingAcknowledgementIsAnError()
    {
        QTemporaryDir dir;
        const QString path = dir.filePath("settings.json"), ready = dir.filePath("ready");
        QProcess peer;
        peer.start(helperPath(), {path, ready, dir.filePath("unused"), "noack"});
        QVERIFY2(peer.waitForStarted(2000), qPrintable(peer.errorString()));
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(ready), 3000);
        InstanceService secondary(path);
        QVERIFY(secondary.start(300) == InstanceService::Result::Error);
        QVERIFY(secondary.error().contains("acknowledge"));
        QVERIFY2(peer.waitForFinished(3000), qPrintable(peer.errorString()));
        QCOMPARE(peer.exitCode(), 0);
    }

#ifndef Q_OS_WIN
    void permissiveHeadroomDirectoryIsRejected()
    {
        QTemporaryDir runtime;
        const QString headroom = runtime.filePath("Headroom");
        QVERIFY(QDir().mkdir(headroom));
        QVERIFY(QFile::setPermissions(headroom, QFileDevice::ReadOwner | QFileDevice::WriteOwner |
                                                   QFileDevice::ExeOwner | QFileDevice::ReadGroup |
                                                   QFileDevice::ExeGroup | QFileDevice::ReadOther |
                                                   QFileDevice::ExeOther));
        const QByteArray previous = qgetenv("XDG_RUNTIME_DIR");
        qputenv("XDG_RUNTIME_DIR", QFile::encodeName(runtime.path()));
        InstanceService instance(runtime.filePath("settings.json"));
        if (previous.isNull()) qunsetenv("XDG_RUNTIME_DIR"); else qputenv("XDG_RUNTIME_DIR", previous);
        QCOMPARE(instance.start(), InstanceService::Result::Error);
        QVERIFY(instance.error().contains("private per-user runtime"));
        QCOMPARE(QDir(headroom).entryList(QDir::AllEntries | QDir::NoDotAndDotDot).size(), 0);
    }

    void symlinkedHeadroomDirectoryDoesNotTouchTarget()
    {
        QTemporaryDir runtime;
        QTemporaryDir external;
        const QString sentinelPath = external.filePath("sentinel");
        QFile sentinel(sentinelPath);
        QVERIFY(sentinel.open(QIODevice::WriteOnly));
        QCOMPARE(sentinel.write("untouched"), qint64(9));
        sentinel.close();
        QVERIFY(QFile::link(external.path(), runtime.filePath("Headroom")));
        const QByteArray previous = qgetenv("XDG_RUNTIME_DIR");
        qputenv("XDG_RUNTIME_DIR", QFile::encodeName(runtime.path()));
        InstanceService instance(runtime.filePath("settings.json"));
        if (previous.isNull()) qunsetenv("XDG_RUNTIME_DIR"); else qputenv("XDG_RUNTIME_DIR", previous);
        QCOMPARE(instance.start(), InstanceService::Result::Error);
        QFile check(sentinelPath);
        QVERIFY(check.open(QIODevice::ReadOnly));
        QCOMPARE(check.readAll(), QByteArray("untouched"));
        QCOMPARE(QDir(external.path()).entryList(QDir::AllEntries | QDir::NoDotAndDotDot),
                 QStringList{"sentinel"});
    }

    void symlinkedEndpointDoesNotTouchTarget_data()
    {
        QTest::addColumn<bool>("lockEndpoint");
        QTest::newRow("lock") << true;
        QTest::newRow("activation-socket") << false;
    }

    void symlinkedEndpointDoesNotTouchTarget()
    {
        QFETCH(bool, lockEndpoint);
        QTemporaryDir runtime;
        const QByteArray previous = qgetenv("XDG_RUNTIME_DIR");
        qputenv("XDG_RUNTIME_DIR", QFile::encodeName(runtime.path()));
        InstanceService instance(runtime.filePath("settings.json"));
        if (previous.isNull()) qunsetenv("XDG_RUNTIME_DIR"); else qputenv("XDG_RUNTIME_DIR", previous);
        const QString sentinelPath = runtime.filePath("sentinel");
        QFile sentinel(sentinelPath);
        QVERIFY(sentinel.open(QIODevice::WriteOnly));
        QCOMPARE(sentinel.write("untouched"), qint64(9));
        sentinel.close();
        QVERIFY(QFile::link(sentinelPath, lockEndpoint ? instance.lockPath() : instance.scopeName()));
        QCOMPARE(instance.start(), InstanceService::Result::Error);
        QVERIFY(instance.error().contains("unsafe instance endpoint"));
        QFile check(sentinelPath);
        QVERIFY(check.open(QIODevice::ReadOnly));
        QCOMPARE(check.readAll(), QByteArray("untouched"));
    }
#endif
};

QTEST_GUILESS_MAIN(InstanceTest)
#include "test_instance.moc"
