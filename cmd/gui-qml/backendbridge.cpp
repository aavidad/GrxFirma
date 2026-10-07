// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "backendbridge.h"
#include "executablelocator.h"
#include "activediagnostics.h"
#include "incidentprivacy.h"
#include "processenvironment.h"
#include "transientsecret.h"
#include "translatorbridge.h"
#include "webcompatibilitylease.h"
#include <QAccessible>
#include <QClipboard>
#include <QCoreApplication>
#include <QCryptographicHash>
#include <QDateTime>
#include <QDesktopServices>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QGuiApplication>
#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QProcessEnvironment>
#include <QRandomGenerator>
#include <QSaveFile>
#include <QSslCertificate>
#include <QSslConfiguration>
#include <QSslError>
#include <QStandardPaths>
#include <QTimer>
#include <QUrl>
#include <QUrlQuery>
#include <cmath>

void BackendBridge::announceAccessible(QObject *target, const QString &text) {
  if (!target || text.isEmpty()) return;
#if QT_VERSION >= QT_VERSION_CHECK(6, 8, 0)
  QAccessibleAnnouncementEvent event(target, text);
#else
  // Qt anterior a 6.8 obtiene el nombre del aviso mediante su interfaz accesible.
  QAccessibleEvent event(target, QAccessible::Alert);
#endif
  QAccessible::updateAccessibility(&event);
}

static QString bt(const QString &key) {
  if (auto *tr = TranslatorBridge::shared())
    return tr->t(key);
  return key;
}

static constexpr qint64 kBackendMaxSignInputBytes = 60LL * 1024LL * 1024LL;
static constexpr qint64 kBackendMaxSealImageBytes = 10LL * 1024LL * 1024LL;
static constexpr qint64 kBackendMaxHashReferenceBytes = 1LL * 1024LL * 1024LL;
static constexpr qint64 kBackendMaxRESTOutputBytes = 75LL * 1024LL * 1024LL;
static constexpr qsizetype kBackendMaxRESTRequestBytes =
    95LL * 1024LL * 1024LL;

static QStringList BackendBridgeExpectedLocalTLSPins();

static bool backendReadRegularFile(const QString &path, qint64 maxBytes,
                                   QByteArray *data, QString *error) {
  if (!data)
    return false;
  const QFileInfo info(path);
  if (!info.exists() || !info.isFile() || info.isSymLink()) {
    if (error)
      *error = bt(QStringLiteral("La ruta seleccionada no es un fichero regular."));
    return false;
  }
  if (info.size() < 0 || info.size() > maxBytes) {
    if (error)
      *error = bt(QStringLiteral("El fichero supera el tamaño máximo permitido."));
    return false;
  }
  QFile file(info.absoluteFilePath());
  if (!file.open(QIODevice::ReadOnly)) {
    if (error)
      *error = file.errorString();
    return false;
  }
  *data = file.read(maxBytes + 1);
  if (data->size() > maxBytes) {
    data->clear();
    if (error)
      *error = bt(QStringLiteral("El fichero supera el tamaño máximo permitido."));
    return false;
  }
  return true;
}

static QString backendMIMETypeForPath(const QString &path) {
  const QString suffix = QFileInfo(path).suffix().trimmed().toLower();
  if (suffix == QStringLiteral("pdf"))
    return QStringLiteral("application/pdf");
  if (suffix == QStringLiteral("odt"))
    return QStringLiteral("application/vnd.oasis.opendocument.text");
  if (suffix == QStringLiteral("ods"))
    return QStringLiteral("application/vnd.oasis.opendocument.spreadsheet");
  if (suffix == QStringLiteral("odp"))
    return QStringLiteral("application/vnd.oasis.opendocument.presentation");
  if (suffix == QStringLiteral("odg"))
    return QStringLiteral("application/vnd.oasis.opendocument.graphics");
  if (suffix == QStringLiteral("docx"))
    return QStringLiteral(
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document");
  if (suffix == QStringLiteral("xlsx"))
    return QStringLiteral(
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");
  if (suffix == QStringLiteral("pptx"))
    return QStringLiteral(
        "application/vnd.openxmlformats-officedocument.presentationml.presentation");
  if (suffix == QStringLiteral("ppsx"))
    return QStringLiteral(
        "application/vnd.openxmlformats-officedocument.presentationml.slideshow");
  if (suffix == QStringLiteral("dsig") ||
      suffix == QStringLiteral("xmlsig"))
    return QStringLiteral("application/xmldsig+xml");
  if (suffix == QStringLiteral("asics"))
    return QStringLiteral("application/vnd.etsi.asic-s+zip");
  if (suffix == QStringLiteral("xml") || suffix == QStringLiteral("xsig"))
    return QStringLiteral("application/xml");
  if (suffix == QStringLiteral("json"))
    return QStringLiteral("application/json");
  if (suffix == QStringLiteral("p7s") || suffix == QStringLiteral("csig"))
    return QStringLiteral("application/pkcs7-signature");
  if (suffix == QStringLiteral("enveloped") ||
      suffix == QStringLiteral("p7m"))
    return QStringLiteral("application/pkcs7-mime");
  if (suffix == QStringLiteral("afp"))
    return QStringLiteral("application/vnd.grxfirma.protected+json");
  return QStringLiteral("application/octet-stream");
}

static bool backendRESTRequestAllowed(const QByteArray &body, QString *error) {
  if (body.size() <= kBackendMaxRESTRequestBytes)
    return true;
  if (error)
    *error =
        bt(QStringLiteral("La operación supera el tamaño máximo REST permitido."));
  return false;
}

static bool backendDecodeResponseBase64(const QJsonObject &object,
                                        const QString &field,
                                        qint64 maxDecodedBytes,
                                        QByteArray *decoded,
                                        QString *error) {
  if (!decoded)
    return false;
  const QByteArray encoded = object.value(field).toString().toLatin1();
  const qint64 maxEncodedBytes = ((maxDecodedBytes + 2) / 3) * 4;
  if (encoded.isEmpty() || encoded.size() > maxEncodedBytes) {
    if (error)
      *error = bt(QStringLiteral(
          "La respuesta REST no contiene un resultado válido o supera el límite."));
    return false;
  }
  const QByteArray value =
      QByteArray::fromBase64(encoded, QByteArray::AbortOnBase64DecodingErrors);
  if (value.size() > maxDecodedBytes || value.toBase64() != encoded) {
    if (error)
      *error = bt(QStringLiteral(
          "La respuesta REST contiene un resultado Base64 no válido."));
    return false;
  }
  *decoded = value;
  return true;
}

static QString backendProtectionOutputPath(const QString &inputPath,
                                           const QString &requestedPath,
                                           const QString &suggestedName) {
  if (!requestedPath.trimmed().isEmpty())
    return requestedPath;
  const QString safeName = QFileInfo(suggestedName).fileName().trimmed();
  if (!safeName.isEmpty() && safeName != QStringLiteral(".") &&
      safeName != QStringLiteral(".."))
    return QDir(QFileInfo(inputPath).absolutePath()).filePath(safeName);
  return QFileInfo(inputPath).absoluteFilePath() + QStringLiteral(".afp");
}

static QString backendUnprotectionOutputPath(const QString &inputPath,
                                             const QString &requestedPath,
                                             const QString &suggestedName) {
  if (!requestedPath.trimmed().isEmpty())
    return requestedPath;
  const QString safeName = QFileInfo(suggestedName).fileName().trimmed();
  if (!safeName.isEmpty() && safeName != QStringLiteral(".") &&
      safeName != QStringLiteral(".."))
    return QDir(QFileInfo(inputPath).absolutePath()).filePath(safeName);
  QFileInfo input(inputPath);
  QString base = input.absoluteFilePath();
  if (input.suffix().toLower() == QStringLiteral("afp"))
    base.chop(input.suffix().size() + 1);
  else
    base = QDir(input.absolutePath())
               .filePath(input.completeBaseName() +
                         QStringLiteral("_desprotegido"));
  return base;
}

static QString backendHashOutputPath(const QString &inputPath,
                                     const QString &requestedPath,
                                     const QString &format) {
  if (!requestedPath.trimmed().isEmpty())
    return requestedPath;
  QString suffix = QStringLiteral(".hexhash");
  const QString normalized = format.trimmed().toLower();
  if (normalized == QStringLiteral("base64") ||
      normalized == QStringLiteral("b64"))
    suffix = QStringLiteral(".hashb64");
  else if (normalized == QStringLiteral("bin") ||
           normalized == QStringLiteral("binary"))
    suffix = QStringLiteral(".hash");
  const QFileInfo input(inputPath);
  return QDir(input.absolutePath()).filePath(input.fileName() + suffix);
}

static QString backendSignedOutputPath(const QString &inputPath,
                                       const QString &outputDir,
                                       const QString &format) {
  const QFileInfo input(inputPath);
  const QString base = input.completeBaseName();
  const QString normalized = format.trimmed().toLower();
  QString suffix = QStringLiteral("_firmado.p7s");
  if (normalized == QStringLiteral("pades"))
    suffix = QStringLiteral("_firmado.pdf");
  else if (normalized == QStringLiteral("xades"))
    suffix = QStringLiteral("_firmado.xsig");
  else if (normalized == QStringLiteral("xmldsig"))
    suffix = QStringLiteral("_firmado.dsig");
  const QString dir =
      outputDir.trimmed().isEmpty() ? input.absolutePath() : outputDir;
  return QDir(dir).filePath(base + suffix);
}

static QString backendAvailableOutputPath(const QString &requested,
                                          const QString &overwrite) {
  if (overwrite == QStringLiteral("overwrite") ||
      overwrite == QStringLiteral("force") || !QFileInfo::exists(requested))
    return requested;
  const QFileInfo info(requested);
  const QString base = info.completeBaseName();
  const QString suffix =
      info.suffix().isEmpty() ? QString() : QStringLiteral(".") + info.suffix();
  for (int i = 2; i < 10000; ++i) {
    const QString candidate =
        QDir(info.absolutePath()).filePath(base + QStringLiteral("_%1").arg(i) +
                                           suffix);
    if (!QFileInfo::exists(candidate))
      return candidate;
  }
  return QString();
}

static bool backendWritePrivateFile(const QString &requested,
                                    const QString &overwrite,
                                    const QByteArray &data,
                                    QString *actualPath, QString *error) {
  if (requested.trimmed().isEmpty()) {
    if (error)
      *error = bt(QStringLiteral("La ruta de salida no puede estar vacía."));
    return false;
  }
  const QString path =
      backendAvailableOutputPath(QFileInfo(requested).absoluteFilePath(),
                                 overwrite.trimmed().toLower());
  if (path.isEmpty()) {
    if (error)
      *error = bt(QStringLiteral(
          "No se pudo reservar un nombre de salida sin sobrescribir ficheros."));
    return false;
  }
  const QFileInfo existing(path);
  if (existing.exists() && (existing.isSymLink() || !existing.isFile())) {
    if (error)
      *error = bt(QStringLiteral("La ruta de salida no es un fichero regular."));
    return false;
  }
  const QDir parent = QFileInfo(path).absoluteDir();
  if (!parent.exists()) {
    if (error)
      *error = bt(QStringLiteral("El directorio de salida no existe."));
    return false;
  }
  QSaveFile file(path);
  if (!file.open(QIODevice::WriteOnly)) {
    if (error)
      *error = file.errorString();
    return false;
  }
  file.setPermissions(QFileDevice::ReadOwner | QFileDevice::WriteOwner);
  if (file.write(data) != data.size() || !file.commit()) {
    if (error)
      *error = file.errorString();
    file.cancelWriting();
    return false;
  }
  if (actualPath)
    *actualPath = path;
  return true;
}

static bool backendBuildSignOptions(const QVariantMap &options,
                                    bool includeCommon,
                                    QJsonObject *signOptions,
                                    QString *error) {
  if (!signOptions)
    return false;
  if (includeCommon && options.contains(QStringLiteral("extraOptions"))) {
    const QVariantMap extraOptions =
        options.value(QStringLiteral("extraOptions")).toMap();
    for (auto it = extraOptions.constBegin(); it != extraOptions.constEnd();
         ++it) {
      const QString key = it.key().trimmed();
      const QString value = it.value().toString().trimmed();
      if (!key.isEmpty() && !value.isEmpty())
        signOptions->insert(key, value);
    }
  }
  if (includeCommon) {
    if (options.value(QStringLiteral("strictCompat"), false).toBool())
      signOptions->insert(QStringLiteral("strictCompat"),
                          QStringLiteral("true"));
    const QList<QPair<QString, QString>> textual = {
        {QStringLiteral("qrContent"), QStringLiteral("qrContent")},
        {QStringLiteral("reason"), QStringLiteral("reason")},
        {QStringLiteral("location"), QStringLiteral("location")},
        {QStringLiteral("contactInfo"), QStringLiteral("contactInfo")}};
    for (const auto &entry : textual) {
      const QString value = options.value(entry.first).toString().trimmed();
      if (!value.isEmpty())
        signOptions->insert(entry.second, value);
    }
  }

  const QVariantMap sealMap =
      options.value(QStringLiteral("visibleSeal")).toMap();
  if (sealMap.isEmpty())
    return true;
  const double pageWidth = sealMap.value(QStringLiteral("pageWidth")).toDouble();
  const double pageHeight =
      sealMap.value(QStringLiteral("pageHeight")).toDouble();
  if (!std::isfinite(pageWidth) || !std::isfinite(pageHeight) ||
      pageWidth <= 0 || pageHeight <= 0) {
    if (error)
      *error = bt(QStringLiteral(
          "No se pudieron obtener las dimensiones reales de la página PDF."));
    return false;
  }
  const double x =
      qBound(0.0, sealMap.value(QStringLiteral("x"), 0.62).toDouble(), 1.0);
  const double y =
      qBound(0.0, sealMap.value(QStringLiteral("y"), 0.04).toDouble(), 1.0);
  const double w =
      qBound(0.0, sealMap.value(QStringLiteral("w"), 0.34).toDouble(), 1.0 - x);
  const double h =
      qBound(0.0, sealMap.value(QStringLiteral("h"), 0.12).toDouble(), 1.0 - y);
  if (!std::isfinite(x) || !std::isfinite(y) || !std::isfinite(w) ||
      !std::isfinite(h) || w <= 0 || h <= 0) {
    if (error)
      *error =
          bt(QStringLiteral("La posición del sello visible no es válida."));
    return false;
  }
  const QVariant pageValue = sealMap.value(QStringLiteral("page"), 1);
  QString page = pageValue.toString().trimmed();
  if (page.isEmpty())
    page = QStringLiteral("1");
  signOptions->insert(QStringLiteral("visibleSeal"), QStringLiteral("true"));
  signOptions->insert(QStringLiteral("page"), page);
  // Las fracciones se miden sobre la página tal como se ve (CropBox y
  // /Rotate), así que los puntos son relativos a su esquina, como en la ruta
  // IPC; sin esta marca el motor los tomaría como absolutos y el sello se
  // desviaría en páginas cuya CropBox no empieza en 0.
  signOptions->insert(QStringLiteral("visibleSealRectRelativeToCrop"),
                      QStringLiteral("true"));
  signOptions->insert(
      QStringLiteral("visibleSealRectX"),
      QString::number(x * pageWidth, 'f', 2));
  signOptions->insert(
      QStringLiteral("visibleSealRectY"),
      QString::number(y * pageHeight, 'f', 2));
  signOptions->insert(
      QStringLiteral("visibleSealRectW"),
      QString::number(w * pageWidth, 'f', 2));
  signOptions->insert(
      QStringLiteral("visibleSealRectH"),
      QString::number(h * pageHeight, 'f', 2));
  signOptions->insert(
      QStringLiteral("rotation"),
      QString::number(sealMap.value(QStringLiteral("rotation"), 0).toInt()));
  signOptions->insert(
      QStringLiteral("visibleSealKeepText"),
      sealMap.value(QStringLiteral("keepText"), true).toBool()
          ? QStringLiteral("true")
          : QStringLiteral("false"));

  const QString imagePath =
      sealMap.value(QStringLiteral("imagePath")).toString().trimmed();
  if (!imagePath.isEmpty()) {
    QByteArray image;
    if (!backendReadRegularFile(imagePath, kBackendMaxSealImageBytes, &image,
                                error))
      return false;
    signOptions->insert(QStringLiteral("visibleSealImageBase64"),
                        QString::fromLatin1(image.toBase64()));
  }
  return true;
}

static QString generateLocalBearerToken() {
  QString token;
  token.reserve(64);
  auto *random = QRandomGenerator::system();
  for (int i = 0; i < 8; ++i)
    token.append(QStringLiteral("%1").arg(random->generate(), 8, 16,
                                         QLatin1Char('0')));
  return token;
}

static bool openLocalizedHelpFallback() {
  auto *tr = TranslatorBridge::shared();
  if (!tr)
    return false;
  QString cacheDir =
      QStandardPaths::writableLocation(QStandardPaths::CacheLocation);
  if (cacheDir.isEmpty())
    cacheDir = QDir::tempPath();
  QDir().mkpath(cacheDir);
  const QString path = QDir(cacheDir).filePath("grxfirma-help-" + tr->locale() + ".html");
  QFile file(path);
  if (!file.open(QIODevice::WriteOnly | QIODevice::Truncate | QIODevice::Text))
    return false;
  file.write(tr->helpHtml().toUtf8());
  file.close();
  return QDesktopServices::openUrl(QUrl::fromLocalFile(path));
}

static QString backendBridgeStateDir() {
  QString stateDir =
      QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation);
  if (stateDir.isEmpty()) {
    QString home = QDir::homePath();
    stateDir =
        home.isEmpty() ? QDir::tempPath() : home + "/.local/state/grxfirma";
  }
  QDir dir(stateDir);
  dir.mkpath(".");
  return dir.absolutePath();
}

static QString sanitizeIncidentKind(QString kind) {
  kind = kind.trimmed().toLower();
  if (kind.isEmpty())
    kind = QStringLiteral("general");
  QString out;
  out.reserve(kind.size());
  for (const QChar ch : kind) {
    if (ch.isLetterOrNumber())
      out.append(ch);
    else if (ch == QChar('-') || ch == QChar('_'))
      out.append(ch);
  }
  if (out.isEmpty())
    out = QStringLiteral("general");
  return out;
}

static QString backendIncidentReportsDirPath() {
  QDir dir(backendBridgeStateDir());
  const QString path = dir.filePath(QStringLiteral("incidents"));
  if (!IncidentEnsurePrivateDirectory(path))
    return QString();
  return IncidentApplyReportRetention(
             path, QDateTime::currentDateTimeUtc(), 30, 50)
             ? path
             : QString();
}

static QString backendDefaultIncidentReportPath(const QString &kind) {
  const QString reportsDir = backendIncidentReportsDirPath();
  if (reportsDir.isEmpty())
    return QString();
  const QString normalizedKind = sanitizeIncidentKind(kind);
  const QString stamp =
      QDateTime::currentDateTime().toString("yyyyMMdd-HHmmss-zzz");
  const QString nonce = QStringLiteral("%1").arg(
      QRandomGenerator::system()->generate(), 8, 16, QLatin1Char('0'));
  return QDir(reportsDir)
      .filePath(QStringLiteral("incident-%1-%2-%3.json")
                    .arg(normalizedKind, stamp, nonce));
}

static QString backendIncidentTextPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".txt");
}

