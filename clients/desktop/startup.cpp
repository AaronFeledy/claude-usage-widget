#include "startup.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QSaveFile>
#include <QSettings>
#include <QSet>
#include <QRegularExpression>
#include <algorithm>

namespace {
const QRegularExpression safeVersion(
    QStringLiteral("^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\\+[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?$"));

QString configDirectory()
{
    const QString configured = qEnvironmentVariable("XDG_CONFIG_HOME");
    return QDir::isAbsolutePath(configured) ? configured : QDir::homePath() + "/.config";
}

QString quotedExecutable(const QString &path)
{
    // Desktop Entry Exec has two escaping layers: the argument, then its string
    // value. It is not shell syntax.
    QString argument;
    for (QChar character : path) {
        if (character == '"' || character == '`' || character == '$' || character == '\\')
            argument += '\\';
        argument += character;
    }
    argument.replace("\\", "\\\\");
    return '"' + argument + '"';
}

QString windowsCommand(const QString &path)
{
    return QStringLiteral("\"") + QDir::toNativeSeparators(path) + "\" --background";
}

bool samePath(const QString &left, const QString &right)
{
#ifdef Q_OS_WIN
    return QString::compare(left, right, Qt::CaseInsensitive) == 0;
#else
    return left == right;
#endif
}

QString cleanAbsolute(const QString &path)
{
    if (path.isEmpty() || path.contains('\n') || path.contains('\r') || path.contains(QChar::Null)) return {};
    const QString native = QDir::fromNativeSeparators(path);
    if (!QDir::isAbsolutePath(native) || QDir::cleanPath(native) != native) return {};
    return QFileInfo(native).absoluteFilePath();
}

QStringList installedApplications(const QString &root, const QString &exactVersion = {})
{
#ifdef Q_OS_WIN
    const QString appName = QStringLiteral("headroom.exe");
#else
    const QString appName = QStringLiteral("headroom");
#endif
    QStringList result;
    const QFileInfo rootInfo(root);
    const QString canonicalRoot = rootInfo.canonicalFilePath();
    if (!rootInfo.isDir() || rootInfo.isSymLink() || canonicalRoot.isEmpty() || !samePath(canonicalRoot, root)) return result;
    const QFileInfo versionsInfo(QDir(root).filePath(QStringLiteral("versions")));
    if (!versionsInfo.isDir() || versionsInfo.isSymLink()
        || !samePath(versionsInfo.canonicalFilePath(), QDir(root).filePath(QStringLiteral("versions")))) return result;
    const QFileInfoList generations = QDir(versionsInfo.absoluteFilePath())
        .entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name);
    if (generations.size() > 2048) return result;
    static const QRegularExpression suffix(QStringLiteral("^(.*)\\.generation-[0-9a-f]{16}-[0-9a-f]{16}$"));
    for (const QFileInfo &generation : generations) {
        if (generation.isSymLink()) continue;
        QString version = generation.fileName();
        const auto match = suffix.match(version);
        if (match.hasMatch()) version = match.captured(1);
        if (!safeVersion.match(version).hasMatch() || (!exactVersion.isEmpty() && version != exactVersion)) continue;
        if (!samePath(generation.canonicalFilePath(), generation.absoluteFilePath())) continue;
        const QFileInfo bin(QDir(generation.absoluteFilePath()).filePath(QStringLiteral("bin")));
        if (!bin.isDir() || bin.isSymLink() || !samePath(bin.canonicalFilePath(), bin.absoluteFilePath())) continue;
        const QFileInfo application(QDir(bin.absoluteFilePath()).filePath(appName));
        if (application.isFile() && !application.isSymLink()
            && samePath(application.canonicalFilePath(), application.absoluteFilePath())) result.append(application.absoluteFilePath());
    }
    return result;
}
}

QString StartupService::defaultExecutablePath()
{
    return packagedExecutablePath(QCoreApplication::applicationFilePath());
}

