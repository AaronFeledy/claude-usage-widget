#include "updateservice.h"

#include <QCoreApplication>
#include <QCryptographicHash>
#include <QDesktopServices>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <QProcessEnvironment>
#include <QRegularExpression>
#include <QSaveFile>
#include <QStandardPaths>
#include <QSysInfo>
#include <QUrl>

namespace {
constexpr qsizetype maximumToolOutput = 256 * 1024;

QString cleanAbsolute(const QString &path) {
    const QString normalized = QDir::fromNativeSeparators(path);
    if (normalized.isEmpty() || !QDir::isAbsolutePath(normalized) || QDir::cleanPath(normalized) != normalized
        || normalized.contains('\n') || normalized.contains('\r') || normalized.contains(QChar::Null)) return {};
    return QDir::toNativeSeparators(normalized);
}

QByteArray digest(const QString &path) {
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly)) return {};
    QCryptographicHash hash(QCryptographicHash::Sha256);
    if (!hash.addData(&file)) return {};
    return hash.result();
}
}

UpdateService::UpdateService(bool allowPublicTraffic, UpdateServiceOptions options, QObject *parent)
    : QObject(parent), m_options(std::move(options)), m_allowed(allowPublicTraffic), m_sessionAllowed(allowPublicTraffic) {
    m_timeout.setSingleShot(true);
    if (m_options.installRoot.isEmpty()) m_options.installRoot = qEnvironmentVariable("HEADROOM_INSTALL_ROOT");
    if (m_options.launcherPath.isEmpty()) m_options.launcherPath = qEnvironmentVariable("HEADROOM_LAUNCHER_PATH");
    if (m_options.packageVersion.isEmpty()) m_options.packageVersion = qEnvironmentVariable("HEADROOM_PACKAGE_VERSION");
#ifdef HEADROOM_SYSTEM_MANAGED
    m_options.systemManaged = true;
#endif
    if (m_options.managerPath.isEmpty()) {
#ifdef Q_OS_WIN
        m_options.managerPath = QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("headroom-package.exe"));
#elif defined(Q_OS_MACOS)
        m_options.managerPath = QDir(QCoreApplication::applicationDirPath()).absoluteFilePath(QStringLiteral("../../../bin/headroom-package"));
#else
        m_options.managerPath = QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("headroom-package"));
#endif
    }
    m_method = m_options.systemManaged ? QStringLiteral("system") : QStringLiteral("source");
    if (!m_allowed) {
        m_status = QStringLiteral("Update checks are disabled for this session.");
        return;
    }
    if (m_options.systemManaged) {
        m_status = QStringLiteral("This installation is managed by your system package manager.");
        return;
    }
    if (m_options.installRoot.isEmpty() || m_options.launcherPath.isEmpty() || m_options.packageVersion.isEmpty()) {
        m_status = QStringLiteral("This source installation is updated from its source checkout.");
        return;
    }
    inspectInstallation();
}

UpdateService::~UpdateService() {
    if (m_process) {
        m_process->disconnect(this);
        m_process->closeWriteChannel();
        if (!m_process->waitForFinished(m_options.cancelGraceMs)) m_process->kill();
    }
}

void UpdateService::inspectInstallation() { run(Operation::Inspect, QStringLiteral("inspect")); }

void UpdateService::startAutomaticCheck() {
    if (m_autoStarted) return;
    m_autoStarted = true;
    if (!m_allowed || m_suppressAutomaticCheck) return;
    m_autoPending = true;
    if (!m_process && m_official) {
        m_autoPending = false;
        m_autoStage = true;
        run(Operation::Check, QStringLiteral("check-update"));
    }
}