static QString backendIncidentLogTailPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".logtail.txt");
}

static QString backendIncidentManifestPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".manifest.json");
}

static QString backendIncidentPreviewPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".preview.json");
}

static QString backendGuiLogPath() {
  QString stateDir =
      QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation);
  if (stateDir.isEmpty()) {
    const QString home = QDir::homePath();
    stateDir =
        home.isEmpty() ? QDir::tempPath() : home + "/.local/state/grxfirma";
  }
  return QDir(stateDir).filePath(QStringLiteral("logs/gui-qml.log"));
}

static QString backendRedactSupportText(QString text) {
  return IncidentSanitizeText(text);
}

static QString backendReadLogTailRedacted(const QString &path, qint64 maxBytes = 65536) {
  return IncidentReadSanitizedTextTail(path, maxBytes);
}

static QVariantMap backendBuildIncidentPreview(const QVariantMap &payload) {
  const QVariantMap providedPreview =
      payload.value(QStringLiteral("supportPreview")).toMap();
  const QVariantMap providedConsent =
      providedPreview.value(QStringLiteral("consent")).toMap();

  QVariantMap consent;
  consent.insert(QStringLiteral("required"), true);
  consent.insert(QStringLiteral("previewShown"),
                 providedConsent.value(QStringLiteral("previewShown"), false));
  consent.insert(QStringLiteral("remoteSend"), false);
  if (providedConsent.contains(QStringLiteral("required")))
    consent.insert(QStringLiteral("required"),
                   providedConsent.value(QStringLiteral("required")));
  if (providedConsent.contains(QStringLiteral("remoteSend")))
    consent.insert(QStringLiteral("remoteSend"),
                   providedConsent.value(QStringLiteral("remoteSend")));

  QVariantMap preview;
  preview.insert(QStringLiteral("schema"),
                 QStringLiteral("grxfirma-support-preview-v1"));
  preview.insert(QStringLiteral("generatedAt"),
                 payload.value(QStringLiteral("generatedAt")));
  preview.insert(QStringLiteral("kind"),
                 payload.value(QStringLiteral("kind")));
  preview.insert(QStringLiteral("title"),
                 providedPreview.value(
                     QStringLiteral("title"),
                     bt(QStringLiteral("Paquete de soporte listo para exportar"))));
  preview.insert(QStringLiteral("summary"),
                 providedPreview.value(
                     QStringLiteral("summary"),
                     bt(QStringLiteral("Se incluirá un resumen saneado de la incidencia y un extracto técnico mínimo."))));
  preview.insert(QStringLiteral("includedData"),
                 providedPreview.value(
                     QStringLiteral("includedData"),
                     QVariantList{
                         bt(QStringLiteral("Resumen de la operación")),
                         bt(QStringLiteral("Versión y plataforma")),
                         bt(QStringLiteral("Metadatos del certificado seleccionado")),
                         bt(QStringLiteral("Nombres de fichero sin ruta completa")),
                         bt(QStringLiteral("Cola de log redactada"))}));
  preview.insert(QStringLiteral("omittedData"),
                 providedPreview.value(
                     QStringLiteral("omittedData"),
                     QVariantList{
                         bt(QStringLiteral("Documentos originales")),
                         bt(QStringLiteral("Rutas completas del perfil de usuario")),
                         bt(QStringLiteral("Claves privadas")),
                         bt(QStringLiteral("Certificados DER/PEM completos")),
                         bt(QStringLiteral("Secretos, tokens y cabeceras sensibles"))}));
  preview.insert(QStringLiteral("consent"), consent);
  return preview;
}

static QVariantMap backendBuildIncidentUploadEnvelope(const QVariantMap &payload) {
  const QVariantMap safePayload = IncidentSanitizePayload(payload, true);
  QVariantMap manifest;
  manifest.insert(QStringLiteral("schema"),
                  QStringLiteral("grxfirma-incident-bundle-v1"));
  manifest.insert(QStringLiteral("generatedAt"),
                  safePayload.value(QStringLiteral("generatedAt")));
  manifest.insert(QStringLiteral("kind"),
                  safePayload.value(QStringLiteral("kind")));
  manifest.insert(QStringLiteral("bundleArtifacts"),
                  QVariantList{QStringLiteral("incident.json"),
                               QStringLiteral("incident.preview.json"),
                               QStringLiteral("incident.manifest.json"),
                               QStringLiteral("incident.logtail.txt")});
  manifest.insert(QStringLiteral("redactions"),
                  QVariantList{QStringLiteral("home-profile-prefix"),
                               QStringLiteral("userprofile-prefix"),
                               QStringLiteral("log-tail-truncated")});
  manifest.insert(QStringLiteral("containsSupportSummary"), true);
  manifest.insert(QStringLiteral("containsLogTail"), true);
  manifest.insert(QStringLiteral("containsPreview"), true);

  QVariantMap client;
  const QVariantMap env =
      safePayload.value(QStringLiteral("environment")).toMap();
  client.insert(QStringLiteral("component"), QStringLiteral("gui-qml"));
  client.insert(QStringLiteral("transport"), QStringLiteral("rest"));
  client.insert(QStringLiteral("appVersion"),
                env.value(QStringLiteral("appVersion")));
  client.insert(QStringLiteral("platform"),
                env.value(QStringLiteral("platform")));
  client.insert(QStringLiteral("language"),
                env.value(QStringLiteral("language")));

  QVariantMap envelope;
  envelope.insert(QStringLiteral("schema"),
                  QStringLiteral("grxfirma-incident-upload-v1"));
  envelope.insert(QStringLiteral("sentAt"),
                  QDateTime::currentDateTimeUtc().toString(Qt::ISODate));
  envelope.insert(QStringLiteral("client"), client);
  envelope.insert(QStringLiteral("incident"), safePayload);
  envelope.insert(QStringLiteral("preview"),
                  backendBuildIncidentPreview(safePayload));
  envelope.insert(QStringLiteral("manifest"), manifest);
  envelope.insert(QStringLiteral("logTail"),
                  backendReadLogTailRedacted(backendGuiLogPath()));
  return envelope;
}

static QStringList helpManualCandidates(const QString &appDir,
                                        const QString &locale) {
  QString normalized = locale.trimmed().toLower();
  normalized.replace('_', '-');
  const QString baseLang = normalized.section('-', 0, 0);

  QStringList baseDirs;
  baseDirs << appDir;
  baseDirs << QDir(appDir).filePath("help");
  baseDirs << QDir(appDir).filePath("../lib/grxfirma/gui-qml");
  baseDirs << QDir(appDir).filePath("../lib/grxfirma/gui-qml/help");

  QStringList names;
  if (!normalized.isEmpty())
    names << QStringLiteral("ayuda-") + normalized + QStringLiteral(".pdf");
  if (!baseLang.isEmpty() && baseLang != normalized)
    names << QStringLiteral("ayuda-") + baseLang + QStringLiteral(".pdf");
  names << QStringLiteral("ayuda.pdf");

  QStringList out;
  for (const QString &dir : baseDirs) {
    for (const QString &name : names)
      out << QDir(dir).filePath(name);
    if (!normalized.isEmpty())
      out << QDir(dir).filePath(normalized + "/ayuda.pdf");
    if (!baseLang.isEmpty() && baseLang != normalized)
      out << QDir(dir).filePath(baseLang + "/ayuda.pdf");
  }
  out.removeDuplicates();
  return out;
}

static QString resolveLocalizedHelpManual(const QString &appDir,
                                          const QString &locale) {
  for (const QString &candidate : helpManualCandidates(appDir, locale)) {
    if (QFile::exists(candidate))
      return candidate;
  }
  return QString();
}

BackendBridge::BackendBridge(QObject *parent) : QObject(parent) {
  m_nam = new QNetworkAccessManager(this);
  m_activeDiagnostics = new ActiveDiagnosticsRunner(this);
  m_webCompatibilityLease =
      new WebCompatibilityLease([this]() { stopBackend(); }, this);
  m_status = bt(QStringLiteral("Iniciando..."));
  connect(m_activeDiagnostics, &ActiveDiagnosticsRunner::runningChanged, this,
          [this](bool running) {
            if (m_activeDiagnosticsRunning == running)
              return;
            m_activeDiagnosticsRunning = running;
            emit activeDiagnosticsRunningChanged();
          });
  connect(m_activeDiagnostics, &ActiveDiagnosticsRunner::finished, this,
          &BackendBridge::activeDiagnosticsFinished);
  connect(m_webCompatibilityLease, &WebCompatibilityLease::activeChanged, this,
          &BackendBridge::webCompatibilityActiveChanged);
  connect(m_webCompatibilityLease, &WebCompatibilityLease::expired, this,
          [this]() {
            const QString message = bt(QStringLiteral(
                "Compatibilidad web desactivada automáticamente al caducar."));
            setStatus(message);
            emit backendLogReceived(message);
            emit webCompatibilityStateChanged(false, 0, message);
          });
}

BackendBridge::~BackendBridge() { shutdownForExit(); }

void BackendBridge::runActiveDiagnostics(bool explicitConsent,
                                         const QVariantMap &context) {
  const bool channelAvailable =
      m_process && m_process->state() != QProcess::NotRunning;
  m_activeDiagnostics->start(m_addr, m_useTLS,
                             BackendBridgeExpectedLocalTLSPins(),
                             channelAvailable, context, explicitConsent);
}

