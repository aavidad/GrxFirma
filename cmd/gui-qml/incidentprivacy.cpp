// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "incidentprivacy.h"

#include <QCryptographicHash>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonParseError>
#include <QRandomGenerator>
#include <QRegularExpression>
#include <QSaveFile>
#include <QUrl>
#include <QVariantList>

#include <algorithm>

namespace {

constexpr int kIncidentMaxDepth = 10;
constexpr int kIncidentMaxMapItems = 128;
constexpr int kIncidentMaxListItems = 128;
constexpr int kIncidentMaxStringChars = 4096;
constexpr int kIncidentCharacterBudget = 128 * 1024;
constexpr int kIncidentItemBudget = 1024;
constexpr qint64 kIncidentMaxSavedReportBytes = 512 * 1024;

struct IncidentRetentionGroup {
  QString baseName;
  QDateTime newestModificationUtc;
  QStringList files;
};

struct IncidentSanitizeState {
  int remainingCharacters = kIncidentCharacterBudget;
  int remainingItems = kIncidentItemBudget;
};

QString normalizedIncidentKey(const QString &key) {
  QString normalized = key.trimmed().toLower();
  normalized.remove(QRegularExpression(QStringLiteral("[^a-z0-9]")));
  return normalized;
}

bool incidentSensitiveKey(const QString &key) {
  const QString normalized = normalizedIncidentKey(key);
  if (normalized == QStringLiteral("dat") ||
      normalized == QStringLiteral("nif") ||
      normalized == QStringLiteral("dni") ||
      normalized == QStringLiteral("payload")) {
    return true;
  }
  const QStringList fragments = {
      QStringLiteral("password"),       QStringLiteral("passwd"),
      QStringLiteral("secret"),         QStringLiteral("token"),
      QStringLiteral("authorization"),  QStringLiteral("cookie"),
      QStringLiteral("credential"),     QStringLiteral("privatekey"),
      QStringLiteral("p12b64"),         QStringLiteral("contentbase64"),
      QStringLiteral("signatureb64"),   QStringLiteral("signedcontent"),
      QStringLiteral("certificatepem"), QStringLiteral("certificateder"),
      QStringLiteral("rawuri"),         QStringLiteral("rawrequest"),
      QStringLiteral("rawresponse"),    QStringLiteral("documentcontent"),
      QStringLiteral("certificate"),    QStringLiteral("signature"),
      QStringLiteral("private"),        QStringLiteral("document"),
      QStringLiteral("content"),        QStringLiteral("header"),
      QStringLiteral("selectedcertificateid"),
      QStringLiteral("selectedcertificatelabel"),
      QStringLiteral("subject"),        QStringLiteral("issuer"),
      QStringLiteral("organization"),   QStringLiteral("email")};
  for (const QString &fragment : fragments) {
    if (normalized.contains(fragment))
      return true;
  }
  return false;
}

bool incidentPathKey(const QString &key) {
  const QString normalized = normalizedIncidentKey(key);
  return normalized.contains(QStringLiteral("path")) ||
         normalized.endsWith(QStringLiteral("file")) ||
         normalized.contains(QStringLiteral("filename")) ||
         normalized.contains(QStringLiteral("directory")) ||
         normalized.contains(QStringLiteral("folder"));
}

QString incidentSafeLogToken(const QString &raw, const QString &fallback) {
  static const QRegularExpression token(
      QStringLiteral("^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$"));
  const QString candidate = raw.trimmed();
  return token.match(candidate).hasMatch() ? candidate : fallback;
}

qint64 incidentJsonValueSize(const QVariant &value) {
  QJsonArray wrapper;
  wrapper.append(QJsonValue::fromVariant(value));
  const qint64 wrappedSize =
      QJsonDocument(wrapper).toJson(QJsonDocument::Compact).size();
  return qMax<qint64>(0, wrappedSize - 2);
}

QString incidentOpaqueLogReference(const QString &raw) {
  if (raw.trimmed().isEmpty())
    return QString();
  static const QByteArray salt = []() {
    QByteArray value(32, '\0');
    auto *random = QRandomGenerator::system();
    for (int offset = 0; offset < value.size(); offset += 4) {
      const quint32 word = random->generate();
      const int remaining = qMin(4, value.size() - offset);
      for (int index = 0; index < remaining; ++index) {
        value[offset + index] =
            static_cast<char>((word >> (index * 8)) & 0xff);
      }
    }
    return value;
  }();
  QCryptographicHash hash(QCryptographicHash::Sha256);
  hash.addData(salt);
  hash.addData(raw.toUtf8());
  return QStringLiteral("ref-") +
         QString::fromLatin1(hash.result().left(8).toHex());
}

QString incidentFileNameOnly(const QString &raw) {
  const QString trimmed = raw.trimmed();
  if (trimmed.isEmpty())
    return QString();
  const QUrl url(trimmed);
  QString path = trimmed;
  if (url.isValid() && url.isLocalFile())
    path = url.toLocalFile();
  path.replace('\\', '/');
  const QString name = QFileInfo(path).fileName();
  if (name.isEmpty() || name == QStringLiteral(".") ||
      name == QStringLiteral("..")) {
    return QStringLiteral("[PATH]");
  }
  return name.left(255);
}

QString incidentRedactText(QString text) {
  static const QRegularExpression pemBlock(
      QStringLiteral("(?is)-----BEGIN [^-\\r\\n]+-----.*?"
                     "-----END [^-\\r\\n]+-----"));
  static const QRegularExpression authorizationHeader(
      QStringLiteral("(?i)\\b(?:Authorization|Proxy-Authorization|Cookie|"
                     "Set-Cookie)\\s*:\\s*[^\\r\\n]+"));
  static const QRegularExpression bearer(
      QStringLiteral("(?i)\\bBearer\\s+[A-Za-z0-9._~+/=-]+"));
  static const QRegularExpression assignment(
      QStringLiteral("(?i)\\b(token|password|passwd|secret|authorization|"
                     "credential|api[_-]?key|private[_-]?key|dat)"
                     "\\s*[=:]\\s*[^\\s,;&]+"));
  static const QRegularExpression jsonSecret(
      QStringLiteral("(?i)\\\"(?:token|password|passwd|secret|authorization|"
                     "credential|api[_-]?key|private[_-]?key|dat|payload|"
                     "content[_-]?base64|signature[_-]?b64|request[_-]?id|"
                     "trace[_-]?id|fingerprint|thumbprint|serial(?:number)?|"
                     "subject|issuer|nif|dni)\\\""
                     "\\s*:\\s*\\\"[^\\\"]*\\\""));
  static const QRegularExpression querySecret(
      QStringLiteral("(?i)([?&](?:token|password|passwd|secret|authorization|"
                     "credential|api[_-]?key|private[_-]?key|dat)=)"
                     "[^&#\\s\\\"'<>]+"));
  static const QRegularExpression afirmaURI(
      QStringLiteral("(?i)\\bafirma://[^\\s\\\"'<>]+"));
  static const QRegularExpression webURL(
      QStringLiteral("(?i)\\bhttps?://[^\\s\\\"'<>]+"));
  static const QRegularExpression fileURL(
      QStringLiteral("(?i)\\bfile:(?://)?[^\\s\\\"'<>]+"));
  static const QRegularExpression windowsPath(
      QStringLiteral("(?i)(?:\\b[A-Z]:[\\\\/]|\\\\\\\\)"
                     "[^\\r\\n\\\"'<>|]+"));
  static const QRegularExpression unixPath(
      QStringLiteral("(?<![:/A-Za-z0-9])/(?:[^\\s\\\"'<>]+/?)+"));
  static const QRegularExpression base64Blob(
      QStringLiteral("(?<![A-Za-z0-9+/=])[A-Za-z0-9+/]{80,}={0,2}"
                     "(?![A-Za-z0-9+/=])"));
  static const QRegularExpression metadataAssignment(
      QStringLiteral("(?i)\\b(?:subject|issuer|serial(?:number)?|fingerprint|"
                     "thumbprint|nif|dni|request[_-]?id|trace[_-]?id|"
                     "certificate[_-]?id|selected[_-]?certificate[_-]?id|"
                     "payload|content[_-]?base64|signature[_-]?b64|"
                     "signed[_-]?content|params|data)\\s*[=:]\\s*"
                     "(?:\\\"[^\\\"]*\\\"|'[^']*'|[^,;|\\]\\}\\r\\n\\s]+)"));
  static const QRegularExpression distinguishedName(
      QStringLiteral("(?i)\\b(?:CN|OU|O|L|ST|C|serialNumber|emailAddress)"
                     "\\s*=\\s*[^,;\\r\\n]+"));
  static const QRegularExpression fingerprint(
      QStringLiteral("(?i)(?<![A-F0-9])(?:[A-F0-9]{2}:){19,31}"
                     "[A-F0-9]{2}(?![A-F0-9])|"
                     "(?<![A-F0-9])[A-F0-9]{40}(?:[A-F0-9]{24})?"
                     "(?![A-F0-9])"));
  static const QRegularExpression rawCorrelationID(
      QStringLiteral("(?i)\\b(?:ipc|trace)-[0-9]{6,}-[0-9]+\\b|"
                     "\\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-"
                     "[89ab][0-9a-f]{3}-[0-9a-f]{12}\\b"));
  static const QRegularExpression ipv4(
      QStringLiteral("(?<![0-9])(?:25[0-5]|2[0-4][0-9]|1?[0-9]{1,2})"
                     "(?:\\.(?:25[0-5]|2[0-4][0-9]|1?[0-9]{1,2})){3}"
                     "(?![0-9])"));
  static const QRegularExpression ipv6(
      QStringLiteral("(?i)(?<![A-F0-9:])(?:[A-F0-9]{1,4}:){3,7}"
                     "[A-F0-9]{1,4}(?![A-F0-9:])|"
                     "(?<![A-F0-9:])::1(?![A-F0-9:])"));
  static const QRegularExpression email(
      QStringLiteral("(?i)\\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\\.[A-Z]{2,}\\b"));
  static const QRegularExpression spanishPersonalID(
      QStringLiteral("(?i)\\b(?:[XYZ]\\d{7}[A-Z]|\\d{8}[A-Z])\\b"));

  text.replace(pemBlock, QStringLiteral("[CRYPTO_MATERIAL]"));
  text.replace(authorizationHeader, QStringLiteral("[AUTH_HEADER]"));
  text.replace(bearer, QStringLiteral("Bearer [REDACTED]"));
  text.replace(jsonSecret, QStringLiteral("\"redacted\":\"[REDACTED]\""));
  text.replace(querySecret, QStringLiteral("\\1[REDACTED]"));
  text.replace(assignment, QStringLiteral("\\1=[REDACTED]"));
  text.replace(afirmaURI, QStringLiteral("afirma://[REDACTED]"));
  text.replace(webURL, QStringLiteral("[URL]"));
  text.replace(fileURL, QStringLiteral("[PATH]"));
  text.replace(windowsPath, QStringLiteral("[PATH]"));
  text.replace(unixPath, QStringLiteral("[PATH]"));
  text.replace(base64Blob, QStringLiteral("[BINARY_DATA]"));
  text.replace(metadataAssignment, QStringLiteral("[SENSITIVE_METADATA]"));
  text.replace(distinguishedName, QStringLiteral("[CERTIFICATE_IDENTITY]"));
  text.replace(fingerprint, QStringLiteral("[FINGERPRINT]"));
  text.replace(rawCorrelationID, QStringLiteral("[OPAQUE_ID]"));
  text.replace(ipv4, QStringLiteral("[IP]"));
  text.replace(ipv6, QStringLiteral("[IP]"));
  text.replace(email, QStringLiteral("[EMAIL]"));
  text.replace(spanishPersonalID, QStringLiteral("[PERSONAL_ID]"));
  return text;
}

QString incidentBoundedText(const QString &raw, IncidentSanitizeState *state) {
  if (!state || state->remainingCharacters <= 0)
    return QStringLiteral("[TRUNCATED]");
  QString text = incidentRedactText(raw);
  const int allowed =
      qMin(kIncidentMaxStringChars, state->remainingCharacters);
  if (text.size() > allowed)
    text = text.left(allowed) + QStringLiteral("…");
  state->remainingCharacters -= qMin(text.size(), allowed);
  return text;
}

QVariant incidentSanitizeValue(const QVariant &value, const QString &key,
                               int depth, IncidentSanitizeState *state);

bool incidentReadEligibleSavedReport(const QString &path,
                                     const QString &reportsDirectory,
                                     QVariantMap *rawPayload,
                                     QString *errorMessage) {
  const QFileInfo directory(reportsDirectory);
  const QFileInfo target(path);
  const auto reject = [errorMessage](const QString &message) {
    if (errorMessage)
      *errorMessage = message;
    return false;
  };
  if (reportsDirectory.trimmed().isEmpty() || !directory.exists() ||
      directory.isSymLink() || !directory.isDir()) {
    return reject(QStringLiteral(
        "El directorio privado de incidencias no es válido."));
  }
  if (path.trimmed().isEmpty() || !target.exists() || target.isSymLink() ||
      !target.isFile()) {
    return reject(
        QStringLiteral("La incidencia de origen no es un fichero regular."));
  }

  const QString canonicalDirectory = directory.canonicalFilePath();
  if (canonicalDirectory.isEmpty() ||
      target.canonicalPath() != canonicalDirectory) {
    return reject(QStringLiteral(
        "La incidencia de origen no pertenece al directorio privado."));
  }
  static const QRegularExpression incidentFileName(
      QStringLiteral("^incident-[A-Za-z0-9_-]+-\\d{8}-\\d{6}-\\d{3}-"
                     "[0-9a-fA-F]{8}\\.json$"));
  if (!incidentFileName.match(target.fileName()).hasMatch()) {
    return reject(
        QStringLiteral("El nombre de la incidencia de origen no es válido."));
  }
  if (target.size() <= 0 || target.size() > kIncidentMaxSavedReportBytes) {
    return reject(
        QStringLiteral("La incidencia de origen supera el límite permitido."));
  }

  QFile file(target.canonicalFilePath());
  if (!file.open(QIODevice::ReadOnly)) {
    return reject(
        QStringLiteral("No se pudo abrir la incidencia de origen."));
  }
  const QByteArray data = file.read(kIncidentMaxSavedReportBytes + 1);
  if (data.isEmpty() || data.size() > kIncidentMaxSavedReportBytes) {
    return reject(
        QStringLiteral("La incidencia de origen supera el límite permitido."));
  }
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(data, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return reject(
        QStringLiteral("La incidencia de origen no contiene JSON válido."));
  }

  const QVariantMap parsed = document.object().toVariantMap();
  if (parsed.value(QStringLiteral("schema")).toString() !=
      QStringLiteral("grxfirma-incident-v1")) {
    return reject(
        QStringLiteral("La incidencia de origen usa un esquema no válido."));
  }
  static const QRegularExpression eligibleFailureKind(
      QStringLiteral("^[a-z0-9][a-z0-9_-]{0,62}-failure$"));
  const QString kind = parsed.value(QStringLiteral("kind")).toString();
  if (!eligibleFailureKind.match(kind).hasMatch()) {
    return reject(QStringLiteral(
        "La incidencia guardada no procede de un fallo detectado."));
  }
  QDateTime generatedAt = QDateTime::fromString(
      parsed.value(QStringLiteral("generatedAt")).toString(),
      Qt::ISODateWithMs);
  if (!generatedAt.isValid()) {
    generatedAt = QDateTime::fromString(
        parsed.value(QStringLiteral("generatedAt")).toString(), Qt::ISODate);
  }
  if (!generatedAt.isValid() || generatedAt.offsetFromUtc() != 0) {
    return reject(QStringLiteral(
        "La incidencia guardada no contiene una fecha UTC válida."));
  }
  if (rawPayload)
    *rawPayload = parsed;
  return true;
}

bool incidentRemoveRecognizedFiles(const QString &directory,
                                   const QStringList &fileNames,
                                   int *removedFiles,
                                   QString *errorMessage) {
  const QFileInfo directoryInfo(directory);
  const QString canonicalDirectory = directoryInfo.canonicalFilePath();
  if (canonicalDirectory.isEmpty()) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("No se pudo validar el directorio de retención.");
    return false;
  }
  for (const QString &fileName : fileNames) {
    const QString path = QDir(directory).filePath(fileName);
    const QFileInfo fileInfo(path);
    if (!fileInfo.exists())
      continue;
    if (fileInfo.isSymLink() || !fileInfo.isFile() ||
        fileInfo.canonicalPath() != canonicalDirectory) {
      if (errorMessage)
        *errorMessage = QStringLiteral(
            "Un artefacto de retención dejó de ser un fichero regular.");
      return false;
    }
    if (!QFile::remove(path)) {
      if (errorMessage)
        *errorMessage =
            QStringLiteral("No se pudo aplicar el borrado de retención.");
      return false;
    }
    if (removedFiles)
      ++(*removedFiles);
  }
  return true;
}

