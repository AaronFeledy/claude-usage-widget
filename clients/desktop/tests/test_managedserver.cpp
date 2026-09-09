#include "managedserver.h"

#include <QFile>
#include <QNetworkRequest>
#include <QProcess>
#include <QRegularExpression>
#include <QSignalSpy>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTemporaryDir>
#include <QtTest>
#ifdef Q_OS_WIN
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#endif

class ScopedEnvironment {
public:
    void set(const QByteArray &name, const QByteArray &value) {
        saved.append({name, qEnvironmentVariableIsSet(name.constData()), qgetenv(name.constData())});
        qputenv(name.constData(), value);
    }
    ~ScopedEnvironment() {
        for (auto it = saved.crbegin(); it != saved.crend(); ++it) {
            if (it->wasSet) qputenv(it->name.constData(), it->value);
            else qunsetenv(it->name.constData());
        }
    }
private:
    struct Value { QByteArray name; bool wasSet; QByteArray value; };
    QList<Value> saved;
};

class HealthFixture : public QTcpServer {
public:
    QByteArray response = R"({"status":"ok","version":"1.7.1","providers":[]})";
    int status = 200;
    bool hold = false;
    int requests = 0;
    HealthFixture() {
        connect(this, &QTcpServer::newConnection, this, [this] {
            while (auto socket = nextPendingConnection()) {
                connect(socket, &QTcpSocket::readyRead, socket, [this, socket] {
                    QByteArray request = socket->property("request").toByteArray() + socket->readAll();
                    socket->setProperty("request", request);
                    if (!request.contains("\r\n\r\n")) return;
                    ++requests;
                    if (hold) return;
                    socket->write("HTTP/1.1 " + QByteArray::number(status) + " Fixture\r\nContent-Type: application/json\r\nContent-Length: "
                        + QByteArray::number(response.size()) + "\r\nConnection: close\r\n\r\n" + response);
                    socket->disconnectFromHost();
                });
                connect(socket, &QTcpSocket::disconnected, socket, &QObject::deleteLater);
            }
        });
    }
    QUrl url() const { return QUrl(QStringLiteral("http://127.0.0.1:%1/").arg(serverPort())); }
};

static quint16 unusedPort()
{
    QTcpServer server;
    if (!server.listen(QHostAddress::LocalHost)) return 0;
    return server.serverPort();
}

static bool processExists(qint64 pid)
{
#ifdef Q_OS_WIN
    HANDLE process = OpenProcess(SYNCHRONIZE, FALSE, static_cast<DWORD>(pid));
    if (!process) return false;
    const bool exists = WaitForSingleObject(process, 0) == WAIT_TIMEOUT;
    CloseHandle(process);
    return exists;
#else
    return QFileInfo::exists(QStringLiteral("/proc/%1").arg(pid));
#endif
}

static qint64 lastPid(const QString &path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly)) return 0;
    const auto matches = QRegularExpression(QStringLiteral("pid=(\\d+)")).globalMatch(QString::fromUtf8(file.readAll()));
    qint64 pid = 0;
    auto iterator = matches;
    while (iterator.hasNext()) pid = iterator.next().captured(1).toLongLong();
    return pid;
}