void BackendBridge::cancelActiveDiagnostics() { m_activeDiagnostics->cancel(); }

static QUrl BackendBridgeMakeUrl(const QString &addr, const QString &path,
                                 bool useTLS = true) {
  QString protocol = useTLS ? "https://" : "http://";
  return QUrl(protocol + addr + path);
}

static QString BackendBridgePortFromAddr(const QString &addr) {
  int sep = addr.lastIndexOf(':');
  if (sep == -1 || sep == addr.size() - 1)
    return QStringLiteral("63118");
  return addr.mid(sep + 1);
}

static QString BackendBridgeNormalizeRestAddr(const QString &addr) {
  if (addr.startsWith("127.0.0.1:") || addr.startsWith("localhost:"))
    return addr;
  return QStringLiteral("127.0.0.1:") + BackendBridgePortFromAddr(addr);
}

static bool BackendBridgeIsLoopback(const QString &addr) {
  return addr.startsWith("127.0.0.1:") || addr.startsWith("localhost:");
}

static bool BackendBridgeIsValidLoopbackEndpoint(const QString &addr) {
  if (!BackendBridgeIsLoopback(addr) || addr.count(QLatin1Char(':')) != 1)
    return false;
  bool ok = false;
  const int port = BackendBridgePortFromAddr(addr).toInt(&ok);
  return ok && port >= 1024 && port <= 65535;
}

static QNetworkRequest BackendBridgeMakeReq(const QString &addr,
                                            const QString &token,
                                            const QString &path,
                                            bool useTLS);

static QString BackendBridgeLocalTLSCertPath() {
  const QString home = QDir::homePath();
  if (!home.isEmpty()) {
    return QDir(home).filePath(
        QStringLiteral(".config/grxfirma/tls/websocket-localhost.crt.pem"));
  }
  return QDir(QDir::tempPath())
      .filePath(QStringLiteral("grxfirma/tls/websocket-localhost.crt.pem"));
}

static QString BackendBridgeCertificatePin(const QSslCertificate &cert) {
  if (cert.isNull())
    return QString();
  return QString::fromLatin1(
             cert.digest(QCryptographicHash::Sha256).toHex())
      .toLower();
}

static QStringList BackendBridgeExpectedLocalTLSPins() {
  QFile file(BackendBridgeLocalTLSCertPath());
  if (!file.open(QIODevice::ReadOnly))
    return QStringList();

  QList<QSslCertificate> certs =
      QSslCertificate::fromDevice(&file, QSsl::Pem);
  if (certs.isEmpty()) {
    file.seek(0);
    certs = QSslCertificate::fromDevice(&file, QSsl::Der);
  }

  QStringList pins;
  for (const QSslCertificate &cert : certs) {
    const QString pin = BackendBridgeCertificatePin(cert);
    if (!pin.isEmpty())
      pins << pin;
  }
  pins.removeDuplicates();
  return pins;
}

static QSslCertificate
BackendBridgePeerCertificate(QNetworkReply *reply,
                             const QList<QSslError> &errors) {
  if (reply) {
    const QSslCertificate peer =
        reply->sslConfiguration().peerCertificate();
    if (!peer.isNull())
      return peer;
  }
  for (const QSslError &error : errors) {
    const QSslCertificate cert = error.certificate();
    if (!cert.isNull())
      return cert;
  }
  return QSslCertificate();
}

static bool
BackendBridgePeerMatchesPinnedLocalTLSCert(QNetworkReply *reply,
                                           const QList<QSslError> &errors) {
  const QString peerPin = BackendBridgeCertificatePin(
      BackendBridgePeerCertificate(reply, errors));
  if (peerPin.isEmpty())
    return false;

  const QStringList expectedPins = BackendBridgeExpectedLocalTLSPins();
  for (const QString &expectedPin : expectedPins) {
    if (peerPin == expectedPin)
      return true;
  }
  return false;
}

static void BackendBridgeAllowLocalTLSErrors(QNetworkReply *reply,
                                             const QString &addr,
                                             bool useTLS) {
  if (!reply || !useTLS || !BackendBridgeIsLoopback(addr))
    return;
  QObject::connect(reply, &QNetworkReply::sslErrors, reply,
                   [reply](const QList<QSslError> &errors) {
                     if (BackendBridgePeerMatchesPinnedLocalTLSCert(reply, errors))
                       reply->ignoreSslErrors(errors);
                   });
}

bool BackendBridge::canStopOwnedBackend() const {
  return m_process && m_process->state() != QProcess::NotRunning;
}

bool BackendBridge::webCompatibilityActive() const {
  return m_webCompatibilityLease && m_webCompatibilityLease->active();
}

