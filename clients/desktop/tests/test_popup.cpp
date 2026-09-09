#include "popup.h"
#include <QtTest>
#include <QQuickWindow>
class PopupTest : public QObject {
    Q_OBJECT
private slots:
    void toggleAndDismiss() {
        QQuickWindow window;
        window.setFlags(Qt::Tool | Qt::FramelessWindowHint);
        TrayPopup popup(&window, true);
        popup.toggle(QPoint(700,580)); QTRY_VERIFY(window.isVisible());
        popup.toggle(QPoint(700,580)); QVERIFY(!window.isVisible());
        popup.show(); QTRY_VERIFY(window.isActive());
        QWindow child; child.setTransientParent(&window); child.setFlags(Qt::Tool);
        child.show(); child.requestActivate(); QTRY_VERIFY(child.isActive());
        QTest::qWait(200); QVERIFY(window.isVisible());
        QWindow outside; outside.show(); outside.requestActivate(); QTRY_VERIFY(outside.isActive());
        // A transient child may have taken focus before an outside window did.
        QTRY_VERIFY(!window.isVisible());
    }
    void placement_data() {
        QTest::addColumn<QRect>("screen"); QTest::addColumn<QRect>("work");
        QTest::addColumn<QPoint>("anchor"); QTest::addColumn<QString>("edge");
        const QRect screen(0, 0, 1920, 1080);
        QTest::newRow("bottom") << screen << QRect(0,0,1920,1032) << QPoint(1800,1056) << "bottom";
        QTest::newRow("top") << screen << QRect(0,48,1920,1032) << QPoint(1500,24) << "top";
        QTest::newRow("left") << screen << QRect(48,0,1872,1080) << QPoint(24,540) << "left";
        QTest::newRow("right") << screen << QRect(0,0,1872,1080) << QPoint(1896,540) << "right";
        QTest::newRow("negative-monitor") << QRect(-1920,0,1920,1080) << QRect(-1920,0,1920,1032) << QPoint(-200,1056) << "bottom";
        QTest::newRow("small-screen") << QRect(0,0,800,600) << QRect(0,0,800,560) << QPoint(700,580) << "bottom";
        QTest::newRow("scaled-logical-monitor") << QRect(0,0,1707,960) << QRect(0,32,1707,888) << QPoint(1600,944) << "bottom";
        QTest::newRow("scaled-negative-monitor") << QRect(-1707,0,1707,960) << QRect(-1707,32,1707,888) << QPoint(-1600,16) << "top";
    }
    void placement() {
        QFETCH(QRect, screen); QFETCH(QRect, work); QFETCH(QPoint, anchor); QFETCH(QString, edge);
        const auto popup = PopupPlacement::bounds(screen, work, anchor, QSize(960,820));
        QVERIFY(work.adjusted(12,12,-12,-12).contains(popup));
        if (edge == "bottom") QVERIFY(popup.bottom() < anchor.y());
        if (edge == "top") QVERIFY(popup.top() > anchor.y());
        if (edge == "left") QVERIFY(popup.left() > anchor.x());
        if (edge == "right") QVERIFY(popup.right() < anchor.x());
    }
    void clampsOversizedPopupAndUnknownAnchor() {
        const QRect screen(100, 200, 640, 480), work(100, 240, 640, 440);
        const auto popup = PopupPlacement::bounds(screen, work, QPoint(-9000, -9000), QSize(1200, 900));
        QCOMPARE(popup, work.adjusted(12, 12, -12, -12));
    }
};
QTEST_MAIN(PopupTest)
#include "test_popup.moc"