QVariantMap incidentSanitizeSelectedMap(const QVariantMap &input,
                                        const QStringList &allowedKeys,
                                        int depth,
                                        IncidentSanitizeState *state) {
  QVariantMap output;
  for (const QString &key : allowedKeys) {
    if (!input.contains(key) || !state || state->remainingItems <= 0)
      continue;
    --state->remainingItems;
    output.insert(key,
                  incidentSanitizeValue(input.value(key), key, depth + 1, state));
  }
  return output;
}

QVariantMap incidentSanitizeMap(const QVariantMap &input, int depth,
                                IncidentSanitizeState *state) {
  QVariantMap output;
  if (depth > kIncidentMaxDepth || !state ||
      state->remainingCharacters <= 0) {
    output.insert(QStringLiteral("_truncated"), true);
    return output;
  }
  int count = 0;
  for (auto it = input.constBegin(); it != input.constEnd(); ++it) {
    if (count >= kIncidentMaxMapItems || state->remainingItems <= 0) {
      output.insert(QStringLiteral("_truncated"), true);
      break;
    }
    ++count;
    --state->remainingItems;
    QString outputKey = incidentRedactText(it.key()).left(128);
    if (outputKey.isEmpty() || outputKey != it.key())
      outputKey = QStringLiteral("_redacted_field_%1").arg(count);
    if (incidentSensitiveKey(it.key())) {
      output.insert(outputKey, QStringLiteral("[REDACTED]"));
      continue;
    }
    output.insert(outputKey,
                  incidentSanitizeValue(it.value(), it.key(), depth + 1, state));
  }
  return output;
}

