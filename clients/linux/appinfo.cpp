#include "appinfo.h"
#include "usage.h"
#include <QCoreApplication>
#include <QDesktopServices>
#include <QDir>
#include <QFileInfo>
#include <QStandardPaths>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QRegularExpression>
#include <QSysInfo>
#include <QTimer>
#include <QVersionNumber>

namespace {
constexpr auto releasesPage = "https://github.com/AaronFeledy/claude-usage-widget/releases/latest";
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
AppInfo::AppInfo(QObject *parent, const QUrl &releaseApi, int timeoutMs)
    : QObject(parent), m_releaseApi(releaseApi), m_timeoutMs(timeoutMs) {}
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
void AppInfo::checkForUpdates() {
    if (m_releaseReply) return;
    auto reply = request(m_releaseApi); m_releaseReply = reply;
    m_releaseStatus = "Checking releases…"; m_linuxAvailable = false; m_latestVersion.clear(); emit changed();
    connect(reply, &QNetworkReply::finished, this, [this, reply] {
        m_releaseReply = nullptr;
        if (!success(reply)) {
            m_releaseStatus = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt() == 404
                ? "No published release is available. This installation is built from source."
                : "Could not check releases. Try again later.";
        } else {
            const auto release = QJsonDocument::fromJson(reply->readAll()).object();
            m_latestVersion = safeVersion(release["tag_name"]);
            if (m_latestVersion.isEmpty() || !release["assets"].isArray()
                || release["draft"].toBool() || release["prerelease"].toBool()) {
                m_latestVersion.clear(); m_releaseStatus = "Release information was not recognized.";
            } else {
                // Linux packages must explicitly identify Headroom and this CPU;
                // usage-server or Windows assets are never application updates.
                const auto cpu = QSysInfo::currentCpuArchitecture();
                const QString architecture = cpu == "x86_64" ? "(?:x86_64|amd64|x64)"
                    : cpu == "arm64" || cpu == "aarch64" ? "(?:aarch64|arm64)" : QRegularExpression::escape(cpu);
                const QRegularExpression assetName("^headroom-linux-" + architecture + "(?:-v?[0-9][0-9A-Za-z.+_-]*)?\\.(?:tar\\.gz|AppImage|deb|rpm)$", QRegularExpression::CaseInsensitiveOption);
                for (const auto &asset : release["assets"].toArray()) {
                    const auto obj = asset.toObject(); const QUrl download(obj["browser_download_url"].toString());
                    if (assetName.match(obj["name"].toString()).hasMatch()
                        && download.scheme() == "https" && download.host() == "github.com"
                        && download.path().startsWith("/AaronFeledy/claude-usage-widget/releases/download/")) m_linuxAvailable = true;
                }
                if (!m_linuxAvailable) m_releaseStatus = "Latest project release: " + m_latestVersion
                    + ". No Linux installer for this computer is published. Update Headroom from source.";
                else {
                    QString version = m_latestVersion; if (version.startsWith('v')) version.remove(0, 1);
                    const auto latest = QVersionNumber::fromString(version), current = QVersionNumber::fromString(applicationVersion());
                    m_releaseStatus = !latest.isNull() && !current.isNull() && latest <= current
                        ? "This version is current. Linux packages are available on the release page."
                        : "A Linux package is available on the release page. Review it before installing.";
                }
            }
        }
        reply->deleteLater(); emit changed();
    });
}
void AppInfo::openReleasePage() { QDesktopServices::openUrl(QUrl(releasesPage)); }
void AppInfo::openInstallGuide() {
    const QString appDir = QCoreApplication::applicationDirPath();
    const QStringList candidates {
        QDir(appDir).filePath("../share/headroom/update-guide.html"),
        QStandardPaths::locate(QStandardPaths::GenericDataLocation, "headroom/update-guide.html"),
        QDir(appDir).filePath("../update-guide.html")
    };
    for (const auto &candidate : candidates) {
        if (!candidate.isEmpty() && QFileInfo::exists(candidate)) {
            if (QDesktopServices::openUrl(QUrl::fromLocalFile(QFileInfo(candidate).absoluteFilePath()))) return;
            break;
        }
    }
    m_releaseStatus = "Could not open the local update guide. Reinstall Headroom's shared files or open clients/linux/update-guide.html in the source checkout.";
    emit changed();
}