QString StartupService::packagedExecutablePath(const QString &applicationPath)
{
    const QString application = cleanAbsolute(applicationPath);
    const QString root = cleanAbsolute(qEnvironmentVariable("HEADROOM_INSTALL_ROOT"));
    const QString launcher = cleanAbsolute(qEnvironmentVariable("HEADROOM_LAUNCHER_PATH"));
    const QString version = qEnvironmentVariable("HEADROOM_PACKAGE_VERSION");
    if (!application.isEmpty() && !root.isEmpty() && !launcher.isEmpty() && safeVersion.match(version).hasMatch()) {
        const QFileInfo rootInfo(root), launcherInfo(launcher), applicationInfo(application);
        const QFileInfo associationInfo(launcher + QStringLiteral(".root"));
        const QByteArray expectedAssociation = QDir::toNativeSeparators(root).toUtf8() + '\n';
        bool trustedAssociation = false;
        if (associationInfo.isFile() && !associationInfo.isSymLink()) {
            QFile association(associationInfo.absoluteFilePath());
            trustedAssociation = association.open(QIODevice::ReadOnly) && association.size() <= 4096
                && association.readAll() == expectedAssociation;
        }
        const QStringList applications = installedApplications(root, version);
        const bool matchesApplication = std::any_of(applications.cbegin(), applications.cend(), [&](const QString &candidate) {
            return samePath(application, candidate);
        });
        if (rootInfo.isDir() && !rootInfo.isSymLink()
            && launcherInfo.isFile() && !launcherInfo.isSymLink() && applicationInfo.isFile() && !applicationInfo.isSymLink()
            && associationInfo.isFile() && !associationInfo.isSymLink() && trustedAssociation && matchesApplication)
            return launcher;
    }
    return application.isEmpty() ? applicationPath : application;
}

StartupService::StartupService(QString configHome, QString executable, bool allowChanges, QObject *parent,
                               Platform platform, QString registryPath)
    : QObject(parent),
      m_entryPath(QDir(configHome.isEmpty() ? configDirectory() : configHome)
                      .filePath("autostart/headroom.desktop")),
      m_executable(packagedExecutablePath(executable.isEmpty() ? QCoreApplication::applicationFilePath() : executable)),
      m_registryPath(registryPath.isEmpty()
          ? QStringLiteral("HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run")
          : std::move(registryPath)),
      m_allowChanges(allowChanges),
      m_windows(platform == Platform::Windows
#ifdef Q_OS_WIN
          || platform == Platform::Current
#endif
      )
{
    const QString application = cleanAbsolute(executable.isEmpty() ? QCoreApplication::applicationFilePath() : executable);
    if (m_allowChanges && !application.isEmpty() && !samePath(application, m_executable))
        repairPackagedRegistration();
    refresh();
}

void StartupService::repairPackagedRegistration()
{
    const QString root = cleanAbsolute(qEnvironmentVariable("HEADROOM_INSTALL_ROOT"));
    if (root.isEmpty()) return;
    const QStringList candidates = installedApplications(root);
    if (m_windows) {
        QSettings registry(m_registryPath, QSettings::NativeFormat);
        const QString command = registry.value(QStringLiteral("Headroom")).toString();
        if (std::none_of(candidates.cbegin(), candidates.cend(), [&](const QString &candidate) {
                return command == windowsCommand(candidate);
            })) return;
        registry.setValue(QStringLiteral("Headroom"), windowsCommand(m_executable));
        registry.sync();
        if (registry.status() != QSettings::NoError
            || registry.value(QStringLiteral("Headroom")).toString() != windowsCommand(m_executable))
            m_error = tr("Could not repair the Headroom startup entry.");
        return;
    }
    const QFileInfo entryInfo(m_entryPath);
    if (!entryInfo.isFile() || entryInfo.isSymLink() || entryInfo.size() > 64 * 1024) return;
    QFile entry(m_entryPath);
    if (!entry.open(QIODevice::ReadOnly)) return;
    const QByteArray original = entry.readAll();
    QList<QByteArray> lines = original.split('\n');
    bool inGroup = false, application = false, hidden = false, disabled = false;
    int matchingExec = -1, execCount = 0, desktopGroups = 0;
    QSet<QByteArray> seen;
    for (int i = 0; i < lines.size(); ++i) {
        const QByteArray line = lines.at(i).trimmed();
        if (line.startsWith('[')) {
            inGroup = line == QByteArrayLiteral("[Desktop Entry]");
            if (inGroup && ++desktopGroups != 1) return;
            continue;
        }
        if (!inGroup || line.startsWith('#')) continue;
        const int separator = line.indexOf('=');
        if (separator < 0) continue;
        const QByteArray key = line.left(separator).trimmed(), value = line.mid(separator + 1).trimmed();
        if (key == "Type" || key == "Hidden" || key == "X-GNOME-Autostart-enabled" || key == "Exec") {
            if (seen.contains(key)) return;
            seen.insert(key);
        }
        if (key == "Type") application = value == "Application";
        if (key == "Hidden") hidden = value == "true";
        if (key == "X-GNOME-Autostart-enabled") disabled = value == "false";
        if (key == "Exec") {
            ++execCount;
            if (std::any_of(candidates.cbegin(), candidates.cend(), [&](const QString &candidate) {
                    return value == quotedExecutable(candidate).toUtf8() + QByteArrayLiteral(" --background");
                })) matchingExec = i;
        }
    }
    if (!application || hidden || disabled || execCount != 1 || matchingExec < 0) return;
    lines[matchingExec] = QByteArrayLiteral("Exec=") + quotedExecutable(m_executable).toUtf8() + QByteArrayLiteral(" --background");
    const QByteArray contents = lines.join('\n');
    QSaveFile replacement(m_entryPath);
    if (!replacement.open(QIODevice::WriteOnly) || !replacement.setPermissions(entryInfo.permissions())
        || replacement.write(contents) != contents.size()
        || !replacement.commit())
        m_error = tr("Could not repair the Headroom startup entry.");
}

