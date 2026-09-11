#pragma once
#include <QObject>
#include <QNetworkAccessManager>
#include <QPointer>
#include <QNetworkReply>
#include <QUrl>
#include <QSslCertificate>
#include "sshnetwork.h"

class AppInfo : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString applicationVersion READ applicationVersion CONSTANT)
    Q_PROPERTY(QString serverVersion READ serverVersion NOTIFY changed)
    Q_PROPERTY(QString serverStatus READ serverStatus NOTIFY changed)
    Q_PROPERTY(bool checkingServer READ checkingServer NOTIFY changed)
    Q_PROPERTY(QString serverUpdateNotice READ serverUpdateNotice NOTIFY changed)
public:
    explicit AppInfo(QObject *parent = nullptr, int timeoutMs = 8000, SshOptions sshOptions = {});
    ~AppInfo() override;
    QString applicationVersion() const;
    QString serverVersion() const { return m_serverVersion; }
    QString serverStatus() const { return m_serverStatus; }
    bool checkingServer() const { return !m_healthReply.isNull(); }
    QString serverUpdateNotice() const;
    // Credentials remain C++-only and are sent solely to the configured API origin.
    void setBackend(const QString &baseUrl, const QString &token,
                    const QSslCertificate &certificate = QSslCertificate(), bool remote = false);
    Q_INVOKABLE void refreshServer();
signals:
    void changed();
    void backendChanged();
private:
    QNetworkReply *request(const QUrl &url, const QByteArray &token, const QSslCertificate &certificate);
    QNetworkAccessManager m_network;
    QNetworkAccessManager m_localNetwork;
    SshNetworkAccessManager m_sshNetwork;
    QPointer<QNetworkReply> m_healthReply;
    QUrl m_healthUrl;
    QByteArray m_token;
    QSslCertificate m_certificate;
    int m_timeoutMs;
    QString m_serverVersion;
    bool m_remote = false;
    QString m_serverStatus = "Connect a backend to see its version.";
};