void UpdateService::setPublicTrafficAllowed(bool allowed) {
    const bool effective = m_sessionAllowed && allowed;
    if (m_operation == Operation::Apply) {
        m_hasDeferredAllowed = true;
        m_deferredAllowed = effective;
        return;
    }
    if (effective == m_allowed) return;
    m_allowed = effective;
    m_autoPending = false; m_autoStage = false;
    if (!effective) {
        if (m_official) {
            if (m_operation == Operation::Stage) {
                m_prePauseState = QStringLiteral("available");
                m_prePauseStatus = QStringLiteral("Headroom %1 is available.").arg(m_latestVersion);
            } else if (m_operation == Operation::Repair) {
                m_prePauseState = QStringLiteral("current");
                m_prePauseStatus = QStringLiteral("The Headroom installation needs repair.");
            } else if (m_operation == Operation::Check) {
                m_prePauseState = QStringLiteral("current");
                m_prePauseStatus = m_repairable ? QStringLiteral("The Headroom installation needs repair.")
                                                : QStringLiteral("Headroom updates automatically after a startup check.");
            } else {
                m_prePauseState = m_state;
                m_prePauseStatus = m_status;
            }
        } else {
            m_prePauseState.clear(); m_prePauseStatus.clear();
        }
        if (m_process) cancel();
        m_state = QStringLiteral("unavailable"); m_status = QStringLiteral("Update checks are paused for this session.");
        emit changed(); return;
    }
    if (m_process) { m_resumeAfterCancel = true; return; }
    restoreAllowedState();
}

void UpdateService::checkForUpdates() {
    if (!canCheck()) return;
    m_suppressAutomaticCheck = false;
    m_autoStage = false;
    run(Operation::Check, QStringLiteral("check-update"));
}

void UpdateService::stageUpdate() {
    if (!canStage()) return;
    run(Operation::Stage, QStringLiteral("stage-update"));
}

void UpdateService::repairInstallation() {
    if (!canRepair()) return;
    run(Operation::Repair, QStringLiteral("stage-repair"));
}

void UpdateService::restartToApply() {
    if (!restartAvailable() || m_verifiedStage.isEmpty() || m_process) return;
    const QString packageRoot = cleanAbsolute(m_verifiedStage.value(QStringLiteral("package_root")).toString());
    if (packageRoot.isEmpty()) { fail(QStringLiteral("The verified update stage was not recognized.")); return; }
    const QString stageRecord = QDir(packageRoot).absoluteFilePath(QStringLiteral("../../verified-stage.json"));
    QStringList arguments{QStringLiteral("prepare-apply"), QStringLiteral("--install-root"), m_options.installRoot,
        QStringLiteral("--entry-path"), m_options.launcherPath, QStringLiteral("--stage-record"), QDir::cleanPath(stageRecord),
        QStringLiteral("--current-pid"), QString::number(QCoreApplication::applicationPid()),
        QStringLiteral("--current-executable"), QCoreApplication::applicationFilePath()};
    if (m_ownedProcessProvider) {
        const auto owned = m_ownedProcessProvider();
        if (owned.first > 0 && !owned.second.isEmpty()) arguments << QStringLiteral("--owned-child-pid") << QString::number(owned.first)
            << QStringLiteral("--owned-child-executable") << owned.second;
    }
    for (const auto &argument : m_relaunchArguments)
        arguments << QStringLiteral("--relaunch-arg") << argument;
    run(Operation::Apply, QStringLiteral("prepare-apply"), arguments);
}

void UpdateService::cancel() {
    if (!m_process || m_operation == Operation::Apply) return;
    m_cancelRequested = true;
    auto process = m_process;
    process->closeWriteChannel();
    QTimer::singleShot(m_options.cancelGraceMs, process, [process] {
        if (process->state() != QProcess::NotRunning) process->kill();
    });
}

