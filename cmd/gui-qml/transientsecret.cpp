// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "transientsecret.h"

#include <QJsonDocument>

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

static QByteArray compactObjectBody(const QJsonObject &object) {
  // "{...}" sin las llaves; vacío si el objeto no tiene campos.
  const QByteArray json = QJsonDocument(object).toJson(QJsonDocument::Compact);
  return json.mid(1, json.size() - 2);
}

QByteArray buildSecretRequestFrame(const QJsonObject &header,
                                   const QJsonObject &plainParams,
                                   const QList<SecretField> &secrets) {
  const QByteArray headerBody = compactObjectBody(header);
  const QByteArray paramsBody = compactObjectBody(plainParams);
  qsizetype capacity = headerBody.size() + paramsBody.size() + 32;
  for (const SecretField &field : secrets) {
    const qsizetype raw = field.second ? field.second->size() : 0;
    capacity += field.first.size() + 8 + (raw + 2) / 3 * 4;
  }
  QByteArray frame;
  // Reservar de una vez evita realojamientos que dejarían trozos del secreto
  // en memoria ya liberada.
  frame.reserve(capacity);
  frame.append('{');
  frame.append(headerBody);
  if (!headerBody.isEmpty())
    frame.append(',');
  frame.append("\"params\":{");
  frame.append(paramsBody);
  bool first = paramsBody.isEmpty();
  for (const SecretField &field : secrets) {
    if (!first)
      frame.append(',');
    first = false;
    frame.append('"');
    frame.append(field.first.toLatin1());
    frame.append("\":\"");
    if (field.second) {
      QByteArray encoded = field.second->toBase64();
      frame.append(encoded);
      zeroize(encoded);
      field.second->fill('\0');
    }
    frame.append('"');
  }
  frame.append("}}\n");
  return frame;
}

} // namespace TransientSecret
