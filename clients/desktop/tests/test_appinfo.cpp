#include "appinfo.h"
#include <QtTest>
#include <QTcpServer>
#include <QTcpSocket>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QSysInfo>

class HttpFixture : public QTcpServer {
public:
    QByteArray body = R"({"status":"ok","version":"1.7.1"})";
    QByteArray headers;
    int status = 200;
    bool respond = true;
    QList<QByteArray> requests;
    HttpFixture() {
        connect(this, &QTcpServer::newConnection, this, [this] {
            while (hasPendingConnections()) {
                auto socket = nextPendingConnection();
                connect(socket, &QTcpSocket::disconnected, socket, &QObject::deleteLater);
                connect(socket, &QTcpSocket::readyRead, socket, [this, socket] {
                    auto bytes = socket->property("bytes").toByteArray() + socket->readAll();
                    socket->setProperty("bytes", bytes);
                    if (!bytes.contains("\r\n\r\n") || socket->property("handled").toBool()) return;
                    socket->setProperty("handled", true); requests.append(bytes);
                    if (!respond) return;
                    socket->write("HTTP/1.1 " + QByteArray::number(status) + " Result\r\nContent-Type: application/json\r\nContent-Length: "
                        + QByteArray::number(body.size()) + "\r\nConnection: close\r\n" + headers + "\r\n" + body);
                    socket->disconnectFromHost();
                });
            }
        });
    }
    QString url() const { return QString("http://127.0.0.1:%1").arg(serverPort()); }
};
class AppInfoTest : public QObject {
    Q_OBJECT
private slots:
    void initTestCase() { QCoreApplication::setApplicationVersion("0.1.0"); }
    void healthAndRedaction() {
        HttpFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost));
        AppInfo info;
        QCOMPARE(info.applicationVersion(), QString("0.1.0"));
        info.setBackend(fixture.url() + "/prefix/", "private-fixture-token"); info.refreshServer();
        QTRY_VERIFY(!info.checkingServer());
        QCOMPARE(info.serverVersion(), QString("1.7.1")); QCOMPARE(info.serverStatus(), QString("Server healthy"));
        QVERIFY(fixture.requests.last().startsWith("GET /prefix/api/v1/health "));
        QVERIFY(fixture.requests.last().contains("Authorization: Bearer private-fixture-token\r\n"));
        fixture.body = R"({"status":"degraded","version":"dev-test"})";
        info.refreshServer(); QTRY_VERIFY(!info.checkingServer());
        QCOMPARE(info.serverVersion(), QString("dev-test")); QVERIFY(info.serverStatus().contains("need attention"));
        fixture.status = 401; fixture.body = "private-fixture-token";
        info.refreshServer(); QTRY_VERIFY(!info.checkingServer());
        QVERIFY(info.serverVersion().isEmpty()); QVERIFY(info.serverStatus().contains("rejected"));
        QVERIFY(!info.serverStatus().contains("private-fixture-token"));
        fixture.status = 200; fixture.body = R"({"status":"ok","version":"<b>invalid</b>"})";
        info.refreshServer(); QTRY_VERIFY(!info.checkingServer());
        QVERIFY(info.serverVersion().isEmpty()); QVERIFY(info.serverStatus().contains("unrecognized"));
        fixture.body = R"({"status":"ok","version":"private-fixture-token"})";
        info.refreshServer(); QTRY_VERIFY(!info.checkingServer());
        QVERIFY(info.serverVersion().isEmpty()); QVERIFY(!info.serverStatus().contains("private-fixture-token"));
        const int count = fixture.requests.size();
        info.setBackend(fixture.url() + "/prefix/", "private-fixture-token");
        QTest::qWait(20); QCOMPARE(fixture.requests.size(), count);
        info.setBackend("https://user:secret@example.org/", "token"); info.refreshServer();
        QVERIFY(!info.checkingServer()); QVERIFY(info.serverVersion().isEmpty());
    }
    void redirectsNeverForwardToken() {
        HttpFixture origin, destination;
        QVERIFY(origin.listen(QHostAddress::LocalHost)); QVERIFY(destination.listen(QHostAddress::LocalHost));
        origin.status = 302; origin.headers = "Location: " + destination.url().toUtf8() + "/health\r\n";
        AppInfo info; info.setBackend(origin.url(), "private-fixture-token"); info.refreshServer();
        QTRY_VERIFY(!info.checkingServer());
        QVERIFY(info.serverStatus().contains("redirected")); QCOMPARE(destination.requests.size(), 0);
    }
    void timeoutsAndBackendSwitch() {
        HttpFixture slow, next; QVERIFY(slow.listen(QHostAddress::LocalHost)); QVERIFY(next.listen(QHostAddress::LocalHost));
        slow.respond = false;
        AppInfo info(nullptr, QUrl(slow.url()), 100);
        info.setBackend(slow.url(), "old-token"); info.refreshServer();
        QTRY_VERIFY(!info.checkingServer()); QVERIFY(info.serverStatus().contains("unavailable"));
        info.refreshServer(); QTRY_VERIFY(slow.requests.size() >= 2);
        info.setBackend(next.url(), "new-token"); info.refreshServer();
        QTRY_VERIFY(!info.checkingServer()); QCOMPARE(info.serverVersion(), QString("1.7.1"));
        QVERIFY(next.requests.last().contains("Bearer new-token")); QVERIFY(!next.requests.last().contains("old-token"));
        info.checkForUpdates(); QTRY_VERIFY(!info.checkingRelease()); QVERIFY(info.releaseStatus().contains("Could not check"));
    }
    void rejectsOversizedResponses() {
        HttpFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost));
        fixture.body = QByteArray(1024 * 1024 + 100, 'x');
        AppInfo info(nullptr, QUrl(fixture.url()), 2000);
        info.setBackend(fixture.url(), "test-token"); info.refreshServer();
        QTRY_VERIFY(!info.checkingServer()); QVERIFY(info.serverVersion().isEmpty());
        info.checkForUpdates(); QTRY_VERIFY(!info.checkingRelease());
        QVERIFY(info.latestVersion().isEmpty()); QVERIFY(!info.linuxDownloadAvailable());
    }
    void releaseAvailabilityAndCredentialIsolation() {
        HttpFixture fixture; QVERIFY(fixture.listen(QHostAddress::LocalHost));
        AppInfo info(nullptr, QUrl(fixture.url() + "/releases/latest"));
        info.setBackend(fixture.url(), "backend-secret");
        QJsonArray assets {QJsonObject{{"name", "ClaudeUsageWidget-win-x64.exe"}}};
        auto release = QJsonObject{{"tag_name", "v9.1.0"}, {"assets", assets}};
        fixture.body = QJsonDocument(release).toJson(); info.checkForUpdates();
        QTRY_VERIFY(!info.checkingRelease()); QCOMPARE(info.latestVersion(), QString("v9.1.0"));
        QVERIFY(!info.linuxDownloadAvailable()); QVERIFY(info.releaseStatus().contains("No Linux installer"));
        QVERIFY(!fixture.requests.last().contains("Authorization")); QVERIFY(!fixture.requests.last().contains("backend-secret"));
        auto cpu = QSysInfo::currentCpuArchitecture(); if (cpu == "arm64") cpu = "aarch64";
        const QString name = "headroom-linux-" + cpu + ".tar.gz";
        assets.append(QJsonObject{{"name", name}, {"browser_download_url", "https://github.com/AaronFeledy/claude-usage-widget/releases/download/v9.1.0/" + name}});
        release["assets"] = assets; fixture.body = QJsonDocument(release).toJson(); info.checkForUpdates();
        QTRY_VERIFY(!info.checkingRelease()); QVERIFY(info.linuxDownloadAvailable());
        QVERIFY(info.releaseStatus().contains("Review it before installing"));
        release["assets"] = QJsonArray{QJsonObject{{"name", name}, {"browser_download_url", "https://unrelated.example/" + name}}};
        fixture.body = QJsonDocument(release).toJson(); info.checkForUpdates();
        QTRY_VERIFY(!info.checkingRelease()); QVERIFY(!info.linuxDownloadAvailable());
        fixture.body = "not json"; info.checkForUpdates(); QTRY_VERIFY(!info.checkingRelease());
        QVERIFY(info.latestVersion().isEmpty()); QVERIFY(info.releaseStatus().contains("not recognized"));
        fixture.status = 404; info.checkForUpdates(); QTRY_VERIFY(!info.checkingRelease());
        QVERIFY(info.releaseStatus().contains("No published release"));
    }
};
QTEST_GUILESS_MAIN(AppInfoTest)
#include "test_appinfo.moc"