bool BackendBridge::startTemporaryWebCompatibility(
    const QString &addr, const QString &token, const QString &fingerprints,
    bool useTLS, int durationMinutes) {
  if (!BackendBridgeIsValidLoopbackEndpoint(addr) || durationMinutes < 5 ||
      durationMinutes > 240) {
    stopWebCompatibility();
    const QString message = bt(QStringLiteral(
        "No se activó la compatibilidad web: duración u origen local no válido."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
    return false;
  }

  if (webCompatibilityActive() && canStopOwnedBackend()) {
    if (!m_webCompatibilityLease->startMilliseconds(durationMinutes * 60 *
                                                      1000)) {
      stopWebCompatibility();
      return false;
    }
    const QString renewed =
        bt(QStringLiteral("Compatibilidad web activa temporalmente durante %1 minutos."))
            .arg(durationMinutes);
    setStatus(renewed);
    emit webCompatibilityStateChanged(true, durationMinutes, renewed);
    return true;
  }
  if (canStopOwnedBackend()) {
    // En el modo REST de compatibilidad el proceso existente es también el
    // backend funcional de la GUI. No convertirlo silenciosamente en una
    // lease cuya caducidad dejaría la aplicación sin motor.
    const QString message = bt(QStringLiteral(
        "No se activó la compatibilidad web temporal: detenga antes el backend REST existente."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
    return false;
  }

  m_nextRESTLifetimeMinutes = durationMinutes;
  startBackend(addr, token, QStringLiteral("rest"), fingerprints, useTLS);
  m_nextRESTLifetimeMinutes = 0;
  if (!canStopOwnedBackend() ||
      !m_webCompatibilityLease->startMilliseconds(durationMinutes * 60 * 1000)) {
    stopBackend();
    const QString message = bt(QStringLiteral(
        "No se activó la compatibilidad web: el servidor local no quedó bajo control de esta aplicación."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
    return false;
  }

  const QString message =
      bt(QStringLiteral("Compatibilidad web activa temporalmente durante %1 minutos."))
          .arg(durationMinutes);
  setStatus(message);
  emit backendLogReceived(message);
  emit webCompatibilityStateChanged(true, durationMinutes, message);
  return true;
}

void BackendBridge::stopWebCompatibility() {
  const bool wasActive = webCompatibilityActive();
  if (m_webCompatibilityLease)
    m_webCompatibilityLease->cancel();
  if (wasActive)
    stopBackend();
  if (wasActive) {
    const QString message =
        bt(QStringLiteral("Compatibilidad web desactivada."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
  }
}

void BackendBridge::setExpertMode(bool v) {
  if (m_expertMode != v) {
    m_expertMode = v;
    emit expertModeChanged();
  }
}

void BackendBridge::setStatus(const QString &s) {
  if (m_status != s) {
    m_status = s;
    emit statusChanged();
  }
}

void BackendBridge::startBackend(const QString &addr, const QString &token,
                                 const QString &mode,
                                 const QString &fingerprints, bool useTLS) {
  Q_UNUSED(mode);
  Q_UNUSED(useTLS);
  m_addr = BackendBridgeNormalizeRestAddr(addr);
  m_token = token.trimmed();
  if (m_token.isEmpty() && fingerprints.trimmed().isEmpty())
    m_token = generateLocalBearerToken();
  m_useTLS = true;

  if (m_process && m_process->state() != QProcess::NotRunning)
    return;

  const QString desktopBin = ExecutableLocator::bundledExecutable(
      QCoreApplication::applicationDirPath(),
      {QStringLiteral("grxfirma")});
  if (desktopBin.isEmpty()) {
    emit backendLogReceived(bt(QStringLiteral(
        "❌ No se encontró el motor local empaquetado para iniciar REST.")));
    setStatus(bt(QStringLiteral("Error al arrancar el backend REST")));
    return;
  }

  m_process = new QProcess(this);
  m_process->setProgram(desktopBin);
  m_process->setProcessEnvironment(
      ChildProcessEnvironment::forRestToken(m_token));

  QStringList args;
  args << "--rest" << "--rest-addr" << m_addr;
  if (m_nextRESTLifetimeMinutes >= 5 && m_nextRESTLifetimeMinutes <= 240)
    args << "--rest-lifetime"
         << QStringLiteral("%1m").arg(m_nextRESTLifetimeMinutes);
  if (!fingerprints.isEmpty())
    args << "--rest-cert-fingerprints" << fingerprints;

  m_process->setArguments(args);
  connect(m_process, &QProcess::readyReadStandardOutput, this,
          &BackendBridge::onBackendReadyRead);
  connect(m_process, &QProcess::readyReadStandardError, this,
          &BackendBridge::onBackendReadyRead);
  QProcess *const launchedProcess = m_process;
  connect(m_process, qOverload<int, QProcess::ExitStatus>(&QProcess::finished),
          this, [this, launchedProcess](int, QProcess::ExitStatus) {
            if (m_process != launchedProcess || !webCompatibilityActive())
              return;
            m_webCompatibilityLease->cancel();
            const QString message = bt(QStringLiteral(
                "Compatibilidad web desactivada: el servidor local terminó inesperadamente."));
            setStatus(message);
            emit webCompatibilityStateChanged(false, 0, message);
          });

  m_process->start();
  if (!m_process->waitForStarted(3000)) {
    setStatus(bt(QStringLiteral("Error al iniciar el backend")));
  } else {
    setStatus(bt(QStringLiteral("Backend activo en ")) + m_addr);
    QTimer::singleShot(700, this, [this]() { refreshCertificates(); });
  }
}

void BackendBridge::stopBackend() {
  if (m_shuttingDown)
    return;
  const bool wasWebCompatibilityActive = webCompatibilityActive();
  if (m_webCompatibilityLease)
    m_webCompatibilityLease->cancel();
  if (m_process) {
    m_process->blockSignals(true);
    disconnect(m_process, nullptr, this, nullptr);
    m_process->terminate();
    if (!m_process->waitForFinished(2000)) {
      m_process->kill();
      m_process->waitForFinished(1000);
    }
    delete m_process;
    m_process = nullptr;
    setStatus(bt(QStringLiteral("Backend detenido")));
  }
  if (wasWebCompatibilityActive) {
    emit webCompatibilityStateChanged(
        false, 0, bt(QStringLiteral("Compatibilidad web desactivada.")));
  }
}

void BackendBridge::shutdownForExit() {
  if (m_shuttingDown)
    return;
  m_shuttingDown = true;
  if (m_webCompatibilityLease)
    m_webCompatibilityLease->cancel();
  if (m_process) {
    m_process->blockSignals(true);
    disconnect(m_process, nullptr, this, nullptr);
    if (m_process->state() != QProcess::NotRunning) {
      m_process->terminate();
      if (!m_process->waitForFinished(1500)) {
        m_process->kill();
        m_process->waitForFinished(1500);
      }
    }
    delete m_process;
    m_process = nullptr;
  }
}

void BackendBridge::onBackendReadyRead() {
  if (!m_process)
    return;
  QString out = QString::fromUtf8(m_process->readAllStandardOutput());
  QString err = QString::fromUtf8(m_process->readAllStandardError());
  if (!out.isEmpty())
    emit backendLogReceived(out);
  if (!err.isEmpty())
    emit backendLogReceived(err);
}

void BackendBridge::openExternal(const QString &path) {
  if (path.isEmpty())
    return;
  if (path.startsWith("http://") || path.startsWith("https://")) {
    if (!QDesktopServices::openUrl(QUrl(path))) {
      emit backendLogReceived(bt(QStringLiteral("❌ No se pudo abrir la URL: ")) +
                              path);
      setStatus(bt(QStringLiteral("No se pudo abrir el recurso solicitado")));
    }
    return;
  }

  QString localPath = path;
  if (localPath.startsWith("file://")) {
    localPath = QUrl(path).toLocalFile();
  }
  QFileInfo info(localPath);
  if (!info.exists()) {
    emit backendLogReceived(bt(QStringLiteral("❌ El fichero no existe: ")) +
                            localPath);
    setStatus(bt(QStringLiteral("El fichero indicado no existe")));
    return;
  }

  const QString suffix = info.suffix().trimmed().toLower();
  const bool preferOpenDir =
      suffix == QStringLiteral("enveloped") || suffix == QStringLiteral("afp");
  if (preferOpenDir) {
    const QString dirPath = info.absolutePath();
    if (!dirPath.isEmpty() &&
        QDesktopServices::openUrl(QUrl::fromLocalFile(dirPath))) {
      return;
    }
  }

  if (QDesktopServices::openUrl(QUrl::fromLocalFile(localPath)))
    return;

#ifdef Q_OS_WIN
  if (QProcess::startDetached("explorer.exe",
                              {QDir::toNativeSeparators(localPath)}))
    return;
#elif defined(Q_OS_MACOS)
  if (QProcess::startDetached("open", {localPath}))
    return;
#else
  if (preferOpenDir) {
    const QString dirPath = info.absolutePath();
    if (!dirPath.isEmpty() && QProcess::startDetached("xdg-open", {dirPath}))
      return;
  }
  if (QProcess::startDetached("xdg-open", {localPath}))
    return;
#endif

  emit backendLogReceived(bt(QStringLiteral("❌ No se pudo abrir el fichero: ")) +
                          localPath);
  setStatus(bt(QStringLiteral("No se pudo abrir el fichero firmado")));
}

void BackendBridge::openSignedDocument(const QString &path) {
  const QString localPath = path.startsWith(QStringLiteral("file://"))
                                ? QUrl(path).toLocalFile()
                                : path;
  const QFileInfo info(localPath);
  if (!info.isFile()) {
    setStatus(bt(QStringLiteral("El fichero indicado no existe")));
    return;
  }
  if (QDesktopServices::openUrl(QUrl::fromLocalFile(info.absoluteFilePath())))
    return;
#ifdef Q_OS_WIN
  if (QProcess::startDetached(QStringLiteral("explorer.exe"),
                              {QDir::toNativeSeparators(info.absoluteFilePath())}))
    return;
#elif defined(Q_OS_MACOS)
  if (QProcess::startDetached(QStringLiteral("open"),
                              {info.absoluteFilePath()}))
    return;
#else
  if (QProcess::startDetached(QStringLiteral("xdg-open"),
                              {info.absoluteFilePath()}))
    return;
#endif
  setStatus(bt(QStringLiteral("No se pudo abrir el fichero firmado")));
}

void BackendBridge::verifyFile(const QString &inputPath) {
  verifyFileWithOriginal(inputPath, QString());
}

void BackendBridge::verifyFileWithOriginal(const QString &inputPath,
                                           const QString &originalPath) {
  emit backendLogReceived(bt(QStringLiteral("⚙ Verificando firma de: ")) +
                          inputPath);
  QByteArray signedContent;
  QString error;
  if (!backendReadRegularFile(inputPath, kBackendMaxSignInputBytes,
                              &signedContent, &error)) {
    setStatus(error);
    emit verificationFinished(false, error, QVariantMap());
    return;
  }

  QJsonObject body;
  body.insert(QStringLiteral("name"), QFileInfo(inputPath).fileName());
  body.insert(QStringLiteral("mime_type"),
              backendMIMETypeForPath(inputPath));
  body.insert(QStringLiteral("content_base64"),
              QString::fromLatin1(signedContent.toBase64()));
  if (!originalPath.trimmed().isEmpty()) {
    QByteArray originalContent;
    if (!backendReadRegularFile(originalPath, kBackendMaxSignInputBytes,
                                &originalContent, &error)) {
      setStatus(error);
      emit verificationFinished(false, error, QVariantMap());
      return;
    }
    body.insert(QStringLiteral("original_content_base64"),
                QString::fromLatin1(originalContent.toBase64()));
  }

  const QByteArray jsonData =
      QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (!backendRESTRequestAllowed(jsonData, &error)) {
    setStatus(error);
    emit verificationFinished(false, error, QVariantMap());
    return;
  }
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/verify", m_useTLS);
  QNetworkReply *reply = m_nam->post(req, jsonData);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    bool success = (reply->error() == QNetworkReply::NoError);
    QByteArray dataRaw = reply->readAll();
    QString data = QString::fromUtf8(dataRaw);

    QVariantMap details;
    QString msg = "";

    if (success) {
      QJsonDocument doc = QJsonDocument::fromJson(dataRaw);
      QJsonObject res = doc.object();
      if (res.value("ok").toBool()) {
        QJsonObject result = res.contains("result") && res.value("result").isObject()
                                 ? res.value("result").toObject()
                                 : res;
        details = result.toVariantMap();
        bool valid = result.value("valid").toBool();
        msg = valid ? bt(QStringLiteral("Firma válida"))
                    : bt(QStringLiteral("Firma no válida: ")) +
						  bt(result.value("reason").toString());
      } else {
        success = false;
        msg = res.value("error").toString();
      }
    } else {
      msg = bt(QStringLiteral("Error de red en verificación: ")) +
            reply->errorString();
    }

    emit backendLogReceived(bt(QStringLiteral("🔍 Respuesta de verificación: ")) +
                            msg);
    setStatus(msg);
    emit verificationFinished(success, msg, details);
    reply->deleteLater();
  });
}

void BackendBridge::loadProtectionRecipients() {
  emit backendLogReceived(bt(QStringLiteral("🛡️ Cargando destinatarios de protección...")));
  auto req =
      BackendBridgeMakeReq(m_addr, m_token, "/protection/recipients", m_useTLS);
  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() != QNetworkReply::NoError) {
      const QString msg = bt(QStringLiteral("Error cargando destinatarios de protección: ")) +
                          reply->errorString();
      setStatus(msg);
      emit protectionRecipientsLoaded(QVariantList());
      reply->deleteLater();
      return;
    }
    const QJsonObject obj = QJsonDocument::fromJson(reply->readAll()).object();
    QVariantList list;
    const QJsonArray arr = obj.value("recipients").toArray();
    for (const auto &v : arr)
      list << v.toVariant();
    emit protectionRecipientsLoaded(list);
    setStatus(bt(QStringLiteral("Destinatarios de protección cargados")));
    reply->deleteLater();
  });
}

void BackendBridge::importProtectionRecipient(const QString &) {
  emit protectionRecipientChanged(false, tr("Esta función requiere el motor IPC local."));
}

void BackendBridge::removeProtectionRecipient(const QString &) {
  emit protectionRecipientChanged(false, tr("Esta función requiere el motor IPC local."));
}

void BackendBridge::requestSmartcardStatus() {
  emit smartcardStatusReceived(false, QVariantList(),
                               tr("Esta función requiere el motor IPC local."));
}

void BackendBridge::getSealPreview(const QVariantMap &, const QString &requestId) {
  emit sealPreviewReceived(requestId, false, QString(),
                           tr("Esta función requiere el motor IPC local."));
}

void BackendBridge::protectFileAdvanced(const QString &inputPath,
                                        const QString &outputPath,
                                        const QVariantList &recipientIds,
                                        int certIndex,
                                        const QVariantMap &options) {
  const bool signToo = options.value("signToo", false).toBool();
  const QString path = signToo ? QStringLiteral("/protect-sign")
                               : QStringLiteral("/protect");
  QByteArray content;
  QString error;
  if (!backendReadRegularFile(inputPath, kBackendMaxSignInputBytes, &content,
                              &error)) {
    setStatus(error);
    emit protectionFinished(false, error, QVariantMap());
    return;
  }
  auto req = BackendBridgeMakeReq(m_addr, m_token, path, m_useTLS);
  QJsonObject body;
  body.insert(QStringLiteral("name"), QFileInfo(inputPath).fileName());
  body.insert(QStringLiteral("mime_type"),
              backendMIMETypeForPath(inputPath));
  body.insert(QStringLiteral("content_base64"),
              QString::fromLatin1(content.toBase64()));
  body.insert("profile",
              options.value("profile", QStringLiteral("compat")).toString());
  if (signToo)
    body.insert("certificateIndex", certIndex);
  body.insert("recipient_ids", QJsonArray::fromVariantList(recipientIds));
  body.insert("saveToDisk", false);
  body.insert("returnProtectedB64", true);
  const QString overwrite =
      options.value("overwrite", QStringLiteral("rename")).toString().trimmed();
  const bool saveToDisk = options.value("saveToDisk", true).toBool();
  const bool returnB64 =
      options.value("returnProtectedB64", false).toBool();
  QVariantMap optionMap = options.value("options").toMap();
  QString secretB64 =
      optionMap.take(QStringLiteral("secret_b64")).toString().trimmed();
  if (!secretB64.isEmpty())
    body.insert(QStringLiteral("secret_b64"), secretB64);
  if (!optionMap.isEmpty())
    body.insert("options", QJsonObject::fromVariantMap(optionMap));
  QByteArray jsonData =
      QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (!backendRESTRequestAllowed(jsonData, &error)) {
    body.insert(QStringLiteral("secret_b64"), QString());
    TransientSecret::zeroize(secretB64);
    TransientSecret::zeroize(jsonData);
    setStatus(error);
    emit protectionFinished(false, error, QVariantMap());
    return;
  }
  QNetworkReply *reply = m_nam->post(req, jsonData);
  body.insert(QStringLiteral("secret_b64"), QString());
  TransientSecret::zeroize(secretB64);
  TransientSecret::zeroize(jsonData);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, signToo, inputPath, outputPath, overwrite,
           saveToDisk, returnB64]() {
    const QByteArray raw = reply->readAll();
    if (reply->error() != QNetworkReply::NoError) {
      const QString msg = reply->errorString();
      setStatus(msg);
      emit protectionFinished(false, msg, QVariantMap());
      reply->deleteLater();
      return;
    }
    const QJsonObject obj = QJsonDocument::fromJson(raw).object();
    if (!obj.value("ok").toBool()) {
      const QString msg = obj.value("error").toString();
      setStatus(msg);
      emit protectionFinished(false, msg, QVariantMap());
      reply->deleteLater();
      return;
    }
    QByteArray protectedContent;
    QString error;
    if (!backendDecodeResponseBase64(
            obj, QStringLiteral("protected_content_base64"),
            kBackendMaxRESTOutputBytes, &protectedContent, &error)) {
      setStatus(error);
      emit protectionFinished(false, error, QVariantMap());
      reply->deleteLater();
      return;
    }
    QJsonObject result = obj;
    if (saveToDisk) {
      const QString requested = backendProtectionOutputPath(
          inputPath, outputPath, obj.value("documentName").toString());
      QString actualPath;
      if (!backendWritePrivateFile(requested, overwrite, protectedContent,
                                   &actualPath, &error)) {
        setStatus(error);
        emit protectionFinished(false, error, QVariantMap());
        reply->deleteLater();
        return;
      }
      result.insert(QStringLiteral("outputPath"), actualPath);
    }
    if (saveToDisk && !returnB64)
      result.remove(QStringLiteral("protected_content_base64"));
    QString msg = signToo ? bt(QStringLiteral("Documento protegido y firmado correctamente"))
                          : bt(QStringLiteral("Documento protegido correctamente"));
    if (!result.value("outputPath").toString().isEmpty())
      msg += bt(QStringLiteral(": ")) + result.value("outputPath").toString();
    setStatus(msg);
    emit protectionFinished(true, msg, result.toVariantMap());
    reply->deleteLater();
  });
}

void BackendBridge::protectEncryptedDataFile(const QString &inputPath,
                                             const QString &outputPath,
                                             QString secretB64) {
  if (!TransientSecret::isCanonicalAES256Base64(secretB64)) {
    TransientSecret::zeroize(secretB64);
    const QString error = bt(QStringLiteral(
        "La clave transitoria debe ser Base64 canónico de 32 bytes (44 caracteres)."));
    setStatus(error);
    emit protectionFinished(false, error, QVariantMap());
    return;
  }

  QVariantMap protectionOptions;
  protectionOptions.insert(QStringLiteral("container"),
                           QStringLiteral("cms-encrypted"));
  protectionOptions.insert(QStringLiteral("secret_b64"), secretB64);
  QVariantMap options;
  options.insert(QStringLiteral("profile"), QStringLiteral("compat"));
  options.insert(QStringLiteral("overwrite"), QStringLiteral("rename"));
  options.insert(QStringLiteral("saveToDisk"), true);
  options.insert(QStringLiteral("signToo"), false);
  options.insert(QStringLiteral("options"), protectionOptions);
  protectFileAdvanced(
      inputPath,
      TransientSecret::normalizedEncryptedDataOutputPath(outputPath),
      QVariantList(), -1, options);

  protectionOptions.insert(QStringLiteral("secret_b64"), QString());
  protectionOptions.clear();
  options.insert(QStringLiteral("options"), QVariantMap());
  options.clear();
  TransientSecret::zeroize(secretB64);
}

void BackendBridge::unprotectFileAdvanced(const QString &inputPath,
                                          const QString &outputPath,
                                          const QVariantMap &options) {
  QByteArray content;
  QString error;
  if (!backendReadRegularFile(inputPath, kBackendMaxSignInputBytes, &content,
                              &error)) {
    setStatus(error);
    emit unprotectionFinished(false, error, QVariantMap());
    return;
  }
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/unprotect", m_useTLS);
  QJsonObject body;
  body.insert(QStringLiteral("name"), QFileInfo(inputPath).fileName());
  body.insert(QStringLiteral("mime_type"),
              backendMIMETypeForPath(inputPath));
  body.insert(QStringLiteral("content_base64"),
              QString::fromLatin1(content.toBase64()));
  body.insert("saveToDisk", false);
  body.insert("returnUnprotectedB64", true);
  const QString overwrite =
      options.value("overwrite", QStringLiteral("rename")).toString().trimmed();
  const bool saveToDisk = options.value("saveToDisk", true).toBool();
  const bool returnB64 =
      options.value("returnUnprotectedB64", false).toBool();
  QVariantMap optionMap = options.value("options").toMap();
  QString secretB64 =
      optionMap.take(QStringLiteral("secret_b64")).toString().trimmed();
  if (!secretB64.isEmpty())
    body.insert(QStringLiteral("secret_b64"), secretB64);
  QByteArray jsonData =
      QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (!backendRESTRequestAllowed(jsonData, &error)) {
    body.insert(QStringLiteral("secret_b64"), QString());
    TransientSecret::zeroize(secretB64);
    TransientSecret::zeroize(jsonData);
    setStatus(error);
    emit unprotectionFinished(false, error, QVariantMap());
    return;
  }
  QNetworkReply *reply = m_nam->post(req, jsonData);
  body.insert(QStringLiteral("secret_b64"), QString());
  TransientSecret::zeroize(secretB64);
  TransientSecret::zeroize(jsonData);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, inputPath, outputPath, overwrite, saveToDisk,
           returnB64]() {
    const QByteArray raw = reply->readAll();
    if (reply->error() != QNetworkReply::NoError) {
      const QString msg = reply->errorString();
      setStatus(msg);
      emit unprotectionFinished(false, msg, QVariantMap());
      reply->deleteLater();
      return;
    }
    const QJsonObject obj = QJsonDocument::fromJson(raw).object();
    if (!obj.value("ok").toBool()) {
      const QString msg = obj.value("error").toString();
      setStatus(msg);
      emit unprotectionFinished(false, msg, QVariantMap());
      reply->deleteLater();
      return;
    }
    QByteArray unprotectedContent;
    QString error;
    if (!backendDecodeResponseBase64(
            obj, QStringLiteral("unprotected_content_base64"),
            kBackendMaxRESTOutputBytes, &unprotectedContent, &error)) {
      setStatus(error);
      emit unprotectionFinished(false, error, QVariantMap());
      reply->deleteLater();
      return;
    }
    QJsonObject result = obj;
    if (saveToDisk) {
      const QString requested = backendUnprotectionOutputPath(
          inputPath, outputPath, obj.value("documentName").toString());
      QString actualPath;
      if (!backendWritePrivateFile(requested, overwrite, unprotectedContent,
                                   &actualPath, &error)) {
        setStatus(error);
        emit unprotectionFinished(false, error, QVariantMap());
        reply->deleteLater();
        return;
      }
      result.insert(QStringLiteral("outputPath"), actualPath);
    }
    if (saveToDisk && !returnB64)
      result.remove(QStringLiteral("unprotected_content_base64"));
    QString msg = bt(QStringLiteral("Documento desprotegido correctamente"));
    if (!result.value("outputPath").toString().isEmpty())
      msg += bt(QStringLiteral(": ")) + result.value("outputPath").toString();
    setStatus(msg);
    emit unprotectionFinished(true, msg, result.toVariantMap());
    reply->deleteLater();
  });
}

