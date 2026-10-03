// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "portalsealbridge.h"
#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSaveFile>

PortalSealBridge::PortalSealBridge(QObject *parent) : QObject(parent) {}

bool PortalSealBridge::load(const QString &requestPath) {
  const QFileInfo requestInfo(requestPath);
  if (!requestInfo.isFile() || requestInfo.isSymLink() ||
      requestInfo.fileName() != QStringLiteral("request.json") ||
      requestInfo.size() < 2 || requestInfo.size() > 4096)
    return false;
  QFile file(requestPath);
  if (!file.open(QIODevice::ReadOnly))
    return false;
  QJsonParseError parseError;
  const auto doc = QJsonDocument::fromJson(file.readAll(), &parseError);
  if (parseError.error != QJsonParseError::NoError || !doc.isObject())
    return false;
  const auto obj = doc.object();
  if (obj.size() != 3 || !obj.value(QStringLiteral("documentPath")).isString() ||
      !obj.value(QStringLiteral("resultPath")).isString() ||
      !obj.value(QStringLiteral("signerName")).isString())
    return false;
  const QString base = requestInfo.absolutePath();
  const QString document = obj.value(QStringLiteral("documentPath")).toString();
  const QString result = obj.value(QStringLiteral("resultPath")).toString();
  const QFileInfo pdfInfo(document), resultInfo(result);
  if (pdfInfo.absolutePath() != base || resultInfo.absolutePath() != base ||
      pdfInfo.fileName() != QStringLiteral("document.pdf") ||
      resultInfo.fileName() != QStringLiteral("result.json") ||
      !pdfInfo.isFile() || pdfInfo.isSymLink() ||
      pdfInfo.size() < 1 || pdfInfo.size() > 100LL * 1024 * 1024 ||
      resultInfo.exists() || resultInfo.isSymLink())
    return false;
  m_documentPath = document;
  m_resultPath = result;
  m_signerName = obj.value(QStringLiteral("signerName")).toString().left(256);
  m_active = true;
  return true;
}

bool PortalSealBridge::submit(const QString &action,
                              const QVariantList &placements,
                              const QVariantMap &appearance) {
  if (!m_active || (action != QStringLiteral("place") &&
                    action != QStringLiteral("without") &&
                    action != QStringLiteral("cancel")))
    return false;
  if ((action == QStringLiteral("place") && (placements.isEmpty() || placements.size() > 128)) ||
      (action != QStringLiteral("place") && (!placements.isEmpty() || !appearance.isEmpty())))
    return false;
  QJsonObject result{{QStringLiteral("action"), action}};
  if (action == QStringLiteral("place"))
    result.insert(QStringLiteral("visibleSealPlacements"),
                  QJsonArray::fromVariantList(placements));
  if (action == QStringLiteral("place") && !appearance.isEmpty())
    result.insert(QStringLiteral("appearance"), QJsonObject::fromVariantMap(appearance));
  const auto encoded = QJsonDocument(result).toJson(QJsonDocument::Compact);
  if (encoded.size() > 32 * 1024)
    return false;
  QSaveFile file(m_resultPath);
  if (!file.open(QIODevice::WriteOnly))
    return false;
  if (file.write(encoded) != encoded.size() || !file.commit())
    return false;
  QFile::setPermissions(m_resultPath, QFileDevice::ReadOwner | QFileDevice::WriteOwner);
  m_active = false;
  QCoreApplication::quit();
  return true;
}
