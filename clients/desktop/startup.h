#pragma once

#include <QObject>
#include <QString>
#include <functional>

class StartupService : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool enabled READ enabled NOTIFY enabledChanged)
    Q_PROPERTY(bool available READ available NOTIFY availableChanged)
    Q_PROPERTY(QString error READ error NOTIFY errorChanged)
public:
    enum class Platform { Current, Linux, Windows };
    explicit StartupService(QString configHome = {}, QString executable = {},
                            bool allowChanges = true, QObject *parent = nullptr,
                            Platform platform = Platform::Current, QString registryPath = {});
    bool enabled() const { return m_enabled; }
    bool available() const { return m_allowChanges && m_platformSupported; }
    QString error() const { return m_error; }
    QString entryPath() const { return m_entryPath; }
    static QString defaultExecutablePath();
    static QString packagedExecutablePath(const QString &applicationPath);
    void setAllowChanges(bool allowed);
    void setPreferenceWriter(std::function<QString(bool)> writer) { m_preferenceWriter = std::move(writer); }
    Q_INVOKABLE bool setEnabled(bool enabled);
    Q_INVOKABLE void refresh();
    bool migrateLegacyRegistration(bool enabled);
signals:
    void enabledChanged();
    void errorChanged();
    void availableChanged();
    void preferenceChanged(bool enabled);
private:
    void repairPackagedRegistration();
    bool fail(const QString &message);
    bool persistPreference(bool enabled);
    void clearError();
    QString m_entryPath;
    QString m_executable;
    QString m_error;
    QString m_registryPath;
    std::function<QString(bool)> m_preferenceWriter;
    bool m_allowChanges;
    bool m_windows;
    bool m_platformSupported = true;
    bool m_enabled = false;
};