void BackendBridge::unprotectEncryptedDataFile(const QString &inputPath,
                                               const QString &outputPath,
                                               QString secretB64) {
  if (!TransientSecret::isCanonicalAES256Base64(secretB64)) {
    TransientSecret::zeroize(secretB64);
    const QString error = bt(QStringLiteral(
        "La clave transitoria debe ser Base64 canónico de 32 bytes (44 caracteres)."));
    setStatus(error);
    emit unprotectionFinished(false, error, QVariantMap());
    return;
  }

  QVariantMap unprotectionOptions;
  unprotectionOptions.insert(QStringLiteral("secret_b64"), secretB64);
  QVariantMap options;
  options.insert(QStringLiteral("overwrite"), QStringLiteral("rename"));
  options.insert(QStringLiteral("saveToDisk"), true);
  options.insert(QStringLiteral("options"), unprotectionOptions);
  unprotectFileAdvanced(inputPath, outputPath, options);

  unprotectionOptions.insert(QStringLiteral("secret_b64"), QString());
  unprotectionOptions.clear();
  options.insert(QStringLiteral("options"), QVariantMap());
  options.clear();
  TransientSecret::zeroize(secretB64);
}

void BackendBridge::exportVerificationReport(const QString &outputPath,
                                             const QVariantMap &details,
                                             const QString &inputPath,
                                             const QString &originalPath) {
  const QString targetPath = outputPath.trimmed();
  if (targetPath.isEmpty()) {
    setStatus(bt(QStringLiteral("Debe indicar una ruta de salida para el informe")));
    return;
  }
  if (details.isEmpty()) {
    setStatus(bt(QStringLiteral("No hay resultado de verificación para exportar")));
    return;
  }

  QJsonObject report;
  report.insert("generatedAt", QDateTime::currentDateTimeUtc().toString(Qt::ISODate));
  report.insert("source", QStringLiteral("grxfirma-gui"));
  if (!inputPath.trimmed().isEmpty())
    report.insert("inputPath", inputPath);
  if (!originalPath.trimmed().isEmpty())
    report.insert("originalPath", originalPath);
  report.insert("result", QJsonObject::fromVariantMap(details));

  QFileInfo info(targetPath);
  QDir().mkpath(info.absolutePath());
  QSaveFile file(targetPath);
  if (!file.open(QIODevice::WriteOnly | QIODevice::Truncate | QIODevice::Text)) {
    setStatus(bt(QStringLiteral("No se pudo guardar el informe de verificación")));
    emit backendLogReceived(bt(QStringLiteral("❌ Error guardando informe de verificación: ")) +
                            targetPath);
    return;
  }
  file.write(QJsonDocument(report).toJson(QJsonDocument::Indented));
  if (!file.commit()) {
    setStatus(bt(QStringLiteral("No se pudo confirmar el informe de verificación")));
    emit backendLogReceived(bt(QStringLiteral("❌ Error confirmando informe de verificación: ")) +
                            targetPath);
    return;
  }
  const QString msg =
      bt(QStringLiteral("Informe de verificación guardado: ")) + targetPath;
  setStatus(msg);
  emit backendLogReceived(msg);
}

void BackendBridge::createHash(const QString &inputPath,
                               const QString &outputPath,
                               const QString &algorithm, const QString &format,
                               bool recursive) {
  emit backendLogReceived(bt(QStringLiteral("🧮 Creando huella para: ")) +
                          inputPath);
  const QFileInfo info(inputPath);
  if (recursive || info.isDir()) {
    const QString error = bt(QStringLiteral(
        "Las huellas de directorio requieren el modo IPC local."));
    setStatus(error);
    emit hashCreateFinished(false, error, QVariantMap());
    return;
  }
  QByteArray content;
  QString error;
  if (!backendReadRegularFile(inputPath, kBackendMaxSignInputBytes, &content,
                              &error)) {
    setStatus(error);
    emit hashCreateFinished(false, error, QVariantMap());
    return;
  }

  QJsonObject body;
  body.insert(QStringLiteral("name"), info.fileName());
  body.insert(QStringLiteral("content_base64"),
              QString::fromLatin1(content.toBase64()));
  body.insert("algorithm", algorithm);
  body.insert("format", format);
  body.insert("recursive", false);
  body.insert("saveToDisk", false);

  const QByteArray jsonData =
      QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (!backendRESTRequestAllowed(jsonData, &error)) {
    setStatus(error);
    emit hashCreateFinished(false, error, QVariantMap());
    return;
  }
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/hash", m_useTLS);
  QNetworkReply *reply = m_nam->post(req, jsonData);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, inputPath, outputPath, format]() {
    const bool success = reply->error() == QNetworkReply::NoError;
    const QByteArray raw = reply->readAll();
    const QJsonDocument doc = QJsonDocument::fromJson(raw);
    const QJsonObject obj = doc.object();
    const bool ok = success && obj.value("ok").toBool();
    const QString err =
        ok ? QString() : obj.value("error").toString(reply->errorString());
    if (!ok) {
      setStatus(err);
      emit hashCreateFinished(false, err, QVariantMap());
      reply->deleteLater();
      return;
    }
    QByteArray payload;
    const QString normalized = format.trimmed().toLower();
    if (normalized == QStringLiteral("bin") ||
        normalized == QStringLiteral("binary")) {
      const QByteArray encoded = obj.value("hash").toString().toLatin1();
      payload =
          QByteArray::fromBase64(encoded, QByteArray::AbortOnBase64DecodingErrors);
      if (payload.isEmpty() || payload.toBase64() != encoded) {
        const QString error =
            bt(QStringLiteral("El backend devolvió una huella binaria no válida."));
        setStatus(error);
        emit hashCreateFinished(false, error, QVariantMap());
        reply->deleteLater();
        return;
      }
    } else {
      payload = obj.value("hash").toString().toUtf8();
    }
    QString actualPath;
    QString error;
    const QString requested =
        backendHashOutputPath(inputPath, outputPath, format);
    if (!backendWritePrivateFile(requested, QStringLiteral("rename"), payload,
                                 &actualPath, &error)) {
      setStatus(error);
      emit hashCreateFinished(false, error, QVariantMap());
      reply->deleteLater();
      return;
    }
    QJsonObject resultObject = obj;
    resultObject.insert(QStringLiteral("outputPath"), actualPath);
    QString msg = bt(QStringLiteral("Huella generada correctamente"));
    msg += bt(QStringLiteral(": ")) + actualPath;
    setStatus(msg);
    emit hashCreateFinished(true, msg, resultObject.toVariantMap());
    reply->deleteLater();
  });
}

void BackendBridge::checkHash(const QString &inputPath, const QString &hashPath,
                              const QString &outputPath,
                              const QString &algorithm, bool recursive,
                              bool saveReportToDisk) {
  emit backendLogReceived(bt(QStringLiteral("🔎 Comprobando huella para: ")) +
                          inputPath);
  const QFileInfo info(inputPath);
  if (recursive || saveReportToDisk || info.isDir()) {
    const QString error = bt(QStringLiteral(
        "La comprobación de directorios e informes requiere el modo IPC local."));
    setStatus(error);
    emit hashCheckFinished(false, error, QVariantMap());
    return;
  }
  QByteArray content;
  QByteArray hashContent;
  QString error;
  if (!backendReadRegularFile(inputPath, kBackendMaxSignInputBytes, &content,
                              &error) ||
      !backendReadRegularFile(hashPath, kBackendMaxHashReferenceBytes,
                              &hashContent, &error)) {
    setStatus(error);
    emit hashCheckFinished(false, error, QVariantMap());
    return;
  }

  QJsonObject body;
  body.insert(QStringLiteral("name"), info.fileName());
  body.insert(QStringLiteral("content_base64"),
              QString::fromLatin1(content.toBase64()));
  body.insert(QStringLiteral("hash_content_base64"),
              QString::fromLatin1(hashContent.toBase64()));
  body.insert("algorithm", algorithm);
  body.insert("recursive", false);
  body.insert("saveReportToDisk", false);
  Q_UNUSED(outputPath);

  const QByteArray jsonData =
      QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (!backendRESTRequestAllowed(jsonData, &error)) {
    setStatus(error);
    emit hashCheckFinished(false, error, QVariantMap());
    return;
  }
  auto req =
      BackendBridgeMakeReq(m_addr, m_token, "/hash/check", m_useTLS);
  QNetworkReply *reply = m_nam->post(req, jsonData);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    const bool success = reply->error() == QNetworkReply::NoError;
    const QByteArray raw = reply->readAll();
    const QJsonDocument doc = QJsonDocument::fromJson(raw);
    const QJsonObject obj = doc.object();
    const bool ok = success && obj.value("ok").toBool();
    const QString err =
        ok ? QString() : obj.value("error").toString(reply->errorString());
    if (!ok) {
      setStatus(err);
      emit hashCheckFinished(false, err, QVariantMap());
      reply->deleteLater();
      return;
    }
    const QVariantMap result = obj.toVariantMap();
    const bool valid = obj.value("valid").toBool();
    QString msg = valid ? bt(QStringLiteral("Huella válida"))
                        : bt(QStringLiteral("Huella no válida"));
    if (!obj.value("report_output_path").toString().isEmpty())
      msg += bt(QStringLiteral(": ")) + obj.value("report_output_path").toString();
    setStatus(msg);
    emit hashCheckFinished(true, msg, result);
    reply->deleteLater();
  });
}

void BackendBridge::refreshCertificates() {
  emit backendLogReceived(bt(QStringLiteral("Refrescando certificados...")));
  QUrl url = BackendBridgeMakeUrl(m_addr, "/certificates?check=1", m_useTLS);
  QNetworkRequest req(url);
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      QByteArray data = reply->readAll();
      emit backendLogReceived(bt(QStringLiteral("Respuesta de certificados recibida: ")) +
                              QString::fromUtf8(data));
      QJsonDocument doc = QJsonDocument::fromJson(data);
      QJsonArray certs = doc.object().value("certificates").toArray();
      QVariantList list;
      for (const auto &c : certs)
        list << c.toVariant();
      emit certificatesLoaded(list);
      setStatus(bt(QStringLiteral("Certificados actualizados")));
    } else {
      QString errorMsg = reply->errorString();
      if (errorMsg.contains("Connection refused"))
        errorMsg = bt(QStringLiteral("Conexión rechazada (¿backend activo?)"));
      emit backendLogReceived(
          bt(QStringLiteral("Error cargando certificados: ")) + errorMsg);
      setStatus(bt(QStringLiteral("Error al cargar certificados: ")) + errorMsg);
    }
    reply->deleteLater();
  });
}

void BackendBridge::signFile(const QString &inputPath,
                             const QString &outputPath, int certIndex,
                             const QString &format) {
  QVariantMap options;
  options.insert("format", format);
  signFileAdvanced(inputPath, outputPath, certIndex, options);
}

void BackendBridge::signFileAdvanced(const QString &inputPath,
                                     const QString &outputPath, int certIndex,
                                     const QVariantMap &options) {
  emit backendLogReceived(bt(QStringLiteral("⚙ Preparando firma de: ")) +
                          inputPath);
  QByteArray input;
  QString localError;
  if (!backendReadRegularFile(inputPath, kBackendMaxSignInputBytes, &input,
                              &localError)) {
    emit signingFinished(false, localError, QString());
    return;
  }

  QJsonObject signOptions;
  if (!backendBuildSignOptions(options, true, &signOptions, &localError)) {
    emit signingFinished(false, localError, QString());
    return;
  }

  QUrl url = BackendBridgeMakeUrl(m_addr, "/sign", m_useTLS);
  QNetworkRequest req(url);
  req.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QJsonObject body;
  body.insert("name", QFileInfo(inputPath).fileName());
  body.insert("content_base64", QString::fromLatin1(input.toBase64()));
  body.insert("certificateIndex", certIndex);
  const QString action = options.value("action").toString().trimmed();
  const QString format = options.value("format").toString().trimmed();
  const QString overwrite = options.value("overwrite").toString().trimmed();
  if (!action.isEmpty())
    body.insert("action", action);
  if (!format.isEmpty())
    body.insert("format", format);
  if (!signOptions.isEmpty())
    body.insert("options", signOptions);

  QByteArray jsonData = QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (jsonData.size() > kBackendMaxRESTRequestBytes) {
    emit signingFinished(
        false,
        bt(QStringLiteral(
            "La petición supera el tamaño máximo permitido por el modo REST.")),
        QString());
    return;
  }
  emit backendLogReceived(
      bt(QStringLiteral("📤 Enviando contenido al servicio REST local: ")) +
      QString::number(input.size()) + bt(QStringLiteral(" bytes")));

  QNetworkReply *reply = m_nam->post(req, jsonData);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, inputPath, outputPath, format, overwrite]() {
    const QByteArray dataRaw = reply->readAll();
    const QJsonObject obj = QJsonDocument::fromJson(dataRaw).object();
    if (reply->error() != QNetworkReply::NoError) {
      const QString msg =
          obj.value(QStringLiteral("error")).toString(reply->errorString());
      emit backendLogReceived(bt(QStringLiteral("❌ ERROR DEL BACKEND: ")) + msg);
      setStatus(bt(QStringLiteral("Error: ")) + msg);
      emit signingFinished(false, msg, QString());
      reply->deleteLater();
      return;
    }
    const QByteArray signature = QByteArray::fromBase64(
        obj.value(QStringLiteral("signed_content_base64"))
            .toString()
            .toLatin1());
    if (signature.isEmpty()) {
      const QString msg =
          bt(QStringLiteral("El servicio no devolvió la firma generada."));
      emit signingFinished(false, msg, QString());
      reply->deleteLater();
      return;
    }
    QString requested = outputPath.trimmed();
    const QString effectiveFormat =
        obj.value(QStringLiteral("format")).toString(format);
    if (requested.isEmpty())
      requested =
          backendSignedOutputPath(inputPath, QString(), effectiveFormat);
    QString actual;
    QString writeError;
    if (!backendWritePrivateFile(requested, overwrite, signature, &actual,
                                 &writeError)) {
      emit signingFinished(false, writeError, QString());
      reply->deleteLater();
      return;
    }
    const QString msg = bt(QStringLiteral("Firma completada con éxito"));
    emit backendLogReceived(
        bt(QStringLiteral("✅ Firma guardada exitosamente en: ")) + actual);
    setStatus(bt(QStringLiteral("Firma completada satisfactoriamente")));
    emit signingFinished(true, msg, actual);
    reply->deleteLater();
  });
}

