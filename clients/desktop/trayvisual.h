#pragma once
#include "warning.h"
#include <QDateTime>
#include <QIcon>
#include <QVariantList>
#include <QVariantMap>
#include <functional>

namespace TrayVisual {
enum class Kind { Setup, Connecting, Idle, Usage, Exhausted, Offline, AuthError, ApiError, Malformed, ProviderError };
struct Model {
    Kind kind = Kind::Setup;
    QString provider;
    QString tooltip;
    double used = -1;
    double expected = -1;
    Usage::WarningLevel level = Usage::WarningLevel::Normal;
    Usage::WarningLevel secondary = Usage::WarningLevel::Normal;
};
using Assessment = std::function<QVariantMap(const QString &, const QVariantMap &)>;
Model build(const QVariantMap &state, const QVariantList &providers, const QString &primary,
            const Assessment &assessment, const QDateTime &now = QDateTime::currentDateTimeUtc());
QIcon icon(const Model &model);
}
