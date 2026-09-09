#include "managedserver.h"

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QProcessEnvironment>
#include <utility>

namespace {
constexpr qsizetype maximumHealthBytes = 1024 * 1024;

bool compatibleHealth(const QByteArray &body)
{
    QJsonParseError error;
    const auto document = QJsonDocument::fromJson(body, &error);
    if (error.error != QJsonParseError::NoError || !document.isObject()) return false;
    const auto object = document.object();
    const QString status = object.value(QStringLiteral("status")).toString();
    if (status != QStringLiteral("ok") && status != QStringLiteral("degraded")) return false;
    if (!object.value(QStringLiteral("version")).isString()
        || object.value(QStringLiteral("version")).toString().trimmed().isEmpty()
        || !object.value(QStringLiteral("providers")).isArray()) return false;
    for (const auto &item : object.value(QStringLiteral("providers")).toArray()) {
        if (!item.isObject()) return false;
        const auto provider = item.toObject();
        if (!provider.value(QStringLiteral("name")).isString()
            || provider.value(QStringLiteral("name")).toString().isEmpty()
            || !provider.value(QStringLiteral("ok")).isBool()
            || !(provider.value(QStringLiteral("fetched_at")).isString()
                 || provider.value(QStringLiteral("fetched_at")).isNull())) return false;
    }
    return true;
}
}

ManagedServer::ManagedServer(ManagedServerOptions options, QObject *parent)
    : QObject(parent), m_options(std::move(options))
{
    m_readinessTimer.setSingleShot(true);
    m_restartTimer.setSingleShot(true);
    connect(&m_readinessTimer, &QTimer::timeout, this, [this] { probe(ProbePurpose::Readiness); });
    connect(&m_restartTimer, &QTimer::timeout, this, [this] { probe(ProbePurpose::Initial); });
}

ManagedServer::~ManagedServer()
{
    m_ensureAfterRetire = false;
    ++m_generation;
    cancelAsync();
    auto process = m_process;
    m_process = nullptr;
    if (process) {
        process->disconnect(this);
        if (process->state() != QProcess::NotRunning) process->kill();
        closeWindowsJob();
        if (process->state() != QProcess::NotRunning) process->waitForFinished(1000);
        delete process;
    } else closeWindowsJob();
    auto retiring = m_retiringProcess;
    m_retiringProcess = nullptr;
    if (retiring) {
        retiring->disconnect(this);
        if (retiring->state() != QProcess::NotRunning) retiring->kill();
        if (retiring->state() != QProcess::NotRunning) retiring->waitForFinished(1000);
        delete retiring;
    }
}

void ManagedServer::configure(const QString &mode, const QString &token)
{
    const QString normalized = mode == QStringLiteral("local") ? QStringLiteral("local") : QStringLiteral("remote");
    if (normalized == m_mode && token == m_token) return;
    ++m_generation;
    cancelAsync();
    m_ensureAfterRetire = normalized == QStringLiteral("local");
    disposeProcess(true);
    m_mode = normalized;
    m_token = token;
    m_available = false;
    m_owned = false;
    m_readinessAttempt = 0;
    m_restartCount = 0;
    m_state = m_mode == QStringLiteral("local") ? QStringLiteral("idle") : QStringLiteral("remote");
    m_message.clear();
    emit stateChanged();
}

void ManagedServer::ensureAvailable()
{
    if (m_mode != QStringLiteral("local")) return;
    if (m_available) {
        emit available();
        return;
    }
    if (m_retiringProcess) {
        m_ensureAfterRetire = true;
        return;
    }
    if (m_probe || m_readinessTimer.isActive() || m_restartTimer.isActive()
        || (m_process && m_process->state() == QProcess::Starting)) return;
    if (m_state == QStringLiteral("failed")) m_restartCount = 0;
    m_readinessAttempt = 0;
    m_state = QStringLiteral("probing");
    m_message.clear();
    emit stateChanged();
    probe(ProbePurpose::Initial);
}

void ManagedServer::reportConnectionFailure()
{
    if (m_mode != QStringLiteral("local") || !m_available) return;
    cancelAsync();
    m_available = false;
    m_state = QStringLiteral("probing");
    emit stateChanged();
    if (m_owned && m_process && m_process->state() != QProcess::NotRunning) {
        beginReadiness();
    } else {
        m_owned = false;
        probe(ProbePurpose::Initial);
    }
}

void ManagedServer::stopOwned()
{
    ++m_generation;
    cancelAsync();
    m_ensureAfterRetire = false;
    disposeProcess(true);
    m_available = false;
    m_owned = false;
    m_state = m_mode == QStringLiteral("local") ? QStringLiteral("idle") : QStringLiteral("remote");
    emit stateChanged();
}

