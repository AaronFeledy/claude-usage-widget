#pragma once

#include <QObject>
#include <QPointer>
#include <QProcess>
#include <QJsonObject>
#include <QTimer>

struct UpdateServiceOptions {
    QString managerPath;
    QString installRoot;
    QString launcherPath;
    QString packageVersion;
    QString applicationPath;
    int timeoutMs = 150000;
    int cancelGraceMs = 3000;
    bool fixtureIdentity = false;
    bool systemManaged = false;
};

class UpdateService : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString state READ state NOTIFY changed)
    Q_PROPERTY(QString statusText READ statusText NOTIFY changed)
    Q_PROPERTY(QString latestVersion READ latestVersion NOTIFY changed)
    Q_PROPERTY(bool busy READ busy NOTIFY changed)
    Q_PROPERTY(bool canCheck READ canCheck NOTIFY changed)
    Q_PROPERTY(bool canStage READ canStage NOTIFY changed)
    Q_PROPERTY(bool canRepair READ canRepair NOTIFY changed)
    Q_PROPERTY(bool restartAvailable READ restartAvailable NOTIFY changed)
    Q_PROPERTY(QString updateMethod READ updateMethod NOTIFY changed)
public:
    explicit UpdateService(bool allowPublicTraffic = true, UpdateServiceOptions options = {}, QObject *parent = nullptr);
    ~UpdateService() override;
    QString state() const { return m_state; }
    QString statusText() const { return m_status; }
    QString latestVersion() const { return m_latestVersion; }
    bool busy() const { return m_process; }
    bool canCheck() const { return m_allowed && m_official && !busy(); }
    bool canStage() const { return m_allowed && m_state == QStringLiteral("available") && !busy(); }
    bool canRepair() const { return m_allowed && m_repairable && !busy(); }
    bool restartAvailable() const { return m_allowed && m_state == QStringLiteral("staged"); }
    QString updateMethod() const { return m_method; }
    QJsonObject verifiedStage() const { return m_verifiedStage; }
    Q_INVOKABLE void checkForUpdates();
    Q_INVOKABLE void stageUpdate();
    Q_INVOKABLE void repairInstallation();
    Q_INVOKABLE void cancel();
    Q_INVOKABLE void openUpdateMethod();
    void startAutomaticCheck();
    void setPublicTrafficAllowed(bool allowed);
signals:
    void changed();
private:
    enum class Operation { None, Inspect, Check, Stage, Repair };
    void inspectInstallation();
    void run(Operation operation, const QString &command);
    void finish(Operation operation, int exitCode, QProcess::ExitStatus exitStatus);
    void fail(const QString &message);
    bool validateIdentity(const QJsonObject &result) const;
    void handleInspection(const QJsonObject &result);
    void handleUpdateResult(Operation operation, const QJsonObject &result);
    void restoreAllowedState();
    QString guidePath() const;
    UpdateServiceOptions m_options;
    QPointer<QProcess> m_process;
    QTimer m_timeout;
    QByteArray m_output;
    QByteArray m_errorOutput;
    QJsonObject m_verifiedStage;
    QString m_state = QStringLiteral("unavailable");
    QString m_status;
    QString m_latestVersion;
    QString m_platform;
    QString m_architecture;
    QString m_method = QStringLiteral("source");
    QString m_prePauseState;
    QString m_prePauseStatus;
    bool m_allowed = true;
    bool m_sessionAllowed = true;
    bool m_official = false;
    bool m_repairable = false;
    bool m_autoPending = false;
    bool m_autoStage = false;
    bool m_cancelRequested = false;
    bool m_timedOut = false;
    bool m_autoStarted = false;
    bool m_resumeAfterCancel = false;
    Operation m_operation = Operation::None;
};