void BackendBridge::signFileMultiAdvanced(
    const QString &inputPath, const QString &outputPath, int certIndex,
    const QVariantList &additionalCertificateIds,
    const QVariantMap &options) {
  Q_UNUSED(inputPath);
  Q_UNUSED(outputPath);
  Q_UNUSED(certIndex);
  Q_UNUSED(additionalCertificateIds);
  Q_UNUSED(options);
  emit signingFinished(
      false,
      bt(QStringLiteral(
          "La cofirma múltiple guiada requiere el backend IPC local.")),
      QString());
}

void BackendBridge::signBatchAdvanced(const QVariantList &inputPaths,
                                      const QString &directoryPath,
                                      const QString &outputDir, int certIndex,
                                      const QVariantMap &options) {
  if (!options.value(QStringLiteral("additionalCertificateIds"))
           .toList()
           .isEmpty()) {
    emit batchSigningFinished(
        false,
        bt(QStringLiteral(
            "La cofirma múltiple por lote requiere el backend IPC local.")),
        QVariantList());
    return;
  }

  QStringList paths;
  for (const QVariant &value : inputPaths) {
    const QString path = value.toString().trimmed();
    if (!path.isEmpty() && !paths.contains(path))
      paths.append(path);
  }
  if (!directoryPath.trimmed().isEmpty()) {
    const QDir dir(directoryPath);
    const QFileInfoList files =
        dir.entryInfoList(QDir::Files | QDir::NoSymLinks, QDir::Name);
    for (const QFileInfo &file : files) {
      const QString path = file.absoluteFilePath();
      if (!paths.contains(path))
        paths.append(path);
    }
  }
  if (paths.isEmpty()) {
    emit batchSigningFinished(
        false, bt(QStringLiteral("Debe seleccionar al menos un fichero.")),
        QVariantList());
    return;
  }

  QString localError;
  QJsonObject baseOptions;
  if (!backendBuildSignOptions(options, true, &baseOptions, &localError)) {
    emit batchSigningFinished(false, localError, QVariantList());
    return;
  }
  QHash<QString, QVariantMap> documentOverrides;
  for (const QVariant &value :
       options.value(QStringLiteral("documentOverrides")).toList()) {
    const QVariantMap override = value.toMap();
    const QString path =
        override.value(QStringLiteral("inputPath")).toString().trimmed();
    if (!path.isEmpty())
      documentOverrides.insert(path, override);
  }

  qint64 totalInputBytes = 0;
  QJsonArray items;
  QStringList orderedPaths;
  for (const QString &path : paths) {
    QByteArray content;
    if (!backendReadRegularFile(path, kBackendMaxSignInputBytes, &content,
                                &localError)) {
      emit batchSigningFinished(false, localError, QVariantList());
      return;
    }
    totalInputBytes += content.size();
    if (totalInputBytes > kBackendMaxSignInputBytes) {
      emit batchSigningFinished(
          false,
          bt(QStringLiteral(
              "El lote supera el tamaño máximo permitido por el modo REST.")),
          QVariantList());
      return;
    }
    QJsonObject item;
    item.insert(QStringLiteral("name"), QFileInfo(path).fileName());
    item.insert(QStringLiteral("content_base64"),
                QString::fromLatin1(content.toBase64()));
    if (documentOverrides.contains(path)) {
      QVariantMap overrideOptions;
      overrideOptions.insert(
          QStringLiteral("visibleSeal"),
          documentOverrides.value(path).value(QStringLiteral("visibleSeal")));
      QJsonObject itemOptions;
      if (!backendBuildSignOptions(overrideOptions, false, &itemOptions,
                                   &localError)) {
        emit batchSigningFinished(false, localError, QVariantList());
        return;
      }
      item.insert(QStringLiteral("options"), itemOptions);
    }
    items.append(item);
    orderedPaths.append(path);
  }

  QJsonObject body;
  body.insert(QStringLiteral("items"), items);
  body.insert(QStringLiteral("certificateIndex"), certIndex);
  body.insert(QStringLiteral("format"),
              options.value(QStringLiteral("format")).toString());
  body.insert(QStringLiteral("action"),
              options.value(QStringLiteral("action"), QStringLiteral("sign"))
                  .toString());
  body.insert(QStringLiteral("options"), baseOptions);
  body.insert(QStringLiteral("saveToDisk"), false);
  body.insert(QStringLiteral("returnSignatureB64"), true);

  const QByteArray jsonData =
      QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (jsonData.size() > kBackendMaxRESTRequestBytes) {
    emit batchSigningFinished(
        false,
        bt(QStringLiteral(
            "La petición supera el tamaño máximo permitido por el modo REST.")),
        QVariantList());
    return;
  }

  QUrl url = BackendBridgeMakeUrl(m_addr, "/sign-batch", m_useTLS);
  QNetworkRequest req(url);
  req.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());
  QNetworkReply *reply = m_nam->post(req, jsonData);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, orderedPaths, outputDir, options]() {
    const QByteArray raw = reply->readAll();
    const QJsonObject obj = QJsonDocument::fromJson(raw).object();
    if (reply->error() != QNetworkReply::NoError) {
      const QString msg =
          obj.value(QStringLiteral("error")).toString(reply->errorString());
      emit batchSigningFinished(false, msg, QVariantList());
      reply->deleteLater();
      return;
    }
    const QJsonArray responseItems = obj.value(QStringLiteral("results")).toArray();
    QVariantList results;
    bool allOK = responseItems.size() == orderedPaths.size();
    const QString format =
        options.value(QStringLiteral("format")).toString().trimmed();
    const QString overwrite =
        options.value(QStringLiteral("overwrite")).toString().trimmed();
    for (int i = 0; i < orderedPaths.size(); ++i) {
      QVariantMap result;
      result.insert(QStringLiteral("inputPath"), orderedPaths.at(i));
      if (i >= responseItems.size()) {
        result.insert(QStringLiteral("ok"), false);
        result.insert(QStringLiteral("error"),
                      bt(QStringLiteral("Resultado de lote inconsistente.")));
        allOK = false;
        results.append(result);
        continue;
      }
      const QJsonObject response = responseItems.at(i).toObject();
      if (!response.value(QStringLiteral("ok")).toBool()) {
        result.insert(QStringLiteral("ok"), false);
        result.insert(QStringLiteral("error"),
                      response.value(QStringLiteral("error")).toString());
        allOK = false;
        results.append(result);
        continue;
      }
      const QByteArray signature = QByteArray::fromBase64(
          response.value(QStringLiteral("signed_content_base64"))
              .toString()
              .toLatin1());
      QString actual;
      QString writeError;
      const QString effectiveFormat =
          response.value(QStringLiteral("format")).toString(format);
      const QString requested =
          backendSignedOutputPath(orderedPaths.at(i), outputDir,
                                  effectiveFormat);
      if (signature.isEmpty() ||
          !backendWritePrivateFile(requested, overwrite, signature, &actual,
                                   &writeError)) {
        result.insert(QStringLiteral("ok"), false);
        result.insert(QStringLiteral("error"),
                      signature.isEmpty()
                          ? bt(QStringLiteral(
                                "El servicio no devolvió la firma generada."))
                          : writeError);
        allOK = false;
        results.append(result);
        continue;
      }
      result.insert(QStringLiteral("ok"), true);
      result.insert(QStringLiteral("outputPath"), actual);
      result.insert(QStringLiteral("format"),
                    response.value(QStringLiteral("format")).toString());
      results.append(result);
    }
    const QString msg =
        allOK ? bt(QStringLiteral("Firma por lote completada"))
              : bt(QStringLiteral("La firma por lote terminó con errores"));
    emit batchSigningFinished(allOK, msg, results);
    reply->deleteLater();
  });
}

void BackendBridge::onNetworkReplyFinished(QNetworkReply *reply) {
  Q_UNUSED(reply);
}

void BackendBridge::openCertManager() {
#ifdef Q_OS_WIN
  QProcess::startDetached("rundll32.exe", {"cryptext.dll,CryptExtOpenCER"});
#elif defined(Q_OS_MACOS)
  if (!QProcess::startDetached(
          "open", {"/System/Applications/Utilities/Keychain Access.app"})) {
    setStatus(bt(QStringLiteral("No se pudo abrir Acceso a Llaveros.")));
  } else {
    setStatus(bt(QStringLiteral("Se ha abierto Acceso a Llaveros.")));
  }
#else
  QStringList tools = {"seahorse", "kleopatra", "gcr-viewer"};
  for (const QString &tool : tools) {
    if (QProcess::startDetached(tool, {})) {
      setStatus(bt(QStringLiteral("Se ha abierto el gestor de certificados.")));
      emit backendLogReceived(
          bt(QStringLiteral("Gestor de certificados abierto con ")) + tool);
      return;
    }
  }
  QList<QUrl> urls = {QUrl("about:preferences#privacy"),
                      QUrl("chrome://settings/certificates"),
                      QUrl("edge://settings/certificates")};
  for (const QUrl &url : urls) {
    if (QDesktopServices::openUrl(url)) {
      setStatus(bt(QStringLiteral(
          "Se ha abierto la configuración de certificados del navegador.")));
      emit backendLogReceived(
          bt(QStringLiteral("Configuración de certificados abierta: ")) +
          url.toString());
      return;
    }
  }
  setStatus(bt(QStringLiteral("No se encontró un gestor de certificados disponible.")));
#endif
}

void BackendBridge::openLogFolder() {
  QString path =
      QDir(QStandardPaths::writableLocation(QStandardPaths::AppConfigLocation))
          .filePath("logs");
  QDesktopServices::openUrl(QUrl::fromLocalFile(path));
}

void BackendBridge::openIncidentFolder() {
  QDesktopServices::openUrl(
      QUrl::fromLocalFile(backendIncidentReportsDirPath()));
}

void BackendBridge::openHelpManual() {
  QString appDir = QCoreApplication::applicationDirPath();
  QString locale = TranslatorBridge::shared() ? TranslatorBridge::shared()->locale()
                                              : QStringLiteral("es");
  QString manual = resolveLocalizedHelpManual(appDir, locale);
  if (!manual.isEmpty()) {
    QDesktopServices::openUrl(QUrl::fromLocalFile(manual));
  } else {
    openLocalizedHelpFallback();
  }
}

void BackendBridge::checkRestHealth(const QString &addr, bool useTLS) {
  QUrl url = BackendBridgeMakeUrl(addr, "/health", useTLS);
  QNetworkRequest req(url);
  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, addr, useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    bool running = false;
    QString message;
    if (reply->error() == QNetworkReply::NoError) {
      running = true;
      message = bt(QStringLiteral("Servidor REST activo"));
    } else {
      message = reply->errorString();
    }
    emit restHealthChecked(running, message);
    reply->deleteLater();
  });
}

void BackendBridge::getProxySecretStoreStatus() {
  QUrl url =
      BackendBridgeMakeUrl(m_addr, "/settings/proxy/secret-store/status", m_useTLS);
  QNetworkRequest req(url);
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    bool available = false;
    QString platform;
    QString backend = QStringLiteral("none");
    QString reason;
    QString runtimeMode;
    if (reply->error() == QNetworkReply::NoError) {
      QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
      QJsonObject obj = doc.object();
      available = obj.value("available").toBool();
      platform = obj.value("platform").toString();
      backend = obj.value("backend").toString();
      reason = obj.value("reason").toString();
      runtimeMode = obj.value("runtimeProxyMode").toString();
    } else {
      reason = reply->errorString();
    }
    emit proxySecretStoreStatusReceived(available, platform, backend, reason,
                                        runtimeMode);
    reply->deleteLater();
  });
}

void BackendBridge::checkCertificateOnline(const QString &certificateId) {
  QUrl url =
      BackendBridgeMakeUrl(m_addr, "/certificates/online-check", m_useTLS);
  QNetworkRequest req(url);
  req.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QJsonObject body;
  body.insert("certificate_id", certificateId);

  QNetworkReply *reply = m_nam->post(req, QJsonDocument(body).toJson());
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    const bool networkOk = (reply->error() == QNetworkReply::NoError);
    const QByteArray raw = reply->readAll();
    const QJsonObject res = QJsonDocument::fromJson(raw).object();
    bool success = networkOk && res.value("ok").toBool();
    QString message;
    QVariantMap details;
    if (success) {
      details = res.toVariantMap();
      message = res.value("userMessage").toString();
      if (message.trimmed().isEmpty()) {
        message =
            bt(QStringLiteral("Comprobación online del certificado finalizada."));
      }
    } else if (networkOk) {
      message = res.value("error").toString();
      if (message.trimmed().isEmpty()) {
        message =
            bt(QStringLiteral("No se pudo comprobar online el certificado."));
      }
    } else {
      message =
          bt(QStringLiteral("Error de red al comprobar online el certificado: ")) +
          reply->errorString();
    }
    emit certificateOnlineCheckFinished(success, message, details);
    setStatus(message);
    reply->deleteLater();
  });
}

