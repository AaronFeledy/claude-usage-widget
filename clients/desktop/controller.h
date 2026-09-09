#pragma once
#include <QObject>
#include <QHash>
#include "warning.h"
#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QPointer>
#include <QTimer>
#include <QVariantList>
#include <QSet>
#include "settings.h"
#include "managedserver.h"

class Controller : public QObject {
    Q_OBJECT
    Q_PROPERTY(QVariantList providers READ providers NOTIFY providersChanged)
    Q_PROPERTY(QVariantMap state READ state NOTIFY changed)
    Q_PROPERTY(QVariantMap settings READ settings NOTIFY settingsChanged)
    Q_PROPERTY(QVariantList diagnostics READ diagnostics NOTIFY diagnosticsChanged)
public:
    explicit Controller(bool demo = false, const QString &configPath = {}, QObject *parent = nullptr,
                        bool allowAutomaticMigration = true, ManagedServerOptions serverOptions = {});
    QVariantList providers() const;
    QVariantMap state() const;
    QVariantMap settings() const;
    QVariantList diagnostics() const { return m_diagnostics; }
    Q_INVOKABLE void clearDiagnostics();
    Q_INVOKABLE QString diagnosticText() const;
    // C++ integration only: the bearer token is never a QML property.
    QString backendUrl() const;
    QString backendToken() const { return m_token; }
    bool startupPreference() const { return m_settingsService.value().startup; }
    bool startupMigrationPending() const { return m_settingsService.value().startupMigrationPending; }
    QString settingsPath() const { return m_settingsService.path(); }
    QString saveStartupPreference(bool enabled);
    QString completeStartupMigration();
    // C++ integration seam for update preparation; attached/remote servers are untouched.
    void stopOwnedServer() { m_server.stopOwned(); }
    Q_INVOKABLE void refresh();
    Q_INVOKABLE QString warningColor(int severity) const;
    Q_INVOKABLE QString displayName(const QString &provider) const;
    Q_INVOKABLE QString saveSettings(QString mode, QString url, QString token, int interval, bool notifications, QString primary, bool forgetToken);
    Q_INVOKABLE void preview(bool enabled);
    Q_INVOKABLE QVariantMap concern(const QString &provider, const QVariantMap &bucket) const;
    Q_INVOKABLE QVariantList notches(const QString &provider, const QVariantMap &bucket) const;
    Q_INVOKABLE QVariantMap pacing(const QString &provider, const QVariantMap &bucket) const;
    Q_INVOKABLE QString countdown(const QString &timestamp) const;
    Q_INVOKABLE void setPrimary(const QString &name);
    Q_INVOKABLE void copyText(const QString &text);
    QString primary() const;
    Q_INVOKABLE void moveProvider(const QString &source, const QString &target, bool after);
    bool isDemo() const { return m_demo; }
signals:
    void changed();
    void diagnosticsChanged();
    void providersChanged();
    void settingsChanged();
    void notify(const QString &title, const QString &message);
    void usageAlert(const QString &title, const QString &message, int severity);
private:
    void updateMeterStates();
    void fail(const QString &message, const QString &kind = "network");
    void log(const QString &category, const QString &message);
    void resetRetry();
    void cancel();
    void requestUsage();
    QString writeSettings(const QString &mode, const QString &url, const QString &token, int interval, bool notifications, const QString &primary);
    QStringList m_order;
    SettingsService m_settingsService;
    QString m_mode = "remote", m_url, m_token, m_primary = "Claude", m_message, m_status = "setup";
    int m_interval = 60, m_retryAttempt = 0;
    QString m_errorKind;
    QVariantList m_diagnostics;
    bool m_notifications = true, m_demo = false, m_loading = false;
    bool m_waitingForUsageRetry = false;
    qint64 m_lastGood = 0;
    QVariantList m_providers;
    using MeterKey = QPair<QString, QString>;
    QHash<MeterKey, Usage::WarningState> m_warningStates;
    QHash<MeterKey, QVariantMap> m_concerns;
    QNetworkAccessManager m_network;
    ManagedServer m_server;
    QPointer<QNetworkReply> m_reply;
    QTimer m_poll, m_clock;
};