void ManagedServer::cancelAsync()
{
    m_readinessTimer.stop();
    m_restartTimer.stop();
    if (m_probe) {
        auto reply = m_probe;
        m_probe.clear();
        reply->disconnect(this);
        reply->abort();
        reply->deleteLater();
    }
}

void ManagedServer::probe(ProbePurpose purpose)
{
    if (m_mode != QStringLiteral("local") || m_probe) return;
    QUrl endpoint = m_options.localUrl.resolved(QUrl(QStringLiteral("api/v1/health")));
    QNetworkRequest request(endpoint);
    request.setRawHeader("Accept", "application/json");
    request.setRawHeader("User-Agent", "Headroom/" HEADROOM_VERSION);
    request.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::ManualRedirectPolicy);
    request.setTransferTimeout(m_options.probeTimeoutMs);
    if (!m_token.isEmpty()) request.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());
    const quint64 generation = m_generation;
    auto reply = m_network.get(request);
    m_probe = reply;
    reply->setReadBufferSize(maximumHealthBytes + 1);
    auto timeout = new QTimer(reply);
    timeout->setSingleShot(true);
    connect(timeout, &QTimer::timeout, reply, [reply] {
        reply->setProperty("headroomTimedOut", true);
        if (!reply->isFinished()) reply->abort();
    });
    timeout->start(m_options.probeTimeoutMs);
    connect(reply, &QIODevice::readyRead, reply, [reply] {
        if (reply->bytesAvailable() > maximumHealthBytes) reply->abort();
    });
    connect(reply, &QNetworkReply::finished, this, [this, reply, generation, purpose] {
        if (m_probe == reply) m_probe.clear();
        const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        const auto error = reply->error();
        const QByteArray body = reply->readAll();
        ProbeResult result = ProbeResult::NetworkFailure;
        if (reply->property("headroomTimedOut").toBool()) result = ProbeResult::TimedOut;
        else if (status == 401 || status == 403) result = ProbeResult::AuthRejected;
        else if (status >= 300 && status < 400) result = ProbeResult::Redirected;
        else if (error == QNetworkReply::ConnectionRefusedError && status == 0) result = ProbeResult::Refused;
        else if (error == QNetworkReply::NoError && status == 200
                 && body.size() <= maximumHealthBytes && compatibleHealth(body)) result = ProbeResult::Compatible;
        else if (error == QNetworkReply::NoError && status == 200) result = ProbeResult::Malformed;
        reply->deleteLater();
        if (generation != m_generation || m_mode != QStringLiteral("local")) return;
        handleProbe(purpose, result);
    });
}

void ManagedServer::handleProbe(ProbePurpose purpose, ProbeResult result)
{
    if (result == ProbeResult::Compatible) {
        setAvailable(purpose == ProbePurpose::Readiness && m_process, purpose == ProbePurpose::Initial
            ? QStringLiteral("attached") : QStringLiteral("started"));
        return;
    }
    if (purpose == ProbePurpose::Initial && result == ProbeResult::Refused) {
        spawn();
        return;
    }
    if (purpose == ProbePurpose::Readiness
        && (result == ProbeResult::Refused || result == ProbeResult::TimedOut || result == ProbeResult::NetworkFailure)
        && ++m_readinessAttempt < m_options.readinessAttempts) {
        m_readinessTimer.start(m_options.readinessIntervalMs);
        return;
    }
    if (purpose == ProbePurpose::Readiness) {
        m_ensureAfterRetire = false;
        disposeProcess(true);
    }
    switch (result) {
    case ProbeResult::AuthRejected:
        setFailure(QStringLiteral("The local server rejected the saved bearer token."), QStringLiteral("auth")); break;
    case ProbeResult::Redirected:
    case ProbeResult::Malformed:
        setFailure(QStringLiteral("Port 7823 is occupied by an incompatible local service."), QStringLiteral("local-service")); break;
    case ProbeResult::TimedOut:
        setFailure(purpose == ProbePurpose::Initial
            ? QStringLiteral("The local server health check timed out. No sidecar was started.")
            : QStringLiteral("The bundled local server did not answer its health check in time."), QStringLiteral("timeout")); break;
    case ProbeResult::NetworkFailure:
        setFailure(QStringLiteral("The local endpoint could not be verified. No sidecar was started."), QStringLiteral("network")); break;
    case ProbeResult::Refused:
        setFailure(QStringLiteral("The bundled local server did not become ready in time."), QStringLiteral("readiness")); break;
    case ProbeResult::Compatible: break;
    }
}