void UpdateService::run(Operation operation, const QString &command, const QStringList &explicitArguments) {
    if (m_process) return;
    if (operation != Operation::Inspect && !m_allowed) return;
    auto process = new QProcess(this);
    m_process = process;
    m_operation = operation;
    m_output.clear(); m_errorOutput.clear(); m_cancelRequested = false; m_timedOut = false;
    QStringList arguments;
    if (operation == Operation::Apply) {
        arguments = explicitArguments;
    } else {
        arguments = {command, QStringLiteral("--install-root"), m_options.installRoot};
        if (operation != Operation::Inspect) arguments.append(QStringLiteral("--cancel-stdin"));
    }
    process->setProgram(m_options.managerPath);
    process->setArguments(arguments);
    process->setProcessChannelMode(QProcess::SeparateChannels);
    auto environment = QProcessEnvironment::systemEnvironment();
    if (operation != Operation::Apply)
        for (const auto &name : {QStringLiteral("USAGE_AUTH_TOKEN"), QStringLiteral("USAGE_CONFIG"),
                                 QStringLiteral("HEADROOM_CREDENTIAL_SNAPSHOT_ROOT")}) environment.remove(name);
    process->setProcessEnvironment(environment);
#ifdef Q_OS_WIN
    if (operation == Operation::Apply)
        process->setCreateProcessArgumentsModifier([](QProcess::CreateProcessArguments *arguments) {
            arguments->flags |= 0x08000000; // CREATE_NO_WINDOW for the console-subsystem private manager.
        });
#endif
    if (operation == Operation::Check || operation == Operation::Inspect) {
        m_state = QStringLiteral("checking");
        m_status = operation == Operation::Inspect ? QStringLiteral("Checking installation…") : QStringLiteral("Checking for Headroom updates…");
    } else if (operation == Operation::Apply) {
        m_state = QStringLiteral("applying");
        m_status = QStringLiteral("Preparing a safe restart…");
    } else {
        m_state = QStringLiteral("downloading");
        m_status = operation == Operation::Repair ? QStringLiteral("Downloading a matching repair package…")
                                                   : QStringLiteral("Downloading and verifying the Headroom update…");
    }
    emit changed();
    connect(process, &QProcess::readyReadStandardOutput, this, [this, process] {
        m_output += process->readAllStandardOutput();
        if (m_output.size() > maximumToolOutput) process->kill();
    });
    connect(process, &QProcess::readyReadStandardError, this, [this, process] {
        m_errorOutput += process->readAllStandardError();
        if (m_errorOutput.size() > maximumToolOutput) process->kill();
    });
    connect(process, &QProcess::finished, this, [this, process, operation](int code, QProcess::ExitStatus status) {
        m_timeout.stop();
        m_output += process->readAllStandardOutput();
        m_errorOutput += process->readAllStandardError();
        m_process = nullptr;
        m_operation = Operation::None;
        process->deleteLater();
        finish(operation, code, status);
    });
    connect(process, &QProcess::errorOccurred, this, [this, process](QProcess::ProcessError error) {
        if (error == QProcess::FailedToStart && process == m_process) {
            m_timeout.stop(); m_process = nullptr; process->deleteLater();
            m_operation = Operation::None;
            if (!m_allowed) {
                m_resumeAfterCancel = false;
                m_state = QStringLiteral("unavailable");
                m_status = QStringLiteral("Update checks are paused for this session.");
                emit changed(); return;
            }
            if (m_resumeAfterCancel) { m_resumeAfterCancel = false; restoreAllowedState(); return; }
            fail(QStringLiteral("The Headroom package service could not be started."));
        }
    });
    connect(&m_timeout, &QTimer::timeout, process, [this, process] {
        if (process != m_process) return;
        m_timedOut = true;
        process->closeWriteChannel();
        QTimer::singleShot(m_options.cancelGraceMs, process, [process] { if (process->state() != QProcess::NotRunning) process->kill(); });
    }, Qt::SingleShotConnection);
    process->start();
    m_timeout.start(m_options.timeoutMs);
}

