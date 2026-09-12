#include "controller.h"
#include "usage.h"
#include <QCryptographicHash>
#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSaveFile>
#include <QRegularExpression>
#include <QUuid>

// IMPORTANT: DO NOT test the reset button, its endpoint, or any code that might
// trigger a reset. It can burn a very valuable banked reset. The corresponding
// skipped tests are deliberate safeguards, not missing tests to implement.
namespace {
bool validFingerprint(const QString &value) {
    static const QRegularExpression pattern(QStringLiteral("^[0-9a-f]{64}$"));
    return value.size() == 64 && pattern.match(value).hasMatch();
}

QString canonicalWindow(const QVariantMap &weekly) {
    const auto reset = QDateTime::fromString(weekly.value("resets_at").toString(), Qt::ISODateWithMs);
    return reset.isValid() ? reset.toUTC().toString(Qt::ISODateWithMs) : QString();
}

bool readReceipt(const QString &path, QJsonObject &receipt) {
    if (!QFileInfo::exists(path)) return true;
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly) || file.size() > 4096) return false;
    QJsonParseError error;
    const auto document = QJsonDocument::fromJson(file.readAll(), &error);
    receipt = document.object();
    return error.error == QJsonParseError::NoError && document.isObject()
        && !QUuid(receipt.value("request_id").toString()).isNull()
        && receipt.value("completed").isBool();
}
bool writeReceipt(const QString &path, const QJsonObject &receipt) {
    if (!QDir().mkpath(QFileInfo(path).absolutePath())) return false;
    QSaveFile file(path);
    if (!file.open(QIODevice::WriteOnly)) return false;
    if (!file.setPermissions(QFileDevice::ReadOwner | QFileDevice::WriteOwner)) return false;
    const auto bytes = QJsonDocument(receipt).toJson(QJsonDocument::Compact);
    return file.write(bytes) == bytes.size() && file.commit();
}
}

QVariantMap Controller::chatGptWeekly() const {
    for (const auto &value : m_providers) {
        const auto provider = value.toMap();
        if (provider.value("provider_name").toString() != "Codex" || !provider.value("is_success").toBool()) continue;
        for (const auto &bucket : provider.value("buckets").toList()) {
            auto weekly = bucket.toMap();
            if (weekly.value("id").toString() != "weekly") continue;
            const auto credits = provider.value("rate_limit_reset_credits").toMap();
            weekly.insert("available_count", credits.value("available_count"));
            weekly.insert("account_fingerprint", credits.value("account_fingerprint"));
            return weekly;
        }
    }
    return {};
}

bool Controller::chatGptResetEligible() const {
    const auto weekly = chatGptWeekly();
    return m_status == "ready" && !m_loading && !m_resetBusy && m_lastGood > 0
        && QDateTime::currentSecsSinceEpoch() - m_lastGood <= qMax(120, m_interval * 2)
        && weekly.value("utilization").toDouble() >= 95
        && weekly.value("available_count").toDouble() > 0
        && validFingerprint(weekly.value("account_fingerprint").toString())
        && m_resetBlockedReceipt != resetReceiptPath();
}

QString Controller::resetConnectionIdentity() const {
    // Confirmation is invalidated by a connection, token, or pinned-peer change.
    const auto bytes = m_mode.toUtf8() + '\0' + backendUrl().toUtf8() + '\0'
        + backendToken().toUtf8() + '\0' + backendCertificate().toDer() + '\0'
        + chatGptWeekly().value("account_fingerprint").toString().toUtf8();
    return QString::fromLatin1(QCryptographicHash::hash(bytes, QCryptographicHash::Sha256).toHex());
}

QString Controller::resetReceiptPath() const {
    // The opaque server-derived fingerprint keeps one redemption identity for
    // an account across local launches, transports, and equivalent addresses.
    const QString fingerprint = chatGptWeekly().value("account_fingerprint").toString();
    if (!validFingerprint(fingerprint)) return {};
    return m_settingsService.path() + ".reset-account-" + fingerprint + ".json";
}

QStringList Controller::legacyResetReceiptPaths() const {
    // Detect every receipt written before account binding, including one made
    // through a different transport or address. None can be safely adopted
    // because its UUID may already have reached another account.
    const QFileInfo settings(m_settingsService.path());
    const QString prefix = settings.fileName() + QStringLiteral(".reset-");
    QStringList paths;
    for (const auto &entry : QDir(settings.absolutePath()).entryInfoList(QDir::Files | QDir::NoDotAndDotDot)) {
        const QString name = entry.fileName();
        if (!name.startsWith(prefix) || !name.endsWith(QStringLiteral(".json"))) continue;
        const QString key = name.mid(prefix.size(), name.size() - prefix.size() - QStringLiteral(".json").size());
        if (validFingerprint(key)) paths.append(entry.absoluteFilePath());
    }
    return paths;
}

QVariantMap Controller::resetAction() const {
    return {{"busy", m_resetBusy}, {"message", m_resetMessage},
        {"enabled", chatGptResetEligible()},
        {"canConfirm", !m_resetConfirmation.isEmpty() && m_resetConfirmation == resetConnectionIdentity() && chatGptResetEligible()}};
}

