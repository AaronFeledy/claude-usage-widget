#include "macos_trust_diagnostics.h"

#include <QByteArray>
#include <QSslCertificate>
#include <Security/Security.h>

namespace {
QString fromCfString(CFStringRef string)
{
    if (!string) return {};
    const CFIndex length = CFStringGetLength(string);
    QByteArray utf8(CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1, '\0');
    if (!CFStringGetCString(string, utf8.data(), utf8.size(), kCFStringEncodingUTF8)) return {};
    utf8.truncate(qstrlen(utf8.constData()));
    return QString::fromUtf8(utf8);
}

QString status(const QString &operation, OSStatus result)
{
    return operation + QStringLiteral("-status=") + QString::number(result);
}

QString normalized(QString description)
{
    description.replace('\r', ' ');
    description.replace('\n', ' ');
    description.replace('\t', ' ');
    return description.simplified().left(512);
}
}

QString MacTrustDiagnostics::evaluate(const QSslCertificate &certificate, const QString &host)
{
    const QByteArray der = certificate.toDer();
    if (der.isEmpty()) return QStringLiteral("certificate-empty");
    CFDataRef data = CFDataCreate(nullptr,
                                  reinterpret_cast<const UInt8 *>(der.constData()), der.size());
    if (!data) return QStringLiteral("certificate-data-allocation-failed");
    SecCertificateRef nativeCertificate = SecCertificateCreateWithData(nullptr, data);
    CFRelease(data);
    if (!nativeCertificate) return QStringLiteral("certificate-conversion-failed");

    const QByteArray hostUtf8 = host.toUtf8();
    CFStringRef nativeHost = CFStringCreateWithBytes(nullptr,
        reinterpret_cast<const UInt8 *>(hostUtf8.constData()), hostUtf8.size(),
        kCFStringEncodingUTF8, false);
    SecPolicyRef policy = nativeHost ? SecPolicyCreateSSL(true, nativeHost) : nullptr;
    if (nativeHost) CFRelease(nativeHost);
    if (!policy) {
        CFRelease(nativeCertificate);
        return QStringLiteral("ssl-policy-allocation-failed");
    }

    SecTrustRef trust = nullptr;
    OSStatus result = SecTrustCreateWithCertificates(nativeCertificate, policy, &trust);
    CFRelease(policy);
    if (result != errSecSuccess || !trust) {
        CFRelease(nativeCertificate);
        return status(QStringLiteral("create-trust"), result);
    }
    const void *anchorValues[] = {nativeCertificate};
    CFArrayRef anchors = CFArrayCreate(nullptr, anchorValues, 1, &kCFTypeArrayCallBacks);
    CFRelease(nativeCertificate);
    if (!anchors) {
        CFRelease(trust);
        return QStringLiteral("anchor-array-allocation-failed");
    }
    result = SecTrustSetAnchorCertificates(trust, anchors);
    CFRelease(anchors);
    if (result == errSecSuccess) result = SecTrustSetAnchorCertificatesOnly(trust, true);
    if (result == errSecSuccess) result = SecTrustSetNetworkFetchAllowed(trust, false);
    if (result != errSecSuccess) {
        CFRelease(trust);
        return status(QStringLiteral("configure-trust"), result);
    }

    CFErrorRef error = nullptr;
    const bool trusted = SecTrustEvaluateWithError(trust, &error);
    CFRelease(trust);
    if (trusted) return QStringLiteral("trusted=true");
    if (!error) return QStringLiteral("untrusted-without-error");
    const QString domain = fromCfString(CFErrorGetDomain(error));
    const CFIndex code = CFErrorGetCode(error);
    CFStringRef description = CFErrorCopyDescription(error);
    const QString detail = normalized(fromCfString(description));
    if (description) CFRelease(description);
    CFRelease(error);
    return QStringLiteral("untrusted domain=%1 code=%2 description=%3")
        .arg(domain, QString::number(code), detail);
}