QVariantList incidentSanitizeList(const QVariantList &input,
                                  const QString &key, int depth,
                                  IncidentSanitizeState *state) {
  QVariantList output;
  if (depth > kIncidentMaxDepth || !state ||
      state->remainingCharacters <= 0) {
    output.append(QStringLiteral("[TRUNCATED]"));
    return output;
  }
  const int count =
      qMin(qMin(input.size(), kIncidentMaxListItems), state->remainingItems);
  output.reserve(count + (input.size() > count ? 1 : 0));
  for (int i = 0; i < count; ++i) {
    --state->remainingItems;
    output.append(incidentSanitizeValue(input.at(i), key, depth + 1, state));
  }
  if (input.size() > count)
    output.append(QStringLiteral("[TRUNCATED]"));
  return output;
}

QVariant incidentSanitizeValue(const QVariant &value, const QString &key,
                               int depth, IncidentSanitizeState *state) {
  if (depth > kIncidentMaxDepth)
    return QStringLiteral("[TRUNCATED]");
  if (value.typeId() == QMetaType::QVariantMap)
    return incidentSanitizeMap(value.toMap(), depth, state);
  if (value.typeId() == QMetaType::QVariantList)
    return incidentSanitizeList(value.toList(), key, depth, state);
  if (value.typeId() == QMetaType::QString) {
    if (incidentPathKey(key))
      return incidentBoundedText(incidentFileNameOnly(value.toString()), state);
    const QString normalized = normalizedIncidentKey(key);
    if (normalized == QStringLiteral("requestid") ||
        normalized == QStringLiteral("traceid")) {
      static const QRegularExpression safeCorrelationID(
          QStringLiteral("^[A-Za-z0-9._:-]{1,128}$"));
      const QString candidate = value.toString().trimmed();
      if (!safeCorrelationID.match(candidate).hasMatch())
        return QStringLiteral("[INVALID_ID]");
      return incidentBoundedText(candidate, state);
    }
    return incidentBoundedText(value.toString(), state);
  }
  if (value.typeId() == QMetaType::Bool || value.canConvert<qlonglong>() ||
      value.canConvert<double>()) {
    return value;
  }
  return incidentBoundedText(value.toString(), state);
}

} // namespace

