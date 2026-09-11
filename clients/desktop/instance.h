#pragma once

#include <QObject>
#include <QString>
#include <QByteArray>
#include <functional>

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
    // Bounded, same-user local control. The handler never receives credentials.
    void setRequestHandler(std::function<QByteArray(const QByteArray &)> handler) { m_requestHandler = std::move(handler); }
    QByteArray request(const QByteArray &message, int timeoutMilliseconds = 3000);
    bool primaryUnavailable() const { return m_primaryUnavailable; }
signals:
    void activationRequested();
private:
    QString m_scopeName;
    QString m_lockPath;
    QString m_error;
    bool m_pathsReady = false;
    bool m_primaryUnavailable = false;
    std::function<QByteArray(const QByteArray &)> m_requestHandler;
    QLocalServer *m_server = nullptr;
    QLockFile *m_lock = nullptr;
};
