#pragma once

#include <QNetworkReply>
#include <QNetworkRequest>
#include <QSslCertificate>
#include <QSslConfiguration>
#include <QSslSocket>
#include <QUrl>

struct ServerConnection {
    QUrl url;
    QByteArray token;
    QSslCertificate certificate;

    bool isPinned() const { return !certificate.isNull(); }
};

namespace ServerTransport {
inline void secureRequest(QNetworkRequest &request, const QSslCertificate &certificate)
{
    if (certificate.isNull()) return;
    QSslConfiguration configuration = QSslConfiguration::defaultConfiguration();
    configuration.setProtocol(QSsl::TlsV1_2OrLater);
    configuration.setPeerVerifyMode(QSslSocket::VerifyPeer);
    configuration.setPeerVerifyDepth(1);
    configuration.setCaCertificates({certificate});
    configuration.setAllowedNextProtocols({QByteArrayLiteral("http/1.1")});
    request.setSslConfiguration(configuration);
}

inline void requirePinnedPeer(QNetworkReply *reply, const QSslCertificate &certificate)
{
    if (certificate.isNull()) return;
    const QByteArray expected = certificate.toDer();
    QObject::connect(reply, &QNetworkReply::encrypted, reply, [reply, expected] {
        if (reply->sslConfiguration().peerCertificate().toDer() != expected) reply->abort();
    });
}
}
