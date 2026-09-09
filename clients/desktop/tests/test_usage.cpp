#include "usage.h"
#include "controller.h"
#include "http_assertions.h"
#include <QtTest>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTemporaryDir>
#include <QFile>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <cmath>
class UsageTest : public QObject {
    Q_OBJECT
private slots:
    void cleanup() {
        qunsetenv("HEADROOM_FIXTURE_MODE");
        qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
    void parseContract() {
        QVariantList providers; QVERIFY(Usage::parse(Usage::demo(), providers)); QCOMPARE(providers.size(), 4);
        QCOMPARE(providers[0].toMap()["provider_name"].toString(), "Claude");
        QCOMPARE(providers[0].toMap()["buckets"].toList().size(), 3);
        QCOMPARE(providers[1].toMap()["provider_name"].toString(), QString("Codex"));
        QCOMPARE(providers[1].toMap()["display_name"].toString(), QString("ChatGPT"));
        const auto original = providers;
        for (const QByteArray bad : {QByteArray("{}"), QByteArray("[null]"), QByteArray("[{}]"), QByteArray("not json")}) {
            QVERIFY(!Usage::parse(bad, providers)); QCOMPARE(providers, original);
        }
    }
    void malformedMeters() {
        auto list = QJsonDocument::fromJson(Usage::demo()).array();
        auto p = list[0].toObject(); auto buckets = p["buckets"].toArray(); auto b = buckets[0].toObject();
        for (const QJsonValue value : {QJsonValue(-1), QJsonValue(101), QJsonValue("40"), QJsonValue(QJsonValue::Null)}) {
            b["utilization"] = value; buckets[0] = b; p["buckets"] = buckets; list[0] = p;
            QVariantList result; QVERIFY(!Usage::parse(QJsonDocument(list).toJson(), result));
        }
    }
    void legacyAndErrors() {
        auto p = QJsonDocument::fromJson(Usage::demo()).array()[0].toObject();
        p.remove("buckets"); p["current"] = QJsonObject{{"utilization", 0}, {"resets_at", QJsonValue::Null}};
        p["primary_label"] = "Session"; p["show_secondary"] = false;
        QVariantList result; QVERIFY(Usage::parse(QJsonDocument(QJsonArray{p}).toJson(), result));
        QCOMPARE(result[0].toMap()["buckets"].toList()[0].toMap()["utilization"].toDouble(), 0.0);
        p["error"] = "Unavailable"; p["is_success"] = false; p["current"] = QJsonValue::Null;
        QVERIFY(Usage::parse(QJsonDocument(QJsonArray{p}).toJson(), result)); QVERIFY(result[0].toMap()["buckets"].toList().isEmpty());
        p["is_success"] = true; QVERIFY(!Usage::parse(QJsonDocument(QJsonArray{p}).toJson(), result));
    }
    void preservesVariableMetersAndHeaderStatus() {
        auto data = QJsonDocument::fromJson(Usage::demo()).array();
        auto cursor = data[2].toObject();
        auto buckets = cursor["buckets"].toArray();
        for (int i = 0; i < buckets.size(); ++i) { auto b = buckets[i].toObject(); b["status_text"] = QJsonValue::Null; buckets[i] = b; }
        cursor["buckets"] = buckets;
        cursor["primary_status_text"] = "$38 / $50 this cycle";
        cursor["secondary_status_text"] = "On-demand enabled";
        data[2] = cursor;
        QVariantList result; QVERIFY(Usage::parse(QJsonDocument(data).toJson(), result));
        const QList<int> expectedCounts {3, 2, 4, 1};
        for (int i = 0; i < result.size(); ++i) QCOMPARE(result[i].toMap()["buckets"].toList().size(), expectedCounts[i]);
        const auto rows = result[2].toMap()["buckets"].toList();
        QVERIFY(rows[0].toMap()["status_text"].toString().isEmpty());
        QCOMPARE(rows[1].toMap()["label"].toString(), QString("Other Models"));
        QCOMPARE(rows[1].toMap()["status_text"].toString(), QString("$38 / $50 this cycle"));
        QCOMPARE(rows[2].toMap()["id"].toString(), QString("weekly_grok_bot"));
        QCOMPARE(rows[3].toMap()["status_text"].toString(), QString("On-demand enabled"));
    }
    void endpoints() {
        QCOMPARE(Usage::endpoint("http://arrowone:7823/").toString(), "http://arrowone:7823/api/v1/usage");
        QCOMPARE(Usage::endpoint("https://example.org/prefix/").toString(), "https://example.org/prefix/api/v1/usage");
        for (auto url : {"ftp://example.org", "http://user:secret@server", "http://", "https://server/?token=x", "https://server/#x"}) QVERIFY(Usage::endpoint(url).isEmpty());
    }
    void pacingWindowsAndDirection() {
        const auto now = QDateTime::fromString("2026-09-07T12:00:00Z", Qt::ISODate);
        QVariantMap bucket{{"id", "session"}, {"label", "5-Hour"}, {"utilization", 60},
            {"resets_at", now.addSecs(9000).toString(Qt::ISODate)}};
        auto pace = Usage::pacing("Claude", bucket, now);
        QCOMPARE(pace["expected"].toDouble(), 50.0);
        QCOMPARE(pace["label"].toString(), QString("10 pp over pace"));
        QVERIFY(pace["over"].toBool());
        bucket["utilization"] = 34;
        pace = Usage::pacing("Codex", bucket, now);
        QCOMPARE(pace["label"].toString(), QString("16 pp under pace"));
        bucket["utilization"] = 50;
        QCOMPARE(Usage::pacing("Codex", bucket, now)["label"].toString(), QString("On pace"));
        bucket["id"] = "weekly_grok_bot";
        bucket["resets_at"] = now.addSecs(7 * 86400 / 2).toString(Qt::ISODate);
        QCOMPARE(Usage::pacing("Cursor", bucket, now)["expected"].toDouble(), 50.0);
        bucket["id"] = "plan";
        bucket["resets_at"] = now.addDays(15).toString(Qt::ISODate);
        QCOMPARE(Usage::pacing("Cursor", bucket, now)["expected"].toDouble(), 50.0);
        bucket["id"] = "session"; bucket["label"] = "Weekly";
        bucket["resets_at"] = now.addSecs(7 * 86400 / 2).toString(Qt::ISODate);
        QCOMPARE(Usage::pacing("Claude", bucket, now)["expected"].toDouble(), 50.0);
    }
    void pacingUnknownAndExpiredWindows() {
        const auto now = QDateTime::fromString("2026-09-07T12:00:00Z", Qt::ISODate);
        QVariantMap bucket{{"id", "session"}, {"utilization", 25}};
        QVERIFY(!Usage::pacing("Claude", bucket, now)["available"].toBool());
        for (int seconds : {-1, 0, 5 * 3600 + 1}) {
            bucket["resets_at"] = now.addSecs(seconds).toString(Qt::ISODate);
            QVERIFY(!Usage::pacing("Claude", bucket, now)["available"].toBool());
        }
        bucket["resets_at"] = now.addSecs(5 * 3600).toString(Qt::ISODate);
        QCOMPARE(Usage::pacing("Claude", bucket, now)["expected"].toDouble(), 0.0);
        QCOMPARE(Usage::pacing("Claude", bucket, now.addSecs(3600))["expected"].toDouble(), 20.0);
        QVERIFY(!Usage::pacing("Unknown", bucket, now)["available"].toBool());
        bucket["id"] = "extra";
        QVERIFY(!Usage::pacing("Claude", bucket, now)["available"].toBool());
        bucket["id"] = "on_demand"; bucket["utilization"] = 0; bucket["status_text"] = "Enabled";
        QVERIFY(!Usage::pacing("Cursor", bucket, now)["available"].toBool());
        bucket["status_text"] = "$0 / $100";
        QVERIFY(Usage::pacing("Cursor", bucket, now)["available"].toBool());
    }
    void pacingCalendarMonths() {
        for (int year : {2024, 2025}) {
            const auto reset = QDateTime(QDate(year, 3, 1), QTime(0, 0), QTimeZone::UTC);
            const auto start = reset.addMonths(-1);
            const auto halfway = start.addSecs(start.secsTo(reset) / 2);
            const QVariantMap bucket{{"id", "credits"}, {"utilization", 35}, {"resets_at", reset.toString(Qt::ISODate)}};
            QCOMPARE(Usage::pacing("Grok", bucket, halfway)["expected"].toDouble(), 50.0);
        }
    }
    void periodNotches() {
        QVariantMap bucket{{"id", "session"}, {"label", "5-Hour"}};
        auto verify = [](const QVariantList &ticks, int count, double step) {
            QCOMPARE(ticks.size(), count);
            for (int i = 0; i < count; ++i)
                QVERIFY(std::abs(ticks[i].toMap()["fraction"].toDouble() - step * (i + 1)) < 0.000001);
        };
        // Known periods retain their scale even when the API omits a reset time.
        verify(Usage::notches("Claude", bucket), 4, 1.0 / 5);
        verify(Usage::notches("Codex", bucket), 4, 1.0 / 5);
        QCOMPARE(Usage::notches("Claude", bucket)[0].toMap()["label"].toString(), QString("Hour 1"));
        bucket["id"] = "weekly_grok_bot";
        verify(Usage::notches("Cursor", bucket), 6, 1.0 / 7);
        bucket["id"] = "session"; bucket["label"] = "Weekly";
        verify(Usage::notches("Claude", bucket), 6, 1.0 / 7);
        bucket["id"] = "plan";
        verify(Usage::notches("Cursor", bucket), 4, 7.0 / 30);
        QCOMPARE(Usage::notches("Cursor", bucket)[3].toMap()["label"].toString(), QString("Week 4 · day 28"));
        for (const auto &date : {"2025-03-01", "2024-03-01", "2025-05-01", "2025-02-01"}) {
            const auto reset = QDateTime::fromString(QString(date) + "T00:00:00Z", Qt::ISODate);
            const int days = reset.addMonths(-1).daysTo(reset);
            bucket["resets_at"] = reset.toString(Qt::ISODate);
            verify(Usage::notches("Grok", bucket), (days - 1) / 7, 7.0 / days);
        }
        bucket.remove("resets_at");
        QVERIFY(Usage::notches("Grok", bucket).isEmpty());
        QVERIFY(Usage::notches("Unknown", bucket).isEmpty());
        bucket["id"] = "extra";
        QVERIFY(Usage::notches("Claude", bucket).isEmpty());
    }
    void concernUsesRemainingWindow() {
        const auto now = QDateTime::fromString("2026-09-08T12:00:00Z", Qt::ISODate);
        auto assess = [&](double used, double elapsed) {
            const QVariantMap bucket{{"id", "session"}, {"utilization", used},
                {"resets_at", now.addSecs(qRound64((100.0 - elapsed) * 180)).toString(Qt::ISODate)}};
            return Usage::concern("Claude", bucket, now);
        };
        struct Case { double used; double elapsed; int severity; };
        const QList<Case> cases {
            {2, 0, 0}, {12, 10, 0}, // Small early bursts are harmless.
            {82, 80, 1}, {92, 90, 1}, {95, 90, 3}, // Same small lead matters later.
            {54.9, 50, 0}, {55, 50, 1}, {62.4, 50, 1}, {62.5, 50, 2},
            {74.9, 50, 2}, {75, 50, 3}, // Remaining-budget boundary values.
            {90, 90, 0}, {94, 96, 0}, // High usage alone is not alarming on pace.
            {95, 96, 1}, {99, 99.5, 2}, {100, 99.9, 3}, // Low-capacity safety floor.
            {50, 80, 0}, {0, 99.9, 0}, {99.95, 99.9, 3}
        };
        for (const auto &c : cases) {
            const auto result = assess(c.used, c.elapsed);
            QVERIFY(result["available"].toBool());
            QCOMPARE(result["severity"].toInt(), c.severity);
            QVERIFY(std::isfinite(result["pressure"].toDouble()));
        }
        QVERIFY(assess(12, 10)["pressure"].toDouble() < 0.03);
        QCOMPARE(assess(95, 90)["pressure"].toDouble(), 0.5);
        // Spending more cannot reduce concern; time passing without spending can.
        for (double elapsed : {0.0, 25.0, 50.0, 90.0, 99.0}) {
            int previous = 0;
            for (int used = 0; used <= 100; ++used) {
                const int severity = assess(used, elapsed)["severity"].toInt();
                QVERIFY(severity >= previous); previous = severity;
            }
        }
        QCOMPARE(assess(60, 20)["severity"].toInt(), 3);
        QCOMPARE(assess(60, 50)["severity"].toInt(), 1);
        QCOMPARE(assess(60, 70)["severity"].toInt(), 0);
    }
    void concernWithoutTiming() {
        const auto now = QDateTime::fromString("2026-09-08T12:00:00Z", Qt::ISODate);
        QVariantMap bucket{{"id", "session"}, {"utilization", 90}};
        auto result = Usage::concern("Codex", bucket, now);
        QVERIFY(!result["available"].toBool()); QCOMPARE(result["severity"].toInt(), 3);
        QVERIFY(result["detail"].toString().contains("timing is unavailable"));
        bucket["utilization"] = 75; QCOMPARE(Usage::concern("Codex", bucket, now)["severity"].toInt(), 2);
        bucket["utilization"] = 50; QCOMPARE(Usage::concern("Codex", bucket, now)["severity"].toInt(), 1);
        bucket["utilization"] = 2; QCOMPARE(Usage::concern("Codex", bucket, now)["severity"].toInt(), 0);
        bucket["utilization"] = 90; bucket["resets_at"] = now.toString(Qt::ISODate);
        result = Usage::concern("Codex", bucket, now);
        QVERIFY(!result["available"].toBool()); QCOMPARE(result["severity"].toInt(), 3);
        bucket["id"] = "on_demand"; bucket["utilization"] = 0; bucket["status_text"] = "Enabled";
        QCOMPARE(Usage::concern("Cursor", bucket, now)["severity"].toInt(), 0);
    }
    void warningTransitionsAndHysteresis() {
        using Level = Usage::WarningLevel;
        Usage::WarningState state;
        auto step = [&](double pressure, double used = 20, QString window = "window-a") {
            return Usage::advanceWarning(state, used, true, pressure, window);
        };
        auto transition = step(0.02); QCOMPARE(transition.to, Level::Normal); QVERIFY(!transition.notify);
        transition = step(0.10); QCOMPARE(transition.to, Level::Watch); QVERIFY(!transition.notify);
        QCOMPARE(step(0.09).to, Level::Watch); // Avoid jitter around the entry boundary.
        QCOMPARE(step(0.079).to, Level::Normal);
        transition = step(0.25); QCOMPARE(transition.to, Level::Warning); QVERIFY(transition.notify);
        QVERIFY(!step(0.26).notify); QCOMPARE(step(0.21).to, Level::Warning);
        transition = step(0.19); QCOMPARE(transition.to, Level::Watch); QVERIFY(!transition.notify);
        QVERIFY(step(0.25).notify); // A genuine recovery re-arms this alert tier.
        transition = step(0.50); QCOMPARE(transition.to, Level::Critical); QVERIFY(transition.notify);
        QCOMPARE(step(0.45).to, Level::Critical); QVERIFY(!step(0.50).notify);
        transition = step(0.39); QCOMPARE(transition.to, Level::Warning); QVERIFY(!transition.notify);
        transition = step(0.50, 20, "window-b"); QVERIFY(transition.reset); QVERIFY(!transition.notify);
        QCOMPARE(step(0.0, 0, "window-c").to, Level::Normal);
        Usage::WarningState independent;
        auto first = Usage::advanceWarning(independent, 80, true, 0.6, "window-a");
        QCOMPARE(first.to, Level::Critical); QVERIFY(!first.notify); // Startup baseline, not an alert storm.
        QCOMPARE(state.level, Level::Normal);
        QCOMPARE(step(0, 100).to, Level::Critical);
        QCOMPARE(step(0, 99.7).to, Level::Critical);
        QCOMPARE(step(0, 99.4).to, Level::Warning);
        QCOMPARE(step(0, 97).to, Level::Watch);
        QCOMPARE(step(0, 93).to, Level::Normal);
        Usage::WarningState untimed;
        Usage::advanceWarning(untimed, 0, false, 0, "");
        QVERIFY(Usage::advanceWarning(untimed, 90, false, 0, "").notify);
        QCOMPARE(Usage::advanceWarning(untimed, 87, false, 0, "").to, Level::Critical);
        QCOMPARE(Usage::advanceWarning(untimed, 84, false, 0, "").to, Level::Warning);
    }
    void controllerOwnsStatesAndAlerts() {
        QTemporaryDir dir;
        QTcpServer server; QVERIFY(server.listen(QHostAddress::LocalHost));
        const auto now = QDateTime::currentDateTimeUtc();
        QString reset = now.addSecs(9000).toString(Qt::ISODate);
        double used = 50; int httpStatus = 200;
        auto payload = [&] {
            auto provider = QJsonDocument::fromJson(Usage::demo()).array()[0].toObject();
            provider["buckets"] = QJsonArray{
                QJsonObject{{"id", "session"}, {"label", "Session"}, {"utilization", used}, {"resets_at", reset}},
                QJsonObject{{"id", "weekly"}, {"label", "Weekly"}, {"utilization", 0}, {"resets_at", now.addDays(7).toString(Qt::ISODate)}}};
            return QJsonDocument(QJsonArray{provider}).toJson();
        };
        connect(&server, &QTcpServer::newConnection, this, [&] {
            auto socket = server.nextPendingConnection();
            connect(socket, &QTcpSocket::disconnected, socket, &QObject::deleteLater);
            connect(socket, &QTcpSocket::readyRead, socket, [&, socket] {
                const auto request = socket->property("request").toByteArray() + socket->readAll();
                socket->setProperty("request", request);
                if (!request.contains("\r\n\r\n")) return;
                const auto body = payload();
                socket->write("HTTP/1.1 " + QByteArray::number(httpStatus) + " Test\r\nContent-Type: application/json\r\nContent-Length: "
                    + QByteArray::number(body.size()) + "\r\nConnection: close\r\n\r\n" + body);
                socket->disconnectFromHost();
            });
        });
        Controller controller(false, dir.filePath("settings.json"));
        QSignalSpy alerts(&controller, &Controller::usageAlert);
        const QString url = QString("http://127.0.0.1:%1").arg(server.serverPort());
        QVERIFY(controller.saveSettings("remote", url, "", 60, true, "Claude", false).isEmpty());
        QTRY_COMPARE(controller.state()["status"].toString(), "ready");
        auto concern = [&] { return controller.concern("Claude", QVariantMap{{"id", "session"}}); };
        QCOMPARE(concern()["severity"].toInt(), 0); QCOMPARE(alerts.size(), 0);
        struct Step { double used; int severity; int alerts; };
        for (const Step step : {Step{60,1,0}, {68,2,1}, {66,2,1}, {62,2,1}, {59,1,1},
                               {68,2,2}, {80,3,3}, {72,3,3}, {68,2,3}, {50,0,3}}) {
            used = step.used; controller.refresh();
            QTRY_VERIFY(!controller.state()["loading"].toBool());
            QCOMPARE(concern()["severity"].toInt(), step.severity);
            QCOMPARE(alerts.size(), step.alerts);
            QCOMPARE(concern()["color"].toString(), controller.warningColor(step.severity));
            QCOMPARE(controller.concern("Claude", QVariantMap{{"id", "weekly"}})["severity"].toInt(), 0);
        }
        QVERIFY(controller.saveSettings("remote", url, "", 60, false, "Claude", false).isEmpty());
        QTRY_VERIFY(!controller.state()["loading"].toBool());
        used = 80; controller.refresh(); QTRY_VERIFY(!controller.state()["loading"].toBool());
        QCOMPARE(concern()["severity"].toInt(), 3); QCOMPARE(alerts.size(), 3);
        QVERIFY(controller.saveSettings("remote", url, "", 60, true, "Claude", false).isEmpty());
        QTRY_VERIFY(!controller.state()["loading"].toBool()); QCOMPARE(alerts.size(), 3);
        httpStatus = 503; controller.refresh(); QTRY_COMPARE(controller.state()["status"].toString(), "offline");
        QCOMPARE(concern()["severity"].toInt(), 3); QCOMPARE(alerts.size(), 3);
        httpStatus = 200; used = 0; reset = now.addSecs(18000).toString(Qt::ISODate);
        controller.refresh(); QTRY_COMPARE(controller.state()["status"].toString(), "ready");
        QCOMPARE(concern()["severity"].toInt(), 0); QCOMPARE(alerts.size(), 3);
        used = 94; reset = QDateTime::currentDateTimeUtc().addSecs(2).toString(Qt::ISODateWithMs);
        controller.refresh(); QTRY_VERIFY(!controller.state()["loading"].toBool());
        QCOMPARE(concern()["severity"].toInt(), 0);
        QTest::qWait(2200); // A cached near-reset reading must not become a new critical alert.
        controller.refresh(); QTRY_VERIFY(!controller.state()["loading"].toBool());
        QCOMPARE(concern()["severity"].toInt(), 0); QCOMPARE(alerts.size(), 3);
        QVERIFY(!concern()["available"].toBool());
        QVERIFY(concern()["detail"].toString().contains("reset time has passed"));
    }
    void connectionLifecycle() {
        QTemporaryDir dir; const QString path = dir.filePath("settings.json");
        QTcpServer server; QVERIFY(server.listen(QHostAddress::LocalHost));
        int status = 200; QByteArray body = Usage::demo(); QByteArray received;
        connect(&server, &QTcpServer::newConnection, this, [&] {
            auto socket = server.nextPendingConnection();
            connect(socket, &QTcpSocket::disconnected, socket, &QObject::deleteLater);
            connect(socket, &QTcpSocket::readyRead, socket, [&, socket] {
                const QByteArray request = socket->readAll(); received += request;
                if (!received.endsWith("\r\n\r\n")) return;
                socket->write("HTTP/1.1 " + QByteArray::number(status) + " Test\r\nContent-Type: application/json\r\nContent-Length: " + QByteArray::number(body.size()) + "\r\nConnection: close\r\n\r\n" + body);
                socket->disconnectFromHost();
            });
        });
        Controller controller(false, path);
        QVERIFY(controller.saveSettings("remote", QString("http://127.0.0.1:%1/base/").arg(server.serverPort()), "test-secret", 60, false, "Claude", false).isEmpty());
        QTRY_COMPARE(controller.state()["status"].toString(), "ready");
        QVERIFY(received.startsWith("GET /base/api/v1/usage "));
        QVERIFY(HttpAssertions::hasHeader(received, "Authorization", "Bearer test-secret"));
        controller.moveProvider("Grok", "Claude", false);
        QCOMPARE(controller.primary(), QString("Grok"));
        QCOMPARE(controller.providers()[1].toMap()["provider_name"].toString(), QString("Claude"));
        controller.moveProvider("Claude", "Cursor", true);
        QCOMPARE(controller.providers()[3].toMap()["provider_name"].toString(), QString("Claude"));
        Controller orderReload(true, path);
        QTRY_COMPARE(orderReload.providers().size(), 4);
        QCOMPARE(orderReload.primary(), QString("Grok"));
        QCOMPARE(orderReload.providers()[3].toMap()["provider_name"].toString(), QString("Claude"));
        controller.moveProvider("unknown", "Grok", false);
        QCOMPARE(controller.primary(), QString("Grok"));
        const auto previous = controller.providers();
        status = 401; body = "{}"; received.clear(); controller.refresh();
        QTRY_COMPARE(controller.state()["status"].toString(), "offline"); QCOMPARE(controller.providers(), previous);
        QVERIFY(!controller.state()["message"].toString().contains("test-secret"));
        QCOMPARE(controller.state()["errorKind"].toString(), QString("auth"));
        QCOMPARE(controller.state()["retryAttempt"].toInt(), 1);
        QVERIFY(controller.state()["retrySeconds"].toInt() > 115);
        QVERIFY(!controller.diagnosticText().contains("test-secret"));
        QVERIFY(!controller.diagnosticText().contains("127.0.0.1"));
        for (int attempt : {2, 3, 4}) {
            controller.refresh(); QTRY_VERIFY(!controller.state()["loading"].toBool());
            QCOMPARE(controller.state()["retryAttempt"].toInt(), attempt);
            const int expected = attempt == 2 ? 240 : 300;
            QVERIFY(controller.state()["retrySeconds"].toInt() <= expected);
            QVERIFY(controller.state()["retrySeconds"].toInt() > expected - 5);
        }
#ifndef Q_OS_WIN
        // POSIX mode bits do not describe the Windows ACL used by QSaveFile.
        QVERIFY(!(QFile::permissions(path) & (QFileDevice::ReadGroup | QFileDevice::ReadOther)));
#endif
        Controller restored(false, path); QCOMPARE(restored.settings()["hasToken"].toBool(), true);
        QVERIFY(!restored.settings().contains("token"));
        received.clear(); status = 200; body = "[{}]"; controller.refresh();
        QTRY_COMPARE(controller.state()["status"].toString(), "offline"); QTRY_VERIFY(!controller.state()["loading"].toBool());
        QCOMPARE(controller.providers(), previous);
        QCOMPARE(controller.state()["errorKind"].toString(), QString("malformed"));
        status = 200; body = Usage::demo(); controller.refresh();
        QTRY_COMPARE(controller.state()["status"].toString(), "ready");
        QCOMPARE(controller.state()["retryAttempt"].toInt(), 0);
        QVERIFY(controller.state()["errorKind"].toString().isEmpty());
        QVERIFY(controller.state()["retrySeconds"].toInt() <= 60);
        QVERIFY(controller.diagnosticText().contains("Snapshot received"));
        controller.clearDiagnostics(); QVERIFY(controller.diagnostics().isEmpty());
        // Reordering is a synchronous event with no response payload or account labels.
        for (int i = 0; i < 502; ++i) controller.moveProvider(i % 2 ? "Claude" : "Grok", i % 2 ? "Grok" : "Claude", false);
        QCOMPARE(controller.diagnostics().size(), 500);
        QVERIFY(!controller.diagnosticText().contains("test-secret"));
    }
    void localUsageTransportFailureKeepsBackoff() {
        QTemporaryDir dir; QTcpServer server; QVERIFY(server.listen(QHostAddress::LocalHost));
        int healthRequests = 0, usageRequests = 0;
        connect(&server, &QTcpServer::newConnection, this, [&] {
            while (auto socket = server.nextPendingConnection()) {
                connect(socket, &QTcpSocket::readyRead, socket, [&, socket] {
                    if (socket->property("handled").toBool()) return;
                    const QByteArray request = socket->property("request").toByteArray() + socket->readAll();
                    socket->setProperty("request", request);
                    if (!request.contains("\r\n\r\n")) return;
                    socket->setProperty("handled", true);
                    if (request.startsWith("GET /api/v1/health ")) {
                        ++healthRequests;
                        const QByteArray body = R"({"status":"ok","version":"fixture","providers":[]})";
                        socket->write("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: "
                            + QByteArray::number(body.size()) + "\r\nConnection: close\r\n\r\n" + body);
                    } else if (request.startsWith("GET /api/v1/usage ")) ++usageRequests;
                    socket->disconnectFromHost();
                });
                connect(socket, &QTcpSocket::disconnected, socket, &QObject::deleteLater);
            }
        });
        ManagedServerOptions options; options.localUrl = QUrl(QString("http://127.0.0.1:%1/").arg(server.serverPort()));
        options.executablePath = dir.filePath("must-not-spawn"); options.probeTimeoutMs = 100;
        Controller controller(false, dir.filePath("settings.json"), nullptr, false, options);
        QVERIFY(controller.saveSettings("local", "", "", 60, false, "Claude", false).isEmpty());
        QTRY_COMPARE(controller.state()["status"].toString(), QString("offline"));
        QVERIFY(usageRequests >= 1 && usageRequests <= 2); // Qt may transparently retry one idempotent GET.
        QTRY_VERIFY(healthRequests >= 2);
        QVERIFY(controller.state()["retrySeconds"].toInt() > 100);
        QCOMPARE(controller.diagnosticText().count("Requesting usage snapshot."), 1);
        const int stableHealth = healthRequests, stableUsage = usageRequests;
        QTest::qWait(300);
        QCOMPARE(usageRequests, stableUsage); QCOMPARE(healthRequests, stableHealth);
        QCOMPARE(controller.diagnosticText().count("Requesting usage snapshot."), 1);
        QCOMPARE(controller.state()["status"].toString(), QString("offline"));
    }
    void localModeIgnoresHiddenInvalidRemoteAddress() {
        QTemporaryDir dir; ManagedServerOptions options;
        options.localUrl = QUrl(QString("http://127.0.0.1:%1/").arg(65534));
        options.executablePath = dir.filePath("missing-server"); options.probeTimeoutMs = 50;
        const QString path = dir.filePath("settings.json");
        SettingsService service(path, false); auto saved = service.value();
        saved.connectionMode = "remote"; saved.url = "https://example.test/base";
        QVERIFY(service.save(saved, true).isEmpty());
        Controller controller(false, path, nullptr, false, options);
        QVERIFY(controller.saveSettings("local", "not a valid hidden URL", "", 60, false, "Claude", false).isEmpty());
        QCOMPARE(controller.settings()["mode"].toString(), QString("local"));
        QCOMPARE(controller.settings()["url"].toString(), QString("https://example.test/base"));
        QCOMPARE(controller.backendUrl(), options.localUrl.toString());
    }
    void terminalLocalFailureCancelsStaleUsage() {
        QTemporaryDir dir; const auto record = dir.filePath("record");
        QTcpServer reservation; QVERIFY(reservation.listen(QHostAddress::LocalHost));
        const quint16 port = reservation.serverPort(); reservation.close();
        qputenv("HEADROOM_FIXTURE_MODE", "ready-crash"); qputenv("HEADROOM_FIXTURE_RECORD", record.toUtf8());
        ManagedServerOptions options;
        options.localUrl = QUrl(QString("http://127.0.0.1:%1/").arg(port));
        options.executablePath = QStringLiteral(MANAGED_FIXTURE_PATH);
        options.probeTimeoutMs = 100; options.readinessIntervalMs = 25; options.readinessAttempts = 30; options.restartLimit = 2;
        Controller controller(false, dir.filePath("settings.json"), nullptr, false, options);
        QVERIFY(controller.saveSettings("local", "", "", 60, false, "Claude", false).isEmpty());
        QTRY_COMPARE_WITH_TIMEOUT(controller.state()["errorKind"].toString(), QString("restart"), 15000);
        QCOMPARE(controller.state()["status"].toString(), QString("offline"));
        QVERIFY(!controller.state()["loading"].toBool());
        QCOMPARE(controller.state()["retrySeconds"].toInt(), 0);
        QFile file(record); QVERIFY(file.open(QIODevice::ReadOnly)); QCOMPARE(file.readAll().count("start\n"), 3);
        QTest::qWait(300);
        QCOMPARE(controller.state()["status"].toString(), QString("offline"));
        QVERIFY(!controller.state()["loading"].toBool());
        file.seek(0); QCOMPARE(file.readAll().count("start\n"), 3);
        qunsetenv("HEADROOM_FIXTURE_MODE"); qunsetenv("HEADROOM_FIXTURE_RECORD");
    }
};
QTEST_GUILESS_MAIN(UsageTest)
#include "test_usage.moc"