QVariantMap IncidentSanitizePayload(const QVariantMap &payload,
                                    bool remoteSend) {
  IncidentSanitizeState state;
  QVariantMap sanitized = incidentSanitizeSelectedMap(
      payload,
      {QStringLiteral("schema"), QStringLiteral("generatedAt"),
       QStringLiteral("kind"), QStringLiteral("userMessage"),
       QStringLiteral("friendlySummary"),
       QStringLiteral("probableResponsibility"),
       QStringLiteral("resolutionScope"), QStringLiteral("suggestedAction"),
       QStringLiteral("summaryText"),
       QStringLiteral("verificationAvailable"),
       QStringLiteral("lastOutputVerificationAvailable")},
      0, &state);

  const QVariantMap rawPreview =
      payload.value(QStringLiteral("supportPreview")).toMap();
  QVariantMap preview = incidentSanitizeSelectedMap(
      rawPreview,
      {QStringLiteral("title"), QStringLiteral("summary"),
       QStringLiteral("includedData"), QStringLiteral("omittedData")},
      1, &state);
  const QVariantMap rawConsent =
      rawPreview.value(QStringLiteral("consent")).toMap();
  QVariantMap consent;
  consent.insert(QStringLiteral("required"), true);
  consent.insert(QStringLiteral("previewShown"),
                 rawConsent.value(QStringLiteral("previewShown"), false)
                     .toBool());
  consent.insert(QStringLiteral("remoteSend"),
                 remoteSend && IncidentHasExplicitRemoteConsent(payload));
  preview.insert(QStringLiteral("consent"), consent);
  sanitized.insert(QStringLiteral("supportPreview"), preview);

  const QVariantMap rawEnvironment =
      payload.value(QStringLiteral("environment")).toMap();
  QVariantMap environment = incidentSanitizeSelectedMap(
      rawEnvironment,
      {QStringLiteral("platform"), QStringLiteral("appVersion"),
       QStringLiteral("language"), QStringLiteral("currentStep"),
       QStringLiteral("operationLabel"), QStringLiteral("requestId"),
       QStringLiteral("traceId")},
      1, &state);
  const QVariantMap diagnostic = incidentSanitizeSelectedMap(
      rawEnvironment.value(QStringLiteral("diagnostic")).toMap(),
      {QStringLiteral("category"), QStringLiteral("code"),
       QStringLiteral("failureCode"), QStringLiteral("likelyOwner"),
       QStringLiteral("userCanResolveDirectly"),
       QStringLiteral("retryable"), QStringLiteral("suggestedAction"),
       QStringLiteral("userMessage"),
       QStringLiteral("responsibilityMessage")},
      2, &state);
  if (!diagnostic.isEmpty())
    environment.insert(QStringLiteral("diagnostic"), diagnostic);
  if (!environment.isEmpty())
    sanitized.insert(QStringLiteral("environment"), environment);

  const QVariantMap operation = incidentSanitizeSelectedMap(
      payload.value(QStringLiteral("operation")).toMap(),
      {QStringLiteral("activeTab"), QStringLiteral("signAction"),
       QStringLiteral("signFormat"), QStringLiteral("signProfile"),
       QStringLiteral("selectedCertIndex"),
       QStringLiteral("certificateSelected"),
       QStringLiteral("inputFileName"), QStringLiteral("outputFileName"),
       QStringLiteral("verifyFileName"),
       QStringLiteral("verifyOriginalFileName"),
       QStringLiteral("protectInputFileName"),
       QStringLiteral("protectOutputFileName"),
       QStringLiteral("unprotectInputFileName"),
       QStringLiteral("unprotectOutputFileName"),
       // Compatibilidad defensiva con payloads anteriores: solo se conserva el
       // nombre, nunca la ruta completa.
       QStringLiteral("inputPath"), QStringLiteral("outputPath"),
       QStringLiteral("verifyFilePath"),
       QStringLiteral("verifyOriginalPath"),
       QStringLiteral("protectInputPath"),
       QStringLiteral("protectOutputPath"),
       QStringLiteral("unprotectInputPath"),
       QStringLiteral("unprotectOutputPath"),
       QStringLiteral("batchCount"), QStringLiteral("runtimeProxyMode"),
       QStringLiteral("expertMode")},
      1, &state);
  if (!operation.isEmpty())
    sanitized.insert(QStringLiteral("operation"), operation);

  const QVariantMap knownIssue = incidentSanitizeSelectedMap(
      payload.value(QStringLiteral("knownIssue")).toMap(),
      {QStringLiteral("known"), QStringLiteral("title"),
       QStringLiteral("summary"), QStringLiteral("fixedIn")},
      1, &state);
  if (!knownIssue.isEmpty())
    sanitized.insert(QStringLiteral("knownIssue"), knownIssue);

  const QVariantMap rawActiveDiagnostic =
      payload.value(QStringLiteral("activeDiagnostic")).toMap();
  QVariantMap activeDiagnostic = incidentSanitizeSelectedMap(
      rawActiveDiagnostic,
      {QStringLiteral("schema"), QStringLiteral("trigger"),
       QStringLiteral("completed"), QStringLiteral("consent"),
       QStringLiteral("conclusive"), QStringLiteral("confidence"),
       QStringLiteral("passiveCategory"), QStringLiteral("scope"),
       QStringLiteral("probableCause"), QStringLiteral("likelyOwner"),
       QStringLiteral("suggestedAction"),
       QStringLiteral("originIncidentFileName")},
      1, &state);
  QVariantList activeProbes;
  const QVariantList rawActiveProbes =
      rawActiveDiagnostic.value(QStringLiteral("probes")).toList();
  const int activeProbeCount = qMin(rawActiveProbes.size(), 16);
  activeProbes.reserve(activeProbeCount);
  for (int index = 0; index < activeProbeCount; ++index) {
    const QVariantMap probe = incidentSanitizeSelectedMap(
        rawActiveProbes.at(index).toMap(),
        {QStringLiteral("kind"), QStringLiteral("state"),
         QStringLiteral("detailCode"), QStringLiteral("durationMs")},
        2, &state);
    if (!probe.isEmpty())
      activeProbes.append(probe);
  }
  if (!activeProbes.isEmpty())
    activeDiagnostic.insert(QStringLiteral("probes"), activeProbes);
  if (!activeDiagnostic.isEmpty())
    sanitized.insert(QStringLiteral("activeDiagnostic"), activeDiagnostic);

  const QVariantMap extra = incidentSanitizeSelectedMap(
      payload.value(QStringLiteral("extra")).toMap(),
      {QStringLiteral("source"), QStringLiteral("category"),
       QStringLiteral("phase"), QStringLiteral("outcome"),
       QStringLiteral("code")},
      1, &state);
  if (!extra.isEmpty())
    sanitized.insert(QStringLiteral("extra"), extra);
  return sanitized;
}

