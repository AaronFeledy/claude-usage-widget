#include "credentialservice.h"
#include "usage.h"

#include <QCoreApplication>
#include <QCryptographicHash>
#include <QDateTime>
#include <QDir>
#include <QFileInfo>
#include <QHostAddress>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <QNetworkProxy>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QProcessEnvironment>
#include <algorithm>
#include <utility>

namespace {
constexpr qsizetype maximumHelperBytes = 64 * 1024;
constexpr qsizetype maximumResponseBytes = 1024 * 1024;
const QString cursorPrompt = QStringLiteral("Log in to cursor.com, or push Cursor credentials from the tray.");

QString canonical(const QString &provider)
{
    if (provider.compare(QStringLiteral("Cursor"), Qt::CaseInsensitive) == 0) return QStringLiteral("Cursor");
    if (provider.compare(QStringLiteral("Grok"), Qt::CaseInsensitive) == 0) return QStringLiteral("Grok");
    return {};
}

bool hasControl(const QString &value)
{
    return std::any_of(value.cbegin(), value.cend(), [](QChar character) {
        return character.unicode() < 0x20 || character.unicode() == 0x7f;
    });
}

bool recoveryResolved(const QString &provider, const QVariantMap &usage)
{
    if (!usage.value(QStringLiteral("is_success")).toBool()) return false;
    if (provider != QStringLiteral("Grok")) return true;
    for (const auto &bucket : usage.value(QStringLiteral("buckets")).toList())
        if (bucket.toMap().value(QStringLiteral("id")).toString().compare(QStringLiteral("weekly"), Qt::CaseInsensitive) == 0)
            return true;
    return false;
}
}

CredentialService::CredentialService(CredentialServiceOptions options, QObject *parent)
    : QObject(parent), m_options(std::move(options))
{
    m_localNetwork.setProxy(QNetworkProxy::NoProxy);
    m_helperDeadline.setSingleShot(true);
    connect(&m_helperDeadline, &QTimer::timeout, this, [this] {
        m_helperFailed = true;
        if (m_process) m_process->kill();
    });
}

CredentialService::~CredentialService()
{
    ++m_operation;
    if (m_reply) { m_reply->abort(); delete m_reply; m_reply = nullptr; }
    if (m_process) {
        disconnect(m_process, nullptr, this, nullptr);
        if (m_process->state() != QProcess::NotRunning) {
            m_process->kill();
            m_process->waitForFinished(2000);
        }
        delete m_process;
        m_process = nullptr;
    }
        m_snapshotDirectory.reset();
    for (auto process : std::as_const(m_retiringProcesses)) {
        disconnect(process, nullptr, this, nullptr);
        if (process->state() != QProcess::NotRunning) {
            process->kill();
            process->waitForFinished(2000);
        }
        delete process;
    }
    m_retiringProcesses.clear();
}

void CredentialService::configure(const QString &mode, const QString &baseUrl, const QString &token,
                                  const QSslCertificate &certificate)
{
    const QString normalizedMode = mode == QStringLiteral("local") ? QStringLiteral("local") : QStringLiteral("remote");
    if (normalizedMode == m_mode && baseUrl == m_baseUrl && token == m_token && certificate == m_certificate) return;
    cancel();
    m_localNetwork.clearConnectionCache();
    m_mode = normalizedMode;
    m_baseUrl = baseUrl;
    m_token = token;
    m_certificate = certificate;
    m_attempts.clear();
}

void CredentialService::cancel()
{
    m_queue.clear();
    m_helperDeadline.stop();
    ++m_operation;
    if (m_reply) {
        disconnect(m_reply, nullptr, this, nullptr);
        m_reply->abort();
        m_reply->deleteLater();
        m_reply.clear();
    }
    if (m_process) {
        auto process = m_process;
        m_process = nullptr;
        disconnect(process, nullptr, this, nullptr);
        const auto snapshot = std::move(m_snapshotDirectory);
        if (process->state() == QProcess::NotRunning) {
            process->deleteLater();
        } else {
            m_retiringProcesses.append(process);
            connect(process, qOverload<int, QProcess::ExitStatus>(&QProcess::finished), this,
                [this, process, snapshot](int, QProcess::ExitStatus) {
                    if (snapshot) QDir(snapshot->path()).removeRecursively();
                    m_retiringProcesses.removeOne(process);
                    process->deleteLater();
                    continueQueue();
                });
            process->kill();
        }
    }
    m_helperOutput.fill('\0');
    m_helperOutput.clear();
    m_snapshotDirectory.reset();
    m_activeProvider.clear();
}

