#pragma once
#include <QString>

namespace Usage {
enum class WarningLevel { Normal, Watch, Warning, Critical };
struct WarningState {
    WarningLevel level = WarningLevel::Normal;
    QString window;
    bool initialized = false;
};
struct WarningTransition {
    WarningLevel from;
    WarningLevel to;
    bool changed;
    bool notify;
    bool reset;
};
WarningLevel warningLevel(double used, bool hasPacing, double pressure,
                          WarningLevel previous = WarningLevel::Normal);
WarningTransition advanceWarning(WarningState &state, double used, bool hasPacing,
                                 double pressure, const QString &window);
QString warningName(WarningLevel level);
QString warningColor(WarningLevel level);
}