QString IncidentSanitizeText(const QString &text) {
  IncidentSanitizeState state;
  return incidentBoundedText(text, &state);
}

QString IncidentSanitizePersistentLogMessage(const QString &message) {
  static const QRegularExpression structuredSensitiveField(
      QStringLiteral("(?i)\\\"(?:params|data|payload|content[_-]?base64|"
                     "signature[_-]?b64|signed[_-]?content|certificate(?:s)?|"
                     "subject|issuer|fingerprint|thumbprint|serial(?:number)?|"
                     "nif|dni|request[_-]?id|trace[_-]?id)\\\"\\s*:"));
  if (structuredSensitiveField.match(message).hasMatch()) {
    return QStringLiteral("[STRUCTURED_DATA_REDACTED] bytes=%1")
        .arg(message.toUtf8().size());
  }
  return IncidentSanitizeText(message);
}

QString IncidentFormatIpcLogEvent(const QVariantMap &message,
                                  const QString &category,
                                  const QString &result,
                                  qint64 wireBytes) {
  QVariantMap event;
  event.insert(QStringLiteral("action"),
               incidentSafeLogToken(
                   message.value(QStringLiteral("action")).toString(),
                   QStringLiteral("unknown")));
  event.insert(QStringLiteral("category"),
               incidentSafeLogToken(category, QStringLiteral("ipc-event")));
  event.insert(QStringLiteral("result"),
               incidentSafeLogToken(result, QStringLiteral("unknown")));
  if (wireBytes >= 0) {
    event.insert(QStringLiteral("wireBytes"),
                 qMin<qint64>(wireBytes, 64 * 1024 * 1024));
  }
  if (message.contains(QStringLiteral("params"))) {
    event.insert(QStringLiteral("paramsBytes"),
                 incidentJsonValueSize(
                     message.value(QStringLiteral("params"))));
  }
  if (message.contains(QStringLiteral("data"))) {
    event.insert(QStringLiteral("dataBytes"),
                 incidentJsonValueSize(
                     message.value(QStringLiteral("data"))));
  }
  QString rawReference =
      message.value(QStringLiteral("requestId")).toString();
  if (rawReference.isEmpty()) {
    rawReference = message.value(QStringLiteral("traceId")).toString();
  }
  const QString reference = incidentOpaqueLogReference(rawReference);
  if (!reference.isEmpty())
    event.insert(QStringLiteral("reference"), reference);
  return QString::fromUtf8(
      QJsonDocument(QJsonObject::fromVariantMap(event))
          .toJson(QJsonDocument::Compact));
}

