#include "controller.h"
#include "usage.h"
#include "trayvisual.h"
#include "startup.h"
#include "appinfo.h"
#include "popup.h"
#include <QCursor>
#include <memory>
#ifdef HEADROOM_KDE_TRAY
#include <KStatusNotifierItem>
#endif
#include <QApplication>
#include <QCommandLineParser>
#include <QFileInfo>
#include <QLocalServer>
#include <QLocalSocket>
#include <QLockFile>
#include <QMenu>
#include <QPainter>
#include <QPalette>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickWindow>
#include <QQuickStyle>
#include <QStandardPaths>
#include <QSystemTrayIcon>

int main(int argc, char **argv) {
    QQuickStyle::setStyle("Basic");
    QApplication app(argc, argv);
    QPalette palette;
    palette.setColor(QPalette::Window, QColor("#282a36"));
    palette.setColor(QPalette::WindowText, QColor("#f8f8f2"));
    palette.setColor(QPalette::Base, QColor("#21222c"));
    palette.setColor(QPalette::AlternateBase, QColor("#303341"));
    palette.setColor(QPalette::Text, QColor("#f8f8f2"));
    palette.setColor(QPalette::Button, QColor("#44475a"));
    palette.setColor(QPalette::ButtonText, QColor("#f8f8f2"));
    palette.setColor(QPalette::Highlight, QColor("#bd93f9"));
    palette.setColor(QPalette::HighlightedText, QColor("#282a36"));
    palette.setColor(QPalette::ToolTipBase, QColor("#44475a"));
    palette.setColor(QPalette::ToolTipText, QColor("#f8f8f2"));
    palette.setColor(QPalette::Link, QColor("#8be9fd"));
    app.setPalette(palette);
    app.setOrganizationName("Headroom"); app.setApplicationName("Headroom"); app.setApplicationVersion(HEADROOM_VERSION);
    app.setDesktopFileName("headroom");
    app.setWindowIcon(QIcon(":/qt/qml/Headroom/headroom.svg"));
    QCommandLineParser parser; parser.setApplicationDescription("A little more room to think. Native AI usage monitor."); parser.addHelpOption(); parser.addVersionOption();
    parser.addOption({"demo", "Show clearly labeled sample data without contacting a backend."});
    parser.addOption({"background", "Start in the system tray."});
    parser.addOption({"screenshot", "Save a screenshot, then exit (for visual verification).", "path"});
    parser.addOption({"config", "Use an alternate settings file.", "path"});
    parser.process(app);
    const bool capture = parser.isSet("screenshot"), demo = parser.isSet("demo");
    QLockFile lock(QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation) + "/headroom.lock");
    const QString socketName = QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation) + "/headroom.socket";
    QLocalServer server;
    if (!capture && !demo) {
        if (!lock.tryLock()) { QLocalSocket socket; socket.connectToServer(socketName); socket.waitForConnected(1000); socket.write("show"); socket.waitForBytesWritten(1000); return 0; }
        QLocalServer::removeServer(socketName); server.setSocketOptions(QLocalServer::UserAccessOption); server.listen(socketName);
    }
    Controller controller(demo, parser.value("config"));
    StartupService startup({}, {}, !demo && !capture);
    AppInfo appInfo;
    const auto syncServices = [&] {
        startup.setAllowChanges(!controller.isDemo() && !capture);
        appInfo.setBackend(controller.isDemo() ? QString() : controller.backendUrl(),
                           controller.isDemo() ? QString() : controller.backendToken());
    };
    QObject::connect(&controller, &Controller::settingsChanged, &app, syncServices);
    QObject::connect(&controller, &Controller::changed, &app, syncServices);
    syncServices();
    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("backend", &controller);
    engine.rootContext()->setContextProperty("startupService", &startup);
    engine.rootContext()->setContextProperty("appInfo", &appInfo);
    const bool hasTray = !capture && QSystemTrayIcon::isSystemTrayAvailable();
    engine.rootContext()->setContextProperty("trayAvailable", hasTray);
    engine.rootContext()->setContextProperty("startHidden", true);
    engine.loadFromModule("Headroom", "Main");
    if (engine.rootObjects().isEmpty()) return 1;
    auto window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
    TrayPopup popup(window, hasTray, &app);
    const auto show = [&popup] { popup.show(); };
    QObject::connect(&server, &QLocalServer::newConnection, &app, [&] { auto socket = server.nextPendingConnection(); socket->deleteLater(); show(); });
    QSystemTrayIcon tray(TrayVisual::icon({}));
    QMenu fallbackMenu;
    QMenu *trayMenu = &fallbackMenu;
