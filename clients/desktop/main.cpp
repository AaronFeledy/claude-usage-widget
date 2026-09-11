#include "controller.h"
#include "usage.h"
#include "trayvisual.h"
#include "startup.h"
#include "appinfo.h"
#include "updateservice.h"
#include "popup.h"
#include "instance.h"
#include "palette.h"
#include <QCursor>
#include <memory>
#ifdef HEADROOM_KDE_TRAY
#include <KStatusNotifierItem>
#endif
#include <QApplication>
#include <QCommandLineParser>
#include <QFileInfo>
#include <QMenu>
#include <QMessageBox>
#include <QPainter>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickWindow>
#include <QQuickStyle>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSaveFile>
#include <QSystemTrayIcon>

int main(int argc, char **argv) {
    QQuickStyle::setStyle("Basic");
    QApplication app(argc, argv);
    app.setPalette(headroomPalette());
    app.setOrganizationName("Headroom"); app.setApplicationName("Headroom"); app.setApplicationVersion(HEADROOM_VERSION);
    app.setDesktopFileName("headroom");
    app.setWindowIcon(QIcon(":/qt/qml/Headroom/headroom.svg"));
    QCommandLineParser parser; parser.setApplicationDescription("A little more room to think. Native AI usage monitor."); parser.addHelpOption(); parser.addVersionOption();
    parser.addOption({"background", "Start in the system tray."});
    parser.addOption({"screenshot", "Save a screenshot, then exit (for visual verification).", "path"});
    parser.addOption({"config", "Use an alternate settings file.", "path"});
    parser.addOption({"headroom-ready-file", "Private update readiness endpoint.", "path"});
    parser.addOption({"headroom-update-restart", "Open the popup after a verified update restart."});
    parser.addOption({"headroom-installed-restart", "Open the popup after an installer restart."});
    parser.process(app);
    const bool capture = parser.isSet("screenshot");
    const bool isolated = parser.isSet("config");
    if (isolated && parser.value("config").trimmed().isEmpty()) {
        QMessageBox::critical(nullptr, "Headroom", "The --config option requires a settings file path.");
        return 2;
    }
    const QString settingsPath = isolated ? parser.value("config") : SettingsService::defaultPath();
    InstanceService instance(settingsPath);
    if (!capture) {
        const auto result = instance.start();
        if (result == InstanceService::Result::Secondary) return 0;
        if (result == InstanceService::Result::Error) {
            QMessageBox::critical(nullptr, "Headroom", "Headroom could not communicate with the running instance.");
            return 1;
        }
    }
    Controller controller(parser.value("config"), nullptr, !capture && !isolated, {}, {}, {}, !capture);
    StartupService startup({}, {}, !capture && !isolated);
    AppInfo appInfo;
    UpdateService updateService(!capture && !isolated);
    updateService.setOwnedProcessProvider([&controller] {
        return qMakePair(controller.ownedServerProcessId(), controller.ownedServerExecutablePath());
    });
    QStringList relaunchArguments;
    if (parser.isSet("background")) relaunchArguments << QStringLiteral("--background");
    if (isolated) relaunchArguments << QStringLiteral("--config") << parser.value("config");
    updateService.setRelaunchArguments(relaunchArguments);
    QObject::connect(&updateService, &UpdateService::applyPrepared, &app, [&] {
        controller.stopOwnedServer();
        QTimer::singleShot(0, &app, &QCoreApplication::quit);
    });
    const auto syncServices = [&] {
        startup.setAllowChanges(!capture && !isolated);
        updateService.setPublicTrafficAllowed(!capture && !isolated);
        appInfo.setBackend(capture ? QString() : controller.backendUrl(),
                           capture ? QString() : controller.backendToken(),
                           capture ? QSslCertificate() : controller.backendCertificate());
    };
    QObject::connect(&controller, &Controller::settingsChanged, &app, syncServices);
    QObject::connect(&controller, &Controller::changed, &app, syncServices);
    startup.setPreferenceWriter([&](bool enabled) { return controller.saveStartupPreference(enabled); });
    syncServices();
    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("backend", &controller);
    engine.rootContext()->setContextProperty("startupService", &startup);
    engine.rootContext()->setContextProperty("appInfo", &appInfo);
    engine.rootContext()->setContextProperty("updateService", &updateService);
    const bool hasTray = !capture && QSystemTrayIcon::isSystemTrayAvailable();
    engine.rootContext()->setContextProperty("trayAvailable", hasTray);
    engine.rootContext()->setContextProperty("startHidden", true);
    engine.rootContext()->setContextProperty("captureMode", capture);
    engine.loadFromModule("Headroom", "Main");
    if (engine.rootObjects().isEmpty()) return 1;
    if (parser.isSet("headroom-ready-file")) {
        const QString readyPath = parser.value("headroom-ready-file");
        const QByteArray nonce = qgetenv("HEADROOM_READY_NONCE");
        QSaveFile ready(readyPath);
        const QJsonObject value{{QStringLiteral("nonce"), QString::fromUtf8(nonce)},
            {QStringLiteral("pid"), qint64(QCoreApplication::applicationPid())},
            {QStringLiteral("version"), QCoreApplication::applicationVersion()},
            {QStringLiteral("executable"), QFileInfo(QCoreApplication::applicationFilePath()).canonicalFilePath()}};
        if (nonce.isEmpty() || !QDir::isAbsolutePath(readyPath) || !ready.open(QIODevice::WriteOnly)
            || ready.write(QJsonDocument(value).toJson(QJsonDocument::Compact) + '\n') < 0 || !ready.commit()) return 1;
#ifndef Q_OS_WIN
        QFile::setPermissions(readyPath, QFileDevice::ReadOwner | QFileDevice::WriteOwner);
#endif
    }
    auto window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
    TrayPopup popup(window, hasTray, &app);
    const auto show = [&popup] { popup.show(); };
    QObject::connect(&instance, &InstanceService::activationRequested, &app, show);
    if (!capture && !isolated && controller.startupMigrationPending() &&
        startup.migrateLegacyRegistration(controller.startupPreference()))
        controller.completeStartupMigration();
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
            [&](bool, const QPoint &pos) { popup.toggle(pos, !pos.isNull()); });
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
    if (!parser.isSet("background") || !hasTray || parser.isSet("headroom-update-restart")
        || parser.isSet("headroom-installed-restart")) show();
    if (!capture && !isolated) QTimer::singleShot(2500, &updateService, &UpdateService::startAutomaticCheck);
    if (capture) QTimer::singleShot(900, &app, [&] { app.exit(window->grabWindow().save(parser.value("screenshot")) ? 0 : 2); });
    return app.exec();
}
