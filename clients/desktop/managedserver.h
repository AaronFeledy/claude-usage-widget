#pragma once

#include <QNetworkAccessManager>
#include <QPointer>
#include <QProcess>
#include <QTimer>
#include <QUrl>

struct ManagedServerOptions {
    QUrl localUrl = QUrl(QStringLiteral("http://127.0.0.1:7823/"));
    QString executablePath;
    int probeTimeoutMs = 3000;
    int readinessProbeTimeoutMs = 1000;
    int readinessIntervalMs = 100;
    int readinessAttempts = 40;
    int restartLimit = 3;
};

class ManagedServer : public QObject {
    Q_OBJECT
public:
    explicit ManagedServer(ManagedServerOptions options = {}, QObject *parent = nullptr);
    ~ManagedServer() override;

    void configure(const QString &mode, const QString &token);
    void ensureAvailable();
    void reportConnectionFailure();
    void stopOwned();
    bool ownsProcess() const { return m_process && m_owned; }
    bool isAttached() const { return m_available && !m_owned; }
    bool isAvailable() const { return m_available; }
    QString state() const { return m_state; }
    QString message() const { return m_message; }
    QUrl localUrl() const { return m_options.localUrl; }

signals:
    void available();
    void unavailable(const QString &message, const QString &kind);
    void stateChanged();

private:
    enum class ProbePurpose { Initial, Readiness };
    enum class ProbeResult { Compatible, Refused, AuthRejected, Redirected, Malformed, TimedOut, NetworkFailure };

    void cancelAsync();
    void probe(ProbePurpose purpose);
    void handleProbe(ProbePurpose purpose, ProbeResult result);
    void spawn();
    void beginReadiness();
    void setAvailable(bool owned, const QString &state);
    void setFailure(const QString &message, const QString &kind);
    void disposeProcess(bool kill);
    QString binaryPath() const;
    bool assignWindowsJob();
    void closeWindowsJob();

    ManagedServerOptions m_options;
    QNetworkAccessManager m_network;
    QPointer<QNetworkReply> m_probe;
    QPointer<QProcess> m_process;
    QPointer<QProcess> m_retiringProcess;
    QTimer m_readinessTimer;
    QTimer m_restartTimer;
    QString m_mode = QStringLiteral("remote");
    QString m_token;
    QString m_state = QStringLiteral("remote");
    QString m_message;
    quint64 m_generation = 0;
    int m_readinessAttempt = 0;
    int m_restartCount = 0;
    bool m_owned = false;
    bool m_available = false;
    bool m_stopping = false;
    bool m_ensureAfterRetire = false;
    void *m_job = nullptr;
};
