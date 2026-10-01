// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef PROCESSENVIRONMENT_H
#define PROCESSENVIRONMENT_H

#include <QByteArray>
#include <QProcessEnvironment>
#include <QString>
#include <QStringList>

namespace ChildProcessEnvironment {

inline const QStringList &sensitiveVariableNames() {
  static const QStringList names = {
      QStringLiteral("GRXFIRMA_PKCS12_PASSWORD"),
      QStringLiteral("GRXFIRMA_REST_TOKEN"),
      QStringLiteral("GRXFIRMA_PROTECTION_SECRET_B64"),
  };
  return names;
}

inline QProcessEnvironment
sanitized(QProcessEnvironment environment =
              QProcessEnvironment::systemEnvironment()) {
  for (const QString &name : sensitiveVariableNames())
    environment.remove(name);
  return environment;
}

inline QProcessEnvironment
forRestToken(const QString &token,
             QProcessEnvironment environment =
                 QProcessEnvironment::systemEnvironment()) {
  environment = sanitized(environment);
  if (!token.isEmpty())
    environment.insert(QStringLiteral("GRXFIRMA_REST_TOKEN"), token);
  return environment;
}

inline bool scrubCurrentProcess() {
  bool success = true;
  for (const QString &name : sensitiveVariableNames()) {
    const QByteArray encodedName = name.toLatin1();
    if (!qunsetenv(encodedName.constData()))
      success = false;
  }
  return success;
}

} // namespace ChildProcessEnvironment

#endif // PROCESSENVIRONMENT_H