void UpdateService::finish(Operation operation, int exitCode, QProcess::ExitStatus exitStatus) {
    if (!m_allowed) {
        m_resumeAfterCancel = false;
        m_autoStage = false;
        m_state = QStringLiteral("unavailable"); m_status = QStringLiteral("Update checks are paused for this session.");
        emit changed(); return;
    }
    if (m_cancelRequested || m_timedOut) {
        if (m_resumeAfterCancel) { m_resumeAfterCancel = false; restoreAllowedState(); return; }
        m_autoStage = false; m_verifiedStage = {};
        m_state = QStringLiteral("failed");
        m_status = m_timedOut ? QStringLiteral("The update operation timed out. Try again later.")
                              : QStringLiteral("The update operation was cancelled.");
		emit changed(); applyDeferredTrafficState(); return;
    }
    if (m_output.size() > maximumToolOutput || m_errorOutput.size() > maximumToolOutput || exitStatus != QProcess::NormalExit || exitCode != 0) {
        fail(QStringLiteral("The update operation did not complete. Try again later.")); return;
    }
    QJsonParseError error;
    const auto document = QJsonDocument::fromJson(m_output, &error);
    if (error.error != QJsonParseError::NoError || !document.isObject()) {
        fail(QStringLiteral("The package service returned an invalid result.")); return;
    }
    const auto object = document.object();
    QString expectedCommand;
    switch (operation) {
    case Operation::Inspect: expectedCommand = QStringLiteral("inspect"); break;
    case Operation::Check: expectedCommand = QStringLiteral("check-update"); break;
    case Operation::Stage: expectedCommand = QStringLiteral("stage-update"); break;
    case Operation::Repair: expectedCommand = QStringLiteral("stage-repair"); break;
    case Operation::Apply: expectedCommand = QStringLiteral("prepare-apply"); break;
    case Operation::None: break;
    }
    if (!object.value(QStringLiteral("ok")).toBool() || object.value(QStringLiteral("command")).toString() != expectedCommand
        || !object.value(QStringLiteral("result")).isObject()) {
        fail(QStringLiteral("The update package was rejected. The current installation was not changed.")); return;
    }
    const auto result = object.value(QStringLiteral("result")).toObject();
    if (operation == Operation::Apply) {
        if (!authorizePreparedApply(result)) {
            fail(QStringLiteral("The update transaction was not accepted.")); return;
        }
        m_status = QStringLiteral("Restarting into the verified Headroom package…"); emit changed(); emit applyPrepared(); return;
    }
    if (operation == Operation::Inspect) handleInspection(result); else handleUpdateResult(operation, result);
}

bool UpdateService::authorizePreparedApply(const QJsonObject &result) const {
    if (result.value(QStringLiteral("schema")).toInt() != 1 || result.value(QStringLiteral("product")).toString() != QStringLiteral("Headroom")) return false;
    const QString root = cleanAbsolute(m_options.installRoot);
    const QString directory = cleanAbsolute(result.value(QStringLiteral("transaction_directory")).toString());
    const QString request = cleanAbsolute(result.value(QStringLiteral("request_path")).toString());
    const QString manager = cleanAbsolute(result.value(QStringLiteral("manager_path")).toString());
    const QString acknowledgement = cleanAbsolute(result.value(QStringLiteral("acknowledgement_path")).toString());
    const QString commit = cleanAbsolute(result.value(QStringLiteral("commit_path")).toString());
    const QString nonce = result.value(QStringLiteral("nonce")).toString();
    const auto equalsPath = [](const QString &left, const QString &right) {
        const QString cleanLeft = cleanAbsolute(left), cleanRight = cleanAbsolute(right);
        return !cleanLeft.isEmpty() && cleanLeft == cleanRight;
    };
    if (root.isEmpty() || directory.isEmpty() || request.isEmpty() || manager.isEmpty() || acknowledgement.isEmpty() || commit.isEmpty()
        || !QRegularExpression(QStringLiteral("^apply-[0-9a-f]{32}$")).match(QFileInfo(directory).fileName()).hasMatch()
        || QFileInfo(directory).dir().canonicalPath() != QFileInfo(QDir(root).filePath(QStringLiteral("transactions"))).canonicalFilePath()
        || !equalsPath(request, QDir(directory).filePath(QStringLiteral("apply-request.json")))
        || !equalsPath(acknowledgement, QDir(directory).filePath(QStringLiteral("accepted.json")))
        || !equalsPath(commit, QDir(directory).filePath(QStringLiteral("commit.json")))
        || !equalsPath(manager, QDir(directory).filePath(
#ifdef Q_OS_WIN
               QStringLiteral("headroom-apply.exe")))
#else
               QStringLiteral("headroom-apply")))
