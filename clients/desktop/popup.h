#pragma once
#include <QObject>
#include <QPoint>
#include <QRect>
#include <QSize>
#include <QTimer>
class QQuickWindow;
namespace PopupPlacement {
QRect bounds(const QRect &screen, const QRect &available, QPoint anchor, QSize preferred);
}
class TrayPopup : public QObject {
    Q_OBJECT
public:
    TrayPopup(QQuickWindow *window, bool attached, QObject *parent = nullptr);
    void show();
    void toggle(const QPoint &anchor = {}, bool hasAnchor = true);
private:
    void position();
    QQuickWindow *m_window;
    bool m_attached;
    QPoint m_anchor;
    bool m_hasAnchor = false;
    QTimer m_dismiss;
};
