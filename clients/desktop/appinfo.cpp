#include "appinfo.h"
#include "usage.h"
#include <QCoreApplication>
#include <QJsonDocument>
#include <QJsonObject>
#include <QRegularExpression>
#include <QTimer>

namespace {
QString safeVersion(const QJsonValue &value) {
    const QString version = value.toString();
    static const QRegularExpression valid("^[A-Za-z0-9][A-Za-z0-9.+_-]{0,79}$");
    return valid.match(version).hasMatch() ? version : QString();
}
bool success(QNetworkReply *reply) {
    return reply->error() == QNetworkReply::NoError
        && reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt() == 200;
}
}
AppInfo::AppInfo(QObject *parent, int timeoutMs)
    : QObject(parent), m_timeoutMs(timeoutMs) {}
QString AppInfo::applicationVersion() const {
    return QCoreApplication::applicationVersion();
}
QNetworkReply *AppInfo::request(const QUrl &url, const QByteArray &token) {
    QNetworkRequest request(url);
    // Manual policy rejects every redirect, including same-origin redirects. In
    // particular a backend cannot redirect the bearer token to a different host.
    request.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::ManualRedirectPolicy);
    request.setRawHeader("User-Agent", "Headroom/" + applicationVersion().toUtf8());
    request.setRawHeader("Accept", "application/json");
    if (!token.isEmpty()) request.setRawHeader("Authorization", "Bearer " + token);
    auto reply = m_network.get(request);
    reply->setReadBufferSize(1024 * 1024 + 1);
    connect(reply, &QIODevice::readyRead, reply, [reply] {
        if (reply->bytesAvailable() > 1024 * 1024) reply->abort();
    });
    QTimer::singleShot(m_timeoutMs, reply, [reply] { if (!reply->isFinished()) reply->abort(); });
    return reply;
}
void AppInfo::setBackend(const QString &baseUrl, const QString &token) {
    auto endpoint = Usage::endpoint(baseUrl);
    if (!endpoint.isEmpty()) {
        QString path = endpoint.path(); path.chop(QString("usage").size());
        endpoint.setPath(path + "health");
    }
    if (endpoint == m_healthUrl && token.toUtf8() == m_token) return;
    if (m_healthReply) {
        auto previous = m_healthReply; m_healthReply = nullptr;
        previous->disconnect(this); previous->abort(); previous->deleteLater();
    }
    m_healthUrl = endpoint; m_token = token.toUtf8(); m_serverVersion.clear();
    m_serverStatus = endpoint.isEmpty() ? "Connect a backend to see its version." : "Server version has not been checked.";
    emit changed();
}
void AppInfo::refreshServer() {
    if (m_healthUrl.isEmpty() || m_healthReply) return;
    auto reply = request(m_healthUrl, m_token); m_healthReply = reply;
    m_serverStatus = "Checking server…"; emit changed();
    connect(reply, &QNetworkReply::finished, this, [this, reply] {
        m_healthReply = nullptr; m_serverVersion.clear();
        const int code = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        if (success(reply)) {
            const auto health = QJsonDocument::fromJson(reply->readAll()).object();
            m_serverVersion = safeVersion(health["version"]);
            if (!m_token.isEmpty() && m_serverVersion.contains(QString::fromUtf8(m_token))) m_serverVersion.clear();
            const auto status = health["status"].toString();
            if (m_serverVersion.isEmpty() || (status != "ok" && status != "degraded")) {
                m_serverVersion.clear(); m_serverStatus = "Server returned unrecognized version details.";
            } else m_serverStatus = status == "ok" ? "Server healthy" : "Server reachable · some providers need attention";
        } else if (code == 401 || code == 403) m_serverStatus = "Server rejected the saved bearer token.";
        else if (code >= 300 && code < 400) m_serverStatus = "Server redirected the version request. Check its address.";
        else m_serverStatus = "Server version unavailable. Try again when connected.";
        reply->deleteLater(); emit changed();
    });
}