#endif
        || !QRegularExpression(QStringLiteral("^[0-9a-f]{48}$")).match(nonce).hasMatch()
        || !QFileInfo(request).isFile() || QFileInfo(request).isSymLink() || !QFileInfo(manager).isFile() || QFileInfo(manager).isSymLink()) return false;
    QFile requestFile(request);
    QJsonParseError requestError;
    const auto requestDocument = requestFile.open(QIODevice::ReadOnly) && requestFile.size() <= maximumToolOutput
        ? QJsonDocument::fromJson(requestFile.readAll(), &requestError) : QJsonDocument();
    if (requestError.error != QJsonParseError::NoError || !requestDocument.isObject()) return false;
    const auto requestObject = requestDocument.object();
    const QString packageRoot = cleanAbsolute(m_verifiedStage.value(QStringLiteral("package_root")).toString());
    const QString expectedStage = packageRoot.isEmpty() ? QString()
        : cleanAbsolute(QDir::cleanPath(QDir(packageRoot).absoluteFilePath(QStringLiteral("../../verified-stage.json"))));
    if (requestObject.value(QStringLiteral("schema")).toInt() != 1 || requestObject.value(QStringLiteral("product")).toString() != QStringLiteral("Headroom")
        || !equalsPath(requestObject.value(QStringLiteral("install_root")).toString(), root)
        || !equalsPath(requestObject.value(QStringLiteral("entry_path")).toString(), m_options.launcherPath)
        || expectedStage.isEmpty() || !equalsPath(requestObject.value(QStringLiteral("stage_record")).toString(), expectedStage)
        || requestObject.value(QStringLiteral("current_pid")).toInteger() != QCoreApplication::applicationPid()
        || !equalsPath(requestObject.value(QStringLiteral("current_executable")).toString(), QCoreApplication::applicationFilePath())
        || !equalsPath(requestObject.value(QStringLiteral("acknowledgement_path")).toString(), acknowledgement)
        || !equalsPath(requestObject.value(QStringLiteral("commit_path")).toString(), commit)
        || requestObject.value(QStringLiteral("nonce")).toString() != nonce) return false;
    QFile accepted(acknowledgement);
    QJsonParseError parseError;
    const auto acceptedDocument = accepted.open(QIODevice::ReadOnly) && accepted.size() <= 4096
        ? QJsonDocument::fromJson(accepted.readAll(), &parseError) : QJsonDocument();
    if (parseError.error != QJsonParseError::NoError || !acceptedDocument.isObject()) return false;
    const auto acceptedObject = acceptedDocument.object();
    if (acceptedObject.value(QStringLiteral("schema")).toInt() != 1 || acceptedObject.value(QStringLiteral("product")).toString() != QStringLiteral("Headroom")
        || !acceptedObject.value(QStringLiteral("accepted")).toBool() || acceptedObject.value(QStringLiteral("nonce")).toString() != nonce) return false;
    QSaveFile authorization(commit);
    const QJsonObject value{{QStringLiteral("schema"), 1}, {QStringLiteral("product"), QStringLiteral("Headroom")},
        {QStringLiteral("commit"), true}, {QStringLiteral("nonce"), nonce}};
    if (!authorization.open(QIODevice::WriteOnly) || authorization.write(QJsonDocument(value).toJson(QJsonDocument::Compact) + '\n') < 0 || !authorization.commit()) return false;
#ifndef Q_OS_WIN
    QFile::setPermissions(commit, QFileDevice::ReadOwner | QFileDevice::WriteOwner);
#endif
    return true;
}

