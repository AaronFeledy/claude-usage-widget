#include <QByteArray>
#include <QCoreApplication>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QThread>

int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    const QStringList args = app.arguments();
    const int separator = args.indexOf(QStringLiteral("--"));
    if (separator < 0 || separator + 2 >= args.size() || args.last() != QStringLiteral("usage-server --ssh-stdio")) return 9;
    const QString host = args.at(separator + 1);
    QFile input; if (!input.open(stdin, QIODevice::ReadOnly)) return 10;
    const QByteArray requestBytes = input.readAll();
    if (host == QStringLiteral("delay")) QThread::msleep(2000);
    if (host == QStringLiteral("failure")) return 8;
    if (host == QStringLiteral("oversized")) { fwrite(QByteArray(2 * 1024 * 1024 + 1, 'x').constData(), 1, 2 * 1024 * 1024 + 1, stdout); return 0; }
    if (host == QStringLiteral("duplicate")) { fputs("{\"schema\":1,\"\\u0073chema\":1,\"status\":200,\"body\":\"W10=\"}\n", stdout); return 0; }
    if (host == QStringLiteral("escaped")) { fputs("{\"\\u0073chema\":1,\"status\":200,\"body\":\"W10=\"}\n", stdout); return 0; }
    if (host == QStringLiteral("badbase64")) { fputs("{\"schema\":1,\"status\":200,\"body\":\"Zh==\"}\n", stdout); return 0; }
    if (host == QStringLiteral("extra")) { fputs("{\"schema\":1,\"status\":200,\"body\":\"W10=\"}\nnoise\n", stdout); return 0; }
    QJsonParseError error;
    const auto request = QJsonDocument::fromJson(requestBytes.trimmed(), &error).object();
    if (error.error != QJsonParseError::NoError || request.size() != 4 || request.value("schema").toInt() != 1) return 7;
    QByteArray body = QByteArrayLiteral("[]");
    if (request.value("path").toString().endsWith(QStringLiteral("health"))) body = QByteArrayLiteral("{\"status\":\"ok\",\"version\":\"test\"}");
    else if (request.value("path").toString().contains(QStringLiteral("/providers/cursor/credentials"))) {
        const QByteArray submitted = QByteArray::fromBase64(request.value("body").toString().toLatin1());
        if (!submitted.contains("synthetic-cursor")) return 6;
        body = QByteArrayLiteral("{\"provider\":\"Cursor\",\"refetched\":true,\"usage\":{\"provider_name\":\"Cursor\",\"error\":null,\"is_success\":true,\"needs_reauth\":false,\"buckets\":[{\"id\":\"weekly\",\"label\":\"Weekly\",\"utilization\":2,\"resets_at\":null,\"status_text\":null}]}}");
    }
    const QByteArray response = QByteArrayLiteral("{\"schema\":1,\"status\":200,\"body\":\"") + body.toBase64() + QByteArrayLiteral("\"}\n");
    fwrite(response.constData(), 1, size_t(response.size()), stdout);
    return 0;
}