void CredentialService::cancelActiveAttempt()
{
    const QStringList queued = m_queue;
    cancel();
    m_queue = queued;
    continueQueue();
}

void CredentialService::consider(const QVariantList &providers)
{
    if (!m_options.enabled || m_baseUrl.isEmpty()) return;
    bool cursorCondition = false;
    bool grokCondition = false;
    for (const auto &value : providers) {
        const auto provider = value.toMap();
        const QString name = provider.value(QStringLiteral("provider_name")).toString();
        if (name == QStringLiteral("Cursor")) {
            cursorCondition = provider.value(QStringLiteral("needs_reauth")).toBool()
                || provider.value(QStringLiteral("error")).toString() == cursorPrompt;
        } else if (name == QStringLiteral("Grok") && provider.value(QStringLiteral("is_success")).toBool()) {
            grokCondition = true;
            for (const auto &bucket : provider.value(QStringLiteral("buckets")).toList())
                if (bucket.toMap().value(QStringLiteral("id")).toString().compare(QStringLiteral("weekly"), Qt::CaseInsensitive) == 0)
                    grokCondition = false;
        }
    }
    const qint64 now = QDateTime::currentMSecsSinceEpoch();
    for (const auto &entry : {qMakePair(QStringLiteral("Cursor"), cursorCondition), qMakePair(QStringLiteral("Grok"), grokCondition)}) {
        auto &attempt = m_attempts[entry.first];
        if (!entry.second) {
            attempt = {};
            m_queue.removeAll(entry.first);
            if (m_activeProvider == entry.first) cancelActiveAttempt();
            continue;
        }
        attempt.condition = true;
        if (now >= attempt.nextDiscovery && entry.first != m_activeProvider && !m_queue.contains(entry.first)) {
            attempt.nextDiscovery = now + qMax(1, m_options.retryCooldownMs);
            m_queue.append(entry.first);
        }
    }
    continueQueue();
}

void CredentialService::continueQueue()
{
    if (busy() || m_queue.isEmpty()) return;
    const QString provider = m_queue.takeFirst();
    if (!m_attempts.value(provider).condition) { continueQueue(); return; }
    resolvePolicy(provider);
}

void CredentialService::resolvePolicy(const QString &provider)
{
    const QUrl endpoint = credentialEndpoint(provider);
    if (endpoint.isEmpty()) { continueQueue(); return; }
    if (hasControl(m_token)) {
        emit event(QStringLiteral("Browser credential forwarding was skipped because the configured token is invalid."));
        continueQueue();
        return;
    }
    if (endpoint.scheme() == QStringLiteral("https")) {
        if (m_mode == QStringLiteral("local") && m_certificate.isNull()) {
            emit event(QStringLiteral("Browser credential forwarding requires Headroom's verified private local connection."));
            continueQueue();
            return;
        }
        startHelper(provider, !m_certificate.isNull());
        return;
    }
    if (endpoint.scheme() == QStringLiteral("http"))
        emit event(QStringLiteral("Browser credential forwarding requires HTTPS or Headroom's verified private local connection."));
    continueQueue();
}

QString CredentialService::helperPath() const
{
    if (!m_options.helperPath.isEmpty()) return m_options.helperPath;
#ifdef Q_OS_WIN
    return QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("headroom-credential-helper.exe"));
#else
    return QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("headroom-credential-helper"));
#endif
}