bool UpdateService::validateInstalledApplicationIdentity(const QJsonObject &result) const {
    if (m_options.fixtureIdentity) return result.value(QStringLiteral("trusted_identity")).toBool();
    const QString root = cleanAbsolute(m_options.installRoot);
    const QString version = result.value(QStringLiteral("version")).toString();
    const QString versionPath = result.value(QStringLiteral("version_path")).toString();
#ifdef Q_OS_WIN
    const QString nativePlatform = QStringLiteral("windows");
#elif defined(Q_OS_MACOS)
    const QString nativePlatform = QStringLiteral("macos");
#else
    const QString nativePlatform = QStringLiteral("linux");
#endif
    QString nativeArchitecture = QSysInfo::currentCpuArchitecture();
    if (nativeArchitecture == QStringLiteral("amd64")) nativeArchitecture = QStringLiteral("x86_64");
    if (nativeArchitecture == QStringLiteral("aarch64")) nativeArchitecture = QStringLiteral("arm64");
    if (root.isEmpty() || m_options.launcherPath.isEmpty() || version != m_options.packageVersion || versionPath.isEmpty()
        || QDir::isAbsolutePath(versionPath) || versionPath.contains(QLatin1Char('\\')) || QDir::cleanPath(versionPath) != versionPath
        || !versionPath.startsWith(QStringLiteral("versions/"))
        || version != QCoreApplication::applicationVersion() || result.value(QStringLiteral("platform")).toString() != nativePlatform
        || result.value(QStringLiteral("architecture")).toString() != nativeArchitecture
        || !result.value(QStringLiteral("trusted_identity")).toBool()) return false;
#ifdef Q_OS_WIN
    const QString relativeApplication = QStringLiteral("bin/headroom.exe");
#elif defined(Q_OS_MACOS)
    const QString relativeApplication = QStringLiteral("Headroom.app/Contents/MacOS/headroom");
#else
    const QString relativeApplication = QStringLiteral("bin/headroom");
#endif
    const QString expectedApp = QDir(root).filePath(versionPath + QLatin1Char('/') + relativeApplication);
    const QString runningApplication = m_options.applicationPath.isEmpty() ? QCoreApplication::applicationFilePath() : m_options.applicationPath;
    return QFileInfo(runningApplication).canonicalFilePath() == QFileInfo(expectedApp).canonicalFilePath();
}

bool UpdateService::validateIdentity(const QJsonObject &result) const {
    if (!validateInstalledApplicationIdentity(result)) return false;
    if (m_options.fixtureIdentity) return true;
    const QString root = cleanAbsolute(m_options.installRoot);
    const QString launcher = cleanAbsolute(m_options.launcherPath);
    QFile association(launcher + QStringLiteral(".root"));
    if (!association.open(QIODevice::ReadOnly) || association.size() > 4096
        || QString::fromUtf8(association.readAll()) != QDir::toNativeSeparators(root) + QLatin1Char('\n')) return false;
    const QString internalLauncher = cleanAbsolute(result.value(QStringLiteral("launcher_path")).toString());
    if (internalLauncher.isEmpty()) return false;
    const QFileInfo externalInfo(launcher), internalInfo(internalLauncher);
    if (!externalInfo.isFile() || externalInfo.isSymLink() || !internalInfo.isFile() || internalInfo.isSymLink()) return false;
    return digest(launcher) == digest(internalLauncher) && !digest(launcher).isEmpty();
}

