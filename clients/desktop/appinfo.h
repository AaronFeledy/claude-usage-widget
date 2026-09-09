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
public:
    explicit AppInfo(QObject *parent = nullptr, int timeoutMs = 8000);
    QString applicationVersion() const;
    QString serverVersion() const { return m_serverVersion; }
    QString serverStatus() const { return m_serverStatus; }
    bool checkingServer() const { return !m_healthReply.isNull(); }
    // Credentials remain C++-only and are sent solely to the configured API origin.
    void setBackend(const QString &baseUrl, const QString &token);
    Q_INVOKABLE void refreshServer();
signals:
    void changed();
private:
    QNetworkReply *request(const QUrl &url, const QByteArray &token = {});
    QNetworkAccessManager m_network;
    QPointer<QNetworkReply> m_healthReply;
    QUrl m_healthUrl;
    QByteArray m_token;
    int m_timeoutMs;
    QString m_serverVersion;
    QString m_serverStatus = "Connect a backend to see its version.";
};