void CredentialService::startHelper(const QString &provider, bool localNoProxy)
{
    const QString executable = helperPath();
    if (!QFileInfo(executable).isExecutable()) {
        emit event(QStringLiteral("Browser credential forwarding helper is unavailable."));
        continueQueue();
        return;
    }
    m_snapshotDirectory = std::make_shared<QTemporaryDir>(QDir(QDir::tempPath()).filePath(QStringLiteral("headroom-credential-XXXXXX")));
    if (!m_snapshotDirectory->isValid()) {
        m_snapshotDirectory.reset();
        emit event(QStringLiteral("Browser credential forwarding could not prepare a private snapshot."));
        continueQueue();
        return;
    }
    m_activeProvider = provider;
    m_activeLocalNoProxy = localNoProxy;
    m_helperFailed = false;
    m_helperOutput.clear();
    auto process = new QProcess(this);
    m_process = process;
    const quint64 operation = ++m_operation;
    process->setProgram(executable);
    process->setArguments({provider.toLower()});
    process->setProcessChannelMode(QProcess::SeparateChannels);
    process->setStandardErrorFile(QProcess::nullDevice());
    auto environment = QProcessEnvironment::systemEnvironment();
    environment.insert(QStringLiteral("HEADROOM_CREDENTIAL_SNAPSHOT_ROOT"), m_snapshotDirectory->path());
    process->setProcessEnvironment(environment);
    connect(process, &QProcess::readyReadStandardOutput, this, [this, process, operation] {
        if (m_process != process || m_operation != operation) return;
        m_helperOutput.append(process->readAllStandardOutput());
        if (m_helperOutput.size() > maximumHelperBytes) { m_helperFailed = true; process->kill(); }
    });
    connect(process, &QProcess::errorOccurred, this, [this, process, operation](QProcess::ProcessError error) {
        if (m_process == process && m_operation == operation && error == QProcess::FailedToStart) finishHelper(false);
    });
    connect(process, qOverload<int, QProcess::ExitStatus>(&QProcess::finished), this,
        [this, process, operation](int code, QProcess::ExitStatus status) {
            if (m_process == process && m_operation == operation)
                finishHelper(!m_helperFailed && code == 0 && status == QProcess::NormalExit);
        });
    process->start();
    m_helperDeadline.start(m_options.helperTimeoutMs);
}

void CredentialService::finishHelper(bool success)
{
    if (!m_process) return;
    m_helperDeadline.stop();
    auto process = m_process;
    m_process = nullptr;
    m_helperOutput.append(process->readAllStandardOutput());
    process->deleteLater();
    m_snapshotDirectory.reset();
    const QString provider = m_activeProvider;
    const bool localNoProxy = m_activeLocalNoProxy;
    m_activeProvider.clear();
    QByteArray cookie;
    if (success && m_helperOutput.size() <= maximumHelperBytes) {
        QJsonParseError error;
        const auto document = QJsonDocument::fromJson(m_helperOutput, &error);
        const auto object = document.object();
        if (error.error == QJsonParseError::NoError && document.isObject()
            && canonical(object.value(QStringLiteral("provider")).toString()) == provider
            && (object.value(QStringLiteral("cookie")).isString() || object.value(QStringLiteral("cookie")).isNull()))
            cookie = object.value(QStringLiteral("cookie")).toString().trimmed().toUtf8();
    }
    m_helperOutput.fill('\0');
    m_helperOutput.clear();
    const QString cookieText = QString::fromUtf8(cookie);
    if (cookie.isEmpty() || cookie.size() > 48 * 1024 || hasControl(cookieText)) {
        cookie.fill('\0');
        continueQueue();
        return;
    }
    const QByteArray fingerprint = QCryptographicHash::hash(cookie, QCryptographicHash::Sha256);
    if (m_attempts.value(provider).successfulFingerprint == fingerprint) {
        cookie.fill('\0');
        continueQueue();
        return;
    }
    submit(provider, std::move(cookie), fingerprint, localNoProxy);
}