class ManagedServerTest : public QObject {
    Q_OBJECT
private:
    ManagedServerOptions options(quint16 port, const QString &binary = QString()) const {
        ManagedServerOptions result;
        result.localUrl = QUrl(QStringLiteral("http://127.0.0.1:%1/").arg(port));
        result.executablePath = binary;
        result.probeTimeoutMs = 2000;
        result.readinessProbeTimeoutMs = 500;
        result.readinessIntervalMs = 50;
        result.readinessAttempts = 30;
        result.restartLimit = 2;
        return result;
    }
private slots:
    void cleanup() {
        qunsetenv("HEADROOM_FIXTURE_MODE");
        qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
    void remoteNeverProbesOrSpawns() {
        HealthFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost));
        ManagedServer server(options(fixture.serverPort(), QStringLiteral(FIXTURE_PATH)));
        server.configure(QStringLiteral("remote"), QString()); server.ensureAvailable(); QTest::qWait(150);
        QCOMPARE(fixture.requests, 0); QVERIFY(!server.ownsProcess()); QCOMPARE(server.state(), QString("remote"));
    }
    void attachesToCompatibleOkAndDegradedWithoutOwnership() {
        for (const auto status : {QByteArray("ok"), QByteArray("degraded")}) {
            HealthFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost));
            fixture.response = "{\"status\":\"" + status + "\",\"version\":\"dev\",\"providers\":[]}";
            ManagedServer server(options(fixture.serverPort(), QStringLiteral("/missing/unused")));
            QSignalSpy ready(&server, &ManagedServer::available);
            server.configure("local", QString()); server.ensureAvailable();
            QTRY_COMPARE(ready.size(), 1); QVERIFY(server.isAttached()); QVERIFY(!server.ownsProcess());
            server.stopOwned(); QVERIFY(fixture.isListening());
        }
    }
    void absentBinaryIsControlled() {
        ManagedServer server(options(unusedPort(), QStringLiteral("/definitely/missing/usage-server")));
        QSignalSpy failed(&server, &ManagedServer::unavailable);
        server.configure("local", QString()); server.ensureAvailable();
        QTRY_COMPARE_WITH_TIMEOUT(failed.size(), 1, 5000);
        QCOMPARE(failed.at(0).at(1).toString(), QString("binary")); QVERIFY(!server.ownsProcess());
    }
    void occupiedOrRejectedEndpointNeverSpawns() {
        const QList<QPair<int,QByteArray>> cases{{401, R"({"error":"unauthorized"})"},
            {200, R"({"status":"ok","version":"dev"})"}, {302, QByteArray()}};
        for (const auto &item : cases) {
            HealthFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost)); fixture.status = item.first; fixture.response = item.second;
            QTemporaryDir dir; const auto record = dir.filePath("record"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
            ManagedServer server(options(fixture.serverPort(), QStringLiteral(FIXTURE_PATH)));
            QSignalSpy failed(&server, &ManagedServer::unavailable);
            server.configure("local", "wrong-token"); server.ensureAvailable(); QTRY_COMPARE(failed.size(), 1);
            QVERIFY(!QFileInfo::exists(record)); QVERIFY(!server.ownsProcess());
        }
        qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
    void timeoutDoesNotSpawn() {
        HealthFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost)); fixture.hold = true;
        QTemporaryDir dir; const auto record = dir.filePath("record"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
        auto configured = options(fixture.serverPort(), QStringLiteral(FIXTURE_PATH));
        configured.probeTimeoutMs = 100;
        ManagedServer server(configured);
        QSignalSpy failed(&server, &ManagedServer::unavailable);
        server.configure("local", QString()); server.ensureAvailable(); QTRY_COMPARE(failed.size(), 1);
        QCOMPARE(failed.at(0).at(1).toString(), QString("timeout")); QVERIFY(!QFileInfo::exists(record));
        qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
    void spawnsWithPrivateEnvironmentAndStopsOwned() {
        QTemporaryDir dir; const auto record = dir.filePath("record");
        qputenv("HEADROOM_FIXTURE_MODE", "degraded"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
        const quint16 port = unusedPort(); QVERIFY(port);
        qint64 pid = 0;
        {
            ManagedServer server(options(port, QStringLiteral(FIXTURE_PATH)));
            QSignalSpy ready(&server, &ManagedServer::available);
            server.configure("local", "fixture-secret"); server.ensureAvailable(); QTRY_COMPARE_WITH_TIMEOUT(ready.size(), 1, 10000);
            QVERIFY(server.ownsProcess()); QTRY_VERIFY((pid = lastPid(record)) > 0); QVERIFY(processExists(pid));
            QFile file(record); QVERIFY(file.open(QIODevice::ReadOnly)); const auto content = file.readAll();
            QVERIFY(content.contains("args=--listen-addr|127.0.0.1:")); QVERIFY(content.contains("token_present=yes"));
            QVERIFY(!content.contains("fixture-secret"));
            server.configure("remote", QString());
            QTRY_VERIFY_WITH_TIMEOUT(!processExists(pid), 5000);
            QCOMPARE(server.state(), QString("remote")); QVERIFY(!server.ownsProcess());
        }
        QTRY_VERIFY_WITH_TIMEOUT(!processExists(pid), 5000);
        qunsetenv("HEADROOM_FIXTURE_MODE"); qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
    void readinessTimeoutKillsOwned() {
        QTemporaryDir dir; const auto record = dir.filePath("record");
        qputenv("HEADROOM_FIXTURE_MODE", "hang"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
        auto configured = options(unusedPort(), QStringLiteral(FIXTURE_PATH));
        configured.readinessProbeTimeoutMs = 75; configured.readinessAttempts = 2;
        ManagedServer server(configured); QSignalSpy failed(&server, &ManagedServer::unavailable);
        server.configure("local", QString()); server.ensureAvailable(); QTRY_COMPARE_WITH_TIMEOUT(failed.size(), 1, 7000);
        qint64 pid = lastPid(record); QVERIFY(pid > 0); QTRY_VERIFY_WITH_TIMEOUT(!processExists(pid), 5000); QVERIFY(!server.ownsProcess());
        qunsetenv("HEADROOM_FIXTURE_MODE"); qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
    void manualRetryDuringOwnedRetirementIsRemembered() {
        QTemporaryDir dir; const auto record = dir.filePath("record");
        qputenv("HEADROOM_FIXTURE_MODE", "degraded"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
        ManagedServer server(options(unusedPort(), QStringLiteral(FIXTURE_PATH)));
        QSignalSpy ready(&server, &ManagedServer::available);
        server.configure("local", QString()); server.ensureAvailable(); QTRY_COMPARE_WITH_TIMEOUT(ready.size(), 1, 10000);
        server.stopOwned(); server.ensureAvailable();
        QTRY_COMPARE_WITH_TIMEOUT(ready.size(), 2, 10000);
        QFile file(record); QVERIFY(file.open(QIODevice::ReadOnly)); QCOMPARE(file.readAll().count("start\n"), 2);
        server.stopOwned();
    }
    void restartBudgetIsBoundedAndManualRetryIsClear() {
        QTemporaryDir dir; const auto record = dir.filePath("record");
        qputenv("HEADROOM_FIXTURE_MODE", "crash"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
        ManagedServer server(options(unusedPort(), QStringLiteral(FIXTURE_PATH))); QSignalSpy failed(&server, &ManagedServer::unavailable);
        server.configure("local", QString()); server.ensureAvailable(); QTRY_COMPARE_WITH_TIMEOUT(failed.size(), 1, 15000);
        QFile file(record); QVERIFY(file.open(QIODevice::ReadOnly)); QCOMPARE(file.readAll().count("start\n"), 3);
        QTest::qWait(250); file.seek(0); QCOMPARE(file.readAll().count("start\n"), 3);
        server.ensureAvailable(); QTRY_VERIFY_WITH_TIMEOUT(failed.size() >= 2, 15000);
        file.seek(0); QCOMPARE(file.readAll().count("start\n"), 6);
        qunsetenv("HEADROOM_FIXTURE_MODE"); qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
    void modeSwitchCancelsOldProbe() {
        HealthFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost)); fixture.hold = true;
        ManagedServer server(options(fixture.serverPort(), QStringLiteral(FIXTURE_PATH)));
        QSignalSpy ready(&server, &ManagedServer::available); QSignalSpy failed(&server, &ManagedServer::unavailable);
        server.configure("local", QString()); server.ensureAvailable(); QTRY_COMPARE(fixture.requests, 1);
        server.configure("remote", QString()); QTest::qWait(200);
        QCOMPARE(ready.size(), 0); QCOMPARE(failed.size(), 0); QCOMPARE(server.state(), QString("remote")); QVERIFY(!server.ownsProcess());
    }
    void stagedRealServerSmoke() {
        const QString binary = qEnvironmentVariable("HEADROOM_REAL_SERVER_FIXTURE");
        if (binary.isEmpty()) QSKIP("Set HEADROOM_REAL_SERVER_FIXTURE to exercise a staged Go usage server.");
        QVERIFY2(QFileInfo::exists(binary), qPrintable(binary));
        QTemporaryDir dir; QVERIFY(dir.isValid());
        const QString configPath = dir.filePath("config.yaml");
        QFile config(configPath); QVERIFY(config.open(QIODevice::WriteOnly | QIODevice::Text));
        QVERIFY(config.write("providers:\n  claude: {enabled: false}\n  codex: {enabled: false}\n  cursor: {enabled: false}\n  grok: {enabled: false}\n") > 0);
        config.close();
        ScopedEnvironment environment;
        environment.set("USAGE_CONFIG", configPath.toUtf8());
        for (const auto name : {"USAGE_PROVIDER_CLAUDE_ENABLED", "USAGE_PROVIDER_CODEX_ENABLED",
                                "USAGE_PROVIDER_CURSOR_ENABLED", "USAGE_PROVIDER_GROK_ENABLED"})
            environment.set(name, "false");
        auto configured = options(unusedPort(), binary);
        configured.probeTimeoutMs = 3000; configured.readinessProbeTimeoutMs = 1000;
        configured.readinessIntervalMs = 50; configured.readinessAttempts = 60;
        ManagedServer server(configured); QSignalSpy ready(&server, &ManagedServer::available);
        server.configure("local", "fixture-secret"); server.ensureAvailable();
        QTRY_COMPARE_WITH_TIMEOUT(ready.size(), 1, 15000);
        QVERIFY(server.ownsProcess()); QCOMPARE(server.state(), QString("started"));
    }
#ifdef Q_OS_WIN
    void abruptOwnerExitClosesJobAndKillsChild() {
        QTemporaryDir dir; const auto record = dir.filePath("record"), ready = dir.filePath("ready");
        qputenv("HEADROOM_FIXTURE_MODE", "degraded"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
        const quint16 port = unusedPort(); QVERIFY(port);
        QProcess owner; owner.start(QStringLiteral(OWNER_PATH), {QStringLiteral(FIXTURE_PATH), QString::number(port), ready});
        QVERIFY(owner.waitForStarted(5000)); QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(ready), 15000);
        qint64 pid = 0; QTRY_VERIFY((pid = lastPid(record)) > 0); QVERIFY(processExists(pid));
        owner.kill(); QVERIFY(owner.waitForFinished(2000));
        QTRY_VERIFY_WITH_TIMEOUT(!processExists(pid), 5000);
        qunsetenv("HEADROOM_FIXTURE_MODE"); qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
#endif
};

QTEST_MAIN(ManagedServerTest)
#include "test_managedserver.moc"
