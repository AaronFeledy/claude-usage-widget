#pragma once
#include <QObject>
#include <QPoint>
#include <QRect>
#include <QSize>
#include <QTimer>
#include <QList>
#include <functional>
class QQuickWindow;
class QScreen;
class QMouseEvent;
namespace PopupPlacement {
QRect bounds(const QRect &screen, const QRect &available, QPoint anchor, QSize preferred);
QSize constrainedSize(QSize available, QSize preferred);
Qt::Edges resizeEdges(QSize window, QPoint position, int margin = 8);
QRect resizedBounds(QRect start, Qt::Edges edges, QPoint delta, const QRect &work,
                    QSize minimum = QSize(460, 420), QSize maximum = QSize(1600, 1200));
}
class TrayPopup : public QObject {
    Q_OBJECT
public:
    using SizeWriter = std::function<void(QSize)>;
    TrayPopup(QQuickWindow *window, bool attached, QSize preferred = {}, SizeWriter sizeWriter = {},
              QObject *parent = nullptr);
    ~TrayPopup() override;
    void show();
    void toggle(const QPoint &anchor = {}, bool hasAnchor = true);
private:
    bool eventFilter(QObject *watched, QEvent *event) override;
    void position();
    void finishResize();
    void applyFallbackResize(const QPoint &globalPosition);
    void constrainToScreen(QScreen *screen);
    void applyGeometry(const QRect &rect, QScreen *screen);
    void writePendingSize();
    void watchScreen(QScreen *screen);
    bool usesLayerShell() const;
    QRect effectiveGeometry() const;
    QPoint resizePointer(const QMouseEvent *event) const;
    void syncLayerConfigure();
    QQuickWindow *m_window;
    bool m_attached;
    QPoint m_anchor;
    bool m_hasAnchor = false;
    QTimer m_dismiss;
    QTimer m_saveSize;
    QTimer m_resizePoll;
    QSize m_preferred;
    SizeWriter m_sizeWriter;
    Qt::Edges m_resizeEdges;
    QRect m_resizeStartGeometry;
    QPoint m_resizeStartPointer;
    bool m_resizing = false;
    bool m_nativeResize = false;
    QSize m_expectedProgrammaticSize;
    QSize m_pendingUserSize;
    QRect m_layerRect;
    QRect m_configuredLayerRect;
    QList<QRect> m_pendingLayerRects;
    QMetaObject::Connection m_workAreaConnection;
};
