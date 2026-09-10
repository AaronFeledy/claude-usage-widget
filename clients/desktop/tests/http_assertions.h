#pragma once

#include <QByteArray>
#include <QList>

namespace HttpAssertions {

inline bool hasHeader(const QByteArray &request, const QByteArray &name,
                      const QByteArray &expectedValue)
{
    const QList<QByteArray> lines = request.split('\n');
    for (qsizetype index = 1; index < lines.size(); ++index) {
        const QByteArray line = lines[index].trimmed();
        if (line.isEmpty()) break;
        const qsizetype separator = line.indexOf(':');
        if (separator < 0) continue;
        if (line.first(separator).trimmed().compare(name, Qt::CaseInsensitive) == 0 &&
            line.sliced(separator + 1).trimmed() == expectedValue)
            return true;
    }
    return false;
}

inline bool hasHeader(const QByteArray &request, const QByteArray &name)
{
    const QList<QByteArray> lines = request.split('\n');
    for (qsizetype index = 1; index < lines.size(); ++index) {
        const QByteArray line = lines[index].trimmed();
        if (line.isEmpty()) break;
        const qsizetype separator = line.indexOf(':');
        if (separator >= 0 &&
            line.first(separator).trimmed().compare(name, Qt::CaseInsensitive) == 0)
            return true;
    }
    return false;
}

} // namespace HttpAssertions