void BackendBridge::checkCertificates() {
  emit backendLogReceived(bt(QStringLiteral(
      "⚙ Realizando chequeo exhaustivo de certificados...")));
  QUrl url =
      BackendBridgeMakeUrl(m_addr, "/certificates?check=true", m_useTLS);
  QNetworkRequest req(url);
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      QByteArray data = reply->readAll();
      QJsonDocument doc = QJsonDocument::fromJson(data);
      QJsonArray certs = doc.object().value("certificates").toArray();
      QVariantList list;
      for (const auto &c : certs)
        list << c.toVariant();
      emit certificatesLoaded(list);
      setStatus(bt(QStringLiteral("Chequeo de certificados finalizado.")));
      emit backendLogReceived(bt(
          QStringLiteral("ℹ️ Chequeo exhaustivo de certificados completado.")));
    } else {
      emit backendLogReceived(
          bt(QStringLiteral("Error en chequeo: ")) + reply->errorString());
      setStatus(bt(QStringLiteral("Error en chequeo certificados")));
    }
    reply->deleteLater();
  });
}

void BackendBridge::checkUpdates() {
  emit updateCheckFinished(
      false,
      bt(QStringLiteral(
          "La comprobación segura de versiones necesita el motor local IPC. "
          "Abra GrxFirma desde el lanzador instalado y vuelva a intentarlo; "
          "la firma local no queda bloqueada.")),
      QVariantMap());
}

void BackendBridge::runTLSDiagnostics() {
  emit backendLogReceived(bt(QStringLiteral("⚙ Obteniendo diagnóstico TLS...")));
  QUrl url = BackendBridgeMakeUrl(m_addr, "/tls/trust-status", m_useTLS);
  QNetworkRequest req(url);
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      const QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
      const QJsonObject tlsStore =
          doc.object().value("tlsStore").toObject();
      const QString msg =
          bt(QStringLiteral(
                 "Diagnóstico TLS:\nEstado del almacén: %1.\nArtefactos: %2 (certificados: %3, claves: %4).\nCertificado local: %5. Confianza del sistema: %6."))
              .arg(tlsStore.value("state").toString())
              .arg(tlsStore.value("artifactCount").toInt())
              .arg(tlsStore.value("certificateCount").toInt())
              .arg(tlsStore.value("keyCount").toInt())
              .arg(tlsStore.value("localCertificateState").toString())
              .arg(tlsStore.value("systemTrustState").toString());
      emit backendLogReceived(msg);
      setStatus(bt(QStringLiteral("Diagnóstico TLS finalizado")));
    } else {
      emit backendLogReceived(
          bt(QStringLiteral("Error al obtener diagnóstico: ")) +
          reply->errorString());
    }
    reply->deleteLater();
  });
}

void BackendBridge::exportDiagnosticReport() {
  emit backendLogReceived(
      bt(QStringLiteral("⚙ Preparando resumen técnico para soporte...")));
  QUrl url = BackendBridgeMakeUrl(m_addr, "/diagnostics/report", m_useTLS);
  QNetworkRequest req(url);
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      const QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
      const QJsonObject res = doc.object();
      const QJsonObject tlsStore = res.value("tlsStore").toObject();
      const QString report =
          bt(QStringLiteral(
                 "Diagnóstico: %1 certs encontrados, %2 válidos para firmar.\nAlmacén TLS: estado %3; %4 artefactos (%5 certificados, %6 claves).\nCertificado local: %7. Confianza del sistema: %8."))
              .arg(res.value("certificateCount").toInt())
              .arg(res.value("canSignCount").toInt())
              .arg(tlsStore.value("state").toString())
              .arg(tlsStore.value("artifactCount").toInt())
              .arg(tlsStore.value("certificateCount").toInt())
              .arg(tlsStore.value("keyCount").toInt())
              .arg(tlsStore.value("localCertificateState").toString())
              .arg(tlsStore.value("systemTrustState").toString());
      emit backendLogReceived(report);
      QGuiApplication::clipboard()->setText(report);
      emit backendLogReceived(
          bt(QStringLiteral("ℹ️ El resumen técnico para soporte se ha copiado al portapapeles.")));
      setStatus(bt(QStringLiteral("Resumen de incidencia preparado para soporte.")));
    } else {
      emit backendLogReceived(
          bt(QStringLiteral("Error al obtener reporte: ")) +
          reply->errorString());
    }
    reply->deleteLater();
  });
}

void BackendBridge::clearTLSTrustStore() {
  emit backendLogReceived(
      bt(QStringLiteral("⚙ Vaciando almacén TLS de confianza...")));
  QUrl url = BackendBridgeMakeUrl(m_addr, "/tls/clear-store", m_useTLS);
  QNetworkRequest req(url);
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QNetworkReply *reply = m_nam->post(req, QByteArray());
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
      int removed = doc.object().value("removed").toInt();
      emit backendLogReceived(bt(QStringLiteral(
                                  "Certificados eliminados del almacén: %1"))
                                  .arg(removed));
      setStatus(bt(QStringLiteral("Almacén TLS limpiado.")));
    } else {
      emit backendLogReceived(
          bt(QStringLiteral("Error al vaciar almacén: ")) +
          reply->errorString());
    }
    reply->deleteLater();
  });
}

void BackendBridge::reinstallBrowserConnectors() {
#ifdef Q_OS_LINUX
  const QString home = QDir::homePath();
  const QStringList candidates = {
      home + "/.local/lib/grxfirma/bin/configure-browsers.sh",
      "/usr/lib/grxfirma/bin/configure-browsers.sh",
      QCoreApplication::applicationDirPath() + "/configure-browsers.sh",
  };
  QString helper;
  for (const QString &candidate : candidates) {
    QFileInfo info(candidate);
    if (info.exists() && info.isExecutable()) {
      helper = candidate;
      break;
    }
  }
  if (helper.isEmpty()) {
    emit backendLogReceived(bt(QStringLiteral("❌ No se encontró el instalador de conectores de navegador.")));
    setStatus(bt(QStringLiteral("No se encontraron conectores para reinstalar")));
    return;
  }

  auto *process = new QProcess(this);
  QProcessEnvironment env = ChildProcessEnvironment::sanitized();
  env.insert(QStringLiteral("GRXFIRMA_TARGET_HOME"), home);
  env.insert(QStringLiteral("GRXFIRMA_DESKTOP_ID"), QStringLiteral("grxfirma.desktop"));
  const QString localBridge = home + "/.local/lib/grxfirma/bin/browser-bridge.sh";
  if (QFileInfo::exists(localBridge))
    env.insert(QStringLiteral("GRXFIRMA_BROWSER_BRIDGE"), localBridge);
  const QString localXpi = home + "/.local/lib/grxfirma/extensions/firefox/grxfirma-extension-firefox.xpi";
  if (QFileInfo::exists(localXpi))
    env.insert(QStringLiteral("GRXFIRMA_FIREFOX_XPI"), localXpi);
  process->setProcessEnvironment(env);

  connect(process, &QProcess::readyReadStandardOutput, this, [this, process]() {
    const QString out = QString::fromUtf8(process->readAllStandardOutput()).trimmed();
    if (!out.isEmpty())
      emit backendLogReceived(out);
  });
  connect(process, &QProcess::readyReadStandardError, this, [this, process]() {
    const QString out = QString::fromUtf8(process->readAllStandardError()).trimmed();
    if (!out.isEmpty())
      emit backendLogReceived(out);
  });
  connect(process, qOverload<int, QProcess::ExitStatus>(&QProcess::finished),
          this, [this, process](int code, QProcess::ExitStatus status) {
            const bool ok = status == QProcess::NormalExit && code == 0;
            setStatus(ok ? bt(QStringLiteral("Conectores de navegador reinstalados. Reinicia el navegador."))
                         : bt(QStringLiteral("Error reinstalando conectores de navegador")));
            emit backendLogReceived(ok ? bt(QStringLiteral("✅ Conectores de navegador reinstalados."))
                                       : bt(QStringLiteral("❌ Error reinstalando conectores de navegador.")));
            process->deleteLater();
          });
  emit backendLogReceived(bt(QStringLiteral("⚙ Reinstalando conectores de navegador...")));
  process->start(helper);
#else
  emit backendLogReceived(bt(QStringLiteral("La reinstalación de conectores desde la app solo está disponible en Linux por ahora.")));
#endif
}

// ─── Service management helpers ──────────────────────────────────────────────

static QNetworkRequest BackendBridgeMakeReq(const QString &addr,
                                            const QString &token,
                                            const QString &path,
                                            bool useTLS = true) {
  QNetworkRequest req(BackendBridgeMakeUrl(addr, path, useTLS));
  // El bridge envía documentos y, en EncryptedData, una clave transitoria.
  // Nunca delegar en Qt un 307/308 que pueda reenviar el POST a otro destino.
  req.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                   QNetworkRequest::ManualRedirectPolicy);
  req.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  if (!token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + token.toUtf8());
  return req;
}

void BackendBridge::getServiceStatus() {
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/service/status", m_useTLS);
  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    QJsonObject obj = QJsonDocument::fromJson(reply->readAll()).object();
    bool installed = obj.value("status").toObject().value("installed").toBool();
    bool running = obj.value("status").toObject().value("running").toBool();
    QString platform =
        obj.value("status").toObject().value("platform").toString();
    QString method = obj.value("status").toObject().value("method").toString();
    emit serviceStatusReceived(installed, running, platform, method);
    reply->deleteLater();
  });
}

void BackendBridge::installService() {
  auto req =
      BackendBridgeMakeReq(m_addr, m_token, "/service/install", m_useTLS);
  QNetworkReply *reply = m_nam->post(req, QByteArray("{}"));
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    QJsonObject obj = QJsonDocument::fromJson(reply->readAll()).object();
    bool ok = obj.value("ok").toBool();
    QString msg = obj.value("message").toString();
    if (!ok)
      msg = obj.value("error").toString();
    emit serviceActionFinished(ok, msg);
    reply->deleteLater();
  });
}

void BackendBridge::uninstallService() {
  auto req =
      BackendBridgeMakeReq(m_addr, m_token, "/service/uninstall", m_useTLS);
  QNetworkReply *reply = m_nam->post(req, QByteArray("{}"));
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    QJsonObject obj = QJsonDocument::fromJson(reply->readAll()).object();
    bool ok = obj.value("ok").toBool();
    QString msg =
        ok ? obj.value("message").toString() : obj.value("error").toString();
    emit serviceActionFinished(ok, msg);
    reply->deleteLater();
  });
}

void BackendBridge::startService() {
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/service/start", m_useTLS);
  QNetworkReply *reply = m_nam->post(req, QByteArray("{}"));
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    QJsonObject obj = QJsonDocument::fromJson(reply->readAll()).object();
    bool ok = obj.value("ok").toBool();
    QString msg =
        ok ? obj.value("message").toString() : obj.value("error").toString();
    emit serviceActionFinished(ok, msg);
    reply->deleteLater();
  });
}

void BackendBridge::stopService() {
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/service/stop", m_useTLS);
  QNetworkReply *reply = m_nam->post(req, QByteArray("{}"));
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    QJsonObject obj = QJsonDocument::fromJson(reply->readAll()).object();
    bool ok = obj.value("ok").toBool();
    QString msg =
        ok ? obj.value("message").toString() : obj.value("error").toString();
    emit serviceActionFinished(ok, msg);
    reply->deleteLater();
  });
}

void BackendBridge::getSettings() {
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/settings", m_useTLS);
  QNetworkReply *reply = m_nam->get(req);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      QJsonObject obj = QJsonDocument::fromJson(reply->readAll()).object();
      // El backend devuelve { "ok": true, "settings": { ... } }
      emit settingsLoaded(obj.value("settings").toObject().toVariantMap());
    } else {
      emit backendLogReceived(
          bt(QStringLiteral("Error al obtener ajustes: ")) +
          reply->errorString());
    }
    reply->deleteLater();
  });
}

void BackendBridge::saveSettings(const QVariantMap &settings) {
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/settings", m_useTLS);
  QByteArray data = QJsonDocument::fromVariant(settings).toJson();
  QNetworkReply *reply = m_nam->post(req, data);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      const QString message = bt(QStringLiteral("Ajustes guardados"));
      emit backendLogReceived(
          bt(QStringLiteral("Ajustes guardados correctamente")));
      setStatus(message);
      emit settingsSaved(true, message);
    } else {
      const QString message =
          bt(QStringLiteral("Error al guardar ajustes: ")) +
          reply->errorString();
      emit backendLogReceived(
          message);
      emit settingsSaved(false, message);
    }
    reply->deleteLater();
  });
}
void BackendBridge::getPdfPreview(const QString &path, int page,
                                  const QString &requestId) {
  const int requestedPage = qMax(1, page);
  const QString previewRequestId =
      requestId.trimmed().isEmpty()
          ? QStringLiteral("preview-rest-%1-%2")
                .arg(QDateTime::currentMSecsSinceEpoch())
                .arg(QRandomGenerator::global()->generate())
          : requestId.trimmed();
  QUrl url = BackendBridgeMakeUrl(m_addr, "/pdf/preview", m_useTLS);
  QNetworkRequest req(url);
  req.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QJsonObject params;
  params.insert("path", path);
  params.insert("page", requestedPage);
  params.insert("requestId", previewRequestId);
  QByteArray body = QJsonDocument(params).toJson();

  m_activePreviewRequestId = previewRequestId;
  QNetworkReply *reply = m_nam->post(req, body);
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, previewRequestId, path, requestedPage]() {
    if (previewRequestId != m_activePreviewRequestId) {
      reply->deleteLater();
      return;
    }
    m_activePreviewRequestId.clear();
    if (reply->error() == QNetworkReply::NoError) {
      QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
      QJsonObject obj = doc.object();
      if (obj.value("ok").toBool()) {
        double w = obj.value("width").toDouble();
        double h = obj.value("height").toDouble();
        emit pdfPreviewReceived(previewRequestId, path, requestedPage, true,
                                obj.value("data").toString(), w, h,
                                obj.value("currentPage").toInt(),
                                obj.value("totalPages").toInt());
      } else {
        emit pdfPreviewReceived(previewRequestId, path, requestedPage, false,
                                obj.value("error").toString(), 0, 0, 0, 0);
      }
    } else {
      emit pdfPreviewReceived(previewRequestId, path, requestedPage, false,
                              reply->errorString(), 0, 0, 0, 0);
    }
    reply->deleteLater();
  });
}

