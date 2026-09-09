#pragma once

#include <QObject>
#include <QString>

class QLocalServer;
class QLockFile;

class InstanceService : public QObject {
    Q_OBJECT
public:
    enum class Result { Primary, Secondary, Error };
    explicit InstanceService(QString configPath, QObject *parent = nullptr);
    ~InstanceService() override;
    Result start(int timeoutMilliseconds = 1000);
    QString error() const { return m_error; }
    QString scopeName() const { return m_scopeName; }
    QString lockPath() const { return m_lockPath; }
signals:
    void activationRequested();
private:
    QString m_scopeName;
    QString m_lockPath;
    QString m_error;
    QLocalServer *m_server = nullptr;
    QLockFile *m_lock = nullptr;
};