void StartupService::refresh()
{
    if (m_windows) {
        QSettings registry(m_registryPath, QSettings::NativeFormat);
        const bool active = registry.value("Headroom").toString() == windowsCommand(m_executable);
        if (m_enabled != active) { m_enabled = active; emit enabledChanged(); }
        return;
    }
    QFile entry(m_entryPath);
    bool inGroup = false, application = false, command = false, hidden = false, disabled = false;
    if (entry.open(QIODevice::ReadOnly)) {
        while (!entry.atEnd()) {
            const QByteArray line = entry.readLine().trimmed();
            if (line.startsWith('[')) {
                inGroup = line == "[Desktop Entry]";
                continue;
            }
            if (!inGroup || line.startsWith('#')) continue;
            const int separator = line.indexOf('=');
            if (separator < 0) continue;
            const QByteArray key = line.left(separator).trimmed();
            const QByteArray value = line.mid(separator + 1).trimmed();
            if (key == "Type") application = value == "Application";
            if (key == "Exec") command = !value.isEmpty();
            if (key == "Hidden") hidden = value == "true";
            if (key == "X-GNOME-Autostart-enabled") disabled = value == "false";
        }
    }
    const bool active = application && command && !hidden && !disabled;
    if (m_enabled != active) {
        m_enabled = active;
        emit enabledChanged();
    }
}

void StartupService::setAllowChanges(bool allowed)
{
    if (m_allowChanges == allowed) return;
    m_allowChanges = allowed;
    clearError();
    emit availableChanged();
}

bool StartupService::fail(const QString &message)
{
    if (m_error != message) {
        m_error = message;
        emit errorChanged();
    }
    refresh();
    return false;
}

bool StartupService::persistPreference(bool enabled)
{
    if (!m_preferenceWriter) return true;
    const QString error = m_preferenceWriter(enabled);
    if (error.isEmpty()) return true;
    if (m_error != error) { m_error = error; emit errorChanged(); }
    return false;
}

void StartupService::clearError()
{
    if (!m_error.isEmpty()) {
        m_error.clear();
        emit errorChanged();
    }
}

