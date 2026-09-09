#include <QCoreApplication>
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTextStream>
#include <QSysInfo>
#include <QDir>

int main(int argc, char **argv) {
    QCoreApplication app(argc, argv);
    const QStringList args = app.arguments();
    const QString command = args.size() > 1 ? args.at(1) : QString();
    if (const QString record = qEnvironmentVariable("HEADROOM_UPDATE_FIXTURE_RECORD"); !record.isEmpty()) {
        QFile file(record);
        if (file.open(QIODevice::Append)) {
            QTextStream stream(&file);
            stream << command << " token=" << (qEnvironmentVariableIsSet("USAGE_AUTH_TOKEN") ? "present" : "absent") << '\n';
        }
    }
    if (command == QStringLiteral("inspect")) {
        QString architecture = QSysInfo::currentCpuArchitecture();
        if (architecture == QStringLiteral("amd64")) architecture = QStringLiteral("x86_64");
        if (architecture == QStringLiteral("aarch64")) architecture = QStringLiteral("arm64");
#ifdef Q_OS_WIN
        const QString platform = QStringLiteral("windows");
        const QString asset = QStringLiteral("Headroom-v0.1.0-windows-") + (architecture == QStringLiteral("x86_64") ? QStringLiteral("x64.zip") : QStringLiteral("arm64.zip"));
#else
        const QString platform = QStringLiteral("linux");
        const QString asset = QStringLiteral("Headroom-v0.1.0-linux-x86_64.tar.gz");
#endif
        QJsonObject result{{"installed", true}, {"trusted_identity", true}, {"complete", !qEnvironmentVariableIsSet("HEADROOM_UPDATE_FIXTURE_MISSING")},
            {"version", "0.1.0"}, {"platform", platform}, {"architecture", architecture}, {"package_asset", asset},
            {"launcher_path", qEnvironmentVariable("HEADROOM_UPDATE_FIXTURE_INTERNAL_LAUNCHER", "/fixture/headroom-launcher")}};
        if (qEnvironmentVariableIsSet("HEADROOM_UPDATE_FIXTURE_MISSING")) result["missing"] = QJsonArray{"bin/usage-server"};
        QTextStream(stdout) << QJsonDocument(QJsonObject{{"ok", true}, {"command", command}, {"result", result}}).toJson(QJsonDocument::Compact) << '\n';
        return 0;
    }
    const QString mode = qEnvironmentVariable("HEADROOM_UPDATE_FIXTURE_MODE", "available");
    if (mode == QStringLiteral("hang")) {
        QFile input; if (input.open(stdin, QIODevice::ReadOnly)) input.read(1);
        QTextStream(stdout) << "{\"ok\":false,\"command\":\"" << command << "\",\"error\":\"cancelled\"}\n";
        return 2;
    }
    if (mode == QStringLiteral("stderr")) { QFile error; if (error.open(stderr, QIODevice::WriteOnly)) error.write(QByteArray(300 * 1024, 'x')); return 2; }
    if (mode == QStringLiteral("malformed")) { QTextStream(stdout) << "not json\n"; return 0; }
    QString status = command == QStringLiteral("check-update") ? mode : QStringLiteral("staged");
    QString version = command == QStringLiteral("stage-repair") ? QStringLiteral("0.1.0") : QStringLiteral("9.1.0");
    QString architecture = QSysInfo::currentCpuArchitecture();
    if (architecture == QStringLiteral("amd64")) architecture = QStringLiteral("x86_64");
    if (architecture == QStringLiteral("aarch64")) architecture = QStringLiteral("arm64");
#ifdef Q_OS_WIN
    const QString platform = QStringLiteral("windows");
#else
    const QString platform = QStringLiteral("linux");
#endif
    QJsonObject result{{"status", status}, {"current_version", "0.1.0"}, {"version", version},
        {"platform", platform}, {"architecture", architecture}, {"asset_name", "fixture-package"}};
    if (status == QStringLiteral("staged")) {
        const int rootIndex = args.indexOf(QStringLiteral("--install-root"));
        const QString installRoot = rootIndex >= 0 && rootIndex + 1 < args.size() ? args.at(rootIndex + 1) : QString();
        const QString stageDirectory = QDir(installRoot).filePath(QStringLiteral("staging/package-fixture"));
        const QString packageRoot = QDir(stageDirectory).filePath(QStringLiteral("contents/package-root"));
        QDir().mkpath(packageRoot);
        const QJsonObject stage{{"schema", 1}, {"product", "Headroom"}, {"version", version}, {"platform", platform},
            {"architecture", architecture}, {"package_asset", "fixture-package"}, {"manifest_sha256", QString(64, 'a')}, {"package_root", packageRoot}};
        result["stage"] = stage;
        QFile verified(QDir(stageDirectory).filePath(QStringLiteral("verified-stage.json")));
        if (verified.open(QIODevice::WriteOnly | QIODevice::Truncate)) verified.write(qEnvironmentVariableIsSet("HEADROOM_UPDATE_FIXTURE_BAD_STAGE") ? QByteArray("{}") : QJsonDocument(stage).toJson());
    }
    QTextStream(stdout) << QJsonDocument(QJsonObject{{"ok", true}, {"command", command}, {"result", result}}).toJson(QJsonDocument::Compact) << '\n';
    return 0;
}