QString IncidentReadSanitizedTextTail(const QString &path, qint64 maxBytes) {
  QFile file(path);
  if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
    return QString();
  const qint64 limit =
      qBound(qint64{1}, maxBytes, qint64{1024 * 1024});
  const qint64 start = qMax<qint64>(0, file.size() - limit);
  if (!file.seek(start))
    return QString();
  QByteArray data = file.read(limit);
  if (start > 0) {
    const qsizetype newline = data.indexOf('\n');
    if (newline >= 0 && newline + 1 < data.size())
      data = data.mid(newline + 1);
  }
  return IncidentSanitizeText(QString::fromUtf8(data).trimmed());
}

bool IncidentHasExplicitRemoteConsent(const QVariantMap &payload) {
  const QVariantMap preview =
      payload.value(QStringLiteral("supportPreview")).toMap();
  const QVariantMap consent = preview.value(QStringLiteral("consent")).toMap();
  const auto explicitlyTrue = [&consent](const QString &key) {
    const QVariant value = consent.value(key);
    return value.typeId() == QMetaType::Bool && value.toBool();
  };
  return explicitlyTrue(QStringLiteral("required")) &&
         explicitlyTrue(QStringLiteral("previewShown")) &&
         explicitlyTrue(QStringLiteral("remoteSend"));
}