bool StartupService::setEnabled(bool enabled)
{
    if (!m_platformSupported)
        return fail(tr("Start at login will be available after Windows integration is installed."));
    if (!m_allowChanges)
        return fail(tr("Start at login is unavailable for this session."));
    if (m_windows) {
        if (!QDir::isAbsolutePath(m_executable) || m_executable.contains('"') ||
            m_executable.contains('\n') || m_executable.contains('\r') || m_executable.contains(QChar::Null))
            return fail(tr("The application path cannot be used in a Windows startup entry."));
        if (!QFileInfo(m_executable).isFile())
            return fail(tr("The Headroom executable is missing."));
        QSettings registry(m_registryPath, QSettings::NativeFormat);
        const bool existed = registry.contains("Headroom");
        const QVariant previous = registry.value("Headroom");
        if (enabled) registry.setValue("Headroom", windowsCommand(m_executable));
        else registry.remove("Headroom");
        registry.sync();
        if (registry.status() != QSettings::NoError ||
            (enabled ? registry.value("Headroom").toString() != windowsCommand(m_executable)
                     : registry.contains("Headroom")))
            return fail(tr("Could not update the Headroom startup entry."));
        if (!persistPreference(enabled)) {
            if (existed) registry.setValue("Headroom", previous); else registry.remove("Headroom");
            registry.sync(); refresh(); return false;
        }
        clearError(); refresh(); emit preferenceChanged(enabled); return true;
    }
    if (!enabled) {
        // Only remove our entry. Never remove or rewrite another autostart item.
        const QByteArray previous = [&] { QFile file(m_entryPath); return file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray{}; }();
        const bool existed = QFileInfo::exists(m_entryPath);
        if (existed && !QFile::remove(m_entryPath))
            return fail(tr("Could not remove the Headroom startup entry. Check folder permissions."));
        if (!persistPreference(false)) {
            if (existed) { QSaveFile restore(m_entryPath); if (restore.open(QIODevice::WriteOnly)) { restore.write(previous); restore.commit(); } }
            refresh(); return false;
        }
        clearError();
        refresh();
        emit preferenceChanged(false);
        return true;
    }
    // '=' is prohibited in Desktop Entry executable paths. GIO cannot resolve
    // a literal '%' executable even with %% escaping, so fail visibly as well.
    if (!QDir::isAbsolutePath(m_executable) || m_executable.contains('=') || m_executable.contains('%') ||
        m_executable.contains('\n') || m_executable.contains('\r') ||
        m_executable.contains('\t') || m_executable.contains(QChar::Null))
        return fail(tr("The application path cannot be used in a desktop startup entry."));
    const QFileInfo executable(m_executable);
    if (!executable.isFile() || !executable.isExecutable())
        return fail(tr("The Headroom executable is missing or is not executable."));
    if (!QDir().mkpath(QFileInfo(m_entryPath).absolutePath()))
        return fail(tr("Could not create the startup folder. Check folder permissions."));
    const QByteArray previous = [&] { QFile file(m_entryPath); return file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray{}; }();
    const bool existed = QFileInfo::exists(m_entryPath);
    QSaveFile entry(m_entryPath);
    if (!entry.open(QIODevice::WriteOnly))
        return fail(tr("Could not write the Headroom startup entry: %1").arg(entry.errorString()));
    const QByteArray contents = QString(
        "[Desktop Entry]\nType=Application\nVersion=1.0\nName=Headroom\n"
        "Comment=Monitor your AI usage from the system tray\n"
        "Exec=%1 --background\nIcon=headroom\nTerminal=false\n"
        "X-GNOME-Autostart-enabled=true\n").arg(quotedExecutable(m_executable)).toUtf8();
    if (entry.write(contents) != contents.size() || !entry.commit())
        return fail(tr("Could not save the Headroom startup entry: %1").arg(entry.errorString()));
    if (!persistPreference(true)) {
        if (existed) { QSaveFile restore(m_entryPath); if (restore.open(QIODevice::WriteOnly)) { restore.write(previous); restore.commit(); } }
        else QFile::remove(m_entryPath);
        refresh(); return false;
    }
    clearError();
    refresh();
    emit preferenceChanged(true);
    return true;
}

bool StartupService::migrateLegacyRegistration(bool enabled)
{
    if (!m_windows || !enabled) return true;
    if (!setEnabled(true)) return false;
    QSettings registry(m_registryPath, QSettings::NativeFormat);
    // Remove only the legacy application's known value, and only after the
    // replacement has been written and read back successfully.
    registry.remove("ClaudeUsageWidget");
    registry.sync();
    if (registry.status() != QSettings::NoError || registry.contains("ClaudeUsageWidget"))
        return fail(tr("Headroom starts at sign-in, but the legacy startup entry could not be removed."));
    clearError();
    return true;
}
