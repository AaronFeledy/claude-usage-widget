#pragma once

#include <QString>

class QSslCertificate;

namespace MacTrustDiagnostics {
QString evaluate(const QSslCertificate &certificate, const QString &host);
}