void UpdateService::handleInspection(const QJsonObject &result) {
    if (!validateIdentity(result)) {
        m_state = QStringLiteral("unavailable");
        m_status = m_options.systemManaged
            ? QStringLiteral("This installation is managed by your system package manager.")
            : validateInstalledApplicationIdentity(result)
                ? QStringLiteral("The Headroom launcher is damaged. Rerun the official installer to repair this installation.")
                : QStringLiteral("This source installation is updated from its source checkout.");
        emit changed(); return;
    }
    m_official = true; m_method = QStringLiteral("automatic");
    m_platform = result.value(QStringLiteral("platform")).toString();
    m_architecture = result.value(QStringLiteral("architecture")).toString();
    const auto missing = result.value(QStringLiteral("missing")).toArray();
    for (const auto &item : missing) {
        const QString path = item.toString();
        if (path == QStringLiteral("bin/usage-server") || path == QStringLiteral("bin/usage-server.exe")
            || path == QStringLiteral("Headroom.app/Contents/MacOS/usage-server")
            || path == QStringLiteral("bin/headroom-credential-helper.exe") || path == QStringLiteral("bootstrap/headroom")
            || path == QStringLiteral("bootstrap/headroom.exe") || path == QStringLiteral("bootstrap/headroom-package")
            || path == QStringLiteral("bootstrap/headroom-package.exe") || path == QStringLiteral("bootstrap/association")) m_repairable = true;
    }
    if (result.value(QStringLiteral("apply_status")).toString() == QStringLiteral("rolled_back")) {
	    m_suppressAutomaticCheck = true;
        m_state = QStringLiteral("failed");
        m_status = QStringLiteral("The update could not start, so Headroom restored the previous installation. %1")
                       .arg(result.value(QStringLiteral("apply_message")).toString());
    } else if (result.value(QStringLiteral("apply_status")).toString() == QStringLiteral("recovery_required")) {
        m_suppressAutomaticCheck = true;
        m_state = QStringLiteral("failed");
        m_status = QStringLiteral("Headroom could not finish restoring the previous installation. Restart Headroom to retry recovery; if the problem remains, rerun the installer. %1")
                       .arg(result.value(QStringLiteral("apply_message")).toString());
    } else {
        m_state = QStringLiteral("current");
        m_status = result.value(QStringLiteral("apply_status")).toString() == QStringLiteral("applied")
            ? QStringLiteral("Headroom updated successfully.")
            : m_repairable ? QStringLiteral("The Headroom installation needs repair.")
                           : QStringLiteral("Headroom updates automatically after a startup check.");
    }
    emit changed();
    if (m_autoPending) {
        m_autoPending = false;
        if (!m_suppressAutomaticCheck) { m_autoStage = true; run(Operation::Check, QStringLiteral("check-update")); }
    }
}

void UpdateService::restoreAllowedState() {
    if (!m_allowed) return;
    if (m_official) {
        if (!m_verifiedStage.isEmpty()) {
            m_state = QStringLiteral("staged");
            m_status = QStringLiteral("A verified Headroom package is staged. Restart to apply it.");
        } else if (!m_prePauseState.isEmpty()) {
            m_state = m_prePauseState;
            m_status = m_prePauseStatus;
        } else {
            m_state = QStringLiteral("current");
            m_status = m_repairable ? QStringLiteral("The Headroom installation needs repair.")
                                    : QStringLiteral("Headroom updates automatically after a startup check.");
        }
        m_prePauseState.clear(); m_prePauseStatus.clear();
        emit changed(); return;
    }
    if (!m_options.installRoot.isEmpty() && !m_options.launcherPath.isEmpty() && !m_options.packageVersion.isEmpty()
        && !m_options.systemManaged) { inspectInstallation(); return; }
    m_state = QStringLiteral("unavailable");
    m_status = m_options.systemManaged ? QStringLiteral("This installation is managed by your system package manager.")
                                       : QStringLiteral("This source installation is updated from its source checkout.");
    emit changed();
}

