#include "trayvisual.h"
#include "usage.h"
#include <QtTest>

class TrayTest : public QObject {
    Q_OBJECT
    const QDateTime now = QDateTime::fromString("2026-09-08T12:00:00Z", Qt::ISODate);
    QVariantMap meter(QString id, double used, int remaining = 9000) const {
        return {{"id", id}, {"label", id == "session" ? "5-Hour" : "Weekly"}, {"utilization", used},
                {"resets_at", now.addSecs(remaining).toString(Qt::ISODate)}};
    }
    QVariantMap provider(QString name, QVariantList buckets, bool success = true) const {
        return {{"provider_name", name}, {"is_success", success}, {"buckets", buckets}};
    }
    QVariantMap ready() const { return {{"status", "ready"}, {"lastGood", now.toSecsSinceEpoch()}, {"updated", "Just updated"}}; }
    TrayVisual::Assessment assess = [](const QString &, const QVariantMap &bucket) {
        // Deliberately disagree with numeric usage: this must use the state machine's cached tier.
        return QVariantMap{{"severity", bucket["id"] == "session" ? 1 : 3}};
    };
private slots:
    void usesSharedLevelsForBothIndicators() {
        const auto model = TrayVisual::build(ready(), {provider("Codex", {meter("session", 12), meter("weekly", 8, 86400)})}, "Codex", assess, now);
        QCOMPARE(model.kind, TrayVisual::Kind::Usage);
        QCOMPARE(model.level, Usage::WarningLevel::Watch);
        QCOMPARE(model.secondary, Usage::WarningLevel::Critical);
        QCOMPARE(model.used, 12);
        QCOMPARE(model.expected, 50);
        QVERIFY(model.tooltip.startsWith("ChatGPT · 12% used"));
        QCOMPARE(model.tooltip.count('\n'), 1);
        QVERIFY(model.tooltip.contains("Resets in 2h 30m · Critical"));
        QVERIFY(!model.tooltip.contains("Weekly"));
        const auto image = TrayVisual::icon(model).pixmap(64, 64).toImage();
        QVERIFY(!image.isNull());
        QCOMPARE(image.pixelColor(53, 52), QColor("#ff5555"));
        QCOMPARE(image.pixelColor(32, 59), QColor("#8be9fd"));
    }
    void keepsSelectedProviderOnError() {
        const auto model = TrayVisual::build(ready(), {
            provider("Claude", {}, false), provider("Codex", {meter("session", 60), meter("weekly", 99)})}, "Claude", assess, now);
        QCOMPARE(model.kind, TrayVisual::Kind::ProviderError);
        QCOMPARE(model.provider, QString("Claude"));
        QCOMPARE(model.used, -1);
        QCOMPARE(model.secondary, Usage::WarningLevel::Normal);
        QVERIFY(model.tooltip.contains("Provider unavailable"));
        QVERIFY(!model.tooltip.contains("ChatGPT"));
    }
    void distinguishesConnectionStates_data() {
        QTest::addColumn<QString>("status"); QTest::addColumn<QString>("error"); QTest::addColumn<int>("kind");
        using K = TrayVisual::Kind;
        QTest::newRow("setup") << "setup" << "" << int(K::Setup);
        QTest::newRow("connecting") << "connecting" << "" << int(K::Connecting);
        QTest::newRow("offline") << "offline" << "network" << int(K::Offline);
        QTest::newRow("token") << "offline" << "auth" << int(K::AuthError);
        QTest::newRow("http") << "offline" << "api" << int(K::ApiError);
        QTest::newRow("malformed") << "offline" << "malformed" << int(K::Malformed);
    }
    void distinguishesConnectionStates() {
        QFETCH(QString, status); QFETCH(QString, error); QFETCH(int, kind);
        auto state = ready(); state["status"] = status; state["errorKind"] = error; state["retrySeconds"] = 30;
        const auto model = TrayVisual::build(state, {provider("Claude", {meter("session", 40)})}, "Claude", assess, now);
        QCOMPARE(int(model.kind), kind);
        QVERIFY(!TrayVisual::icon(model).isNull());
        QCOMPARE(model.tooltip.count('\n'), 1);
    }
    void loadingWithoutGoodDataAndBackgroundRefresh() {
        auto state = ready(); state["loading"] = true; state["lastGood"] = 0;
        QCOMPARE(TrayVisual::build(state, {}, "Claude", assess, now).kind, TrayVisual::Kind::Connecting);
        state["lastGood"] = now.toSecsSinceEpoch();
        QCOMPARE(TrayVisual::build(state, {provider("Claude", {meter("session", 40)})}, "Claude", assess, now).kind, TrayVisual::Kind::Usage);
    }
    void exhaustionAndExpiredPacing() {
        const auto model = TrayVisual::build(ready(), {provider("Claude", {meter("session", 100, -60)})}, "Claude", assess, now);
        QCOMPARE(model.kind, TrayVisual::Kind::Exhausted);
        QCOMPARE(model.expected, -1);
        QVERIFY(model.tooltip.contains("awaiting reset update"));
        QVERIFY(model.tooltip.contains("100% used"));
        // The graphic uses the supplied tier even when a synthetic test's level is unusual.
        QCOMPARE(model.level, Usage::WarningLevel::Watch);
    }
    void billingStatusAndMissingSelectedProvider() {
        auto billing = meter("on_demand", 0); billing["label"] = "On-Demand"; billing["status_text"] = "$4.00 billed";
        const auto model = TrayVisual::build(ready(), {provider("Cursor", {meter("session", 40), billing})}, "Cursor", assess, now);
        QVERIFY(!model.tooltip.contains("On-Demand"));
        QVERIFY(!model.tooltip.contains("On-Demand · 0%"));
        QCOMPARE(model.secondary, Usage::WarningLevel::Normal);
        const auto missing = TrayVisual::build(ready(), {provider("Cursor", {meter("session", 40)})}, "Claude", assess, now);
        QCOMPARE(missing.kind, TrayVisual::Kind::Idle);
        QCOMPARE(missing.used, -1);
    }
};
QTEST_MAIN(TrayTest)
#include "test_tray.moc"
