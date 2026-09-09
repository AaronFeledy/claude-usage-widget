#include "instance.h"

#include <QCryptographicHash>
#include <QDir>
#include <QFileDevice>
#include <QFileInfo>
#include <QLocalServer>
#include <QLocalSocket>
#include <QLockFile>
#include <QStandardPaths>
#include <QTimer>

InstanceService::InstanceService(QString configPath, QObject *parent) : QObject(parent)
{
    QFileInfo info(configPath);
    const QFileInfo parentInfo(info.absolutePath());
    QString canonical = info.exists() ? info.canonicalFilePath()
                                      : QDir(parentInfo.exists() ? parentInfo.canonicalFilePath() : parentInfo.absoluteFilePath())
                                            .filePath(info.fileName());
    QString userRoot = QDir::cleanPath(QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation));
#ifdef Q_OS_WIN
    canonical = canonical.toCaseFolded();
    userRoot = userRoot.toCaseFolded();
#endif
    const QByteArray identity = userRoot.toUtf8() + '\0' + QDir::cleanPath(canonical).toUtf8();
    const QString digest = QString::fromLatin1(QCryptographicHash::hash(identity, QCryptographicHash::Sha256).toHex().left(24));
    const QString lockDirectory = QDir(QStandardPaths::writableLocation(QStandardPaths::TempLocation))
                                      .filePath("Headroom/" + digest);
    QDir().mkpath(lockDirectory);
#ifndef Q_OS_WIN
    QFile(lockDirectory).setPermissions(QFileDevice::ReadOwner | QFileDevice::WriteOwner | QFileDevice::ExeOwner);
#endif
    m_lockPath = QDir(lockDirectory).filePath("instance.lock");
#ifdef Q_OS_WIN
    m_scopeName = "headroom-" + digest;
#else
    m_scopeName = QDir(lockDirectory).filePath("activation.socket");
#endif
}

InstanceService::~InstanceService()
{
    if (m_server) { m_server->close(); delete m_server; }
    if (m_lock) { m_lock->unlock(); delete m_lock; }
}

InstanceService::Result InstanceService::start(int timeoutMilliseconds)
{
    if (m_server || m_lock) return m_server ? Result::Primary : Result::Error;
    m_lock = new QLockFile(m_lockPath);
    if (!m_lock->tryLock()) {
        QLocalSocket socket;
        socket.connectToServer(m_scopeName, QIODevice::WriteOnly);
        if (!socket.waitForConnected(timeoutMilliseconds)) {
            m_error = "Another Headroom instance owns this settings scope but could not be activated.";
            delete m_lock; m_lock = nullptr;
            return Result::Error;
        }
        socket.write("activate\n");
        if (!socket.waitForBytesWritten(timeoutMilliseconds)) {
            m_error = "The running Headroom instance did not accept the activation request.";
            delete m_lock; m_lock = nullptr;
            return Result::Error;
        }
        socket.disconnectFromServer();
        delete m_lock; m_lock = nullptr;
        return Result::Secondary;
    }
    QLocalServer::removeServer(m_scopeName);
    m_server = new QLocalServer(this);
#ifdef Q_OS_WIN
    m_server->setSocketOptions(QLocalServer::UserAccessOption);
#endif
    if (!m_server->listen(m_scopeName)) {
        m_error = "Headroom could not create its private activation endpoint: " + m_server->errorString();
        m_lock->unlock(); delete m_lock; m_lock = nullptr;
        delete m_server; m_server = nullptr;
        return Result::Error;
    }
    connect(m_server, &QLocalServer::newConnection, this, [this] {
        while (auto socket = m_server->nextPendingConnection()) {
            socket->setReadBufferSize(33);
            const auto readActivation = [this, socket] {
                QByteArray buffered = socket->property("headroomActivation").toByteArray();
                buffered += socket->read(qMax<qint64>(0, 33 - buffered.size()));
                if (buffered.size() > 32) { socket->disconnectFromServer(); return; }
                socket->setProperty("headroomActivation", buffered);
                if (buffered.contains('\n')) {
                    if (buffered == "activate\n") emit activationRequested();
                    socket->disconnectFromServer();
                } else if (buffered.size() >= 32 || socket->bytesAvailable() > 0) socket->disconnectFromServer();
            };
            connect(socket, &QLocalSocket::readyRead, this, readActivation);
            connect(socket, &QLocalSocket::disconnected, socket, &QObject::deleteLater);
            QMetaObject::invokeMethod(socket, readActivation, Qt::QueuedConnection);
            QTimer::singleShot(1000, socket, [socket] { socket->disconnectFromServer(); });
        }
    });
    return Result::Primary;
}