void UpdateService::handleUpdateResult(Operation operation, const QJsonObject &result) {
    const QString status = result.value(QStringLiteral("status")).toString();
    const QString current = result.value(QStringLiteral("current_version")).toString();
    const QString version = result.value(QStringLiteral("version")).toString();
    if (!m_official || current != m_options.packageVersion
        || result.value(QStringLiteral("platform")).toString() != m_platform
        || result.value(QStringLiteral("architecture")).toString() != m_architecture ||
        (status != QStringLiteral("unavailable") && status != QStringLiteral("current")
         && status != QStringLiteral("available") && status != QStringLiteral("staged"))) {
        fail(QStringLiteral("The package service returned an inconsistent update result.")); return;
    }
    m_latestVersion = version;
    if (status == QStringLiteral("available") && operation == Operation::Check && m_autoStage) {
        m_autoStage = false;
        m_state = QStringLiteral("available"); emit changed();
        run(Operation::Stage, QStringLiteral("stage-update")); return;
    }
    m_autoStage = false;
    if (status == QStringLiteral("staged")) {
        const auto stage = result.value(QStringLiteral("stage")).toObject();
        const QString packageRoot = cleanAbsolute(stage.value(QStringLiteral("package_root")).toString());
        const QString stagingRoot = QFileInfo(QDir(m_options.installRoot).filePath(QStringLiteral("staging"))).canonicalFilePath();
        const QString canonicalPackage = QFileInfo(packageRoot).canonicalFilePath();
        const QString prefix = QDir::fromNativeSeparators(stagingRoot) + QLatin1Char('/');
        const QFileInfo packageInfo(packageRoot);
        const QString stageDirectory = QDir::cleanPath(packageInfo.dir().absoluteFilePath(QStringLiteral("..")));
        QFile record(QDir(stageDirectory).filePath(QStringLiteral("verified-stage.json")));
        QJsonDocument recorded;
        if (record.open(QIODevice::ReadOnly) && record.size() <= maximumToolOutput) recorded = QJsonDocument::fromJson(record.readAll());
        if (stage.value(QStringLiteral("schema")).toInt() != 1 || stage.value(QStringLiteral("product")).toString() != QStringLiteral("Headroom")
            || stage.value(QStringLiteral("version")).toString() != version
            || stage.value(QStringLiteral("platform")).toString() != m_platform
            || stage.value(QStringLiteral("architecture")).toString() != m_architecture
            || stage.value(QStringLiteral("package_asset")).toString() != result.value(QStringLiteral("asset_name")).toString()
            || packageRoot.isEmpty() || stagingRoot.isEmpty() || canonicalPackage.isEmpty()
            || !QDir::fromNativeSeparators(canonicalPackage).startsWith(prefix)
            || !packageInfo.isDir() || packageInfo.isSymLink() || !recorded.isObject() || recorded.object() != stage) {
            fail(QStringLiteral("The verified update stage was not recognized.")); return;
        }
        m_state = QStringLiteral("staged");
        m_verifiedStage = stage;
        m_status = operation == Operation::Repair ? QStringLiteral("A matching repair package is staged. Restart to apply it.")
                                                  : QStringLiteral("Headroom %1 is staged. Restart to apply it.").arg(version);
    } else if (status == QStringLiteral("available")) {
        m_state = QStringLiteral("available");
        m_status = QStringLiteral("Headroom %1 is available.").arg(version);
    } else if (status == QStringLiteral("current")) {
        m_state = QStringLiteral("current");
        m_status = QStringLiteral("This version of Headroom is current.");
    } else {
        m_state = QStringLiteral("unavailable");
        m_status = QStringLiteral("No compatible Headroom package is published yet.");
    }
    emit changed();
}

void UpdateService::fail(const QString &message) {
    m_autoStage = false;
    m_verifiedStage = {};
	m_state = QStringLiteral("failed"); m_status = message; emit changed(); applyDeferredTrafficState();
}

void UpdateService::applyDeferredTrafficState() {
    if (!m_hasDeferredAllowed) return;
    const bool desired = m_deferredAllowed;
    m_hasDeferredAllowed = false;
    setPublicTrafficAllowed(desired);
}

QString UpdateService::guidePath() const {
    const QString appDir = QCoreApplication::applicationDirPath();
    const QStringList candidates {
#ifdef Q_OS_MACOS
        QDir(appDir).filePath(QStringLiteral("../Resources/update-guide.html")),
        QDir(appDir).filePath(QStringLiteral("../../../share/headroom/update-guide.html")),
#endif
        QDir(appDir).filePath(QStringLiteral("../share/headroom/update-guide.html")),
        QStandardPaths::locate(QStandardPaths::GenericDataLocation, QStringLiteral("headroom/update-guide.html")),
        QDir(appDir).filePath(QStringLiteral("../update-guide.html"))};
    for (const auto &candidate : candidates) if (!candidate.isEmpty() && QFileInfo::exists(candidate)) return QFileInfo(candidate).absoluteFilePath();
    return {};
}

void UpdateService::openUpdateMethod() {
    const QString path = guidePath();
    if (!path.isEmpty() && QDesktopServices::openUrl(QUrl::fromLocalFile(path))) return;
    fail(QStringLiteral("Could not open the installed update instructions."));
}