bool IncidentLoadEligibleSavedReport(const QString &path,
                                     const QString &reportsDirectory,
                                     QVariantMap *payload,
                                     QString *errorMessage) {
  QVariantMap rawPayload;
  if (!incidentReadEligibleSavedReport(path, reportsDirectory, &rawPayload,
                                       errorMessage)) {
    return false;
  }
  if (!payload) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("No se indicó dónde cargar la incidencia.");
    return false;
  }
  *payload = IncidentSanitizePayload(rawPayload, false);
  return true;
}

bool IncidentRecordRemoteAttempt(const QString &path,
                                 const QString &reportsDirectory,
                                 bool succeeded,
                                 const QDateTime &attemptedAtUtc,
                                 QString *errorMessage) {
  QVariantMap rawPayload;
  if (!incidentReadEligibleSavedReport(path, reportsDirectory, &rawPayload,
                                       errorMessage)) {
    return false;
  }
  if (!attemptedAtUtc.isValid()) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("La fecha del intento remoto no es válida.");
    return false;
  }

  QVariantMap payload = IncidentSanitizePayload(rawPayload, false);
  QVariantMap attempt;
  attempt.insert(
      QStringLiteral("attemptedAtUtc"),
      attemptedAtUtc.toUTC().toString(Qt::ISODateWithMs));
  attempt.insert(QStringLiteral("result"),
                 succeeded ? QStringLiteral("sent")
                           : QStringLiteral("failed"));
  payload.insert(QStringLiteral("remoteAttempt"), attempt);
  const QByteArray data =
      QJsonDocument(QJsonObject::fromVariantMap(payload))
          .toJson(QJsonDocument::Indented);
  return IncidentWritePrivateFile(path, data, errorMessage);
}

bool IncidentEnsurePrivateDirectory(const QString &path,
                                    QString *errorMessage) {
  const QFileInfo before(path);
  if (before.exists() && (before.isSymLink() || !before.isDir())) {
    if (errorMessage)
      *errorMessage = QStringLiteral("El destino de incidencias no es un "
                                    "directorio privado válido.");
    return false;
  }
  if (!QDir().mkpath(path)) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("No se pudo crear el directorio de incidencias.");
    return false;
  }
  const QFileInfo after(path);
  if (after.isSymLink() || !after.isDir()) {
    if (errorMessage)
      *errorMessage = QStringLiteral("El destino de incidencias cambió "
                                    "durante su preparación.");
    return false;
  }
#ifdef Q_OS_UNIX
  const QFile::Permissions ownerOnly =
      QFileDevice::ReadOwner | QFileDevice::WriteOwner |
      QFileDevice::ExeOwner;
  if (!QFile::setPermissions(path, ownerOnly)) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("No se pudieron restringir los permisos del "
                         "directorio de incidencias.");
    return false;
  }
#endif
  return true;
}

bool IncidentSetPrivateFilePermissions(QFileDevice *file,
                                       QString *errorMessage) {
  if (!file || !file->isOpen()) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("El fichero de incidencia no está abierto.");
    return false;
  }
#ifdef Q_OS_UNIX
  if (!file->setPermissions(QFileDevice::ReadOwner |
                            QFileDevice::WriteOwner)) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("No se pudieron restringir los permisos del "
                         "fichero de incidencia.");
    return false;
  }
#endif
  return true;
}

bool IncidentWritePrivateFile(const QString &path, const QByteArray &data,
                              QString *errorMessage) {
  if (path.trimmed().isEmpty()) {
    if (errorMessage)
      *errorMessage = QStringLiteral("La ruta de incidencia está vacía.");
    return false;
  }
  const QFileInfo target(path);
  if (target.isSymLink() || (target.exists() && !target.isFile())) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("El destino de incidencia no es un fichero regular.");
    return false;
  }
  if (!IncidentEnsurePrivateDirectory(target.absolutePath(), errorMessage))
    return false;

  QSaveFile file(target.absoluteFilePath());
  if (!file.open(QIODevice::WriteOnly) ||
      !IncidentSetPrivateFilePermissions(&file, errorMessage)) {
    file.cancelWriting();
    return false;
  }
  if (file.write(data) != data.size() || !file.commit()) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("No se pudo confirmar el fichero de incidencia.");
    file.cancelWriting();
    return false;
  }
  const QFileInfo written(target.absoluteFilePath());
  if (written.isSymLink() || !written.isFile()) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("El destino de incidencia cambió durante la escritura.");
    return false;
  }
  return true;
}