QString ManagedServer::binaryPath() const
{
    if (!m_options.executablePath.isEmpty()) return QFileInfo(m_options.executablePath).absoluteFilePath();
#ifdef Q_OS_WIN
    return QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("usage-server.exe"));
#else
    return QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("usage-server"));
#endif
}

void ManagedServer::spawn()
{
    const QString path = binaryPath();
    const QFileInfo binary(path);
    if (!binary.isFile() || !binary.isExecutable()) {
        setFailure(QStringLiteral("The bundled usage server is missing. Reinstall or repair Headroom."), QStringLiteral("binary"));
        return;
    }
    m_ensureAfterRetire = false;
    disposeProcess(true);
    auto process = new QProcess(this);
    m_process = process;
    m_owned = true;
    m_stopping = false;
    process->setProgram(path);
    process->setArguments({QStringLiteral("--listen-addr"),
        QStringLiteral("127.0.0.1:%1").arg(m_options.localUrl.port(7823))});
    auto environment = QProcessEnvironment::systemEnvironment();
    if (!m_token.isEmpty()) environment.insert(QStringLiteral("USAGE_AUTH_TOKEN"), m_token);
    process->setProcessEnvironment(environment);
    process->setStandardOutputFile(QProcess::nullDevice());
    process->setStandardErrorFile(QProcess::nullDevice());
    const quint64 generation = m_generation;
    connect(process, &QProcess::started, this, [this, process, generation] {
        if (generation != m_generation || process != m_process) return;
        if (!assignWindowsJob()) {
            m_ensureAfterRetire = false;
            disposeProcess(true);
            setFailure(QStringLiteral("Headroom could not safely own the local server process."), QStringLiteral("ownership"));
            return;
        }
        m_state = QStringLiteral("starting");
        emit stateChanged();
        beginReadiness();
    });
    connect(process, &QProcess::errorOccurred, this, [this, process, generation](QProcess::ProcessError error) {
        if (generation != m_generation || process != m_process || error != QProcess::FailedToStart) return;
        disposeProcess(false);
        setFailure(QStringLiteral("The bundled usage server could not be started."), QStringLiteral("launch"));
    });
    connect(process, qOverload<int, QProcess::ExitStatus>(&QProcess::finished), this,
            [this, process, generation](int, QProcess::ExitStatus) {
        if (generation != m_generation || process != m_process || m_stopping) return;
        cancelAsync();
        ++m_generation;
        closeWindowsJob();
        m_process = nullptr;
        process->deleteLater();
        m_owned = false;
        m_available = false;
        m_readinessTimer.stop();
        if (m_mode != QStringLiteral("local")) return;
        if (m_restartCount >= m_options.restartLimit) {
            setFailure(QStringLiteral("The local server stopped repeatedly. Use Refresh to retry."), QStringLiteral("restart"));
            return;
        }
        ++m_restartCount;
        m_state = QStringLiteral("restarting");
        emit stateChanged();
        m_restartTimer.start(qMin(1000, 100 * m_restartCount));
    });
    m_state = QStringLiteral("starting");
    emit stateChanged();
    process->start();
}

void ManagedServer::beginReadiness()
{
    m_readinessAttempt = 0;
    probe(ProbePurpose::Readiness);
}

void ManagedServer::setAvailable(bool owned, const QString &state)
{
    m_owned = owned;
    m_available = true;
    m_state = state;
    m_message.clear();
    emit stateChanged();
    emit available();
}

void ManagedServer::setFailure(const QString &message, const QString &kind)
{
    m_available = false;
    m_owned = false;
    m_state = QStringLiteral("failed");
    m_message = message;
    emit stateChanged();
    emit unavailable(message, kind);
}

void ManagedServer::disposeProcess(bool kill)
{
    auto process = m_process;
    m_process = nullptr;
    m_owned = false;
    m_available = false;
    if (!process) {
        closeWindowsJob();
        return;
    }
    m_stopping = true;
    process->disconnect(this);
    if (kill && process->state() != QProcess::NotRunning) process->kill();
    closeWindowsJob();
    if (process->state() == QProcess::NotRunning) process->deleteLater();
    else {
        m_retiringProcess = process;
        connect(process, qOverload<int, QProcess::ExitStatus>(&QProcess::finished), this,
                [this, process](int, QProcess::ExitStatus) {
            if (m_retiringProcess == process) m_retiringProcess.clear();
            process->deleteLater();
            if (m_ensureAfterRetire && m_mode == QStringLiteral("local")) {
                m_ensureAfterRetire = false;
                QTimer::singleShot(0, this, &ManagedServer::ensureAvailable);
            }
        });
    }
    m_stopping = false;
}
