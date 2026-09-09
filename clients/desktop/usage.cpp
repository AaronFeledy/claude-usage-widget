#include "usage.h"
#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSet>
#include <QRegularExpression>
#include <cmath>

QString Usage::displayName(const QString &provider) {
    return provider.compare("Codex", Qt::CaseInsensitive) == 0 ? QString("ChatGPT") : provider;
}

QUrl Usage::endpoint(const QString &base) {
    const QUrl url(base.trimmed(), QUrl::StrictMode);
    if (!url.isValid() || (url.scheme() != "http" && url.scheme() != "https") || url.host().isEmpty()
        || !url.userInfo().isEmpty() || url.hasQuery() || url.hasFragment()) return {};
    QUrl result(url);
    result.setPath(url.path().remove(QRegularExpression("/+$")) + "/api/v1/usage");
    return result;
}

bool Usage::parse(const QByteArray &json, QVariantList &providers) {
    QJsonParseError error;
    const auto doc = QJsonDocument::fromJson(json, &error);
    if (error.error != QJsonParseError::NoError || !doc.isArray() || doc.array().size() > 64) return false;
    QVariantList result;
    QSet<QString> names;
    for (const auto &value : doc.array()) {
        if (!value.isObject()) return false;
        auto p = value.toObject();
        const auto name = p["provider_name"].toString().trimmed();
        if (name.isEmpty() || names.contains(name.toLower()) || !p.contains("error")
            || (!p["error"].isNull() && !p["error"].isString())
            || !p["is_success"].isBool() || p["is_success"].toBool() != p["error"].isNull()
            || !p["needs_reauth"].isBool()) return false;
        names.insert(name.toLower());
        QJsonArray buckets;
        if (p["error"].isNull()) {
            if (p.contains("buckets") && !p["buckets"].isArray()) return false;
            buckets = p["buckets"].toArray();
            if (buckets.isEmpty()) {
                if (!p["current"].isObject() || !p["show_secondary"].isBool()) return false;
                auto primary = p["current"].toObject();
                primary["id"] = "session"; primary["label"] = p["primary_label"];
                primary["status_text"] = p["primary_status_text"];
                buckets.append(primary);
                if (p["show_secondary"].toBool()) {
                    auto weekly = p["weekly"].toObject();
                    weekly["id"] = "weekly"; weekly["label"] = p["secondary_label"];
                    weekly["status_text"] = p["secondary_status_text"];
                    buckets.append(weekly);
                }
            }
        }
        if (buckets.size() > 12) return false;
        QSet<QString> ids;
        for (const auto &item : buckets) {
            if (!item.isObject()) return false;
            const auto b = item.toObject();
            const double utilization = b["utilization"].toDouble(-1);
            const QString id = b["id"].toString();
            if (id.isEmpty() || ids.contains(id) || b["label"].toString().trimmed().isEmpty()
                || !b["utilization"].isDouble() || !std::isfinite(utilization) || utilization < 0 || utilization > 100) return false;
            if (!b["resets_at"].isNull() && !b["resets_at"].isUndefined()
                && (!b["resets_at"].isString() || !QDateTime::fromString(b["resets_at"].toString(), Qt::ISODateWithMs).isValid())) return false;
            if (!b["status_text"].isUndefined() && !b["status_text"].isNull() && !b["status_text"].isString()) return false;
            ids.insert(id);
        }
        // Match Windows' presentation fallback for older API responses. Auto
        // keeps its countdown; Other Models uses the primary billing status.
        for (int i = 0; i < buckets.size(); ++i) {
            auto b = buckets[i].toObject();
            if (b["status_text"].toString().trimmed().isEmpty()) {
                const auto id = b["id"].toString().toLower();
                if ((id == "api" || (i == 0 && id != "auto")) && !p["primary_status_text"].toString().trimmed().isEmpty())
                    b["status_text"] = p["primary_status_text"];
                else if ((id == "weekly" || id == "on_demand") && !p["secondary_status_text"].toString().trimmed().isEmpty())
                    b["status_text"] = p["secondary_status_text"];
            }
            buckets[i] = b;
        }
        p["buckets"] = buckets;
        p["provider_name"] = name;
        p["display_name"] = displayName(name);
        result.append(p.toVariantMap());
    }
    const QStringList order {"claude", "codex", "cursor", "grok"};
    std::stable_sort(result.begin(), result.end(), [&](const QVariant &a, const QVariant &b) {
        auto rank = [&](const QVariant &v) { auto i = order.indexOf(v.toMap()["provider_name"].toString().toLower()); return i < 0 ? 99 : i; };
        return rank(a) < rank(b);
    });
    providers = result;
    return true;
}

