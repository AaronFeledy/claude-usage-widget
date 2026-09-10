#include "popup.h"
#include <QGuiApplication>
#include <QCursor>
#include <QQuickWindow>
#include <QScreen>
#include <algorithm>
#ifdef HEADROOM_LAYER_SHELL
#include <LayerShellQt/Window>
#endif

QRect PopupPlacement::bounds(const QRect &screen, const QRect &available, QPoint anchor, QSize preferred) {
    const QRect work = available.adjusted(12, 12, -12, -12);
    preferred = preferred.boundedTo(work.size());
    if (!screen.contains(anchor)) anchor = QPoint(available.right() - 24, screen.bottom() - 12);
    const int distances[] = {qAbs(anchor.y() - screen.top()), qAbs(screen.bottom() - anchor.y()),
                             qAbs(anchor.x() - screen.left()), qAbs(screen.right() - anchor.x())};
    const int edge = int(std::min_element(std::begin(distances), std::end(distances)) - std::begin(distances));
    QPoint pos(anchor.x() - preferred.width() / 2, anchor.y() - preferred.height() / 2);
    if (edge == 0) pos.setY(anchor.y() + 24);
    if (edge == 1) pos.setY(anchor.y() - 24 - preferred.height());
    if (edge == 2) pos.setX(anchor.x() + 24);
    if (edge == 3) pos.setX(anchor.x() - 24 - preferred.width());
    pos.setX(qBound(work.left(), pos.x(), work.right() - preferred.width() + 1));
    pos.setY(qBound(work.top(), pos.y(), work.bottom() - preferred.height() + 1));
    return QRect(pos, preferred);
}
TrayPopup::TrayPopup(QQuickWindow *window, bool attached, QObject *parent)
    : QObject(parent), m_window(window), m_attached(attached) {
    if (!attached) return;
#ifdef HEADROOM_LAYER_SHELL
    if (QGuiApplication::platformName().startsWith("wayland")) {
        auto layer = LayerShellQt::Window::get(window);
        layer->setScope("headroom-tray-popup");
        layer->setLayer(LayerShellQt::Window::LayerTop);
        layer->setExclusiveZone(-1);
        layer->setKeyboardInteractivity(LayerShellQt::Window::KeyboardInteractivityOnDemand);
        layer->setActivateOnShow(true);
        layer->setAnchors(LayerShellQt::Window::Anchors(LayerShellQt::Window::AnchorTop) | LayerShellQt::Window::AnchorLeft);
    }
#endif
    m_dismiss.setSingleShot(true); m_dismiss.setInterval(150);
    connect(qGuiApp, &QGuiApplication::focusWindowChanged, this, [this](QWindow *) {
        if (m_window->isVisible() && !m_window->isActive()) m_dismiss.start();
    });
    connect(window, &QWindow::activeChanged, this, [this] {
        if (m_window->isActive()) m_dismiss.stop();
        else if (m_window->isVisible()) m_dismiss.start();
    });
    connect(&m_dismiss, &QTimer::timeout, this, [this] {
        for (auto focus = QGuiApplication::focusWindow(); focus; focus = focus->transientParent())
            if (focus == m_window) return;
        m_window->hide();
    });
}
void TrayPopup::position() {
    if (!m_attached) return;
    auto screen = m_hasAnchor ? QGuiApplication::screenAt(m_anchor) : QGuiApplication::screenAt(QCursor::pos());
    if (!screen) screen = QGuiApplication::primaryScreen();
    if (!screen) return;
    const auto full = screen->geometry(), work = screen->availableGeometry();
    const QPoint anchor = m_hasAnchor ? m_anchor : QPoint(work.right() - 24, full.bottom() - 12);
    const auto rect = PopupPlacement::bounds(full, work, anchor, QSize(960, 820));
    m_window->setScreen(screen);
    m_window->setMinimumSize(QSize(qMin(420, rect.width()), qMin(400, rect.height())));
    m_window->resize(rect.size());
#ifdef HEADROOM_LAYER_SHELL
    if (QGuiApplication::platformName().startsWith("wayland")) {
        auto layer = LayerShellQt::Window::get(m_window);
        layer->setScreen(screen);
        layer->setDesiredSize(rect.size());
        layer->setMargins(QMargins(rect.left() - full.left(), rect.top() - full.top(), 0, 0));
        return;
    }
#endif
    m_window->setPosition(rect.topLeft());
}
void TrayPopup::show() {
    m_dismiss.stop(); position(); m_window->show(); m_window->raise(); m_window->requestActivate();
}
void TrayPopup::toggle(const QPoint &anchor, bool hasAnchor) {
    m_anchor = anchor; m_hasAnchor = hasAnchor;
    if (m_window->isVisible()) { m_dismiss.stop(); m_window->hide(); }
    else show();
}