void BackendBridge::installCamerfirmaCerts() {
  if (m_addr.isEmpty()) {
    emit backendLogReceived(
        bt(QStringLiteral("❌ No se puede instalar: Backend no configurado.")));
    return;
  }

  emit backendLogReceived(bt(QStringLiteral(
      "🛠 Iniciando instalación de confianza TLS local...")));

  QUrl url =
      BackendBridgeMakeUrl(m_addr, "/trust/install-public-roots", m_useTLS);
  QNetworkRequest req(url);
  req.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  if (!m_token.isEmpty())
    req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

  QNetworkReply *reply = m_nam->post(req, QByteArray());
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    if (reply->error() == QNetworkReply::NoError) {
      QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
      QJsonObject obj = doc.object();
      if (obj.value("ok").toBool()) {
        emit backendLogReceived(
            bt(QStringLiteral("Certificado TLS local: %1. Confianza del sistema: %2."))
                .arg(obj.value("localCertificateState").toString())
                .arg(obj.value("systemTrustState").toString()));
        emit backendLogReceived(
            bt(QStringLiteral("✅ Proceso de instalación finalizado.")));
        setStatus(bt(QStringLiteral("Confianza TLS local instalada")));
      } else {
        emit backendLogReceived(
            bt(QStringLiteral("❌ Error: ")) + obj.value("error").toString());
        setStatus(bt(QStringLiteral("Error instalando la confianza TLS local")));
      }
    } else {
      emit backendLogReceived(
          bt(QStringLiteral("❌ Error de red: ")) + reply->errorString());
      setStatus(bt(QStringLiteral("Error de conexión")));
    }
    reply->deleteLater();
  });
}

void BackendBridge::importCertificate(const QString &path,
                                      const QString &password) {
  QString localPath = path;
  if (localPath.startsWith("file://")) {
    localPath = QUrl(path).toLocalFile();
  }

  QFile file(localPath);
  if (!file.open(QIODevice::ReadOnly)) {
    emit certificateImportFinished(
        false, bt(QStringLiteral("No se pudo abrir el archivo de certificado.")));
    return;
  }
  QByteArray data = file.readAll();
  file.close();

  QString b64 = data.toBase64();

  QJsonObject body;
  body.insert("p12B64", b64);
  body.insert("password", password);

  auto req = BackendBridgeMakeReq(m_addr, m_token, "/certificates/import");
  QNetworkReply *reply = m_nam->post(req, QJsonDocument(body).toJson());
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);

  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    bool ok = (reply->error() == QNetworkReply::NoError);
    QString msg = "";
    if (ok) {
      refreshCertificates();
      msg = bt(QStringLiteral("Certificado importado correctamente."));
    } else {
      QByteArray response = reply->readAll();
      QJsonDocument doc = QJsonDocument::fromJson(response);
      msg = doc.object().value("error").toString();
      if (msg.isEmpty())
        msg = reply->errorString();
    }
    emit certificateImportFinished(ok, msg);
    reply->deleteLater();
  });
}

void BackendBridge::saveTextReport(const QString &outputPath,
                                   const QString &content) {
  QString localPath = outputPath;
  if (localPath.startsWith("file://")) {
    localPath = QUrl(outputPath).toLocalFile();
  }
  if (localPath.trimmed().isEmpty()) {
    setStatus(bt(QStringLiteral("No se pudo guardar el informe.")));
    return;
  }

  QSaveFile file(localPath);
  if (!file.open(QIODevice::WriteOnly | QIODevice::Text)) {
    setStatus(bt(QStringLiteral("No se pudo guardar el informe.")));
    return;
  }
  file.write(content.toUtf8());
  if (!file.commit()) {
    setStatus(bt(QStringLiteral("No se pudo guardar el informe.")));
    return;
  }
  setStatus(bt(QStringLiteral("Informe guardado correctamente.")));
}

QString BackendBridge::incidentReportsDir() const {
  return backendIncidentReportsDirPath();
}

QString BackendBridge::defaultIncidentReportPath(const QString &kind) const {
  return backendDefaultIncidentReportPath(kind);
}

QString BackendBridge::saveIncidentReport(const QVariantMap &report,
                                          const QString &kind) {
  QVariantMap payload = report;
  if (!payload.contains("generatedAt")) {
    payload.insert("generatedAt",
                   QDateTime::currentDateTimeUtc().toString(Qt::ISODate));
  }
  payload = IncidentSanitizePayload(payload, false);
  const QString targetPath = backendDefaultIncidentReportPath(kind);
  if (targetPath.isEmpty()) {
    setStatus(bt(QStringLiteral("No se pudo preparar el directorio privado de incidencias.")));
    emit backendLogReceived(
        bt(QStringLiteral("❌ No se pudo preparar el almacenamiento privado de incidencias.")));
    return QString();
  }
  const QString summaryPath = backendIncidentTextPathForJson(targetPath);
  const QString logTailPath = backendIncidentLogTailPathForJson(targetPath);
  const QString manifestPath = backendIncidentManifestPathForJson(targetPath);
  const QString previewPath = backendIncidentPreviewPathForJson(targetPath);
  QString writeError;
  const QByteArray incidentJSON =
      QJsonDocument(QJsonObject::fromVariantMap(payload))
          .toJson(QJsonDocument::Indented);
  if (!IncidentWritePrivateFile(targetPath, incidentJSON, &writeError)) {
    setStatus(bt(QStringLiteral("No se pudo guardar la incidencia.")));
    emit backendLogReceived(
        bt(QStringLiteral("❌ No se pudo guardar la incidencia saneada.")));
    return QString();
  }
  const QString summaryText =
      backendRedactSupportText(payload.value(QStringLiteral("summaryText")).toString().trimmed());
  const bool wroteSummary =
      !summaryText.isEmpty() &&
      IncidentWritePrivateFile(summaryPath,
                               summaryText.toUtf8() + QByteArrayLiteral("\n"));
  const QString logTail = backendReadLogTailRedacted(backendGuiLogPath());
  const bool wroteLogTail =
      !logTail.isEmpty() &&
      IncidentWritePrivateFile(logTailPath,
                               logTail.toUtf8() + QByteArrayLiteral("\n"));
  const QByteArray previewJSON =
      QJsonDocument(
          QJsonObject::fromVariantMap(backendBuildIncidentPreview(payload)))
          .toJson(QJsonDocument::Indented);
  const bool wrotePreview =
      IncidentWritePrivateFile(previewPath, previewJSON);
  QVariantList files;
  files << QFileInfo(targetPath).fileName();
  if (wroteSummary)
    files << QFileInfo(summaryPath).fileName();
  if (wroteLogTail)
    files << QFileInfo(logTailPath).fileName();
  if (wrotePreview)
    files << QFileInfo(previewPath).fileName();
  QVariantMap manifest;
  manifest.insert(QStringLiteral("schema"),
                  QStringLiteral("grxfirma-incident-bundle-v1"));
  manifest.insert(QStringLiteral("generatedAt"),
                  payload.value(QStringLiteral("generatedAt")));
  manifest.insert(QStringLiteral("kind"),
                  payload.value(QStringLiteral("kind")));
  manifest.insert(QStringLiteral("bundleFiles"), files);
  manifest.insert(QStringLiteral("redactions"),
                  QVariantList{QStringLiteral("allowlisted-fields"),
                               QStringLiteral("paths-to-file-names"),
                               QStringLiteral("credentials-and-content"),
                               QStringLiteral("log-tail-sanitized-and-truncated")});
  manifest.insert(QStringLiteral("containsSupportSummary"), wroteSummary);
  manifest.insert(QStringLiteral("containsLogTail"), wroteLogTail);
  manifest.insert(QStringLiteral("containsPreview"), wrotePreview);
  const QByteArray manifestJSON =
      QJsonDocument(QJsonObject::fromVariantMap(manifest))
          .toJson(QJsonDocument::Indented);
  IncidentWritePrivateFile(manifestPath, manifestJSON);
  const QString msg =
      bt(QStringLiteral("Incidencia guardada: ")) +
      QFileInfo(targetPath).fileName();
  setStatus(msg);
  emit backendLogReceived(msg);
  return targetPath;
}

void BackendBridge::sendIncidentReport(const QVariantMap &report,
                                       const QString &endpoint,
                                       const QString &savedIncidentPath) {
  if (!IncidentHasExplicitRemoteConsent(report)) {
    const QString message =
        bt(QStringLiteral("El envío requiere previsualización y consentimiento explícito."));
    setStatus(message);
    emit incidentReportSent(false, message, QString());
    return;
  }

  const QString reportsDirectory = backendIncidentReportsDirPath();
  QVariantMap storedPayload;
  QString sourceError;
  if (reportsDirectory.isEmpty() ||
      !IncidentLoadEligibleSavedReport(savedIncidentPath, reportsDirectory,
                                       &storedPayload, &sourceError)) {
    const QString message = bt(QStringLiteral(
        "El envío remoto requiere una incidencia de fallo guardada por la aplicación."));
    setStatus(message);
    emit backendLogReceived(
        bt(QStringLiteral("❌ Se rechazó un envío remoto sin incidencia de origen válida.")));
    emit incidentReportSent(false, message, QString());
    return;
  }
  const QString localPath = QFileInfo(savedIncidentPath).canonicalFilePath();

  const QString trimmedEndpoint = endpoint.trimmed();
  if (trimmedEndpoint.isEmpty()) {
    const QString message =
        bt(QStringLiteral("No se ha configurado un destino HTTPS para soporte."));
    setStatus(message);
    emit incidentReportSent(false, message, QString());
    return;
  }

  const QUrl url(trimmedEndpoint);
  if (!url.isValid() || url.scheme().toLower() != QStringLiteral("https") ||
      url.host().isEmpty() || !url.userInfo().isEmpty() || url.hasFragment()) {
    const QString message =
        bt(QStringLiteral("El envío remoto solo permite destinos HTTPS válidos."));
    setStatus(message);
    emit incidentReportSent(false, message, QString());
    return;
  }

  QVariantMap storedPreview =
      storedPayload.value(QStringLiteral("supportPreview")).toMap();
  storedPreview.insert(
      QStringLiteral("consent"),
      report.value(QStringLiteral("supportPreview"))
          .toMap()
          .value(QStringLiteral("consent"))
          .toMap());
  storedPayload.insert(QStringLiteral("supportPreview"), storedPreview);
  const QVariantMap safePayload =
      IncidentSanitizePayload(storedPayload, true);

  emit backendLogReceived(
      bt(QStringLiteral("⚙ Enviando incidencia por HTTPS.")));

  QNetworkRequest req(url);
  req.setHeader(QNetworkRequest::ContentTypeHeader,
                QStringLiteral("application/json"));
  req.setRawHeader("Accept", "application/json");
  req.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                   QNetworkRequest::ManualRedirectPolicy);

  const QByteArray body =
      QJsonDocument(QJsonObject::fromVariantMap(
                        backendBuildIncidentUploadEnvelope(safePayload)))
          .toJson(QJsonDocument::Compact);
  const QDateTime attemptedAtUtc = QDateTime::currentDateTimeUtc();
  QNetworkReply *reply = m_nam->post(req, body);
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, localPath, reportsDirectory, attemptedAtUtc]() {
            const int status =
                reply->attribute(QNetworkRequest::HttpStatusCodeAttribute)
                    .toInt();
            const bool redirected =
                reply->attribute(QNetworkRequest::RedirectionTargetAttribute)
                    .isValid();
            const bool ok = reply->error() == QNetworkReply::NoError &&
                            !redirected && status >= 200 && status < 300;
            QString auditError;
            const bool attemptRecorded = IncidentRecordRemoteAttempt(
                localPath, reportsDirectory, ok, attemptedAtUtc, &auditError);
            QString message;
            if (ok) {
              message = bt(QStringLiteral("Incidencia enviada correctamente."));
              emit backendLogReceived(
                  bt(QStringLiteral("ℹ️ Incidencia remota enviada correctamente.")));
              setStatus(message);
            } else {
              message = bt(QStringLiteral(
                  "No se pudo enviar la incidencia por el canal HTTPS configurado."));
              emit backendLogReceived(
                  bt(QStringLiteral("❌ El canal HTTPS rechazó la incidencia.")));
              setStatus(message);
            }
            if (!attemptRecorded) {
              emit backendLogReceived(bt(QStringLiteral(
                  "❌ No se pudo registrar localmente el resultado del envío remoto.")));
              if (ok) {
                message = bt(QStringLiteral(
                    "La incidencia se envió, pero no se pudo registrar el intento localmente."));
                setStatus(message);
              }
            }
            emit incidentReportSent(ok && attemptRecorded, message, localPath);
            reply->deleteLater();
          });
}

void BackendBridge::installPublicRoots() {
  emit backendLogReceived(
      bt(QStringLiteral("⚙ Instalando confianza TLS local...")));
  auto req = BackendBridgeMakeReq(m_addr, m_token, "/confianza/instalar");
  QNetworkReply *reply = m_nam->post(req, QByteArray());
  BackendBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    bool ok = (reply->error() == QNetworkReply::NoError);
    QString msg =
        ok ? bt(QStringLiteral("Confianza TLS local instalada con éxito"))
           : reply->errorString();
    QByteArray response = reply->readAll();
    QJsonDocument doc = QJsonDocument::fromJson(response);
    if (doc.isObject()) {
      QJsonObject obj = doc.object();
      if (obj.contains("ok"))
        ok = obj.value("ok").toBool(ok);
      if (obj.contains("error") && !obj.value("error").toString().isEmpty())
        msg = obj.value("error").toString();
      else if (ok)
        msg = bt(QStringLiteral(
                     "Certificado TLS local: %1. Confianza del sistema: %2."))
                  .arg(obj.value("localCertificateState").toString())
                  .arg(obj.value("systemTrustState").toString());
    }
    emit publicRootsInstallationFinished(ok, msg);
    reply->deleteLater();
  });
}

// El modo REST no administra el registro de Windows: el selector de afirma://
// solo existe con el motor de escritorio (IPC).
void BackendBridge::getAfirmaHandlerStatus() {
  emit afirmaHandlerFinished(QStringLiteral("afirma_handler_status"), false,
                             QVariantMap(),
                             QStringLiteral("afirma_handler_unsupported"));
}

void BackendBridge::selectAfirmaHandler(const QString &) {
  emit afirmaHandlerFinished(QStringLiteral("afirma_handler_select"), false,
                             QVariantMap(),
                             QStringLiteral("afirma_handler_unsupported"));
}
