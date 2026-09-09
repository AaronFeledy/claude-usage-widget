#include "startup.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QSaveFile>

namespace {
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
}

StartupService::StartupService(QString configHome, QString executable, bool allowChanges, QObject *parent)
    : QObject(parent),
      m_entryPath(QDir(configHome.isEmpty() ? configDirectory() : configHome)
                      .filePath("autostart/headroom.desktop")),
      m_executable(executable.isEmpty() ? QCoreApplication::applicationFilePath() : executable),
      m_allowChanges(allowChanges)
{
    refresh();
}

void StartupService::refresh()
{
    if (!m_platformSupported) {
        if (m_enabled) {
            m_enabled = false;
            emit enabledChanged();
        }
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
        return fail(tr("Start at login is unavailable in preview mode."));
    if (!enabled) {
        // Only remove our entry. Never remove or rewrite another autostart item.
        if (QFileInfo::exists(m_entryPath) && !QFile::remove(m_entryPath))
            return fail(tr("Could not remove the Headroom startup entry. Check folder permissions."));
        clearError();
        refresh();
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
    clearError();
    refresh();
    return true;
}