void CredentialService::submit(const QString &provider, QByteArray cookie, const QByteArray &fingerprint, bool localNoProxy)
{
    const QUrl endpoint = credentialEndpoint(provider);
    if (endpoint.isEmpty()) { cookie.fill('\0'); continueQueue(); return; }
    if (localNoProxy && (m_mode != QStringLiteral("local") || m_certificate.isNull())) {
        cookie.fill('\0');
        emit event(QStringLiteral("Browser credential forwarding requires Headroom's verified private local connection."));
        continueQueue();
        return;
    }
    QNetworkRequest request(endpoint);
    request.setRawHeader("Accept", "application/json");
    request.setRawHeader("Content-Type", "application/json");
    request.setRawHeader("User-Agent", "Headroom/0.1");
    request.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::ManualRedirectPolicy);
    request.setTransferTimeout(m_options.requestTimeoutMs);
    if (!m_token.isEmpty()) request.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());
    if (localNoProxy) ServerTransport::secureRequest(request, m_certificate);
    const QByteArray body = QJsonDocument(QJsonObject{{QStringLiteral("cookie"), QString::fromUtf8(cookie)}}).toJson(QJsonDocument::Compact);
    cookie.fill('\0');
    m_activeProvider = provider;
    auto reply = (localNoProxy ? &m_localNetwork : &m_remoteNetwork)->put(request, body);
    if (localNoProxy) ServerTransport::requirePinnedPeer(reply, m_certificate);
    m_reply = reply;
    const quint64 operation = ++m_operation;
    auto deadline = new QTimer(reply);
    deadline->setSingleShot(true);
    connect(deadline, &QTimer::timeout, reply, &QNetworkReply::abort);
    deadline->start(m_options.requestTimeoutMs);
    connect(reply, &QNetworkReply::readyRead, this, [this, reply, operation] {
        if (m_reply == reply && m_operation == operation && reply->bytesAvailable() > maximumResponseBytes) reply->abort();
    });
    connect(reply, &QNetworkReply::finished, this, [this, provider, fingerprint, reply, operation] {
        if (m_reply == reply && m_operation == operation) finishRequest(provider, fingerprint, reply);
    });
}

void CredentialService::finishRequest(const QString &provider, const QByteArray &fingerprint, QNetworkReply *reply)
{
    if (reply != m_reply) return;
    m_reply.clear();
    m_activeProvider.clear();
    const auto status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    const bool redirected = reply->attribute(QNetworkRequest::RedirectionTargetAttribute).isValid();
    const auto error = reply->error();
    QByteArray body = reply->readAll();
    reply->deleteLater();
    QVariantMap recovered;
    if (!redirected && error == QNetworkReply::NoError && status == 200 && body.size() <= maximumResponseBytes) {
        QJsonParseError parseError;
        const auto document = QJsonDocument::fromJson(body, &parseError);
        const auto object = document.object();
        const auto usage = object.value(QStringLiteral("usage"));
        QVariantList parsed;
        if (parseError.error == QJsonParseError::NoError && document.isObject()
            && canonical(object.value(QStringLiteral("provider")).toString()) == provider
            && object.value(QStringLiteral("refetched")).toBool(false) && usage.isObject()
            && Usage::parse(QJsonDocument(QJsonArray{usage}).toJson(), parsed) && parsed.size() == 1
            && parsed.first().toMap().value(QStringLiteral("provider_name")).toString() == provider
            && recoveryResolved(provider, parsed.first().toMap()))
            recovered = parsed.first().toMap();
    }
    body.fill('\0');
    if (!recovered.isEmpty()) {
        m_attempts[provider].successfulFingerprint = fingerprint;
        emit event(QStringLiteral("Browser credentials refreshed a provider."));
        emit providerRecovered(recovered);
    } else {
        emit event(QStringLiteral("Browser credential forwarding did not refresh the provider."));
    }
    continueQueue();
}

QUrl CredentialService::credentialEndpoint(const QString &provider) const
{
    const QUrl usageEndpoint = Usage::endpoint(m_baseUrl);
    if (usageEndpoint.isEmpty() || canonical(provider).isEmpty()) return {};
    QUrl result(usageEndpoint);
    QString path = result.path();
    path.chop(QStringLiteral("api/v1/usage").size());
    result.setPath(path + QStringLiteral("api/v1/providers/") + provider.toLower() + QStringLiteral("/credentials"));
    return result;
}