#ifdef HEADROOM_KDE_TRAY
    std::unique_ptr<KStatusNotifierItem> nativeTray;
    if (hasTray) {
        nativeTray = std::make_unique<KStatusNotifierItem>("headroom");
        nativeTray->setTitle("Headroom");
        nativeTray->setCategory(KStatusNotifierItem::ApplicationStatus);
        nativeTray->setStandardActionsEnabled(false);
        nativeTray->setStatus(KStatusNotifierItem::Active);
        trayMenu = new QMenu;
        nativeTray->setContextMenu(trayMenu);
        QObject::connect(nativeTray.get(), &KStatusNotifierItem::activateRequested, &app,
            [&](bool, const QPoint &pos) { popup.toggle(pos); });
    }
#endif
    QMenu &menu = *trayMenu;
    menu.addAction("Open Headroom", &app, show); menu.addAction("Refresh usage", &controller, &Controller::refresh);
    menu.addAction("Settings…", &app, [&] {
        show();
        if (auto settings = window->findChild<QObject *>("settingsPanel")) QMetaObject::invokeMethod(settings, "open");
    });
    menu.addSeparator(); menu.addAction("Quit Headroom", &app, &QApplication::quit); tray.setContextMenu(&fallbackMenu);
    QObject::connect(&tray, &QSystemTrayIcon::activated, &app, [&](QSystemTrayIcon::ActivationReason reason) { if (reason == QSystemTrayIcon::Trigger || reason == QSystemTrayIcon::DoubleClick) { popup.toggle(tray.geometry().isValid() ? tray.geometry().center() : QCursor::pos()); } });
    const auto notify = [&](const QString &title, const QString &message, int severity) {
        if (!hasTray) return;
#ifdef HEADROOM_KDE_TRAY
        if (nativeTray) {
            nativeTray->showMessage(title, message, severity >= 3 ? "dialog-error" : severity >= 2 ? "dialog-warning" : "dialog-information", 7000);
            return;
        }
#endif
        tray.showMessage(title, message, severity >= 3 ? QSystemTrayIcon::Critical : severity >= 2 ? QSystemTrayIcon::Warning : QSystemTrayIcon::Information, 7000);
    };
    QObject::connect(&controller, &Controller::notify, &app, [&](const QString &title, const QString &message) { notify(title, message, 0); });
    QObject::connect(&controller, &Controller::usageAlert, &app, notify);
    auto updateTray = [&] {
        const auto model = TrayVisual::build(controller.state(), controller.providers(), controller.primary(),
            [&](const QString &provider, const QVariantMap &bucket) { return controller.concern(provider, bucket); });
#ifdef HEADROOM_KDE_TRAY
        if (nativeTray) {
            nativeTray->setIconByPixmap(TrayVisual::icon(model));
            nativeTray->setToolTipTitle(model.tooltip.section('\n', 0, 0));
            nativeTray->setToolTipSubTitle(model.tooltip.section('\n', 1));
            return;
        }
#endif
        tray.setToolTip(model.tooltip);
        tray.setIcon(TrayVisual::icon(model));
    };
    QObject::connect(&controller, &Controller::changed, &app, updateTray);
    QObject::connect(&controller, &Controller::settingsChanged, &app, updateTray);
    updateTray();
    if (hasTray) {
        app.setQuitOnLastWindowClosed(false);
#ifdef HEADROOM_KDE_TRAY
        if (!nativeTray) tray.show();
#else
        tray.show();
#endif
    }
    if (!parser.isSet("background") || !hasTray) show();
    if (capture) QTimer::singleShot(900, &app, [&] { app.exit(window->grabWindow().save(parser.value("screenshot")) ? 0 : 2); });
    return app.exec();
}