QString Usage::countdown(const QString &timestamp) {
    const auto when = QDateTime::fromString(timestamp, Qt::ISODateWithMs);
    if (!when.isValid()) return "No scheduled reset";
    const auto seconds = QDateTime::currentDateTimeUtc().secsTo(when);
    if (seconds <= 0) return "Reset pending";
    const auto minutes = (seconds + 59) / 60;
    if (minutes >= 1440) return QString("Resets in %1d %2h").arg(minutes / 1440).arg((minutes % 1440) / 60);
    if (minutes >= 60) return QString("Resets in %1h %2m").arg(minutes / 60).arg(minutes % 60);
    return QString("Resets in %1m").arg(minutes);
}

QVariantMap Usage::period(const QString &provider, const QVariantMap &bucket) {
    const QString name = provider.toLower(), id = bucket["id"].toString().toLower();
    const bool knownProvider = QStringList{"claude", "codex", "cursor", "grok"}.contains(name);
    qint64 duration = 0, step = 0;
    QString window, unit;
    if (knownProvider && (id == "weekly" || id.startsWith("weekly_"))) {
        duration = 7 * 86400; step = 86400; window = "7-day window"; unit = "Day";
    } else if ((name == "claude" || name == "codex") && id == "session") {
        const bool weeklyFallback = bucket["label"].toString().contains("weekly", Qt::CaseInsensitive);
        duration = weeklyFallback ? 7 * 86400 : 5 * 3600;
        step = weeklyFallback ? 86400 : 3600;
        window = weeklyFallback ? "7-day window" : "5-hour window";
        unit = weeklyFallback ? "Day" : "Hour";
    } else if (name == "cursor" && QStringList{"session", "plan", "auto", "api", "on_demand"}.contains(id)) {
        duration = 30 * 86400; step = 7 * 86400; window = "30-day billing estimate"; unit = "Week";
    } else if (name == "grok" && QStringList{"session", "credits", "plan"}.contains(id)) {
        const auto reset = QDateTime::fromString(bucket["resets_at"].toString(), Qt::ISODateWithMs).toUTC();
        if (reset.isValid()) duration = reset.addMonths(-1).secsTo(reset);
        step = 7 * 86400; window = "calendar-month billing estimate"; unit = "Week";
    }
    return {{"seconds", duration}, {"step", step}, {"unit", unit}, {"label", window}};
}

QVariantList Usage::notches(const QString &provider, const QVariantMap &bucket) {
    const auto window = period(provider, bucket);
    const auto duration = window["seconds"].toLongLong(), step = window["step"].toLongLong();
    QVariantList result;
    if (duration <= 0 || step <= 0) return result;
    for (qint64 elapsed = step; elapsed < duration; elapsed += step) {
        const auto label = QString("%1 %2").arg(window["unit"].toString()).arg(elapsed / step);
        result.append(QVariantMap{{"fraction", double(elapsed) / duration},
            {"label", label + (window["unit"] == "Week" ? QString(" · day %1").arg(elapsed / 86400) : QString())}});
    }
    return result;
}

