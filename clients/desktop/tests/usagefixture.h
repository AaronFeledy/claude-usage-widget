#pragma once
// Synthetic readings are linked into tests only, never into the desktop app.
#include "controller.h"
#include "usage.h"
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <utility>

namespace TestUsage {
inline QByteArray snapshot() {
    // Synthetic readings, using the real API's labels and variable bucket shapes.
    // Actual accounts may expose other model-specific or billable buckets.
    const auto now = QDateTime::currentDateTimeUtc();
    auto bucket = [&](const QString &id, const QString &label, double used, int seconds, const QString &status = {}) {
        return QJsonObject{{"id", id}, {"label", label}, {"utilization", used},
            {"resets_at", seconds ? QJsonValue(now.addSecs(seconds).toString(Qt::ISODate)) : QJsonValue(QJsonValue::Null)},
            {"status_text", status.isEmpty() ? QJsonValue(QJsonValue::Null) : QJsonValue(status)}};
    };
    const QList<QJsonArray> meters {
        {bucket("session", "Current Session", 34, 8400), bucket("weekly", "Weekly", 62, 225000), bucket("weekly_fable", "Fable", 27, 225000)},
        {bucket("session", "5-Hour", 0, 0), bucket("weekly", "Weekly", 41, 228000)},
        {bucket("auto", "Cursor Models", 38, 12 * 86400), bucket("api", "Other Models", 76, 12 * 86400, "$38 / $50 this cycle"),
            bucket("weekly_grok_bot", "Grok Bot", 28, 231000), bucket("on_demand", "On-Demand", 0, 12 * 86400, "On-demand enabled")},
        {bucket("weekly", "Weekly", 8, 234000)}
    };
    const QStringList names {"Claude", "Codex", "Cursor", "Grok"};
    QJsonArray providers;
    for (int i = 0; i < names.size(); ++i) {
        QJsonObject provider{{"provider_name", names[i]}, {"subtitle", "Sample account"}, {"error", QJsonValue::Null},
            {"is_success", true}, {"needs_reauth", false}, {"buckets", meters[i]}};
        if (names[i] == "Codex") provider["rate_limit_reset_credits"] = QJsonObject{{"available_count", 3}};
        providers.append(provider);
    }
    return QJsonDocument(providers).toJson();
}

}

class ControllerFixture final : public Controller {
public:
    explicit ControllerFixture(const QString &settingsPath, QByteArray payload = TestUsage::snapshot())
        : Controller(settingsPath, nullptr, false, {}, disabledCredentials(), {}, false), m_payload(std::move(payload)) {
        QTimer::singleShot(0, this, &ControllerFixture::refresh);
    }
    void refresh() override {
        QVariantList providers;
        if (!Usage::parse(m_payload, providers)) qFatal("Invalid test usage fixture");
        acceptSnapshot(providers);
    }
private:
    static CredentialServiceOptions disabledCredentials() {
        CredentialServiceOptions options; options.enabled = false; return options;
    }
    QByteArray m_payload;
};