bool Controller::prepareChatGptReset() {
    m_resetConfirmation.clear();
    m_resetMessage.clear();
    if (!chatGptResetEligible()) {
        m_resetMessage = "Refresh usage before using a reset. Weekly usage must be at least 95% and a banked reset must be available.";
        emit changed(); return false;
    }
    const auto weekly = chatGptWeekly();
    const QString fingerprint = weekly.value("account_fingerprint").toString();
    const QString receiptPath = resetReceiptPath();
    QJsonObject receipt;
    if (!readReceipt(receiptPath, receipt)) {
        m_resetMessage = "The previous reset request could not be read safely. Check your usage in ChatGPT.";
        emit changed(); return false;
    }
    if (!receipt.isEmpty() && receipt.value("account_fingerprint").toString() != fingerprint) {
        m_resetMessage = "The previous reset request is not bound to this ChatGPT account. Check your usage in ChatGPT.";
        emit changed(); return false;
    }
    for (const QString &legacyPath : legacyResetReceiptPaths()) {
        QJsonObject legacy;
        if (!readReceipt(legacyPath, legacy)) {
            m_resetMessage = "The previous reset request could not be read safely. Check your usage in ChatGPT.";
        } else if (!legacy.value("completed").toBool()) {
            m_resetMessage = "A previous reset request is not bound to a ChatGPT account and cannot be retried safely. Check it in ChatGPT.";
        } else {
            m_resetBlockedReceipt = legacyPath;
            m_resetMessage = "A reset was already used. Waiting for updated weekly usage.";
        }
        emit changed(); return false;
    }
    if (receipt.value("completed").toBool()) {
        m_resetBlockedReceipt = receiptPath;
        m_resetMessage = "A reset was already used. Waiting for updated weekly usage.";
        emit changed(); return false;
    }
    if (!receipt.isEmpty()) m_resetMessage = "The previous result was uncertain. Continuing will reuse the same reset request.";
    m_resetConfirmation = resetConnectionIdentity();
    emit changed(); return true;
}

void Controller::cancelChatGptResetConfirmation() {
    m_resetConfirmation.clear(); emit changed();
}

void Controller::observeResetUsage() {
    // Read-only usage polling never sends a redemption. Only a successful new
    // reading below the threshold, zero credits, or a later weekly window
    // releases a completed account-bound receipt.
    const auto weekly = chatGptWeekly();
    if (weekly.isEmpty()) return;
    const bool lowOrEmpty = weekly.value("utilization").toDouble() < 95
        || (weekly.value("available_count").isValid() && weekly.value("available_count").toDouble() <= 0);
    const QString path = resetReceiptPath();
    QJsonObject receipt;
    const QString currentWindow = canonicalWindow(weekly);
    if (!path.isEmpty() && readReceipt(path, receipt)) {
        const QString receiptWindow = receipt.value("weekly_resets_at").toString();
        const auto receiptReset = QDateTime::fromString(receiptWindow, Qt::ISODateWithMs);
        const auto currentReset = QDateTime::fromString(currentWindow, Qt::ISODateWithMs);
        const bool newWindow = receiptReset.isValid() && currentReset.isValid()
            && currentReset.toUTC() > receiptReset.toUTC();
        if (receipt.value("completed").toBool()
            && receipt.value("account_fingerprint").toString() == weekly.value("account_fingerprint").toString()
            && (lowOrEmpty || newWindow) && QFile::remove(path)) {
            if (m_resetBlockedReceipt == path) m_resetBlockedReceipt.clear();
        }
    }
    if (lowOrEmpty) {
        for (const QString &legacyPath : legacyResetReceiptPaths()) {
            QJsonObject legacy;
            if (readReceipt(legacyPath, legacy) && legacy.value("completed").toBool()
                && QFile::remove(legacyPath) && m_resetBlockedReceipt == legacyPath) {
                m_resetBlockedReceipt.clear();
            }
        }
    }
}

void Controller::cancelResetRequest() {
    m_resetConfirmation.clear();
    if (!m_resetReply) return;
    disconnect(m_resetReply, nullptr, this, nullptr);
    m_resetReply->abort(); m_resetReply->deleteLater(); m_resetReply.clear();
    m_resetBusy = false;
    m_resetMessage = "The reset result is unknown. Check your usage; any retry will reuse the same request.";
    // Leave the durable receipt intact, including when the app quits mid-request.
}

