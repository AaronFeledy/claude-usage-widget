#include "warning.h"
#include <array>

namespace {
struct Tier {
    const char *name;
    const char *color;
    double enterPressure, exitPressure;
    double enterCapacity, exitCapacity;
    double enterFallback, exitFallback;
    bool notify;
};
// One policy for bars, pacing text, provider attention, tray, and pop-up alerts.
constexpr std::array<Tier, 4> tiers {{
    {"Normal",   "#bd93f9", 0,    0,    0,   0,    0,  0, false},
    {"Watch",    "#f1fa8c", 0.10, 0.08, 95,  94,  50, 45, false},
    {"Warning",  "#ffb86c", 0.25, 0.20, 99,  98,  75, 70, true},
    {"Critical", "#ff5555", 0.50, 0.40, 100, 99.5,90, 85, true}
}};
}

Usage::WarningLevel Usage::warningLevel(double used, bool hasPacing, double pressure, WarningLevel previous) {
    auto result = WarningLevel::Normal;
    for (int i = 1; i < int(tiers.size()); ++i) {
        const auto &tier = tiers[i];
        const bool recovering = int(previous) >= i;
        const bool reached = hasPacing
            ? pressure >= (recovering ? tier.exitPressure : tier.enterPressure) - 1e-9
                || used >= (recovering ? tier.exitCapacity : tier.enterCapacity)
            : used >= (recovering ? tier.exitFallback : tier.enterFallback);
        if (reached) result = WarningLevel(i);
    }
    return result;
}

Usage::WarningTransition Usage::advanceWarning(WarningState &state, double used, bool hasPacing,
                                                double pressure, const QString &window) {
    const auto previous = state.level;
    const bool reset = state.initialized && state.window != window;
    const bool baseline = !state.initialized || reset;
    state.level = warningLevel(used, hasPacing, pressure, baseline ? WarningLevel::Normal : previous);
    state.window = window;
    state.initialized = true;
    // Start-up/new-window snapshots establish a baseline. Only later upward
    // transitions into an alerting tier notify. Recovery and steady polls don't.
    return {previous, state.level, previous != state.level,
        !baseline && int(state.level) > int(previous) && tiers[int(state.level)].notify, reset};
}

QString Usage::warningName(WarningLevel level) { return QString::fromLatin1(tiers[int(level)].name); }
QString Usage::warningColor(WarningLevel level) { return QString::fromLatin1(tiers[int(level)].color); }
