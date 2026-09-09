#include "instance.h"

#include <QDir>
#include <QFile>
#include <QLocalSocket>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QtTest>

class InstanceTest : public QObject {
    Q_OBJECT
private slots:
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
        InstanceService primary(path); QVERIFY2(primary.start() == InstanceService::Result::Primary, qPrintable(primary.error()));
        QSignalSpy activated(&primary, &InstanceService::activationRequested);
        InstanceService secondary(path); QCOMPARE(secondary.start(), InstanceService::Result::Secondary);
        QTRY_COMPARE(activated.size(), 1);
    }

    void fragmentedMessageActivatesOnce()
    {
        QTemporaryDir dir;
        InstanceService primary(dir.filePath("settings.json"));
        QVERIFY2(primary.start() == InstanceService::Result::Primary, qPrintable(primary.error()));
        QSignalSpy activated(&primary, &InstanceService::activationRequested);
        QLocalSocket socket; socket.connectToServer(primary.scopeName());
        QVERIFY(socket.waitForConnected(1000));
        socket.write("acti"); QVERIFY(socket.waitForBytesWritten(1000)); QTest::qWait(10);
        socket.write("vate\n"); QVERIFY(socket.waitForBytesWritten(1000));
        QTRY_COMPARE(activated.size(), 1);
    }
};

QTEST_GUILESS_MAIN(InstanceTest)
#include "test_instance.moc"
