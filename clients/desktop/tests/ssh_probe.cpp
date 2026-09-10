#include "sshnetwork.h"

#include <QCoreApplication>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QTimer>

class Probe final : public QObject {
public:
    Probe(QString executable, QUrl address)
        : m_network(SshOptions{std::move(executable), 25000}), m_address(std::move(address)) { next(); }

private:
    void next()
    {
        QUrl url = m_address;
        QByteArray body;
        if (m_step == 0) url.setPath(QStringLiteral("/api/v1/health"));
        else if (m_step == 1) url.setPath(QStringLiteral("/api/v1/usage"));
        else {
            url.setPath(QStringLiteral("/api/v1/providers/grok/credentials"));
            body = QByteArrayLiteral("{\"cookie\":\"sso=synthetic\"}");
        }
        QNetworkRequest request(url);
        QNetworkReply *reply = m_step < 2 ? m_network.get(request) : m_network.put(request, body);
        connect(reply, &QNetworkReply::finished, this, [this, reply] {
            const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
            const QByteArray response = reply->readAll();
            bool valid = reply->error() == QNetworkReply::NoError;
            if (m_step == 0) {
                const QJsonObject health = QJsonDocument::fromJson(response).object();
                valid = valid && status == 200 && health.value(QStringLiteral("status")).toString() == QStringLiteral("ok");
            } else if (m_step == 1) {
                const auto usage = QJsonDocument::fromJson(response);
                valid = valid && status == 200 && usage.isArray() && usage.array().isEmpty();
            } else valid = valid && status == 404;
            reply->deleteLater();
            if (!valid) { qCritical("SSH probe failed at step %d.", m_step + 1); QCoreApplication::exit(1); return; }
            if (++m_step == 3) { QCoreApplication::exit(0); return; }
            next();
        });
    }

    SshNetworkAccessManager m_network;
    QUrl m_address;
    int m_step = 0;
};

int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    if (app.arguments().size() != 3) { qCritical("usage: headroom-ssh-probe <ssh-executable> <ssh-url>"); return 2; }
    QUrl address;
    if (!SshTransport::parseAddress(app.arguments().at(2), &address)) { qCritical("Invalid SSH address."); return 2; }
    Probe probe(app.arguments().at(1), address);
    QTimer::singleShot(80000, &app, [] { qCritical("SSH probe timed out."); QCoreApplication::exit(1); });
    return app.exec();
}
