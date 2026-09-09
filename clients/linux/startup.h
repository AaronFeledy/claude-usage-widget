#pragma once

#include <QObject>
#include <QString>

class StartupService : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool enabled READ enabled NOTIFY enabledChanged)
    Q_PROPERTY(bool available READ available NOTIFY availableChanged)
    Q_PROPERTY(QString error READ error NOTIFY errorChanged)
public:
    explicit StartupService(QString configHome = {}, QString executable = {},
                            bool allowChanges = true, QObject *parent = nullptr);
    bool enabled() const { return m_enabled; }
    bool available() const { return m_allowChanges; }
    QString error() const { return m_error; }
    QString entryPath() const { return m_entryPath; }
    void setAllowChanges(bool allowed);
    Q_INVOKABLE bool setEnabled(bool enabled);
    Q_INVOKABLE void refresh();
signals:
    void enabledChanged();
    void errorChanged();
    void availableChanged();
private:
    bool fail(const QString &message);
    void clearError();
    QString m_entryPath;
    QString m_executable;
    QString m_error;
    bool m_allowChanges;
    bool m_enabled = false;
};