bool IncidentApplyReportRetention(const QString &reportsDirectory,
                                  const QDateTime &nowUtc, int maxAgeDays,
                                  int maxIncidentGroups, int *removedFiles,
                                  QString *errorMessage) {
  if (removedFiles)
    *removedFiles = 0;
  if (!nowUtc.isValid() || maxAgeDays < 1 || maxIncidentGroups < 1) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("La política de retención no es válida.");
    return false;
  }
  if (!IncidentEnsurePrivateDirectory(reportsDirectory, errorMessage))
    return false;

  static const QRegularExpression retainedFileName(QStringLiteral(
      "^(incident-[A-Za-z0-9_-]+-\\d{8}-\\d{6}-\\d{3}-"
      "[0-9a-fA-F]{8})(?:\\.json|\\.txt|\\.logtail\\.txt|"
      "\\.manifest\\.json|\\.preview\\.json)$"));
  QMap<QString, IncidentRetentionGroup> groups;
  const QFileInfoList entries =
      QDir(reportsDirectory)
          .entryInfoList(QDir::Files | QDir::NoDotAndDotDot, QDir::Name);
  for (const QFileInfo &entry : entries) {
    const QRegularExpressionMatch match =
        retainedFileName.match(entry.fileName());
    if (!match.hasMatch() || entry.isSymLink() || !entry.isFile())
      continue;
    const QString baseName = match.captured(1);
    IncidentRetentionGroup &group = groups[baseName];
    group.baseName = baseName;
    group.files.append(entry.fileName());
    const QDateTime modified = entry.lastModified().toUTC();
    if (!group.newestModificationUtc.isValid() ||
        modified > group.newestModificationUtc) {
      group.newestModificationUtc = modified;
    }
  }

  QList<IncidentRetentionGroup> ordered = groups.values();
  std::sort(ordered.begin(), ordered.end(),
            [](const IncidentRetentionGroup &left,
               const IncidentRetentionGroup &right) {
              if (left.newestModificationUtc ==
                  right.newestModificationUtc) {
                return left.baseName > right.baseName;
              }
              return left.newestModificationUtc >
                     right.newestModificationUtc;
            });
  const QDateTime cutoff = nowUtc.toUTC().addDays(-maxAgeDays);
  for (qsizetype index = 0; index < ordered.size(); ++index) {
    const IncidentRetentionGroup &group = ordered.at(index);
    const bool expired = !group.newestModificationUtc.isValid() ||
                         group.newestModificationUtc < cutoff;
    const bool overCount = index >= maxIncidentGroups;
    if ((expired || overCount) &&
        !incidentRemoveRecognizedFiles(reportsDirectory, group.files,
                                       removedFiles, errorMessage)) {
      return false;
    }
  }
  return true;
}

bool IncidentApplyPrivateLogRetention(const QString &path,
                                      const QDateTime &nowUtc,
                                      int maxAgeDays, int *removedFiles,
                                      QString *errorMessage) {
  if (removedFiles)
    *removedFiles = 0;
  if (path.trimmed().isEmpty() || !nowUtc.isValid() || maxAgeDays < 1) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("La política de retención del log no es válida.");
    return false;
  }
  const QFileInfo target(path);
  if (!IncidentEnsurePrivateDirectory(target.absolutePath(), errorMessage))
    return false;
  const QDateTime cutoff = nowUtc.toUTC().addDays(-maxAgeDays);
  for (const QString &candidate :
       {target.absoluteFilePath(),
        target.absoluteFilePath() + QStringLiteral(".1")}) {
    const QFileInfo info(candidate);
    if (!info.exists())
      continue;
    if (info.isSymLink() || !info.isFile()) {
      if (errorMessage)
        *errorMessage =
            QStringLiteral("El destino del log retenido no es regular.");
      return false;
    }
    if (info.lastModified().toUTC() >= cutoff)
      continue;
    if (!QFile::remove(candidate)) {
      if (errorMessage)
        *errorMessage =
            QStringLiteral("No se pudo retirar un log fuera de retención.");
      return false;
    }
    if (removedFiles)
      ++(*removedFiles);
  }
  return true;
}

bool IncidentOpenPrivateAppendFile(QFile *file, QString *errorMessage) {
  if (!file || file->fileName().trimmed().isEmpty()) {
    if (errorMessage)
      *errorMessage = QStringLiteral("La ruta del log está vacía.");
    return false;
  }
  const QFileInfo before(file->fileName());
  if (before.isSymLink() || (before.exists() && !before.isFile())) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("El destino del log no es un fichero regular.");
    return false;
  }
  if (!IncidentEnsurePrivateDirectory(before.absolutePath(), errorMessage))
    return false;
  const QFileInfo prepared(file->fileName());
  if (prepared.isSymLink() ||
      (prepared.exists() && !prepared.isFile())) {
    if (errorMessage)
      *errorMessage =
          QStringLiteral("El destino del log cambió durante su preparación.");
    return false;
  }
  if (!file->open(QIODevice::WriteOnly | QIODevice::Append |
                  QIODevice::Text)) {
    if (errorMessage)
      *errorMessage = QStringLiteral("No se pudo abrir el log privado.");
    return false;
  }
  const QFileInfo opened(file->fileName());
  if (opened.isSymLink() || !opened.isFile()) {
    file->close();
    if (errorMessage)
      *errorMessage =
          QStringLiteral("El destino del log cambió durante su apertura.");
    return false;
  }
  if (!IncidentSetPrivateFilePermissions(file, errorMessage)) {
    file->close();
    return false;
  }
  return true;
}
