#pragma once
#include <QObject>
#include <QNetworkAccessManager>
#include <QPointer>
#include <QNetworkReply>
#include <QUrl>

class AppInfo : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString applicationVersion READ applicationVersion CONSTANT)
    Q_PROPERTY(QString serverVersion READ serverVersion NOTIFY changed)
    Q_PROPERTY(QString serverStatus READ serverStatus NOTIFY changed)
    Q_PROPERTY(bool checkingServer READ checkingServer NOTIFY changed)
    Q_PROPERTY(QString releaseStatus READ releaseStatus NOTIFY changed)
    Q_PROPERTY(QString latestVersion READ latestVersion NOTIFY changed)
    Q_PROPERTY(bool checkingRelease READ checkingRelease NOTIFY changed)
    Q_PROPERTY(bool linuxDownloadAvailable READ linuxDownloadAvailable NOTIFY changed)
public:
    explicit AppInfo(QObject *parent = nullptr,
        const QUrl &releaseApi = QUrl("https://api.github.com/repos/AaronFeledy/claude-usage-widget/releases/latest"),
        int timeoutMs = 8000);
    QString applicationVersion() const;
    QString serverVersion() const { return m_serverVersion; }
    QString serverStatus() const { return m_serverStatus; }
    QString releaseStatus() const { return m_releaseStatus; }
    QString latestVersion() const { return m_latestVersion; }
    bool checkingServer() const { return !m_healthReply.isNull(); }
    bool checkingRelease() const { return !m_releaseReply.isNull(); }
    bool linuxDownloadAvailable() const { return m_linuxAvailable; }
    // Credentials remain C++-only and are sent solely to the configured API origin.
    void setBackend(const QString &baseUrl, const QString &token);
    Q_INVOKABLE void refreshServer();
    Q_INVOKABLE void checkForUpdates();
    Q_INVOKABLE void openReleasePage();
    Q_INVOKABLE void openInstallGuide();
signals:
    void changed();
private:
    QNetworkReply *request(const QUrl &url, const QByteArray &token = {});
    QNetworkAccessManager m_network;
    QPointer<QNetworkReply> m_healthReply, m_releaseReply;
    QUrl m_healthUrl, m_releaseApi;
    QByteArray m_token;
    int m_timeoutMs;
    QString m_serverVersion, m_latestVersion;
    QString m_serverStatus = "Connect a backend to see its version.";
    QString m_releaseStatus = "Source installation · updates are checked on request.";
    bool m_linuxAvailable = false;
};
