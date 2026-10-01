// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef IPCSOCKETPATH_H
#define IPCSOCKETPATH_H

#include <QChar>
#include <QString>

namespace IpcSocketPath {

inline QString normalizeWindows(const QString &candidate) {
  const QString prefix = QStringLiteral("\\\\.\\pipe\\");
  QString name;
  if (candidate.startsWith(prefix, Qt::CaseInsensitive)) {
    name = candidate.mid(prefix.size());
  } else if (!candidate.isEmpty() && !candidate.contains(QLatin1Char('/')) &&
             !candidate.contains(QLatin1Char('\\'))) {
    name = candidate;
  } else {
    return QString();
  }

  if (name.isEmpty() || prefix.size() + name.size() > 256)
    return QString();
  for (const QChar character : name) {
    if (character == QLatin1Char('/') || character == QLatin1Char('\\') ||
        character.category() == QChar::Other_Control) {
      return QString();
    }
  }
  return prefix + name;
}

} // namespace IpcSocketPath

#endif // IPCSOCKETPATH_H