void Controller::consumeChatGptReset() {
    // DO NOT TEST OR INVOKE for validation: this spends a valuable real reset.
    // Only the explicit confirmation button may call this method. Never retry
    // automatically, follow redirects, or replace an uncertain request's UUID.
    if (!resetAction().value("canConfirm").toBool()) return;
    const QString receiptPath = resetReceiptPath();
    const auto weekly = chatGptWeekly();
    const QString fingerprint = weekly.value("account_fingerprint").toString();
    QJsonObject receipt;
    if (!readReceipt(receiptPath, receipt) || receipt.value("completed").toBool()) {
        m_resetMessage = "Check your previous reset in ChatGPT before continuing.";
        m_resetConfirmation.clear(); emit changed(); return;
    }
    if (!receipt.isEmpty() && receipt.value("account_fingerprint").toString() != fingerprint) {
        m_resetMessage = "The saved reset request does not match this ChatGPT account.";
        m_resetConfirmation.clear(); emit changed(); return;
    }
    if (!legacyResetReceiptPaths().isEmpty()) {
        m_resetMessage = "A previous reset request is not bound to a ChatGPT account and cannot be retried safely. Check it in ChatGPT.";
        m_resetConfirmation.clear(); emit changed(); return;
    }
    const QString requestWindow = canonicalWindow(weekly);
    if (receipt.isEmpty()) {
        receipt = {{"request_id", QUuid::createUuid().toString(QUuid::WithoutBraces)},
            {"completed", false}, {"account_fingerprint", fingerprint},
            {"weekly_resets_at", requestWindow.isEmpty() ? QJsonValue(QJsonValue::Null) : QJsonValue(requestWindow)}};
    }
    if (!writeReceipt(receiptPath, receipt)) {
        m_resetMessage = "The reset request could not be saved safely. No reset was requested.";
        m_resetConfirmation.clear(); emit changed(); return;
    }
    auto url = Usage::endpoint(backendUrl());
    QString path = url.path();
    path.chop(QStringLiteral("usage").size());
    url.setPath(path + QStringLiteral("providers/codex/reset"));
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    request.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::ManualRedirectPolicy);
    request.setTransferTimeout(95000);
    const ServerConnection transport = m_mode == "local" ? m_server.connection()
        : ServerConnection{QUrl(backendUrl()), backendToken().toUtf8(), QSslCertificate()};
    if (!transport.token.isEmpty()) request.setRawHeader("Authorization", "Bearer " + transport.token);
    if (m_mode == "local") ServerTransport::secureRequest(request, transport.certificate);
    const auto body = QJsonDocument(QJsonObject{{"request_id", receipt.value("request_id")},
        {"confirmed", true}, {"account_fingerprint", fingerprint}}).toJson(QJsonDocument::Compact);
    cancel(); m_poll.stop(); m_resetBusy = true; m_resetConfirmation.clear(); m_resetMessage = "Using one banked reset…";
    emit changed();
    QNetworkAccessManager *network = m_mode == "local" ? &m_localNetwork
        : m_mode == "ssh" ? static_cast<QNetworkAccessManager *>(&m_sshNetwork) : &m_network;
    auto reply = network->post(request, body); m_resetReply = reply;
    if (m_mode == "local") ServerTransport::requirePinnedPeer(reply, transport.certificate);
    auto deadline = new QTimer(reply); deadline->setSingleShot(true);
    connect(deadline, &QTimer::timeout, reply, &QNetworkReply::abort); deadline->start(100000);
    connect(reply, &QNetworkReply::readyRead, this, [reply] { if (reply->bytesAvailable() > 4096) reply->abort(); });
    connect(reply, &QNetworkReply::finished, this, [this, reply, receiptPath, receipt, requestWindow]() mutable {
        const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        const bool redirected = reply->attribute(QNetworkRequest::RedirectionTargetAttribute).isValid();
        const auto error = reply->error(); const auto bytes = reply->readAll();
        reply->deleteLater(); m_resetReply.clear(); m_resetBusy = false;
        const auto outcome = bytes.size() <= 4096 ? QJsonDocument::fromJson(bytes).object().value("outcome").toString() : QString();
        const bool terminal = outcome == "reset" || outcome == "already_redeemed" || outcome == "nothing_to_reset" || outcome == "no_credit";
        if (!redirected && error == QNetworkReply::NoError && status == 200 && terminal) {
            // A reset outcome may represent a new spend after a delayed retry;
            // bind it to the window in which this request was sent. An
            // already_redeemed outcome retains the original receipt window.
            if (outcome == "reset") receipt.insert("weekly_resets_at",
                requestWindow.isEmpty() ? QJsonValue(QJsonValue::Null) : QJsonValue(requestWindow));
            receipt.insert("completed", true);
            writeReceipt(receiptPath, receipt); // A failed write retains the original UUID for a safe manual retry.
            m_resetBlockedReceipt = receiptPath;
            if (outcome == "reset") m_resetMessage = "One banked reset was used. Refreshing usage…";
            else if (outcome == "already_redeemed") m_resetMessage = "This reset request was already completed. Refreshing usage…";
            else if (outcome == "no_credit") m_resetMessage = "No banked resets are available. Refreshing usage…";
            else m_resetMessage = "There is no usage to reset. Refreshing usage…";
        } else if (status == 404) m_resetMessage = "Update your usage server to use resets here, or open ChatGPT's usage page.";
        else if (status == 409) m_resetMessage = "The server could not safely proceed. Refresh usage or check the previous reset in ChatGPT.";
        else if (status == 401 || status == 403) m_resetMessage = "The reset request was rejected. Check your connection and ChatGPT sign-in.";
        else m_resetMessage = "The reset result is unknown. Check your usage; any retry will reuse the same request.";
        emit changed();
        refresh(); // Only GET usage; never repeat the mutation automatically.
    });
}
