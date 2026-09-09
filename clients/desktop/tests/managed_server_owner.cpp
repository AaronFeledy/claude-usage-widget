#include "managedserver.h"

#include <QCoreApplication>
#include <QFile>
#include <QTextStream>

int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    if (app.arguments().size() != 4) return 2;
    ManagedServerOptions options;
    options.executablePath = app.arguments().at(1);
    options.localUrl = QUrl(QStringLiteral("http://127.0.0.1:%1/").arg(app.arguments().at(2)));
    options.probeTimeoutMs = 3000;
    options.readinessProbeTimeoutMs = 750;
    options.readinessIntervalMs = 50;
    options.readinessAttempts = 30;
    ManagedServer server(options);
    QObject::connect(&server, &ManagedServer::available, &app, [&] {
        QFile marker(app.arguments().at(3));
        if (!marker.open(QIODevice::WriteOnly | QIODevice::Text)) { app.exit(3); return; }
        QTextStream(&marker) << "ready\n";
    });
    QObject::connect(&server, &ManagedServer::unavailable, &app, [&app] { app.exit(4); });
    server.configure(QStringLiteral("local"), QString());
    server.ensureAvailable();
    return app.exec();
}
