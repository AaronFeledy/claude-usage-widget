#pragma once

#include <QJsonObject>
#include <QString>
#include <QStringList>

struct DesktopSettings {
    QString connectionMode = "remote";
    QString url;
    QString token;
    int interval = 60;
    bool notifications = true;
    QString primary = "Claude";
    QStringList order;
    bool startup = false;
    bool startupMigrationPending = false;
};

class SettingsService {
public:
    enum class Platform { Current, Linux, Windows };

    explicit SettingsService(QString path = {}, bool allowAutomaticMigration = true,
                             Platform platform = Platform::Current, QString legacyPath = {},
                             QString defaultPathOverride = {});

    const DesktopSettings &value() const { return m_value; }
    QString path() const { return m_path; }
    QString legacyBackupPath() const { return m_legacyBackupPath; }
    QString loadError() const { return m_loadError; }
    bool importedLegacy() const { return m_importedLegacy; }

    QString save(const DesktopSettings &settings, bool explicitUserSave = false);
    QString saveOrder(const QStringList &order, const QString &primary);
    QString saveStartupPreference(bool enabled);
    QString completeStartupMigration();

    static QString defaultPath(Platform platform = Platform::Current);
    static QString defaultLegacyPath();
    static QStringList normalizeOrder(const QStringList &order, const QString &legacyPrimary = {});
    static QString normalizeProvider(const QString &provider);

private:
    bool isWindows() const;
    bool loadHeadroom();
    bool importLegacy();
    QString write(const DesktopSettings &settings);
    QJsonObject serialized(const DesktopSettings &settings) const;

    Platform m_platform;
    bool m_explicitPath = false;
    QString m_path;
    QString m_legacyPath;
    QString m_legacyBackupPath;
    QString m_loadError;
    QJsonObject m_document;
    DesktopSettings m_value;
    bool m_importedLegacy = false;
    bool m_blockImplicitWrites = false;
    bool m_allowAutomaticMigration = true;
};