QVariantMap Usage::pacing(const QString &provider, const QVariantMap &bucket, const QDateTime &now) {
    auto unavailable = [](const QString &reason) {
        return QVariantMap{{"available", false}, {"label", "Pace unavailable"}, {"detail", reason}};
    };
    const auto reset = QDateTime::fromString(bucket["resets_at"].toString(), Qt::ISODateWithMs).toUTC();
    if (!reset.isValid()) return unavailable("This meter has no reset time, so its pacing cannot be estimated.");
    if (reset <= now) return unavailable("The reset time has passed. Waiting for the backend's next usage window.");
    const auto periodInfo = period(provider, bucket);
    const auto duration = periodInfo["seconds"].toLongLong();
    const auto window = periodInfo["label"].toString();
    const auto id = bucket["id"].toString().toLower();
    if (duration <= 0) return unavailable("This meter has no known usage-window length. A reset time alone cannot establish pacing.");
    const QString status = bucket["status_text"].toString();
    if (id == "on_demand" && bucket["utilization"].toDouble() <= 0 && !status.isEmpty() && !status.contains(" / "))
        return unavailable("This meter reports billing status without a measured usage percentage.");
    const auto remaining = now.secsTo(reset);
    if (remaining > duration) return unavailable("The reset is outside the expected usage window; no pacing estimate is shown.");
    const double expected = 100.0 * (duration - remaining) / duration;
    const double used = bucket["utilization"].toDouble();
    const double difference = used - expected;
    const int points = qRound(std::abs(difference));
    const QString label = points == 0 ? "On pace" : QString("%1 pp %2 pace").arg(points).arg(difference > 0 ? "over" : "under");
    const QString detail = QString("%1% used · %2% expected by now. %3\nThe marker estimates steady spending across a %4.\nOver pace means using your allowance faster than time is passing. Under pace means you have room to use more.")
        .arg(used, 0, 'f', 1).arg(expected, 0, 'f', 1).arg(label).arg(window);
    return {{"available", true}, {"expected", expected}, {"difference", difference},
        {"over", points > 0 && difference > 0}, {"label", label}, {"detail", detail}};
}

QVariantMap Usage::concern(const QString &provider, const QVariantMap &bucket, const QDateTime &now) {
    const auto pace = pacing(provider, bucket, now);
    const double used = qBound(0.0, bucket["utilization"].toDouble(), 100.0);
    const double remaining = 100.0 - used;
    const bool available = pace["available"].toBool();
    double pressure = 0;
    QString explanation;
    if (available) {
        const double timeRemaining = 100.0 - pace["expected"].toDouble();
        // Fraction of the allowance for the remaining time already spent early.
        // Divide by remaining time, not elapsed time: small early bursts stay calm.
        pressure = qBound(0.0, (timeRemaining - remaining) / qMax(0.000001, timeRemaining), 1.0);
        explanation = QString("%1% allowance left · %2% of the window left.\n%3% of the expected remaining allowance has been spent ahead of schedule.")
            .arg(remaining, 0, 'f', 1).arg(timeRemaining, 0, 'f', 1).arg(pressure * 100, 0, 'f', 1);
        if (used >= 95)
            explanation += used >= 100 ? "\nThe allowance is exhausted." : "\nVery little allowance remains, even if usage is on pace.";
        explanation += "\nPacing concern: yellow at 10%, orange at 25%, red at 50% of remaining allowance spent early.";
    } else {
        // Retain the original Windows percentage fallback only without timing.
        explanation = "Window timing is unavailable. Color uses usage alone: yellow at 50%, orange at 75%, red at 90%.";
    }
    const auto level = warningLevel(used, available, pressure);
    return {{"severity", int(level)}, {"color", warningColor(level)}, {"level", warningName(level)}, {"pressure", pressure}, {"available", available},
        {"remaining", remaining}, {"detail", pace["detail"].toString() + "\n\n" + explanation}};
}

QByteArray Usage::demo() {
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
    for (int i = 0; i < names.size(); ++i)
        providers.append(QJsonObject{{"provider_name", names[i]}, {"subtitle", "Sample account"}, {"error", QJsonValue::Null},
            {"is_success", true}, {"needs_reauth", false}, {"buckets", meters[i]}});
    return QJsonDocument(providers).toJson();
}
