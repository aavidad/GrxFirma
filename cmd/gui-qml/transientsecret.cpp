// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "transientsecret.h"

namespace TransientSecret {

bool isCanonicalAES256Base64(const QString &value) {
  if (value.size() != AES256Base64Length)
    return false;
  for (const QChar ch : value) {
    if (ch.unicode() > 0x7f)
      return false;
  }

  QByteArray encoded = value.toLatin1();
  QByteArray decoded = QByteArray::fromBase64(
      encoded, QByteArray::AbortOnBase64DecodingErrors);
  const bool valid =
      decoded.size() == AES256KeyBytes && decoded.toBase64() == encoded;
  zeroize(decoded);
  zeroize(encoded);
  return valid;
}

QString normalizedEncryptedDataOutputPath(const QString &value) {
  const QString path = value.trimmed();
  if (path.isEmpty() ||
      path.endsWith(QStringLiteral(".encrypted.p7m"), Qt::CaseInsensitive)) {
    return path;
  }
  return path + QStringLiteral(".encrypted.p7m");
}

void zeroize(QString &value) {
  if (value.isEmpty())
    return;
  value.detach();
  value.fill(QChar('\0'));
  value.clear();
  value.squeeze();
}

void zeroize(QByteArray &value) {
  if (value.isEmpty())
    return;
  value.detach();
  value.fill('\0');
  value.clear();
  value.squeeze();
}

} // namespace TransientSecret
