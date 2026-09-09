#include <QCoreApplication>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTextStream>
#include <QTimer>
#include "http_assertions.h"

int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    const QStringList arguments = app.arguments();
    const int listenIndex = arguments.indexOf(QStringLiteral("--listen-addr"));
    if (listenIndex < 0 || listenIndex + 1 >= arguments.size()) return 2;
    const QString address = arguments.at(listenIndex + 1);
    const int colon = address.lastIndexOf(':');
    if (!address.startsWith(QStringLiteral("127.0.0.1:")) || colon < 0) return 3;
    bool ok = false;
    const quint16 port = address.mid(colon + 1).toUShort(&ok);
    if (!ok || !port) return 4;

    const auto mode = qEnvironmentVariable("HEADROOM_FIXTURE_MODE", "degraded");
    const auto recordPath = qEnvironmentVariable("HEADROOM_FIXTURE_RECORD");
    if (!recordPath.isEmpty()) {
        QFile record(recordPath);
        if (record.open(QIODevice::WriteOnly | QIODevice::Append | QIODevice::Text)) {
            QTextStream stream(&record);
            stream << "start\nargs=" << arguments.mid(1).join('|') << "\n"
                   << "token_present=" << (!qEnvironmentVariable("USAGE_AUTH_TOKEN").isEmpty() ? "yes" : "no") << "\n"
                   << "pid=" << QCoreApplication::applicationPid() << "\n";
        }
    }
    if (mode == QStringLiteral("crash")) {
        QTimer::singleShot(20, &app, [&app] { app.exit(7); });
        return app.exec();
    }

    QTcpServer server;
    if (!server.listen(QHostAddress::LocalHost, port)) return 5;
    QObject::connect(&server, &QTcpServer::newConnection, &app, [&] {
        while (auto socket = server.nextPendingConnection()) {
            QObject::connect(socket, &QTcpSocket::readyRead, socket, [socket, mode] {
                QByteArray request = socket->property("request").toByteArray() + socket->readAll();
                socket->setProperty("request", request);
                if (!request.contains("\r\n\r\n")) return;
                if (mode == QStringLiteral("hang")) return;
                const bool authorized = qEnvironmentVariable("USAGE_AUTH_TOKEN").isEmpty()
                    || HttpAssertions::hasHeader(request, "Authorization", "Bearer fixture-secret");
                QByteArray body;
                int status = 200;
                if (!authorized) {
                    status = 401; body = R"({"error":"unauthorized"})";
                } else if (request.startsWith("GET /api/v1/health ")) {
                    body = R"({"status":"degraded","version":"fixture-1","providers":[{"name":"fixture","ok":false,"fetched_at":null}]})";
                    if (mode == QStringLiteral("ready-crash"))
                        QTimer::singleShot(75, qApp, [] { QCoreApplication::exit(8); });
                } else if (request.startsWith("GET /api/v1/usage ")) {
                    if (mode == QStringLiteral("ready-crash")) return;
                    body = QByteArrayLiteral("[]");
                } else {
                    status = 404; body = R"({"error":"not found"})";
                }
                socket->write("HTTP/1.1 " + QByteArray::number(status) + " Fixture\r\nContent-Type: application/json\r\nContent-Length: "
                    + QByteArray::number(body.size()) + "\r\nConnection: close\r\n\r\n" + body);
                socket->disconnectFromHost();
            });
            QObject::connect(socket, &QTcpSocket::disconnected, socket, &QObject::deleteLater);
        }
    });
    return app.exec();
}
