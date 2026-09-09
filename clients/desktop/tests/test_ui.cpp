#include "controller.h"
#include "usage.h"
#include "startup.h"
#include "appinfo.h"
#include "updateservice.h"
#include "palette.h"
#include <QApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickWindow>
#include <QQuickStyle>
#include <QQuickItem>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QtTest>
#include <cmath>

QQuickItem *findItem(QQuickItem *root, const QString &name) {
    if (root->objectName() == name) return root;
    for (auto child : root->childItems()) if (auto found = findItem(child, name)) return found;
    return nullptr;
}
class UiTest : public QObject {
    Q_OBJECT
private slots:
    void dragReordersAndDrivesTray() {
        QTemporaryDir dir;
        const auto capture = [&](const QString &name) {
            return dir.filePath(name);
        };
        CredentialServiceOptions credentialOptions; credentialOptions.enabled = false;
        Controller controller(true, dir.filePath("settings.json"), nullptr, true, {}, credentialOptions);
        StartupService startup(dir.path(), QCoreApplication::applicationFilePath(), false);
        AppInfo appInfo;
        UpdateService updateService(false);
        QQmlApplicationEngine engine;
        engine.rootContext()->setContextProperty("backend", &controller);
        engine.rootContext()->setContextProperty("startupService", &startup);
        engine.rootContext()->setContextProperty("appInfo", &appInfo);
        engine.rootContext()->setContextProperty("updateService", &updateService);
        engine.rootContext()->setContextProperty("trayAvailable", false);
        engine.rootContext()->setContextProperty("startHidden", false);
        engine.rootContext()->setContextProperty("captureMode", true);
        engine.load(QUrl::fromLocalFile(QString(SOURCE_DIR) + "/qml/Main.qml"));
        QVERIFY(!engine.rootObjects().isEmpty());
        auto window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
        QVERIFY(window); QVERIFY(QTest::qWaitForWindowExposed(window));
        QCOMPARE(QQuickStyle::name(), QString("Basic"));
        QVERIFY(window->flags().testFlag(Qt::FramelessWindowHint));
        QVERIFY(!window->flags().testFlag(Qt::WindowMinMaxButtonsHint));
        QTRY_COMPARE(controller.providers().size(), 4);
        auto marker = findItem(window->contentItem(), "paceMarker_Claude_session");
        auto label = findItem(window->contentItem(), "paceLabel_Claude_session");
        QVERIFY(marker); QVERIFY(marker->isVisible()); QVERIFY(label);
        QCOMPARE(marker->property("color").value<QColor>(), QColor("#8be9fd"));
        QVERIFY(label->property("text").toString().contains("under pace"));
        const double markerFraction = (marker->x() + marker->width() / 2) / marker->parentItem()->width();
        QVERIFY(std::abs(markerFraction - (1.0 - 8400.0 / 18000)) < 0.01);
        auto verifyNotches = [&]() {
            struct Scale { QString name; int count; double step; };
            for (const auto &scale : {Scale{"Claude_session", 4, 1.0 / 5},
                    Scale{"Claude_weekly", 6, 1.0 / 7}, Scale{"Cursor_auto", 4, 7.0 / 30}}) {
                QTRY_VERIFY_WITH_TIMEOUT(findItem(window->contentItem(), "meter_" + scale.name), 1000);
                auto scaleMeter = findItem(window->contentItem(), "meter_" + scale.name); QVERIFY(scaleMeter);
                const auto ticks = controller.notches(scale.name.section('_', 0, 0), scaleMeter->property("bucket").toMap());
                QCOMPARE(ticks.size(), scale.count);
                for (int i = 0; i < scale.count; ++i) {
                    QVERIFY(std::abs(ticks[i].toMap()["fraction"].toDouble() - scale.step * (i + 1)) < 0.000001);
                }
            }
        };
        verifyNotches();
        auto cursorLabel = findItem(window->contentItem(), "meterLabel_Cursor_auto");
        QVERIFY(cursorLabel); QCOMPARE(cursorLabel->property("text").toString(), QString("Cursor Models"));
        QVERIFY(findItem(window->contentItem(), "meter_Claude_weekly_fable"));
        QVERIFY(findItem(window->contentItem(), "meter_Cursor_weekly_grok_bot"));
        auto statusOnlyGraph = findItem(window->contentItem(), "usageGraph_Cursor_on_demand");
        QVERIFY(statusOnlyGraph); QVERIFY(!statusOnlyGraph->isVisible());
        auto statusOnlyText = findItem(window->contentItem(), "meterStatus_Cursor_on_demand");
        QVERIFY(statusOnlyText); QCOMPARE(statusOnlyText->property("text").toString(), QString("On-demand enabled"));
        auto measuredStatus = findItem(window->contentItem(), "meterStatus_Cursor_api");
        auto measuredReset = findItem(window->contentItem(), "meterReset_Cursor_api");
        QVERIFY(measuredStatus); QVERIFY(measuredStatus->isVisible()); QVERIFY(measuredReset); QVERIFY(!measuredReset->isVisible());
        QVERIFY(!findItem(window->contentItem(), "meter_Grok_session"));
        const auto claudeCard = findItem(window->contentItem(), "providerCard_Claude");
        const auto grokCard = findItem(window->contentItem(), "providerCard_Grok");
        QVERIFY(claudeCard); QVERIFY(grokCard);
        auto hover = claudeCard->findChild<QObject *>("providerHover_Claude"); QVERIFY(hover);
        auto track = findItem(window->contentItem(), "meterTrack_Claude_session"); QVERIFY(track);
        auto fill = findItem(window->contentItem(), "meterFill_Claude_session"); QVERIFY(fill);
        const auto restingColor = claudeCard->property("color").value<QColor>();
        QTest::mouseMove(window, track->mapToScene(QPointF(track->width() / 2, 3)).toPoint());
        QTRY_VERIFY(hover->property("hovered").toBool());
        QCOMPARE(claudeCard->property("color").value<QColor>(), restingColor);
        QVERIFY(track->property("color").value<QColor>() != restingColor);
        QCOMPARE(fill->property("color").value<QColor>(), QColor("#bd93f9"));
        QVERIFY(window->grabWindow().save(capture("headroom-hover.png")));
        auto meter = findItem(window->contentItem(), "meter_Claude_session"); QVERIFY(meter);
        const auto originalBucket = meter->property("bucket").toMap();
        const auto originalConcern = meter->property("concern").toMap();
        for (double used : {52.0, 60.0, 68.0, 80.0, 100.0}) {
            auto bucket = originalBucket; bucket["utilization"] = used;
            bucket["resets_at"] = QDateTime::currentDateTimeUtc().addSecs(9000).toString(Qt::ISODate);
            QVERIFY(meter->setProperty("bucket", bucket));
            QVERIFY(meter->setProperty("concern", Usage::concern("Claude", bucket)));
            const QColor expected(used >= 80 ? "#ff5555" : used >= 68 ? "#ffb86c" : used >= 60 ? "#f1fa8c" : "#bd93f9");
            QTRY_COMPARE(fill->property("color").value<QColor>(), expected);
        }
        QTRY_COMPARE(fill->width(), track->width());
        QVERIFY(window->grabWindow().save(capture("headroom-critical.png")));
        QVERIFY(meter->setProperty("bucket", originalBucket));
        QVERIFY(meter->setProperty("concern", originalConcern));
        auto rows = findItem(window->contentItem(), "providerRows"); QVERIFY(rows);
        QQuickItem *previous = nullptr;
        for (const auto &name : {"Claude", "Codex", "Cursor", "Grok"}) {
            auto row = findItem(window->contentItem(), "providerCard_" + QString(name));
            QVERIFY(row); QCOMPARE(row->width(), rows->width());
            if (previous) QVERIFY(std::abs(row->y() - previous->y() - previous->height() - 12) < 1);
            previous = row;
        }
        auto chatgpt = findItem(window->contentItem(), "providerLabel_Codex");
        QVERIFY(chatgpt); QCOMPARE(chatgpt->property("text").toString(), QString("ChatGPT"));
        auto source = findItem(window->contentItem(), "dragHandle_Codex");
        auto target = findItem(window->contentItem(), "dragHandle_Claude");
        QVERIFY(source); QVERIFY(target);
        const QPoint from = source->mapToScene(QPointF(14, 18)).toPoint();
        const QPoint to = claudeCard->mapToScene(QPointF(claudeCard->width() / 2, 20)).toPoint();
        QTest::mousePress(window, Qt::LeftButton, Qt::NoModifier, from);
        for (int i = 1; i <= 20; ++i) QTest::mouseMove(window, from + (to - from) * i / 20, 15);
        QTest::mouseRelease(window, Qt::LeftButton, Qt::NoModifier, to);
        QTRY_COMPARE(controller.primary(), QString("Codex"));
        QCOMPARE(controller.providers()[0].toMap()["provider_name"].toString(), QString("Codex"));
        QVERIFY(window->grabWindow().save(capture("headroom-reordered.png")));
        auto filterButton = findItem(window->contentItem(), "providerFilter"); QVERIFY(filterButton);
        QTest::mouseClick(window, Qt::LeftButton, Qt::NoModifier,
                          filterButton->mapToScene(QPointF(filterButton->width() / 2, filterButton->height() / 2)).toPoint());
        auto filterMenu = window->findChild<QObject *>("providerFilterMenu"); QVERIFY(filterMenu);
        QTRY_VERIFY(filterMenu->property("opened").toBool());
        QTest::qWait(200); QVERIFY(window->isVisible());
        QVERIFY(QMetaObject::invokeMethod(filterMenu, "close"));
        QVERIFY(window->setProperty("filter", QStringLiteral("All providers")));
        auto panel = window->findChild<QObject *>("settingsPanel"); QVERIFY(panel);
        QVERIFY(QMetaObject::invokeMethod(panel, "open"));
        QTest::qWait(150);
        auto save = findItem(window->contentItem(), "saveConnection"); QVERIFY(save);
        QVERIFY(save->isVisible());
        auto localMode = findItem(window->contentItem(), "localMode");
        auto remoteMode = findItem(window->contentItem(), "remoteMode");
        QVERIFY(localMode); QVERIFY(remoteMode);
        const bool startsLocal = controller.settings()["mode"].toString() == QStringLiteral("local");
        QCOMPARE(localMode->property("checked").toBool(), startsLocal);
        QCOMPARE(remoteMode->property("checked").toBool(), !startsLocal);
        auto backendUrl = findItem(window->contentItem(), "backendUrl"); QVERIFY(backendUrl);
        QCOMPARE(backendUrl->isVisible(), !startsLocal);
        if (startsLocal) {
            QVERIFY(remoteMode->setProperty("checked", true));
            QTRY_VERIFY(!localMode->property("checked").toBool());
            QTRY_VERIFY(backendUrl->isVisible());
        }
        QVERIFY(localMode->setProperty("checked", true)); QTRY_VERIFY(!remoteMode->property("checked").toBool());
        QTRY_VERIFY(!backendUrl->isVisible());
        QVERIFY(window->grabWindow().save(capture("headroom-settings.png")));
        auto settingsScroll = window->findChild<QObject *>("settingsScroll"); QVERIFY(settingsScroll);
        auto flickable = settingsScroll->property("contentItem").value<QObject *>(); QVERIFY(flickable);
        QVERIFY(flickable->setProperty("contentY", flickable->property("contentHeight").toDouble() - flickable->property("height").toDouble()));
        QTest::qWait(100);
        QVERIFY(window->grabWindow().save(capture("headroom-settings-lower.png")));
        QVERIFY(QMetaObject::invokeMethod(panel, "saveAndConnect"));
        QTRY_VERIFY(!panel->property("opened").toBool());
        QCOMPARE(controller.settings()["interval"].toInt(), 60);
        auto diagnostics = window->findChild<QObject *>("diagnosticsPanel"); QVERIFY(diagnostics);
        QVERIFY(QMetaObject::invokeMethod(diagnostics, "open")); QTest::qWait(100);
        auto log = findItem(window->contentItem(), "diagnosticLog"); QVERIFY(log);
        QVERIFY(log->property("text").toString().contains("Usage monitor started"));
        QVERIFY(window->grabWindow().save(capture("headroom-diagnostics.png")));
        controller.clearDiagnostics(); QTRY_COMPARE(log->property("text").toString(), QString());
        QVERIFY(QMetaObject::invokeMethod(diagnostics, "close"));
        for (int width : {820, 460}) {
            window->resize(width, 800); QTest::qWait(100);
            QQuickItem *prior = nullptr;
            for (const auto &value : controller.providers()) {
                auto provider = value.toMap();
                auto name = provider["provider_name"].toString();
                auto row = findItem(window->contentItem(), "providerCard_" + name); QVERIFY(row);
                QCOMPARE(row->width(), rows->width());
                if (prior) QVERIFY(std::abs(row->y() - prior->y() - prior->height() - 12) < 1);
                bool firstMeter = true;
                for (const auto &bucket : provider["buckets"].toList()) {
                    const auto bucketId = bucket.toMap()["id"].toString();
                    auto meter = findItem(window->contentItem(), "meter_" + name + "_" + bucketId);
                    QVERIFY(meter);
                    QCOMPARE(meter->property("accent").value<QColor>(), QColor("#bd93f9"));
                    if (firstMeter) QCOMPARE(meter->width(), meter->parentItem()->width());
                    firstMeter = false;
                    const auto origin = meter->mapToItem(row, QPointF(0, 0));
                    const QString geometry = QString(
                        "%1/%2 at window %3: origin=(%4,%5), meter=%6x%7, row=%8x%9")
                        .arg(name, bucketId).arg(width).arg(origin.x()).arg(origin.y())
                        .arg(meter->width()).arg(meter->height()).arg(row->width()).arg(row->height());
                    QVERIFY2(origin.x() >= 0, qPrintable(geometry));
                    QVERIFY2(origin.y() >= 0, qPrintable(geometry));
                    QVERIFY2(origin.x() + meter->width() <= row->width() + 1, qPrintable(geometry));
                    QVERIFY2(origin.y() + meter->height() <= row->height() + 1, qPrintable(geometry));
                }
                prior = row;
            }
            QVERIFY(window->grabWindow().save(capture(width == 460 ? "headroom-compact.png" : "headroom-medium.png")));
        }
    }
    void preservesAndDisplaysCustomInterval() {
        QTemporaryDir dir; QVERIFY(dir.isValid());
        QFile settings(dir.filePath("settings.json")); QVERIFY(settings.open(QIODevice::WriteOnly));
        QVERIFY(settings.write(R"({"schemaVersion":1,"connectionMode":"remote","url":"","token":"","interval":900,"notifications":true,"primary":"Claude","order":["Claude","Codex","Cursor","Grok"]})") > 0);
        settings.close();
        CredentialServiceOptions credentials; credentials.enabled = false;
        Controller controller(true, settings.fileName(), nullptr, true, {}, credentials);
        StartupService startup(dir.path(), QCoreApplication::applicationFilePath(), false);
        AppInfo appInfo; UpdateService updateService(false);
        QQmlApplicationEngine engine;
        engine.rootContext()->setContextProperty("backend", &controller);
        engine.rootContext()->setContextProperty("startupService", &startup);
        engine.rootContext()->setContextProperty("appInfo", &appInfo);
        engine.rootContext()->setContextProperty("updateService", &updateService);
        engine.rootContext()->setContextProperty("trayAvailable", false);
        engine.rootContext()->setContextProperty("startHidden", false);
        engine.rootContext()->setContextProperty("captureMode", true);
        engine.load(QUrl::fromLocalFile(QString(SOURCE_DIR) + "/qml/Main.qml"));
        QVERIFY(!engine.rootObjects().isEmpty()); auto window = qobject_cast<QQuickWindow *>(engine.rootObjects().first()); QVERIFY(window);
        QVERIFY(QTest::qWaitForWindowExposed(window));
        auto panel = window->findChild<QObject *>("settingsPanel"); QVERIFY(panel); QVERIFY(QMetaObject::invokeMethod(panel, "open"));
        QTRY_VERIFY(panel->property("opened").toBool()); QCOMPARE(panel->property("selectedInterval").toInt(), 900);
        auto interval = findItem(window->contentItem(), "refreshInterval"); QVERIFY(interval);
        QCOMPARE(interval->property("displayText").toString(), QString("15 minutes"));
        auto save = findItem(window->contentItem(), "saveConnection"); QVERIFY(save);
        QVERIFY(QMetaObject::invokeMethod(panel, "saveAndConnect"));
        QCOMPARE(controller.settings()["interval"].toInt(), 900);
        QVERIFY(QMetaObject::invokeMethod(panel, "close"));
    }
    void rendersOneTwoFourAndTwelveMeters() {
        QTemporaryDir dir; QVERIFY(dir.isValid());
        QJsonArray providers;
        const QList<QPair<QString, int>> shapes{{"Claude", 1}, {"Codex", 2}, {"Cursor", 4}, {"Grok", 12}};
        for (const auto &[name, count] : shapes) {
            QJsonArray buckets;
            for (int i = 0; i < count; ++i) {
                buckets.append(QJsonObject{{"id", QString("meter_%1").arg(i)},
                    {"label", QString("Allowance 測定 %1").arg(i + 1)}, {"utilization", double((i * 7 + 13) % 101)},
                    {"resets_at", QDateTime::currentDateTimeUtc().addDays(7).toString(Qt::ISODate)},
                    {"status_text", i == count - 1 ? QJsonValue(QString("Status %1 / %2").arg(i + 1).arg(count)) : QJsonValue(QJsonValue::Null)}});
            }
            providers.append(QJsonObject{{"provider_name", name}, {"is_success", true}, {"error", QJsonValue::Null},
                {"needs_reauth", false}, {"reauth_command", QJsonValue::Null}, {"buckets", buckets}});
        }
        CredentialServiceOptions credentials; credentials.enabled = false;
        Controller controller(true, dir.filePath("settings.json"), nullptr, true, {}, credentials,
                              QJsonDocument(providers).toJson(QJsonDocument::Compact));
        StartupService startup(dir.path(), QCoreApplication::applicationFilePath(), false);
        AppInfo appInfo; UpdateService updateService(false);
        QQmlApplicationEngine engine;
        engine.rootContext()->setContextProperty("backend", &controller);
        engine.rootContext()->setContextProperty("startupService", &startup);
        engine.rootContext()->setContextProperty("appInfo", &appInfo);
        engine.rootContext()->setContextProperty("updateService", &updateService);
        engine.rootContext()->setContextProperty("trayAvailable", false);
        engine.rootContext()->setContextProperty("startHidden", false);
        engine.rootContext()->setContextProperty("captureMode", true);
        engine.load(QUrl::fromLocalFile(QString(SOURCE_DIR) + "/qml/Main.qml"));
        QVERIFY(!engine.rootObjects().isEmpty());
        auto window = qobject_cast<QQuickWindow *>(engine.rootObjects().first()); QVERIFY(window);
        QVERIFY(QTest::qWaitForWindowExposed(window)); QTRY_COMPARE(controller.providers().size(), 4);
        auto rows = findItem(window->contentItem(), "providerRows");
        auto footer = findItem(window->contentItem(), "stickyFooter");
        QVERIFY(rows); QVERIFY(footer);
        for (int width : {960, 420}) {
            window->resize(width, 900); QTest::qWait(50);
            QVERIFY(footer->y() >= 0); QVERIFY(footer->y() + footer->height() <= window->height() + 1);
            for (const auto &[name, count] : shapes) {
                auto card = findItem(window->contentItem(), "providerCard_" + name); QVERIFY(card);
                for (int i = 0; i < count; ++i) {
                    auto meter = findItem(window->contentItem(), QString("meter_%1_meter_%2").arg(name).arg(i));
                    QVERIFY2(meter, qPrintable(QString("missing %1 meter %2").arg(name).arg(i)));
                    const auto at = meter->mapToItem(card, QPointF());
                    QVERIFY(at.x() >= -1 && at.y() >= -1);
                    QVERIFY(at.x() + meter->width() <= card->width() + 1);
                    QVERIFY(at.y() + meter->height() <= card->height() + 1);
                    if (width == 420 && i == 0) QCOMPARE(meter->width(), meter->parentItem()->width());
                }
            }
        }
    }
};
int main(int argc, char **argv) { QQuickStyle::setStyle("Basic"); QApplication app(argc, argv); app.setPalette(headroomPalette()); app.setApplicationVersion(HEADROOM_VERSION); UiTest test; return QTest::qExec(&test, argc, argv); }
#include "test_ui.moc"
