// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "ipcbridge.h"
#include "linuxstartupregistration.h"
#include "executablelocator.h"
#include "activediagnostics.h"
#include "incidentprivacy.h"
#include "ipcsocketpath.h"
#include "processenvironment.h"
#include "transientsecret.h"
#include "translatorbridge.h"
#include "webcompatibilitylease.h"
#include <QClipboard>
#include <QCoreApplication>
#include <QCryptographicHash>
#include <QDateTime>
#include <QDesktopServices>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QGuiApplication>
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
#include <QVariantMap>

static QString it(const QString &key) {
  if (auto *tr = TranslatorBridge::shared())
    return tr->t(key);
  return key;
}

static QStringList IpcBridgeExpectedLocalTLSPins();

static bool readCredentialFileIPC(const QString &path, QByteArray *data,
                                  QString *error) {
  if (!data || !error)
    return false;
  QString localPath = path;
  if (localPath.startsWith(QStringLiteral("file://")))
    localPath = QUrl(path).toLocalFile();
  const QFileInfo info(localPath);
  constexpr qint64 maxCredentialBytes = 2 * 1024 * 1024;
  if (!info.exists() || !info.isFile() || info.isSymLink()) {
    *error = it(QStringLiteral(
        "La credencial debe ser un archivo regular y no puede ser un enlace."));
    return false;
  }
  if (info.size() <= 0 || info.size() > maxCredentialBytes) {
    *error = it(QStringLiteral(
        "La credencial está vacía o supera el límite de 2 MiB."));
    return false;
  }
  QFile file(info.absoluteFilePath());
  if (!file.open(QIODevice::ReadOnly)) {
    *error =
        it(QStringLiteral("No se pudo abrir el archivo de certificado."));
    return false;
  }
  *data = file.read(maxCredentialBytes + 1);
  file.close();
  if (data->isEmpty() || data->size() > maxCredentialBytes) {
    data->fill('\0');
    data->clear();
    *error = it(QStringLiteral(
        "La credencial está vacía o supera el límite de 2 MiB."));
    return false;
  }
  return true;
}

static QString generateLocalBearerTokenIPC() {
  QString token;
  token.reserve(64);
  auto *random = QRandomGenerator::system();
  for (int i = 0; i < 8; ++i)
    token.append(QStringLiteral("%1").arg(random->generate(), 8, 16,
                                         QLatin1Char('0')));
  return token;
}

static bool openLocalizedHelpFallbackIPC() {
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

static QString ipcBridgeStateDir() {
  QString stateDir =
      QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation);
  if (stateDir.isEmpty()) {
    QString home =
        QStandardPaths::writableLocation(QStandardPaths::HomeLocation);
    stateDir =
        home.isEmpty() ? QDir::tempPath() : home + "/.local/state/grxfirma";
  }
  QDir dir(stateDir);
  dir.mkpath(".");
  return dir.absolutePath();
}

static QString sanitizeIpcIncidentKind(QString kind) {
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

static QString ipcIncidentReportsDirPath() {
  QDir dir(ipcBridgeStateDir());
  const QString path = dir.filePath(QStringLiteral("incidents"));
  if (!IncidentEnsurePrivateDirectory(path))
    return QString();
  return IncidentApplyReportRetention(
             path, QDateTime::currentDateTimeUtc(), 30, 50)
             ? path
             : QString();
}

static QString ipcDefaultIncidentReportPath(const QString &kind) {
  const QString reportsDir = ipcIncidentReportsDirPath();
  if (reportsDir.isEmpty())
    return QString();
  const QString normalizedKind = sanitizeIpcIncidentKind(kind);
  const QString stamp =
      QDateTime::currentDateTime().toString("yyyyMMdd-HHmmss-zzz");
  const QString nonce = QStringLiteral("%1").arg(
      QRandomGenerator::system()->generate(), 8, 16, QLatin1Char('0'));
  return QDir(reportsDir)
      .filePath(QStringLiteral("incident-%1-%2-%3.json")
                    .arg(normalizedKind, stamp, nonce));
}

static QString ipcIncidentTextPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".txt");
}

static QString ipcIncidentLogTailPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".logtail.txt");
}

static QString ipcIncidentManifestPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".manifest.json");
}

static QString ipcIncidentPreviewPathForJson(const QString &jsonPath) {
  QString path = jsonPath;
  if (path.endsWith(QStringLiteral(".json")))
    path.chop(5);
  return path + QStringLiteral(".preview.json");
}

static QString ipcGuiLogPath() {
  QString stateDir =
      QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation);
  if (stateDir.isEmpty()) {
    const QString home =
        QStandardPaths::writableLocation(QStandardPaths::HomeLocation);
    stateDir =
        home.isEmpty() ? QDir::tempPath() : home + "/.local/state/grxfirma";
  }
  return QDir(stateDir).filePath(QStringLiteral("logs/gui-qml.log"));
}

static QString ipcRedactSupportText(QString text) {
  return IncidentSanitizeText(text);
}

static QString ipcReadLogTailRedacted(const QString &path, qint64 maxBytes = 65536) {
  return IncidentReadSanitizedTextTail(path, maxBytes);
}

static QVariantMap ipcBuildIncidentPreview(const QVariantMap &payload) {
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
                     it(QStringLiteral("Paquete de soporte listo para exportar"))));
  preview.insert(QStringLiteral("summary"),
                 providedPreview.value(
                     QStringLiteral("summary"),
                     it(QStringLiteral("Se incluirá un resumen saneado de la incidencia y un extracto técnico mínimo."))));
  preview.insert(QStringLiteral("includedData"),
                 providedPreview.value(
                     QStringLiteral("includedData"),
                     QVariantList{
                         it(QStringLiteral("Resumen de la operación")),
                         it(QStringLiteral("Versión y plataforma")),
                         it(QStringLiteral("Metadatos del certificado seleccionado")),
                         it(QStringLiteral("Nombres de fichero sin ruta completa")),
                         it(QStringLiteral("Cola de log redactada"))}));
  preview.insert(QStringLiteral("omittedData"),
                 providedPreview.value(
                     QStringLiteral("omittedData"),
                     QVariantList{
                         it(QStringLiteral("Documentos originales")),
                         it(QStringLiteral("Rutas completas del perfil de usuario")),
                         it(QStringLiteral("Claves privadas")),
                         it(QStringLiteral("Certificados DER/PEM completos")),
                         it(QStringLiteral("Secretos, tokens y cabeceras sensibles"))}));
  preview.insert(QStringLiteral("consent"), consent);
  return preview;
}

static QVariantMap ipcBuildIncidentUploadEnvelope(const QVariantMap &payload) {
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
  client.insert(QStringLiteral("transport"), QStringLiteral("ipc"));
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
                  ipcBuildIncidentPreview(safePayload));
  envelope.insert(QStringLiteral("manifest"), manifest);
  envelope.insert(QStringLiteral("logTail"),
                  ipcReadLogTailRedacted(ipcGuiLogPath()));
  return envelope;
}

static QStringList ipcHelpManualCandidates(const QString &appDir,
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

static QString resolveLocalizedHelpManualIPC(const QString &appDir,
                                             const QString &locale) {
  for (const QString &candidate : ipcHelpManualCandidates(appDir, locale)) {
    if (QFile::exists(candidate))
      return candidate;
  }
  return QString();
}

static QString ipcSocketStateName(QLocalSocket::LocalSocketState state) {
  switch (state) {
  case QLocalSocket::UnconnectedState:
    return QStringLiteral("Unconnected");
  case QLocalSocket::ConnectingState:
    return QStringLiteral("Connecting");
  case QLocalSocket::ConnectedState:
    return QStringLiteral("Connected");
  case QLocalSocket::ClosingState:
    return QStringLiteral("Closing");
  }
  return QStringLiteral("Unknown");
}

static bool ipcHasTransientProtectionSecret(const QVariantMap &params) {
  if (!params.value(QStringLiteral("secret_b64")).toString().isEmpty())
    return true;
  const QVariantMap options =
      params.value(QStringLiteral("options")).toMap();
  return !options.value(QStringLiteral("secret_b64")).toString().isEmpty();
}

static bool ipcHasRemoteSigningSecret(const QVariantMap &params) {
  return params.contains(QStringLiteral("remotePin")) ||
         params.contains(QStringLiteral("remoteOtp"));
}

// El PIN y el OTP de un certificado remoto viajan en Base64 (el motor los
// decodifica en []byte y los borra). Se retiran de las opciones de QML y se
// borran las copias controladas por el bridge.
static void ipcMoveRemoteSigningSecrets(QVariantMap &params,
                                        const QVariantMap &options) {
  const QString keys[] = {QStringLiteral("remotePin"),
                          QStringLiteral("remoteOtp")};
  for (const QString &key : keys) {
    QString value = options.value(key).toString();
    if (value.isEmpty())
      continue;
    QByteArray bytes = value.toUtf8();
    QByteArray encoded = bytes.toBase64();
    params.insert(key, QString::fromLatin1(encoded));
    TransientSecret::zeroize(value);
    TransientSecret::zeroize(bytes);
    TransientSecret::zeroize(encoded);
  }
}

static void ipcForgetRemoteSigningSecrets(QVariantMap &params) {
  params.remove(QStringLiteral("remotePin"));
  params.remove(QStringLiteral("remoteOtp"));
}

static QString ipcNewCorrelationId(const QString &prefix, quint64 seq) {
  return QStringLiteral("%1-%2-%3")
      .arg(prefix)
      .arg(QDateTime::currentMSecsSinceEpoch())
      .arg(seq);
}

static QString ipcOriginHint(const QString &action, const QString &message) {
  const QString haystack =
      (action + QStringLiteral(" ") + message).toLower();
  if (haystack.contains(QStringLiteral("ipc")) ||
      haystack.contains(QStringLiteral("socket")) ||
      haystack.contains(QStringLiteral("pipe")) ||
      haystack.contains(QStringLiteral("sin conexión con el motor"))) {
    return it(QStringLiteral("Posible origen: puente IPC local o backend desktop."));
  }
  if (haystack.contains(QStringLiteral("proxy")) ||
      haystack.contains(QStringLiteral("tls")) ||
      haystack.contains(QStringLiteral("ssl")) ||
      haystack.contains(QStringLiteral("rest")) ||
      haystack.contains(QStringLiteral("http"))) {
    return it(QStringLiteral("Posible origen: red local, proxy o servicio web local."));
  }
  if (haystack.contains(QStringLiteral("@firma")) ||
      haystack.contains(QStringLiteral("afirma")) ||
      haystack.contains(QStringLiteral("trif")) ||
      haystack.contains(QStringLiteral("triphase")) ||
      haystack.contains(QStringLiteral("sede")) ||
      haystack.contains(QStringLiteral("portal"))) {
    return it(QStringLiteral("Posible origen: servicio remoto o flujo trifásico externo."));
  }
  if (haystack.contains(QStringLiteral("cert")) ||
      haystack.contains(QStringLiteral("pkcs")) ||
      haystack.contains(QStringLiteral("almac")) ||
      haystack.contains(QStringLiteral("keystore")) ||
      haystack.contains(QStringLiteral("alias")) ||
      haystack.contains(QStringLiteral("clave")) ||
      haystack.contains(QStringLiteral("firma no soportada"))) {
    return it(QStringLiteral("Posible origen: certificado, almacén o dispositivo criptográfico."));
  }
  return it(QStringLiteral("Posible origen no clasificado todavía; revisar traza completa."));
}

IpcBridge::IpcBridge(QObject *parent) : QObject(parent) {
  m_socket = new QLocalSocket(this);
  m_nam = new QNetworkAccessManager(this);
  m_activeDiagnostics = new ActiveDiagnosticsRunner(this);
  m_webCompatibilityLease = new WebCompatibilityLease(
      [this]() { stopOwnedWebCompatibilityServer(); }, this);
  m_status = it(QStringLiteral("Iniciando..."));
  connect(m_socket, &QLocalSocket::readyRead, this, &IpcBridge::onReadyRead);
  connect(m_socket, &QLocalSocket::connected, this, &IpcBridge::onConnected);
  connect(m_socket, &QLocalSocket::errorOccurred, this, &IpcBridge::onError);
  connect(m_activeDiagnostics, &ActiveDiagnosticsRunner::runningChanged, this,
          [this](bool running) {
            if (m_activeDiagnosticsRunning == running)
              return;
            m_activeDiagnosticsRunning = running;
            emit activeDiagnosticsRunningChanged();
          });
  connect(m_activeDiagnostics, &ActiveDiagnosticsRunner::finished, this,
          &IpcBridge::activeDiagnosticsFinished);
  connect(m_socket, &QLocalSocket::disconnected, this, [this]() {
    emit backendLogReceived(
        it(QStringLiteral("⚠️ Conexión IPC cerrada. estado=")) +
        ipcSocketStateName(m_socket->state()) +
        it(QStringLiteral(", pending=")) + m_pendingAction +
        it(QStringLiteral(", deferred=")) + m_deferredAction);
    setStatus(it(QStringLiteral("Conexión IPC cerrada")));
    const auto pendingSeals = m_sealPreviewRequests.keys();
    m_sealPreviewRequests.clear();
    for (const QString &id : pendingSeals)
      emit sealPreviewReceived(id, false, QString(),
                               it(QStringLiteral("Se cerró la conexión con el motor de firma")));
    if (!m_shuttingDown) {
      failPendingActionDueToConnection(
          it(QStringLiteral("La conexión IPC se cerró antes de recibir la respuesta")));
      // Un cierre inesperado no debe dejar el frontend inutilizable. Las
      // operaciones con credencial transitoria ya han fallado arriba y nunca
      // se encolan: la reconexión solo recupera el canal. Solo reconectamos si
      // el backend que posee esta GUI sigue vivo; al detenerlo de forma
      // explícita el guard evita reabrir el canal.
      QTimer::singleShot(0, this, [this]() {
        if (m_shuttingDown || !m_process ||
            m_process->state() == QProcess::NotRunning ||
            m_socket->state() == QLocalSocket::ConnectedState) {
          return;
        }
        setStatus(it(QStringLiteral("Reconectando con el motor de firma...")));
        tryConnect();
      });
    }
  });
  connect(m_webCompatibilityLease, &WebCompatibilityLease::activeChanged, this,
          &IpcBridge::webCompatibilityActiveChanged);
  connect(m_webCompatibilityLease, &WebCompatibilityLease::expired, this,
          [this]() {
            const QString message = it(QStringLiteral(
                "Compatibilidad web desactivada automáticamente al caducar."));
            setStatus(message);
            emit backendLogReceived(message);
            emit webCompatibilityStateChanged(false, 0, message);
          });
}

IpcBridge::~IpcBridge() { shutdownForExit(); }

void IpcBridge::runActiveDiagnostics(bool explicitConsent,
                                     const QVariantMap &context) {
  const bool restMode = m_serverMode == QStringLiteral("rest");
  const bool channelAvailable =
      restMode
          ? (m_process && m_process->state() != QProcess::NotRunning)
          : (m_socket &&
             m_socket->state() == QLocalSocket::ConnectedState);
  m_activeDiagnostics->start(restMode ? m_addr : QString(),
                             restMode && m_useTLS,
                             IpcBridgeExpectedLocalTLSPins(),
                             channelAvailable, context, explicitConsent);
}

void IpcBridge::cancelActiveDiagnostics() { m_activeDiagnostics->cancel(); }

static bool IpcBridgeIsLoopback(const QString &addr) {
  return addr.startsWith("127.0.0.1:") || addr.startsWith("localhost:");
}

static QString IpcBridgePortFromAddr(const QString &addr) {
  int sep = addr.lastIndexOf(':');
  if (sep == -1 || sep == addr.size() - 1)
    return QStringLiteral("63118");
  return addr.mid(sep + 1);
}

static bool IpcBridgeIsValidLoopbackEndpoint(const QString &addr) {
  if (!IpcBridgeIsLoopback(addr) || addr.count(QLatin1Char(':')) != 1)
    return false;
  bool ok = false;
  const int port = IpcBridgePortFromAddr(addr).toInt(&ok);
  return ok && port >= 1024 && port <= 65535;
}

static QString IpcBridgeNormalizeRestAddr(const QString &addr) {
  if (addr.startsWith("127.0.0.1:") || addr.startsWith("localhost:"))
    return addr;
  return QStringLiteral("127.0.0.1:") + IpcBridgePortFromAddr(addr);
}

static QUrl IpcBridgeMakeUrl(const QString &addr, const QString &path,
                             bool useTLS = true) {
  QString protocol = useTLS ? "https://" : "http://";
  return QUrl(protocol + IpcBridgeNormalizeRestAddr(addr) + path);
}

static QString IpcBridgeLocalTLSCertPath() {
  const QString home = QDir::homePath();
  if (!home.isEmpty()) {
    return QDir(home).filePath(
        QStringLiteral(".config/grxfirma/tls/websocket-localhost.crt.pem"));
  }
  return QDir(QDir::tempPath())
      .filePath(QStringLiteral("grxfirma/tls/websocket-localhost.crt.pem"));
}

static QString IpcBridgeCertificatePin(const QSslCertificate &cert) {
  if (cert.isNull())
    return QString();
  return QString::fromLatin1(
             cert.digest(QCryptographicHash::Sha256).toHex())
      .toLower();
}

static QStringList IpcBridgeExpectedLocalTLSPins() {
  QFile file(IpcBridgeLocalTLSCertPath());
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
    const QString pin = IpcBridgeCertificatePin(cert);
    if (!pin.isEmpty())
      pins << pin;
  }
  pins.removeDuplicates();
  return pins;
}

static QSslCertificate
IpcBridgePeerCertificate(QNetworkReply *reply,
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
IpcBridgePeerMatchesPinnedLocalTLSCert(QNetworkReply *reply,
                                       const QList<QSslError> &errors) {
  const QString peerPin = IpcBridgeCertificatePin(
      IpcBridgePeerCertificate(reply, errors));
  if (peerPin.isEmpty())
    return false;

  const QStringList expectedPins = IpcBridgeExpectedLocalTLSPins();
  for (const QString &expectedPin : expectedPins) {
    if (peerPin == expectedPin)
      return true;
  }
  return false;
}

static void IpcBridgeAllowLocalTLSErrors(QNetworkReply *reply,
                                         const QString &addr, bool useTLS) {
  if (!reply || !useTLS || !IpcBridgeIsLoopback(addr))
    return;
  QObject::connect(reply, &QNetworkReply::sslErrors, reply,
                   [reply](const QList<QSslError> &errors) {
                     if (IpcBridgePeerMatchesPinnedLocalTLSCert(reply, errors))
                       reply->ignoreSslErrors(errors);
                   });
}

bool IpcBridge::canStopOwnedBackend() const {
  return m_process && m_process->state() != QProcess::NotRunning;
}

bool IpcBridge::webCompatibilityActive() const {
  return m_webCompatibilityLease && m_webCompatibilityLease->active();
}

bool IpcBridge::startTemporaryWebCompatibility(
    const QString &addr, const QString &token, const QString &fingerprints,
    bool useTLS, int durationMinutes) {
  Q_UNUSED(useTLS);
  if (!IpcBridgeIsValidLoopbackEndpoint(addr) || durationMinutes < 5 ||
      durationMinutes > 240) {
    stopWebCompatibility();
    const QString message = it(QStringLiteral(
        "No se activó la compatibilidad web: duración u origen local no válido."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
    return false;
  }

  if (webCompatibilityActive())
    m_webCompatibilityLease->cancel();
  stopOwnedWebCompatibilityServer();

  const QString backendBin = ExecutableLocator::bundledExecutable(
      QCoreApplication::applicationDirPath(),
      {QStringLiteral("grxfirma")});
  if (backendBin.isEmpty()) {
    const QString message = it(QStringLiteral(
        "No se activó la compatibilidad web: no se encontró el motor local."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
    return false;
  }

  const QString normalizedAddr = IpcBridgeNormalizeRestAddr(addr);
  QString effectiveToken = token.trimmed();
  if (effectiveToken.isEmpty() && fingerprints.trimmed().isEmpty())
    effectiveToken = generateLocalBearerTokenIPC();

  m_webCompatibilityProcess = new QProcess(this);
  m_webCompatibilityProcess->setProgram(backendBin);
  m_webCompatibilityProcess->setProcessEnvironment(
      ChildProcessEnvironment::forRestToken(effectiveToken));
  QStringList args{QStringLiteral("--rest"), QStringLiteral("--rest-addr"),
                   normalizedAddr};
  args << QStringLiteral("--rest-lifetime")
       << QStringLiteral("%1m").arg(durationMinutes);
  if (!fingerprints.trimmed().isEmpty())
    args << QStringLiteral("--rest-cert-fingerprints") << fingerprints.trimmed();
  m_webCompatibilityProcess->setArguments(args);
  connect(m_webCompatibilityProcess, &QProcess::readyReadStandardOutput, this,
          [this]() {
            if (m_webCompatibilityProcess)
              emit backendLogReceived(
                  QString::fromUtf8(
                      m_webCompatibilityProcess->readAllStandardOutput())
                      .trimmed());
          });
  connect(m_webCompatibilityProcess, &QProcess::readyReadStandardError, this,
          [this]() {
            if (m_webCompatibilityProcess)
              emit backendLogReceived(
                  QString::fromUtf8(
                      m_webCompatibilityProcess->readAllStandardError())
                      .trimmed());
          });
  QProcess *const launchedProcess = m_webCompatibilityProcess;
  connect(m_webCompatibilityProcess,
          qOverload<int, QProcess::ExitStatus>(&QProcess::finished), this,
          [this, launchedProcess](int, QProcess::ExitStatus) {
            if (m_webCompatibilityProcess != launchedProcess ||
                !webCompatibilityActive())
              return;
            m_webCompatibilityLease->cancel();
            const QString message = it(QStringLiteral(
                "Compatibilidad web desactivada: el servidor local terminó inesperadamente."));
            setStatus(message);
            emit webCompatibilityStateChanged(false, 0, message);
          });
  m_webCompatibilityProcess->start();

  if (!m_webCompatibilityProcess->waitForStarted(3000) ||
      !m_webCompatibilityLease->startMilliseconds(durationMinutes * 60 * 1000)) {
    stopOwnedWebCompatibilityServer();
    const QString message = it(QStringLiteral(
        "No se activó la compatibilidad web: el servidor local no quedó bajo control de esta aplicación."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
    return false;
  }

  const QString message =
      it(QStringLiteral("Compatibilidad web activa temporalmente durante %1 minutos."))
          .arg(durationMinutes);
  setStatus(message);
  emit backendLogReceived(message);
  emit webCompatibilityStateChanged(true, durationMinutes, message);
  return true;
}

void IpcBridge::stopWebCompatibility() {
  const bool wasActive = webCompatibilityActive();
  if (m_webCompatibilityLease)
    m_webCompatibilityLease->cancel();
  stopOwnedWebCompatibilityServer();
  if (wasActive) {
    const QString message =
        it(QStringLiteral("Compatibilidad web desactivada."));
    setStatus(message);
    emit webCompatibilityStateChanged(false, 0, message);
  }
}

void IpcBridge::stopOwnedWebCompatibilityServer() {
  if (!m_webCompatibilityProcess)
    return;
  m_webCompatibilityProcess->blockSignals(true);
  disconnect(m_webCompatibilityProcess, nullptr, this, nullptr);
  if (m_webCompatibilityProcess->state() != QProcess::NotRunning) {
    m_webCompatibilityProcess->terminate();
    if (!m_webCompatibilityProcess->waitForFinished(1000)) {
      m_webCompatibilityProcess->kill();
      m_webCompatibilityProcess->waitForFinished(1000);
    }
  }
  delete m_webCompatibilityProcess;
  m_webCompatibilityProcess = nullptr;
}

void IpcBridge::setStatus(const QString &s) {
  if (m_status != s) {
    m_status = s;
    emit statusChanged();
  }
}

void IpcBridge::setLastDiagnostic(const QVariantMap &diagnostic) {
  if (m_lastDiagnostic != diagnostic) {
    m_lastDiagnostic = diagnostic;
    emit lastDiagnosticChanged();
  }
}

void IpcBridge::setLastRequestId(const QString &requestId) {
  if (m_lastRequestId != requestId) {
    m_lastRequestId = requestId;
    emit lastRequestIdChanged();
  }
}

void IpcBridge::setLastTraceId(const QString &traceId) {
  if (m_lastTraceId != traceId) {
    m_lastTraceId = traceId;
    emit lastTraceIdChanged();
  }
}

void IpcBridge::setExpertMode(bool v) {
  if (m_expertMode != v) {
    m_expertMode = v;
    emit expertModeChanged();
  }
}

void IpcBridge::startBackend(const QString &addr, const QString &token,
                             const QString &mode, const QString &fingerprints,
                             bool useTLS) {
  if (!m_localTLSStartupStatus.isEmpty()) {
    m_localTLSStartupStatus.clear();
    emit localTLSStartupStatusChanged();
  }
  m_requestLocalTLSStartupAfterDeferred = false;
  m_refreshCertificatesAfterLocalTLSStartup = false;
  const QString oldMode = m_serverMode;
  m_addr = (mode == "rest") ? IpcBridgeNormalizeRestAddr(addr) : addr;
  m_token = token.trimmed();
  if (mode == "rest" && m_token.isEmpty() && fingerprints.trimmed().isEmpty())
    m_token = generateLocalBearerTokenIPC();
  m_serverMode = mode;
  m_fingerprints = fingerprints;
  m_useTLS = useTLS;

  // Determinar ruta del socket de forma segura.
#if defined(Q_OS_WIN)
  // QLocalSocket acepta nombres simples, pero el backend Go exige una ruta
  // local completa para rechazar de forma inequívoca UNC remotas.
  m_socketPath = IpcSocketPath::normalizeWindows(m_addr);
  if (m_socketPath.isEmpty()) {
    emit backendLogReceived(it(QStringLiteral(
        "❌ Ruta IPC de Windows inválida; use un nombre simple o \\\\.\\pipe\\<nombre>.")));
    setStatus(it(QStringLiteral("Error: Ruta IPC de Windows inválida")));
    return;
  }
#else
  if (m_addr.contains('/')) {
    m_socketPath = m_addr;
  } else {
    QString userName = QDir::home().dirName();
    if (userName.isEmpty())
      userName = "default";
    QString runtimeDir =
        QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
    if (runtimeDir.isEmpty())
      runtimeDir = QDir::tempPath();
    m_socketPath = QDir(runtimeDir)
                       .absoluteFilePath("grxfirma_ipc_" + userName + ".sock");
  }
#endif
  qDebug() << "[IpcBridge] Ruta IPC privada calculada.";

  // Si ya hay un proceso, detenerlo para asegurar un reinicio limpio
  if (m_process && m_process->state() != QProcess::NotRunning) {
    stopBackend();
    m_process->waitForFinished(2000);
  }

  // Comprobar si el socket esta vivo (solo si no es modo puramente REST)
  bool needsLaunch = true;
  if (m_serverMode != "rest") {
#if defined(Q_OS_WIN)
    QLocalSocket testSocket;
    testSocket.connectToServer(m_socketPath);
    if (testSocket.waitForConnected(500)) {
      testSocket.disconnectFromServer();
      needsLaunch = false;
      emit backendLogReceived(
          it(QStringLiteral("Named pipe activo encontrado. Conectando...")));
    } else {
      emit backendLogReceived(
          it(QStringLiteral("Named pipe no activo. Arrancando backend Go...")));
    }
#else
    if (QFileInfo::exists(m_socketPath)) {
      QLocalSocket testSocket;
      testSocket.connectToServer(m_socketPath);
      if (testSocket.waitForConnected(500)) {
        testSocket.disconnectFromServer();
        needsLaunch = false;
        emit backendLogReceived(
            it(QStringLiteral("Socket activo encontrado. Conectando...")));
      } else {
        emit backendLogReceived(
            it(QStringLiteral("Socket muerto encontrado. Limpiando y arrancando...")));
        QFile::remove(m_socketPath);
      }
    } else {
      emit backendLogReceived(
          it(QStringLiteral("Socket no encontrado. Arrancando backend Go...")));
    }
#endif
  } else {
    emit backendLogReceived(
        it(QStringLiteral("Modo REST activo. Arrancando backend Go...")));
  }

  if (needsLaunch) {
    launchBackendProcess();
  } else {
    tryConnect();
  }
}

void IpcBridge::launchBackendProcess() {
  if (m_process && m_process->state() != QProcess::NotRunning)
    return;

  const QString appDir = QCoreApplication::applicationDirPath();
  const QString wrapperBin = ExecutableLocator::bundledExecutable(
      appDir, {QStringLiteral("grxfirma-gui")});
  const QString backendBin = ExecutableLocator::bundledExecutable(
      appDir, {QStringLiteral("grxfirma")});

  QString bin;
  QStringList args;
  if (m_serverMode == "ipc") {
    if (wrapperBin.isEmpty()) {
      emit backendLogReceived(
          it(QStringLiteral("❌ No se encontró grxfirma-gui para iniciar el backend IPC.")));
      setStatus(it(QStringLiteral("Error al arrancar el backend IPC")));
      return;
    }
    bin = wrapperBin;
    args << "--server" << "--server-modo" << "ipc" << "--ipc-socket"
         << m_socketPath;
#if defined(Q_OS_WIN) || defined(Q_OS_LINUX)
    // El backend valida este valor contra las credenciales que entrega el SO
    // para el peer real del named pipe/socket. No es un secreto ni se usa como
    // dato de confianza por sí solo.
    args << "--ipc-client-pid"
         << QString::number(QCoreApplication::applicationPid());
#endif
  } else if (m_serverMode == "rest") {
    if (backendBin.isEmpty()) {
      emit backendLogReceived(it(QStringLiteral(
          "❌ No se encontró el motor local empaquetado para iniciar REST.")));
      setStatus(it(QStringLiteral("Error al arrancar el backend REST")));
      return;
    }
    bin = backendBin;
    args << "--rest" << "--rest-addr" << m_addr;
    if (!m_fingerprints.isEmpty())
      args << "--rest-cert-fingerprints" << m_fingerprints;
  } else {
    emit backendLogReceived(
        it(QStringLiteral("❌ Modo de backend no soportado: ")) + m_serverMode);
    setStatus(it(QStringLiteral("Modo backend no soportado")));
    return;
  }

  m_process = new QProcess(this);
  m_process->setProgram(bin);
  if (m_serverMode == QStringLiteral("rest")) {
    m_process->setProcessEnvironment(
        ChildProcessEnvironment::forRestToken(m_token));
  } else
    m_process->setProcessEnvironment(ChildProcessEnvironment::sanitized());
  m_process->setArguments(args);
  connect(m_process, &QProcess::readyReadStandardOutput, this, [this]() {
    emit backendLogReceived(
        QString::fromUtf8(m_process->readAllStandardOutput()).trimmed());
  });
  connect(m_process, &QProcess::readyReadStandardError, this, [this]() {
    emit backendLogReceived(
        QString::fromUtf8(m_process->readAllStandardError()).trimmed());
  });
  m_process->start();
  if (!m_process->waitForStarted(3000)) {
    emit backendLogReceived(it(QStringLiteral("❌ No se pudo arrancar GrxFirma: ")) +
                            m_process->errorString());
    setStatus(it(QStringLiteral("Error al arrancar el backend")));
  } else {
    emit backendLogReceived(it(QStringLiteral("✅ Backend Go arrancado (PID ")) +
                            QString::number(m_process->processId()) +
                            ") modo [" + m_serverMode + "]");

    if (m_serverMode != "rest") {
      tryConnect();
    } else {
      setStatus(it(QStringLiteral("Backend activo (REST)")));
      refreshCertificates();
    }
  }
}

void IpcBridge::tryConnect() {
  if (m_socket->state() == QLocalSocket::ConnectedState) {
    emit backendLogReceived(
        it(QStringLiteral("ℹ️ IPC ya conectado. Se omite nuevo connect().")));
    return;
  }
  if (m_socketPath.isEmpty()) {
    emit backendLogReceived(it(QStringLiteral("❌ Error: m_socketPath está vacía.")));
    setStatus(it(QStringLiteral("Error: Ruta IPC vacía")));
    return;
  }
  if (m_socket->state() == QLocalSocket::ConnectingState) {
    emit backendLogReceived(
        it(QStringLiteral("⏳ IPC sigue reconectando. estado=")) +
        ipcSocketStateName(m_socket->state()) +
        it(QStringLiteral(", retry=")) + QString::number(m_retryCount) +
        it(QStringLiteral(", deferred=")) + m_deferredAction);
  } else {
    emit backendLogReceived(it(QStringLiteral("🔌 Intentando conectar a: ")) +
                            m_socketPath +
                            it(QStringLiteral(" [estado=")) +
                            ipcSocketStateName(m_socket->state()) +
                            it(QStringLiteral(", retry=")) +
                            QString::number(m_retryCount) + "]");
    m_socket->connectToServer(m_socketPath);
  }

  // If not connected within 500ms, retry (up to 20 times = 10s)
  if (m_retryCount < 20) {
    m_retryCount++;
    QTimer::singleShot(500, this, [this]() {
      if (m_socket->state() != QLocalSocket::ConnectedState) {
        emit backendLogReceived(
            it(QStringLiteral("Reintentando conexión IPC... estado=")) +
            ipcSocketStateName(m_socket->state()) +
            it(QStringLiteral(" (")) + QString::number(m_retryCount) + "/20), deferred=" +
            m_deferredAction);
        tryConnect();
      }
    });
  } else {
    setStatus(it(QStringLiteral("No se pudo conectar al backend tras 10 segundos")));
    if (!m_deferredAction.isEmpty()) {
      const QString action = m_deferredAction;
      m_deferredAction.clear();
      m_deferredRequest.fill('\0');
      m_deferredRequest.clear();
      failActionDueToConnection(
          action,
          it(QStringLiteral("Sin conexión con el motor de firma tras varios reintentos")));
    }
  }
}

void IpcBridge::stopBackend() {
  if (m_shuttingDown)
    return;
  if (m_socket->isOpen()) {
    m_socket->close();
  }
  if (m_process) {
    m_process->blockSignals(true);
    disconnect(m_process, nullptr, this, nullptr);
  }
  if (m_process && m_process->state() != QProcess::NotRunning) {
    emit backendLogReceived(it(QStringLiteral("🛑 Deteniendo backend...")));
    m_process->terminate();
    if (!m_process->waitForFinished(1000)) {
      m_process->kill();
      m_process->waitForFinished(1000);
    }
  }
  if (m_process) {
    delete m_process;
    m_process = nullptr;
  }
  setStatus(it(QStringLiteral("Backend detenido")));
}

void IpcBridge::shutdownForExit() {
  if (m_shuttingDown)
    return;
  m_shuttingDown = true;
  if (m_webCompatibilityLease)
    m_webCompatibilityLease->cancel();
  stopOwnedWebCompatibilityServer();
  m_retryCount = 1000;
  m_deferredRequest.fill('\0');
  m_deferredRequest.clear();
  m_deferredAction.clear();

  if (m_socket) {
    m_socket->blockSignals(true);
    if (m_socket->isOpen())
      m_socket->abort();
  }

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

bool IpcBridge::queueDeferredRequest(const QString &action,
                                     const QVariantMap &params) {
  // Administrative configuration is explicit: never replay on reconnection.
  if (action == "get_token_settings" || action == "save_token_settings" ||
      action == "diagnose_token_settings")
    return false;
  if (action.startsWith(QStringLiteral("csc_")) ||
      ipcHasRemoteSigningSecret(params)) {
    // Conectar abre el navegador y el PIN/OTP es de una sola firma: nunca se
    // repiten solos tras una reconexión.
    return false;
  }
  if (ipcHasTransientProtectionSecret(params)) {
    // EncryptedData usa una clave de una sola operación: no debe sobrevivir en
    // la cola de reconexión ni reintentarse sin una nueva acción del usuario.
    return false;
  }
  if (action == QStringLiteral("import_certificate") ||
      action == QStringLiteral("import_certificate_to_store") ||
      action == QStringLiteral("use_temporary_certificate") ||
      action == QStringLiteral("proxy_secret_store")) {
    // No conservar contraseñas ni credenciales completas durante una
    // reconexión. La UI informa del fallo y permite repetir la acción.
    return false;
  }
  QJsonObject req;
  const quint64 seq = ++m_requestSeq;
  const QString requestId = ipcNewCorrelationId(QStringLiteral("ipc"), seq);
  const QString traceId = ipcNewCorrelationId(QStringLiteral("trace"), seq);
  req.insert("requestId", requestId);
  req.insert("traceId", traceId);
  req.insert("action", action);
  req.insert("params", QJsonObject::fromVariantMap(params));
  setLastRequestId(requestId);
  setLastTraceId(traceId);
  m_deferredAction = action;
  m_deferredRequest =
      QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n";
  if (m_pendingAction == action) {
    // La escritura que originó este diferido ya no está en vuelo.
    m_pendingAction.clear();
    m_pendingRequestId.clear();
    m_pendingTraceId.clear();
  }
  emit backendLogReceived(
      it(QStringLiteral("🕓 Acción diferida por falta de conexión IPC: ")) +
      action + it(QStringLiteral(" | estado=")) +
      ipcSocketStateName(m_socket->state()) + QStringLiteral(" | ") +
      IncidentFormatIpcLogEvent(
          req.toVariantMap(), QStringLiteral("ipc-request"),
          QStringLiteral("deferred"), m_deferredRequest.size()));
  setStatus(it(QStringLiteral("Reconectando con el motor de firma...")));
  tryConnect();
  return true;
}

void IpcBridge::flushDeferredRequest() {
  if (m_deferredRequest.isEmpty() || m_deferredAction.isEmpty())
    return;
  QByteArray request = m_deferredRequest;
  const QString action = m_deferredAction;
  QString requestId;
  QString traceId;
  const QJsonDocument deferredDoc = QJsonDocument::fromJson(request);
  if (!deferredDoc.isNull()) {
    requestId = deferredDoc.object().value("requestId").toString();
    traceId = deferredDoc.object().value("traceId").toString();
  }
  m_deferredRequest.fill('\0');
  m_deferredRequest.clear();
  m_deferredAction.clear();
  m_pendingAction = action;
  m_pendingRequestId = requestId;
  m_pendingTraceId = traceId;
  setLastRequestId(requestId);
  setLastTraceId(traceId);
  emit backendLogReceived(
      it(QStringLiteral("📨 Reintentando acción diferida: ")) +
      IncidentFormatIpcLogEvent(
          deferredDoc.object().toVariantMap(),
          QStringLiteral("ipc-request"), QStringLiteral("retrying"),
          request.size()) +
      it(QStringLiteral(" | estado=")) + ipcSocketStateName(m_socket->state()));
  if (m_socket->write(request) == -1) {
    request.fill('\0');
    const QString err =
        it(QStringLiteral("No se pudo reenviar la petición IPC: ")) +
        m_socket->errorString();
    emit backendLogReceived(it(QStringLiteral("❌ ")) + err);
    emit backendLogReceived(ipcOriginHint(action, err));
    failActionDueToConnection(action, err);
    return;
  }
  m_socket->flush();
  request.fill('\0');
  emit backendLogReceived(
      it(QStringLiteral("📤 Acción diferida reenviada: ")) + action);
}

void IpcBridge::failPendingActionDueToConnection(const QString &message) {
  if (!m_tokenSettingsAction.isEmpty()) {
    const QString tokenAction = m_tokenSettingsAction;
    m_tokenSettingsAction.clear();
    m_tokenSettingsRequestId.clear();
    emit tokenSettingsFinished(tokenAction, false, QVariantMap(), message);
  }
  const QString action = m_pendingAction;
  if (action.isEmpty())
    return;

  // Limpiar antes de emitir: errorOccurred y disconnected suelen llegar para
  // el mismo corte y la UI debe recibir exactamente un resultado terminal.
  m_pendingAction.clear();
  failActionDueToConnection(action, message);
}

void IpcBridge::failActionDueToConnection(const QString &action,
                                          const QString &message) {
  const QString safeMessage = IncidentSanitizeText(message);
  if (action == m_tokenSettingsAction && !action.isEmpty()) {
    m_tokenSettingsAction.clear();
    m_tokenSettingsRequestId.clear();
    emit tokenSettingsFinished(action, false, QVariantMap(), safeMessage);
  }
  if (m_pendingAction == action)
    m_pendingAction.clear();
  m_pendingRequestId.clear();
  m_pendingTraceId.clear();
  setLastTraceId(QString());
  const QString hint = ipcOriginHint(action, safeMessage);
  emit backendLogReceived(hint);
  if (action == "sign" || action == "sign_multicosign") {
    emit signingFinished(false, safeMessage, "");
  } else if (action == "sign_batch") {
    emit batchSigningFinished(false, safeMessage, QVariantList());
  } else if (action == "verify") {
    emit verificationFinished(false, safeMessage, QVariantMap());
  } else if (action == "protect" || action == "protect_sign") {
    emit protectionFinished(false, safeMessage, QVariantMap());
  } else if (action == "unprotect") {
    emit unprotectionFinished(false, safeMessage, QVariantMap());
  } else if (action == "certificate_export_public") {
    emit certificatePublicExportFinished(false, safeMessage);
  } else if (action == "hash_create") {
    emit hashCreateFinished(false, safeMessage, QVariantMap());
  } else if (action == "hash_check") {
    emit hashCheckFinished(false, safeMessage, QVariantMap());
  } else if (action == "check_updates") {
    emit updateCheckFinished(false, safeMessage, QVariantMap());
  } else if (action == "save_settings") {
    emit settingsSaved(false, safeMessage);
  } else if (action == "certificate_access_options") {
    emit certificateAccessOptionsLoaded(false, QVariantMap(), safeMessage);
  } else if (action == "smartcard_status") {
    emit smartcardStatusReceived(false, QVariantList(), safeMessage);
  } else if (action.startsWith(QStringLiteral("csc_"))) {
    emit cscFinished(action, false, QVariantMap(), safeMessage);
  } else if (action == "facturae_create") {
    emit facturaeCreated(false, QVariantMap(), safeMessage);
  } else if (action == "validate_verifactu") {
    emit verifactuValidated(false, QVariantMap(), safeMessage);
  } else if (action == "detect_verifactu") {
    emit verifactuDetected(false, QVariantMap());
  } else if (action == "read_verifactu_qr" || action == "query_verifactu_qr") {
    emit verifactuQRFinished(action, false, QVariantMap(), safeMessage);
  } else if (action == "validate_invoice") {
    emit invoiceValidated(false, QVariantMap(), safeMessage);
  } else if (action == "generate_eni_document" || action == "generate_eni_file") {
    emit eniGenerated(action, false, QVariantMap(), safeMessage);
  } else if (action == "protection_recipient_import" ||
             action == "protection_recipient_remove") {
    emit protectionRecipientChanged(false, safeMessage);
  } else if (action == "clear_temporary_certificates") {
    emit temporaryCertificatesCleared(false, safeMessage);
  } else if (action == "import_certificate" ||
             action == "import_certificate_to_store") {
    emit certificateImportFinished(false, safeMessage);
  } else if (action == "use_temporary_certificate" ||
             action == "remove_temporary_certificate") {
    emit temporaryCertificateFinished(false, safeMessage, QVariantMap());
  } else if (action == "proxy_secret_store" ||
             action == "proxy_secret_delete") {
    emit proxyCredentialsFinished(false, safeMessage, false, QString(), QString());
  } else if (action == "service_status" || action == "service_install" ||
             action == "service_uninstall" || action == "service_start" ||
             action == "service_stop") {
    emit serviceActionFinished(false, safeMessage);
  }
}

void IpcBridge::exportPublicCertificate(const QString &certificateId,
                                        const QString &outputPath,
                                        const QString &format) {
  QVariantMap params;
  params.insert(QStringLiteral("certificateId"), certificateId.trimmed());
  params.insert(QStringLiteral("outputPath"), outputPath);
  params.insert(QStringLiteral("format"), format);
  m_pendingAction = QStringLiteral("certificate_export_public");
  sendRequest(QStringLiteral("certificate_export_public"), params);
}

void IpcBridge::refreshCertificates() {
  QString modeLabel = m_serverMode.toUpper();
  if (modeLabel == "AMBAS")
    modeLabel = "REST+IPC";
  emit backendLogReceived(
      it(QStringLiteral("🔄 Solicitando certificados vía %1...")).arg(modeLabel));

  if (m_serverMode == "rest") {
    QUrl url = IpcBridgeMakeUrl(m_addr, "/certificates?check=1", m_useTLS);
    QNetworkRequest req(url);
    if (!m_token.isEmpty())
      req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

    QNetworkReply *reply = m_nam->get(req);
    IpcBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
    connect(reply, &QNetworkReply::finished, this, [this, reply]() {
      if (reply->error() == QNetworkReply::NoError) {
        QByteArray data = reply->readAll();
        QJsonDocument doc = QJsonDocument::fromJson(data);
        QJsonArray certs = doc.object().value("certificates").toArray();
        QVariantList list;
        for (const auto &c : certs)
          list << c.toVariant();
        emit certificatesLoaded(list);
        setStatus(it(QStringLiteral("Certificados actualizados (REST)")));
      } else {
        emit backendLogReceived(
            it(QStringLiteral("Error REST: ")) + reply->errorString());
      }
      reply->deleteLater();
    });
  } else {
    sendRequest("certificates");
  }
}

void IpcBridge::signFile(const QString &inputPath, const QString &outputPath,
                         int certIndex, const QString &format) {
  QVariantMap options;
  options["format"] = format;
  signFileAdvanced(inputPath, outputPath, certIndex, options);
}

void IpcBridge::signFileAdvanced(const QString &inputPath,
                                 const QString &outputPath, int certIndex,
                                 const QVariantMap &options) {
  emit backendLogReceived(
      it(QStringLiteral("⚙ Iniciando firma IPC para: ")) + inputPath);
  QVariantMap params;
  params["inputPath"] = inputPath;
  params["outputPath"] = outputPath;
  params["certificateIndex"] = certIndex;
  params["certificateId"] = options.value("certificateId").toString();
  params["format"] = options.value("format").toString();
  params["action"] = options.value("action", "sign").toString();
  params["strictCompat"] = options.value("strictCompat", false).toBool();
  params["overwrite"] = options.value("overwrite", "rename").toString();
  params["saveToDisk"] = options.value("saveToDisk", true).toBool();
  params["returnSignatureB64"] =
      options.value("returnSignatureB64", false).toBool();
  params["qrContent"] = options.value("qrContent").toString();
  params["reason"] = options.value("reason").toString();
  params["location"] = options.value("location").toString();
  params["contactInfo"] = options.value("contactInfo").toString();
  if (options.contains("extraOptions")) {
    params["extraOptions"] = options.value("extraOptions").toMap();
  }
  if (options.contains("visibleSeal")) {
    params["visibleSeal"] = options.value("visibleSeal").toMap();
  }
  ipcMoveRemoteSigningSecrets(params, options);
  sendRequest("sign", params);
  ipcForgetRemoteSigningSecrets(params);
}

void IpcBridge::signFileMultiAdvanced(const QString &inputPath,
                                      const QString &outputPath, int certIndex,
                                      const QVariantList &additionalCertificateIds,
                                      const QVariantMap &options) {
  emit backendLogReceived(
      it(QStringLiteral("⚙ Iniciando cofirma múltiple IPC para: ")) + inputPath);
  QVariantMap params;
  params["inputPath"] = inputPath;
  params["outputPath"] = outputPath;
  params["certificateIndex"] = certIndex;
  params["certificateId"] = options.value("certificateId").toString();
  params["additionalCertificateIds"] = additionalCertificateIds;
  params["format"] = options.value("format").toString();
  params["action"] = options.value("action", "sign").toString();
  params["strictCompat"] = options.value("strictCompat", false).toBool();
  params["overwrite"] = options.value("overwrite", "rename").toString();
  params["saveToDisk"] = options.value("saveToDisk", true).toBool();
  params["returnSignatureB64"] =
      options.value("returnSignatureB64", false).toBool();
  params["qrContent"] = options.value("qrContent").toString();
  params["reason"] = options.value("reason").toString();
  params["location"] = options.value("location").toString();
  params["contactInfo"] = options.value("contactInfo").toString();
  if (options.contains("extraOptions")) {
    params["extraOptions"] = options.value("extraOptions").toMap();
  }
  if (options.contains("visibleSeal")) {
    params["visibleSeal"] = options.value("visibleSeal").toMap();
  }
  ipcMoveRemoteSigningSecrets(params, options);
  sendRequest("sign_multicosign", params);
  ipcForgetRemoteSigningSecrets(params);
}

void IpcBridge::signBatchAdvanced(const QVariantList &inputPaths,
                                  const QString &directoryPath,
                                  const QString &outputDir, int certIndex,
                                  const QVariantMap &options) {
  emit backendLogReceived(it(QStringLiteral("⚙ Iniciando firma por lote IPC")));
  QVariantMap params;
  params["inputPaths"] = inputPaths;
  params["directoryPath"] = directoryPath;
  params["outputDir"] = outputDir;
  params["certificateIndex"] = certIndex;
  params["certificateId"] = options.value("certificateId").toString();
  if (options.contains("additionalCertificateIds")) {
    params["additionalCertificateIds"] =
        options.value("additionalCertificateIds").toList();
  }
  params["format"] = options.value("format").toString();
  params["action"] = options.value("action", "sign").toString();
  params["strictCompat"] = options.value("strictCompat", false).toBool();
  params["overwrite"] = options.value("overwrite", "rename").toString();
  params["qrContent"] = options.value("qrContent").toString();
  params["reason"] = options.value("reason").toString();
  params["location"] = options.value("location").toString();
  params["contactInfo"] = options.value("contactInfo").toString();
  if (options.contains("extraOptions")) {
    params["extraOptions"] = options.value("extraOptions").toMap();
  }
  if (options.contains("visibleSeal")) {
    params["visibleSeal"] = options.value("visibleSeal").toMap();
  }
  if (options.contains("documentOverrides")) {
    params["documentOverrides"] = options.value("documentOverrides").toList();
  }
  ipcMoveRemoteSigningSecrets(params, options);
  sendRequest("sign_batch", params);
  ipcForgetRemoteSigningSecrets(params);
}

void IpcBridge::verifyFile(const QString &inputPath) {
  verifyFileWithOriginal(inputPath, QString());
}

void IpcBridge::verifyFileWithOriginal(const QString &inputPath,
                                       const QString &originalPath) {
  emit backendLogReceived(
      it(QStringLiteral("🔍 Solicitando verificación IPC para: ")) +
      inputPath);
  QVariantMap params;
  params["inputPath"] = inputPath;
  if (!originalPath.trimmed().isEmpty()) {
    params["originalPath"] = originalPath;
  }
  sendRequest("verify", params);
}

void IpcBridge::loadProtectionRecipients() {
  emit backendLogReceived(
      it(QStringLiteral("🛡️ Solicitando destinatarios de protección IPC")));
  sendRequest("protection_recipients", QVariantMap());
}

void IpcBridge::importProtectionRecipient(const QString &path) {
  sendRequest(QStringLiteral("protection_recipient_import"),
              {{QStringLiteral("path"), path}});
}

void IpcBridge::removeProtectionRecipient(const QString &id) {
  sendRequest(QStringLiteral("protection_recipient_remove"),
              {{QStringLiteral("id"), id}});
}

bool IpcBridge::startupEnabled() const {
#ifdef Q_OS_LINUX
  return LinuxStartupRegistration::isEnabled();
#else
  return false;
#endif
}

bool IpcBridge::setStartupEnabled(bool enabled) {
#ifdef Q_OS_LINUX
  return LinuxStartupRegistration::setEnabled(enabled);
#else
  Q_UNUSED(enabled);
  return false;
#endif
}

void IpcBridge::createFacturae(const QVariantMap &draft, const QString &outputPath) {
  sendRequest(QStringLiteral("facturae_create"),
              {{QStringLiteral("draft"), draft},
               {QStringLiteral("outputPath"), outputPath}});
}

void IpcBridge::validateInvoice(const QString &inputPath) {
  sendRequest(QStringLiteral("validate_invoice"), {{QStringLiteral("inputPath"), inputPath}});
}

void IpcBridge::validateVeriFactu(const QString &inputPath) {
  sendRequest(QStringLiteral("validate_verifactu"), {{QStringLiteral("inputPath"), inputPath}});
}
void IpcBridge::detectVeriFactu(const QString &inputPath) {
  sendRequest(QStringLiteral("detect_verifactu"), {{QStringLiteral("inputPath"), inputPath}});
}
void IpcBridge::readVeriFactuQR(const QString &url) {
  sendRequest(QStringLiteral("read_verifactu_qr"), {{QStringLiteral("url"), url}});
}
void IpcBridge::readVeriFactuQRFile(const QString &inputPath) {
  // El motor valida la ruta, el tipo y los límites; no hay red en la lectura.
  sendRequest(QStringLiteral("read_verifactu_qr"), {{QStringLiteral("inputPath"), inputPath}});
}
void IpcBridge::queryVeriFactuQR(const QString &url) {
  sendRequest(QStringLiteral("query_verifactu_qr"), {{QStringLiteral("url"), url}});
}

void IpcBridge::generateENIDocument(const QVariantMap &params) {
  sendRequest(QStringLiteral("generate_eni_document"), params);
}

void IpcBridge::generateENIFile(const QVariantMap &params) {
  sendRequest(QStringLiteral("generate_eni_file"), params);
}

void IpcBridge::cscStatus() {
  sendRequest(QStringLiteral("csc_status"), QVariantMap());
}

void IpcBridge::cscConfigure(const QString &serviceUrl,
                             const QString &clientId) {
  QVariantMap params;
  params.insert(QStringLiteral("serviceUrl"), serviceUrl.trimmed());
  params.insert(QStringLiteral("clientId"), clientId.trimmed());
  sendRequest(QStringLiteral("csc_configure"), params);
}

void IpcBridge::cscConnect() {
  sendRequest(QStringLiteral("csc_connect"), QVariantMap());
}

void IpcBridge::cscDisconnect() {
  sendRequest(QStringLiteral("csc_disconnect"), QVariantMap());
}

void IpcBridge::cscSendOtp(const QString &certificateId) {
  QVariantMap params;
  params.insert(QStringLiteral("certificateId"), certificateId.trimmed());
  sendRequest(QStringLiteral("csc_send_otp"), params);
}

void IpcBridge::requestSmartcardStatus() {
  sendRequest(QStringLiteral("smartcard_status"));
}

void IpcBridge::getSealPreview(const QVariantMap &options,
                               const QString &requestId) {
  const QString id = requestId.trimmed();
  if (id.isEmpty())
    return;
  if (m_socket->state() != QLocalSocket::ConnectedState) {
    emit sealPreviewReceived(id, false, QString(),
                             it(QStringLiteral("Sin conexión con el motor de firma")));
    return;
  }
  const quint64 seq = ++m_requestSeq;
  QJsonObject req{{QStringLiteral("requestId"), id},
                  {QStringLiteral("traceId"), ipcNewCorrelationId(QStringLiteral("seal-trace"), seq)},
                  {QStringLiteral("action"), QStringLiteral("seal_preview")},
                  {QStringLiteral("params"), QJsonObject::fromVariantMap(options)}};
  const QByteArray payload = QJsonDocument(req).toJson(QJsonDocument::Compact) + '\n';
  // Solo conservamos el contexto de la última vista. Las peticiones previas
  // siguen procesándose en el motor; su respuesta se descarta al llegar.
  m_sealPreviewRequests.clear();
  m_sealPreviewRequests.insert(id, true);
  if (m_socket->write(payload) == -1) {
    m_sealPreviewRequests.remove(id);
    emit sealPreviewReceived(id, false, QString(), m_socket->errorString());
    return;
  }
  // No cancelar la petición ya enviada: el motor usa esta misma conexión.
  m_socket->flush();
}

void IpcBridge::protectFileAdvanced(const QString &inputPath,
                                    const QString &outputPath,
                                    const QVariantList &recipientIds,
                                    int certIndex,
                                    const QVariantMap &options) {
  const bool signToo = options.value("signToo", false).toBool();
  const QString action =
      signToo ? QStringLiteral("protect_sign") : QStringLiteral("protect");
  emit backendLogReceived((signToo ? it(QStringLiteral("🔐✍ Solicitando protección firmada IPC para: "))
                                   : it(QStringLiteral("🔐 Solicitando protección IPC para: "))) +
                          inputPath);
  QVariantMap params;
  params["inputPath"] = inputPath;
  params["outputPath"] = outputPath;
  params["certificateIndex"] = certIndex;
  params["profile"] = options.value("profile", QStringLiteral("compat")).toString();
  params["recipientIds"] = recipientIds;
  params["overwrite"] = options.value("overwrite", QStringLiteral("rename")).toString();
  params["saveToDisk"] = options.value("saveToDisk", true).toBool();
  params["returnProtectedB64"] =
      options.value("returnProtectedB64", false).toBool();
  QVariantMap optionMap = options.value("options").toMap();
  if (!optionMap.isEmpty())
    params["options"] = optionMap;
  // Solo «Proteger y firmar» usa el certificado remoto: el PIN/OTP va ligado
  // al certificado elegido y nunca acompaña a una protección sin firma.
  if (signToo) {
    const QString certificateId =
        options.value(QStringLiteral("certificateId")).toString().trimmed();
    if (!certificateId.isEmpty())
      params["certificateId"] = certificateId;
    ipcMoveRemoteSigningSecrets(params, options);
  }
  sendRequest(action, params);
  ipcForgetRemoteSigningSecrets(params);
  QString secretB64 =
      optionMap.value(QStringLiteral("secret_b64")).toString();
  optionMap.insert(QStringLiteral("secret_b64"), QString());
  optionMap.clear();
  params.insert(QStringLiteral("options"), QVariantMap());
  params.clear();
  TransientSecret::zeroize(secretB64);
}

void IpcBridge::protectEncryptedDataFile(const QString &inputPath,
                                         const QString &outputPath,
                                         QString secretB64) {
  if (!TransientSecret::isCanonicalAES256Base64(secretB64)) {
    TransientSecret::zeroize(secretB64);
    const QString error = it(QStringLiteral(
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

void IpcBridge::unprotectFileAdvanced(const QString &inputPath,
                                      const QString &outputPath,
                                      const QVariantMap &options) {
  emit backendLogReceived(
      it(QStringLiteral("🔓 Solicitando desprotección IPC para: ")) + inputPath);
  QVariantMap params;
  params["inputPath"] = inputPath;
  params["outputPath"] = outputPath;
  params["overwrite"] = options.value("overwrite", QStringLiteral("rename")).toString();
  params["saveToDisk"] = options.value("saveToDisk", true).toBool();
  params["returnUnprotectedB64"] =
      options.value("returnUnprotectedB64", false).toBool();
  QVariantMap optionMap = options.value("options").toMap();
  if (!optionMap.isEmpty())
    params["options"] = optionMap;
  sendRequest("unprotect", params);
  QString secretB64 =
      optionMap.value(QStringLiteral("secret_b64")).toString();
  optionMap.insert(QStringLiteral("secret_b64"), QString());
  optionMap.clear();
  params.insert(QStringLiteral("options"), QVariantMap());
  params.clear();
  TransientSecret::zeroize(secretB64);
}

void IpcBridge::unprotectEncryptedDataFile(const QString &inputPath,
                                           const QString &outputPath,
                                           QString secretB64) {
  if (!TransientSecret::isCanonicalAES256Base64(secretB64)) {
    TransientSecret::zeroize(secretB64);
    const QString error = it(QStringLiteral(
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

void IpcBridge::exportVerificationReport(const QString &outputPath,
                                         const QVariantMap &details,
                                         const QString &inputPath,
                                         const QString &originalPath) {
  const QString targetPath = outputPath.trimmed();
  if (targetPath.isEmpty()) {
    setStatus(it(QStringLiteral("Debe indicar una ruta de salida para el informe")));
    return;
  }
  if (details.isEmpty()) {
    setStatus(it(QStringLiteral("No hay resultado de verificación para exportar")));
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
    setStatus(it(QStringLiteral("No se pudo guardar el informe de verificación")));
    emit backendLogReceived(it(QStringLiteral("❌ Error guardando informe de verificación: ")) +
                            targetPath);
    return;
  }
  file.write(QJsonDocument(report).toJson(QJsonDocument::Indented));
  if (!file.commit()) {
    setStatus(it(QStringLiteral("No se pudo confirmar el informe de verificación")));
    emit backendLogReceived(it(QStringLiteral("❌ Error confirmando informe de verificación: ")) +
                            targetPath);
    return;
  }
  const QString msg =
      it(QStringLiteral("Informe de verificación guardado: ")) + targetPath;
  setStatus(msg);
  emit backendLogReceived(msg);
}

void IpcBridge::createHash(const QString &inputPath, const QString &outputPath,
                           const QString &algorithm, const QString &format,
                           bool recursive) {
  emit backendLogReceived(
      it(QStringLiteral("🧮 Solicitando creación de huella IPC para: ")) +
      inputPath);
  QVariantMap params;
  params["inputPath"] = inputPath;
  params["outputPath"] = outputPath;
  params["algorithm"] = algorithm;
  params["format"] = format;
  params["recursive"] = recursive;
  sendRequest("hash_create", params);
}

void IpcBridge::checkHash(const QString &inputPath, const QString &hashPath,
                          const QString &outputPath,
                          const QString &algorithm, bool recursive,
                          bool saveReportToDisk) {
  emit backendLogReceived(
      it(QStringLiteral("🔎 Solicitando comprobación de huella IPC para: ")) +
      inputPath);
  QVariantMap params;
  params["inputPath"] = inputPath;
  params["hashPath"] = hashPath;
  params["outputPath"] = outputPath;
  params["algorithm"] = algorithm;
  params["recursive"] = recursive;
  params["saveReportToDisk"] = saveReportToDisk;
  sendRequest("hash_check", params);
}

void IpcBridge::onConnected() {
  const bool hadDeferredRequest = !m_deferredRequest.isEmpty();
  m_requestLocalTLSStartupAfterDeferred = false;
  m_refreshCertificatesAfterLocalTLSStartup = false;
  emit backendLogReceived(
      it(QStringLiteral("✅ Conexión establecida con el motor de firma. estado=")) +
      ipcSocketStateName(m_socket->state()) +
      it(QStringLiteral(", pending=")) + m_pendingAction +
      it(QStringLiteral(", deferred=")) + m_deferredAction);
  m_retryCount = 0;
  setStatus(it(QStringLiteral("Conectado vía IPC")));
  flushDeferredRequest();
  if (hadDeferredRequest) {
    m_requestLocalTLSStartupAfterDeferred = true;
    emit backendLogReceived(
        it(QStringLiteral("ℹ️ Se pospone refreshCertificates para no pisar la acción IPC diferida.")));
    return;
  }
  m_refreshCertificatesAfterLocalTLSStartup = true;
  sendRequest(QStringLiteral("local_tls_startup_status"));
}

void IpcBridge::onError(QLocalSocket::LocalSocketError error) {
  QString errStr = m_socket->errorString();
  emit backendLogReceived(it(QStringLiteral("❌ Error en Socket: código=")) +
                          QString::number(static_cast<int>(error)) +
                          it(QStringLiteral(" mensaje=")) + errStr +
                          it(QStringLiteral(" [estado=")) +
                          ipcSocketStateName(m_socket->state()) +
                          it(QStringLiteral(", pending=")) + m_pendingAction +
                          it(QStringLiteral(", deferred=")) + m_deferredAction +
                          "]");
  setStatus(it(QStringLiteral("Error IPC: ")) + errStr);
  const auto pendingSeals = m_sealPreviewRequests.keys();
  m_sealPreviewRequests.clear();
  for (const QString &id : pendingSeals)
    emit sealPreviewReceived(id, false, QString(),
                             it(QStringLiteral("Se perdió la conexión con el motor de firma")));
  if (!m_shuttingDown)
    failPendingActionDueToConnection(it(QStringLiteral("Error IPC: ")) + errStr);
}

void IpcBridge::onReadyRead() {
  while (m_socket->canReadLine()) {
    QByteArray line = m_socket->readLine();
    QJsonDocument doc = QJsonDocument::fromJson(line);
    if (doc.isNull()) {
      emit backendLogReceived(
          it(QStringLiteral("❌ Respuesta IPC no parseable como JSON.")) +
          it(QStringLiteral(" bytes=")) + QString::number(line.size()));
      continue;
    }
    QJsonObject obj = doc.object();
    bool ok = obj.value("ok").toBool();
    emit backendLogReceived(
        it(QStringLiteral("📥 Respuesta IPC mínima: ")) +
        IncidentFormatIpcLogEvent(
            obj.toVariantMap(), QStringLiteral("ipc-response"),
            ok ? QStringLiteral("success") : QStringLiteral("failure"),
            line.size()));
    QString errMsg = IncidentSanitizeText(obj.value("error").toString());
    QJsonValue data = obj.value("data");
    const QVariantMap diagnostic =
        obj.value("diagnostic").toObject().toVariantMap();
    if (!diagnostic.isEmpty()) {
      setLastDiagnostic(diagnostic);
      emit backendLogReceived(
          it(QStringLiteral("🩺 Diagnóstico guiado: ")) +
          diagnostic.value(QStringLiteral("category")).toString() +
          it(QStringLiteral(" | owner=")) +
          diagnostic.value(QStringLiteral("likelyOwner")).toString());
    }

    // Intentamos obtener la acción del JSON de respuesta (el backend lo añade)
    QString action = obj.value("action").toString();
    if (action.isEmpty()) {
      action = m_pendingAction;
    }
    QString requestId = obj.value("requestId").toString();
    if (requestId.isEmpty()) {
      requestId = m_pendingRequestId;
    }
    QString traceId = obj.value("traceId").toString();
    if (traceId.isEmpty()) {
      traceId = m_pendingTraceId;
    }
    setLastRequestId(requestId);
    setLastTraceId(traceId);
    const bool matchesPendingRequest =
        (!m_pendingRequestId.isEmpty() &&
         requestId == m_pendingRequestId) ||
        (m_pendingRequestId.isEmpty() && !m_pendingAction.isEmpty() &&
         action == m_pendingAction);
    if (matchesPendingRequest) {
      m_pendingAction.clear();
      m_pendingRequestId.clear();
      m_pendingTraceId.clear();
      if (m_requestLocalTLSStartupAfterDeferred) {
        m_requestLocalTLSStartupAfterDeferred = false;
        sendRequest(QStringLiteral("local_tls_startup_status"));
      }
    }

    if (action == "get_token_settings" || action == "save_token_settings" ||
        action == "diagnose_token_settings") {
      // Never accept a stale, unsolicited or uncorrelated administrative reply.
      if (obj.value("requestId").toString().isEmpty() ||
          requestId != m_tokenSettingsRequestId || action != m_tokenSettingsAction)
        continue;
      m_tokenSettingsRequestId.clear();
      m_tokenSettingsAction.clear();
      const QString receivedCode = obj.value("errorCode").toString();
      const QStringList allowedCodes = {
          "token_settings_frontend_required", "token_settings_unavailable",
          "token_settings_invalid", "token_settings_unsafe", "token_settings_conflict",
          "token_settings_confirmation", "token_settings_write", "token_settings_failed"};
      const QString errorCode = allowedCodes.contains(receivedCode)
                                    ? receivedCode : QStringLiteral("token_settings_failed");
      emit tokenSettingsFinished(action, ok && data.isObject(),
                                 data.toObject().toVariantMap(), errorCode);
      continue;
    }

    if (action == QStringLiteral("local_tls_startup_status")) {
      if (ok && data.isObject()) {
        const QJsonObject snapshot = data.toObject();
        const QString state = snapshot.value(QStringLiteral("state")).toString();
        if (state == QStringLiteral("ready") ||
            state == QStringLiteral("error") ||
            state == QStringLiteral("unknown")) {
          const QVariantMap status = snapshot.toVariantMap();
          if (m_localTLSStartupStatus != status) {
            m_localTLSStartupStatus = status;
            emit localTLSStartupStatusChanged();
          }
        }
      }
      if (m_refreshCertificatesAfterLocalTLSStartup) {
        m_refreshCertificatesAfterLocalTLSStartup = false;
        refreshCertificates();
      }
      continue;
    }

    if (action == "pdf_preview") {
      const auto previewIt = m_previewRequests.constFind(requestId);
      if (previewIt == m_previewRequests.constEnd()) {
        emit backendLogReceived(
            it(QStringLiteral("QML: Preview obsoleta ignorada para")));
        continue;
      }
      const PreviewRequestContext preview = previewIt.value();
      m_previewRequests.remove(requestId);
      if (ok) {
        const QJsonObject res = data.toObject();
        emit pdfPreviewReceived(
            requestId, preview.path, preview.page, true,
            res.value("data").toString(), res.value("width").toDouble(),
            res.value("height").toDouble(),
            res.value("currentPage").toInt(),
            res.value("totalPages").toInt());
      } else {
        emit pdfPreviewReceived(requestId, preview.path, preview.page, false,
                                errMsg, 0, 0, 0, 0);
      }
      continue;
    }

    if (action == QStringLiteral("seal_preview")) {
      if (!m_sealPreviewRequests.remove(requestId))
        continue;
      emit sealPreviewReceived(requestId, ok,
                               ok ? data.toObject().value(QStringLiteral("image")).toString() : QString(),
                               ok ? QString() : errMsg);
      continue;
    }

    if (action == QStringLiteral("facturae_create")) {
      emit facturaeCreated(ok, ok ? data.toObject().toVariantMap() : QVariantMap(),
                           ok ? QString() : errMsg);
      continue;
    }
    if (action == QStringLiteral("detect_verifactu")) {
      emit verifactuDetected(ok, ok ? data.toObject().toVariantMap() : QVariantMap());
      continue;
    }
    if (action == QStringLiteral("validate_verifactu")) {
      emit verifactuValidated(ok, ok ? data.toObject().toVariantMap() : QVariantMap(), ok ? QString() : errMsg);
      continue;
    }
    if (action == QStringLiteral("read_verifactu_qr") || action == QStringLiteral("query_verifactu_qr")) {
      emit verifactuQRFinished(action, ok, ok ? data.toObject().toVariantMap() : QVariantMap(), ok ? QString() : errMsg);
      continue;
    }
    if (action == QStringLiteral("validate_invoice")) {
      emit invoiceValidated(ok, ok ? data.toObject().toVariantMap() : QVariantMap(),
                            ok ? QString() : errMsg);
      continue;
    }
    if (action == QStringLiteral("generate_eni_document") ||
        action == QStringLiteral("generate_eni_file")) {
      emit eniGenerated(action, ok, ok ? data.toObject().toVariantMap() : QVariantMap(),
                        ok ? QString() : errMsg);
      continue;
    }

    if (action == QStringLiteral("smartcard_status")) {
      emit smartcardStatusReceived(ok,
                                   data.toObject().value(QStringLiteral("readers")).toArray().toVariantList(),
                                   ok ? QString() : errMsg);
      continue;
    }

    if (action.startsWith(QStringLiteral("csc_"))) {
      emit cscFinished(action, ok,
                       ok ? data.toObject().toVariantMap() : QVariantMap(),
                       ok ? QString() : errMsg);
      if (ok && (action == QStringLiteral("csc_connect") ||
                 action == QStringLiteral("csc_disconnect") ||
                 action == QStringLiteral("csc_configure")))
        refreshCertificates();
      continue;
    }

    if (action == QStringLiteral("protection_recipient_import") ||
        action == QStringLiteral("protection_recipient_remove")) {
      emit protectionRecipientChanged(ok, ok ? QString() : errMsg);
      if (ok)
        loadProtectionRecipients();
      continue;
    }

    // Respuestas de gestion del servicio
    if (action.startsWith("service_")) {
      if (!ok) {
        emit serviceActionFinished(false, errMsg);
      } else if (action == "service_status") {
        // data es un objeto con installed, running, platform, method
        QJsonObject st = data.toObject();
        bool installed = st.value("installed").toBool();
        bool running = st.value("running").toBool();
        QString platform = st.value("platform").toString();
        QString method = st.value("method").toString();
        emit serviceStatusReceived(installed, running, platform, method);
      } else {
        emit serviceActionFinished(true, data.toString());
      }
      continue;
    }

    if (!ok) {
      emit backendLogReceived(it(QStringLiteral("Error IPC: ")) + errMsg);
      emit backendLogReceived(ipcOriginHint(action, errMsg));
      if (action == "verify") {
        setStatus(errMsg);
        emit verificationFinished(false, errMsg, QVariantMap());
      } else if (action == "protect" || action == "protect_sign") {
        setStatus(errMsg);
        emit protectionFinished(false, errMsg, QVariantMap());
      } else if (action == "unprotect") {
        setStatus(errMsg);
        emit unprotectionFinished(false, errMsg, QVariantMap());
      } else if (action == "save_settings") {
        emit settingsSaved(false, errMsg);
      } else if (action == "proxy_secret_store" ||
                 action == "proxy_secret_delete") {
        emit proxyCredentialsFinished(false, errMsg, false, QString(),
                                      QString());
      } else if (action == "validate_certificate_online") {
        emit certificateOnlineCheckFinished(false, errMsg, QVariantMap());
      } else if (action == "certificate_export_public") {
        emit certificatePublicExportFinished(false, errMsg);
      } else if (action == "hash_create") {
        emit hashCreateFinished(false, errMsg, QVariantMap());
      } else if (action == "hash_check") {
        emit hashCheckFinished(false, errMsg, QVariantMap());
      } else if (action == "check_updates") {
        emit updateCheckFinished(false, errMsg, QVariantMap());
      } else if (action == "sign_batch") {
        emit batchSigningFinished(false, errMsg, QVariantList());
      } else if (action == "certificate_access_options") {
        emit certificateAccessOptionsLoaded(false, QVariantMap(), errMsg);
      } else if (action == "import_certificate" ||
                 action == "import_certificate_to_store") {
        emit certificateImportFinished(false, errMsg);
      } else if (action == "use_temporary_certificate" ||
                 action == "remove_temporary_certificate") {
        emit temporaryCertificateFinished(false, errMsg, QVariantMap());
      } else if (action == "clear_temporary_certificates") {
        emit temporaryCertificatesCleared(false, errMsg);
      } else if (action == "open_certificate_manager") {
        setStatus(errMsg);
      } else {
        emit signingFinished(false, errMsg, "");
      }
      continue;
    }

    // --- LOGICA BASADA EN ACCION (PREFERIDA) ---
    if (action == "certificate_access_options") {
      emit certificateAccessOptionsLoaded(
          true, data.toObject().toVariantMap(),
          it(QStringLiteral("Opciones de certificados cargadas.")));
      continue;
    }

    if (action == "certificate_export_public") {
      const QString message = it(QStringLiteral("Es su certificado público: puede enviarlo sin riesgo. Quien lo reciba podrá proteger archivos que solo usted podrá abrir con GrxFirma (Desproteger)."));
      setStatus(message);
      emit certificatePublicExportFinished(true, message);
      continue;
    }

    if (action == "open_certificate_manager") {
      const QString message =
          it(QStringLiteral("Gestor de certificados abierto."));
      setStatus(message);
      emit backendLogReceived(message);
      continue;
    }

    if (action == "import_certificate_to_store") {
      const QString message = it(QStringLiteral(
          "Certificado importado en el almacén seleccionado."));
      setStatus(message);
      emit certificateImportFinished(true, message);
      refreshCertificates();
      continue;
    }

    if (action == "use_temporary_certificate") {
      const QVariantMap certificate = data.toObject().toVariantMap();
      const QString message = it(QStringLiteral(
          "Credencial cargada solo para esta sesión; no se ha instalado."));
      setStatus(message);
      emit temporaryCertificateFinished(true, message, certificate);
      refreshCertificates();
      continue;
    }

    if (action == "remove_temporary_certificate") {
      const QString message =
          it(QStringLiteral("Credencial temporal retirada de la sesión."));
      setStatus(message);
      emit temporaryCertificateFinished(true, message, QVariantMap());
      refreshCertificates();
      continue;
    }

    if (action == "clear_temporary_certificates") {
      const QString message = it(QStringLiteral("Operación completada"));
      setStatus(message);
      emit temporaryCertificatesCleared(true, message);
      refreshCertificates();
      continue;
    }

    if (action == "tls_diagnostics") {
      const QJsonObject tlsStore = data.toObject();
      const QString report =
          it(QStringLiteral(
                 "Diagnóstico TLS:\nEstado del almacén: %1.\nArtefactos: %2 (certificados: %3, claves: %4)."))
              .arg(tlsStore.value("state").toString())
              .arg(tlsStore.value("artifactCount").toInt())
              .arg(tlsStore.value("certificateCount").toInt())
              .arg(tlsStore.value("keyCount").toInt());
      emit backendLogReceived(report);
      setStatus(it(QStringLiteral("Diagnóstico TLS finalizado")));
      continue;
    }

    if (action == "clear_tls_trust") {
      emit backendLogReceived(
          it(QStringLiteral("Certificados eliminados del almacén: %1"))
              .arg(data.toInt()));
      setStatus(it(QStringLiteral("Almacén TLS limpiado.")));
      continue;
    }

    if (action == "export_diagnostic") {
      const QJsonObject res = data.toObject();
      const QJsonObject tlsStore = res.value("tlsStore").toObject();
      const QString report =
          it(QStringLiteral(
                 "Diagnóstico: %1 certs encontrados, %2 válidos para firmar.\nAlmacén TLS: estado %3; %4 artefactos (%5 certificados, %6 claves)."))
              .arg(res.value("certificates").toInt())
              .arg(res.value("canSign").toInt())
              .arg(tlsStore.value("state").toString())
              .arg(tlsStore.value("artifactCount").toInt())
              .arg(tlsStore.value("certificateCount").toInt())
              .arg(tlsStore.value("keyCount").toInt());

      emit backendLogReceived(report);
      setStatus(it(QStringLiteral("Resumen de incidencia preparado para soporte.")));

      // Copiar al portapapeles automáticamente
      QGuiApplication::clipboard()->setText(report);
      emit backendLogReceived(
          it(QStringLiteral("ℹ️ El resumen técnico para soporte se ha copiado al portapapeles.")));
      continue;
    }

    if (action == "get_settings") {
      emit settingsLoaded(data.toObject().toVariantMap());
      setStatus(it(QStringLiteral("Configuración cargada")));
      continue;
    }

    if (action == "check_updates") {
      emit updateCheckFinished(
          true, it(QStringLiteral("Comprobación de versiones finalizada.")),
          data.toObject().toVariantMap());
      continue;
    }

    if (action == "proxy_secret_store_status") {
      QJsonObject st = data.toObject();
      emit proxySecretStoreStatusReceived(
          st.value("available").toBool(),
          st.value("platform").toString(),
          st.value("backend").toString(),
          st.value("reason").toString(),
          st.value("runtimeProxyMode").toString());
      continue;
    }

    if (action == "proxy_secret_store" || action == "proxy_secret_delete") {
      const QJsonObject result = data.toObject();
      const bool configured = result.value("configured").toBool();
      const QString message =
          action == "proxy_secret_store"
              ? it(QStringLiteral("Configuración guardada"))
              : it(QStringLiteral("Operación completada"));
      emit proxyCredentialsFinished(
          true, message, configured, result.value("realm").toString(),
          result.value("username").toString());
      setStatus(message);
      continue;
    }

    if (action == "validate_certificate_online") {
      QJsonObject res = data.toObject();
      const QString message =
          res.value("userMessage").toString().trimmed().isEmpty()
              ? it(QStringLiteral("Comprobación online del certificado finalizada."))
              : res.value("userMessage").toString();
      emit certificateOnlineCheckFinished(true, message, res.toVariantMap());
      setStatus(message);
      continue;
    }

    if (action == "save_settings") {
      const QString message = it(QStringLiteral("Configuración guardada"));
      emit backendLogReceived(
          it(QStringLiteral("Configuración guardada correctamente")));
      setStatus(message);
      emit settingsSaved(true, message);
      continue;
    }

    if (action == "check_certificates") {
      QJsonObject res = data.toObject();
      QVariantList certs;
      QJsonArray arr = res.value("certificates").toArray();
      for (const auto &v : arr)
        certs << v.toVariant();
      emit certificatesLoaded(certs);
      int ok = res.value("okCount").toInt();
      int fail = res.value("failCount").toInt();
      QString msg =
          it(QStringLiteral("Chequeo finalizado: %1 válidos, %2 fallidos."))
              .arg(ok)
              .arg(fail);
      setStatus(msg);
      emit backendLogReceived(it(QStringLiteral("ℹ️ ")) + msg);
      continue;
    }

    if (action == "protection_recipients") {
      QJsonObject res = data.toObject();
      QVariantList recipients;
      QJsonArray arr = res.value("recipients").toArray();
      for (const auto &v : arr)
        recipients << v.toVariant();
      emit protectionRecipientsLoaded(recipients);
      setStatus(it(QStringLiteral("Destinatarios de protección cargados")));
      continue;
    }

    if (action == "protect" || action == "protect_sign") {
      QJsonObject res = data.toObject();
      QString msg = (action == "protect_sign")
                        ? it(QStringLiteral("Documento protegido y firmado correctamente"))
                        : it(QStringLiteral("Documento protegido correctamente"));
      if (!res.value("outputPath").toString().isEmpty()) {
        msg += it(QStringLiteral(": ")) + res.value("outputPath").toString();
      }
      setStatus(msg);
      emit protectionFinished(true, msg, res.toVariantMap());
      continue;
    }

    if (action == "unprotect") {
      QJsonObject res = data.toObject();
      QString msg = it(QStringLiteral("Documento desprotegido correctamente"));
      if (!res.value("outputPath").toString().isEmpty()) {
        msg += it(QStringLiteral(": ")) + res.value("outputPath").toString();
      }
      setStatus(msg);
      emit unprotectionFinished(true, msg, res.toVariantMap());
      continue;
    }

    if (action == "sign_batch") {
      QJsonObject res = data.toObject();
      QVariantList results;
      QJsonArray arr = res.value("results").toArray();
      for (const auto &v : arr)
        results << v.toVariant();
      int okCount = res.value("okCount").toInt();
      int failCount = res.value("failCount").toInt();
      QString msg =
          it(QStringLiteral("Lote completado: %1 correctos, %2 fallidos."))
              .arg(okCount)
              .arg(failCount);
      setStatus(msg);
      emit batchSigningFinished(failCount == 0, msg, results);
      continue;
    }

    if (action == "hash_create") {
      QJsonObject res = data.toObject();
      QString msg = it(QStringLiteral("Huella generada correctamente"));
      if (!res.value("outputPath").toString().isEmpty()) {
        msg += it(QStringLiteral(": ")) + res.value("outputPath").toString();
      }
      setStatus(msg);
      emit hashCreateFinished(true, msg, res.toVariantMap());
      continue;
    }

    if (action == "hash_check") {
      QJsonObject res = data.toObject();
      bool valid = res.value("valid").toBool();
      QString msg = valid ? it(QStringLiteral("Huella válida"))
                          : it(QStringLiteral("Huella no válida"));
      if (!res.value("reportOutputPath").toString().isEmpty()) {
        msg += it(QStringLiteral(": ")) + res.value("reportOutputPath").toString();
      }
      setStatus(msg);
      emit hashCheckFinished(true, msg, res.toVariantMap());
      continue;
    }

    // --- LOGICA BASADA EN ANALISIS DE TIPO (FALLBACK/GENERAL) ---
    if (data.isArray()) {
      QVariantList certs;
      QJsonArray arr = data.toArray();
      for (const auto &v : arr)
        certs << v.toVariant();
      emit certificatesLoaded(certs);
      setStatus(it(QStringLiteral("Certificados cargados")));
    } else if (data.isObject()) {
      QJsonObject res = data.toObject();
      if (res.contains("OutputPath")) {
        QString out = res.value("OutputPath").toString();
        emit backendLogReceived(it(QStringLiteral("Firma completada: ")) + out);
        emit signingFinished(true,
                             it(QStringLiteral("Firma completada correctamente")),
                             out);
      } else if (res.contains("valid")) {
        bool valid = res.value("valid").toBool();
        QVariantMap details =
            (res.contains("result") && res.value("result").isObject())
                ? res.value("result").toObject().toVariantMap()
                : res.toVariantMap();
        QString msg =
            valid ? it(QStringLiteral("Firma válida"))
                  : it(QStringLiteral("Firma no válida: ")) +
						it(res.value("reason").toString());
        setStatus(msg);
        emit verificationFinished(true, msg, details);
      }
    } else if (action == "import_certificate") {
      emit certificateImportFinished(
          ok, ok ? it(QStringLiteral("Certificado importado correctamente"))
                 : errMsg);
      if (ok) {
        refreshCertificates();
      }
    } else if (action == "install_public_roots") {
      emit publicRootsInstallationFinished(
          ok, ok ? it(QStringLiteral("Confianza TLS local instalada correctamente"))
                 : errMsg);
    }
  }
}

void IpcBridge::sendRequest(const QString &action, const QVariantMap &params) {
  if (!m_socket->isOpen() ||
      m_socket->state() != QLocalSocket::ConnectedState) {
    emit backendLogReceived(it(QStringLiteral("⚠️ No hay conexión con el motor. estado=")) +
                            ipcSocketStateName(m_socket->state()) +
                            it(QStringLiteral(", action=")) + action);
    if (!queueDeferredRequest(action, params)) {
      failActionDueToConnection(
          action, it(QStringLiteral("Sin conexión con el motor de firma")));
    }
    return;
  }
  QJsonObject req;
  const quint64 seq = ++m_requestSeq;
  const QString requestId = ipcNewCorrelationId(QStringLiteral("ipc"), seq);
  const QString traceId = ipcNewCorrelationId(QStringLiteral("trace"), seq);
  req.insert("requestId", requestId);
  req.insert("traceId", traceId);
  req.insert("action", action);
  req.insert("params", QJsonObject::fromVariantMap(params));

  QByteArray data = QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n";
  emit backendLogReceived(
      it(QStringLiteral("📤 Petición IPC mínima: ")) +
      IncidentFormatIpcLogEvent(
          req.toVariantMap(), QStringLiteral("ipc-request"),
          QStringLiteral("pending"), data.size()));
  m_pendingAction = action;
  m_pendingRequestId = requestId;
  m_pendingTraceId = traceId;
  setLastRequestId(requestId);
  setLastTraceId(traceId);
  if (action == m_tokenSettingsAction) {
    m_tokenSettingsRequestId = requestId;
    QTimer::singleShot(20000, this, [this, requestId, action]() {
      if (m_tokenSettingsRequestId != requestId)
        return;
      m_tokenSettingsRequestId.clear();
      m_tokenSettingsAction.clear();
      emit tokenSettingsFinished(action, false, QVariantMap(),
                                 it(QStringLiteral("token_settings.timeout")));
    });
  }
  if (m_socket->write(data) == -1) {
    data.fill('\0');
    QString err = it(QStringLiteral("No se pudo enviar la petición IPC: ")) +
                  m_socket->errorString();
    emit backendLogReceived(it(QStringLiteral("❌ ")) + err);
    if (!queueDeferredRequest(action, params))
      failActionDueToConnection(action, err);
    return;
  }
  m_socket->flush();
  data.fill('\0');
  emit backendLogReceived(
      it(QStringLiteral("📬 Petición IPC enviada: ")) +
      IncidentFormatIpcLogEvent(
          req.toVariantMap(), QStringLiteral("ipc-request"),
          QStringLiteral("sent")));
}

void IpcBridge::openExternal(const QString &path) {
  if (path.isEmpty())
    return;
  if (path.startsWith("http://") || path.startsWith("https://")) {
    if (!QDesktopServices::openUrl(QUrl(path))) {
      emit backendLogReceived(it(QStringLiteral("❌ No se pudo abrir la URL: ")) +
                              path);
      setStatus(it(QStringLiteral("No se pudo abrir el recurso solicitado")));
    }
    return;
  }

  QString localPath = path;
  if (localPath.startsWith("file://")) {
    localPath = QUrl(path).toLocalFile();
  }
  QFileInfo info(localPath);
  if (!info.exists()) {
    emit backendLogReceived(it(QStringLiteral("❌ El fichero no existe: ")) +
                            localPath);
    setStatus(it(QStringLiteral("El fichero indicado no existe")));
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

  emit backendLogReceived(it(QStringLiteral("❌ No se pudo abrir el fichero: ")) +
                          localPath);
  setStatus(it(QStringLiteral("No se pudo abrir el fichero firmado")));
}

void IpcBridge::openSignedDocument(const QString &path) {
  const QString localPath = path.startsWith(QStringLiteral("file://"))
                                ? QUrl(path).toLocalFile()
                                : path;
  const QFileInfo info(localPath);
  if (!info.isFile()) {
    setStatus(it(QStringLiteral("El fichero indicado no existe")));
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
  setStatus(it(QStringLiteral("No se pudo abrir el fichero firmado")));
}

void IpcBridge::openCertManager() {
#ifdef Q_OS_WIN
  QProcess::startDetached("rundll32.exe", {"cryptext.dll,CryptExtOpenCER"});
#elif defined(Q_OS_MACOS)
  QProcess::startDetached(
      "open", {"/System/Applications/Utilities/Keychain Access.app"});
#else
  // Linux: intentamos abrir gestores comunes
  bool opened = false;
  QStringList tools = {"seahorse", "kleopatra", "gcr-viewer"};
  for (const QString &tool : tools) {
    if (QProcess::startDetached(tool, {})) {
      opened = true;
      break;
    }
  }
  if (!opened) {
    // Fallback: abrir manual o dar guia
    emit backendLogReceived(it(QStringLiteral(
        "ℹ️ No se encontró un gestor de certificados nativo (seahorse/kleopatra).")));
    setStatus(
        it(QStringLiteral("Abra la configuración de certificados de su navegador.")));
    QDesktopServices::openUrl(
        QUrl("https://github.com/aavidad/GrxFirma"));
  }
#endif
}

void IpcBridge::openLogFolder() {
  QString base =
      QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation);
  if (base.isEmpty()) {
    QString home =
        QStandardPaths::writableLocation(QStandardPaths::HomeLocation);
    base = home.isEmpty() ? QDir::tempPath() : home + "/.local/state/grxfirma";
  }
  QString path = QDir(base).filePath("logs");

  QDir d(path);
  if (!d.exists()) {
    d.mkpath(".");
  }
  QDesktopServices::openUrl(QUrl::fromLocalFile(path));
}

void IpcBridge::openIncidentFolder() {
  QDesktopServices::openUrl(
      QUrl::fromLocalFile(ipcIncidentReportsDirPath()));
}

void IpcBridge::openHelpManual() {
  QString appDir = QCoreApplication::applicationDirPath();
  QString locale = TranslatorBridge::shared() ? TranslatorBridge::shared()->locale()
                                              : QStringLiteral("es");
  QString manual = resolveLocalizedHelpManualIPC(appDir, locale);
  if (!manual.isEmpty()) {
    QDesktopServices::openUrl(QUrl::fromLocalFile(manual));
  } else {
    openLocalizedHelpFallbackIPC();
  }
}

void IpcBridge::checkRestHealth(const QString &addr, bool useTLS) {
  QUrl url = IpcBridgeMakeUrl(addr, "/health", useTLS);
  QNetworkRequest req(url);
  QNetworkReply *reply = m_nam->get(req);
  IpcBridgeAllowLocalTLSErrors(reply, IpcBridgeNormalizeRestAddr(addr),
                               useTLS);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    bool running = false;
    QString message;
    if (reply->error() == QNetworkReply::NoError) {
      running = true;
      message = it(QStringLiteral("Servidor REST activo"));
    } else {
      message = reply->errorString();
    }
    emit restHealthChecked(running, message);
    reply->deleteLater();
  });
}

void IpcBridge::checkCertificates() {
  QString modeLabel = m_serverMode.toUpper();
  if (modeLabel == "AMBAS")
    modeLabel = "REST+IPC";
  emit backendLogReceived(
      it(QStringLiteral("⚙ Realizando chequeo exhaustivo de certificados vía %1..."))
          .arg(modeLabel));

  if (m_serverMode == "rest") {
    QUrl url = IpcBridgeMakeUrl(m_addr, "/certificates?check=true", m_useTLS);
    QNetworkRequest req(url);
    if (!m_token.isEmpty())
      req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

    QNetworkReply *reply = m_nam->get(req);
    IpcBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
    connect(reply, &QNetworkReply::finished, this, [this, reply]() {
      if (reply->error() == QNetworkReply::NoError) {
        QByteArray data = reply->readAll();
        QJsonDocument doc = QJsonDocument::fromJson(data);
        QJsonArray certs = doc.object().value("certificates").toArray();
        QVariantList list;
        for (const auto &c : certs)
          list << c.toVariant();
        emit certificatesLoaded(list);
        setStatus(it(QStringLiteral("Chequeo finalizado (REST)")));
      } else {
        emit backendLogReceived(
            it(QStringLiteral("Error REST: ")) + reply->errorString());
      }
      reply->deleteLater();
    });
  } else {
    sendRequest("check_certificates");
  }
}

bool IpcBridge::updateEngineAvailable() const {
  return m_socket && m_socket->state() == QLocalSocket::ConnectedState;
}

void IpcBridge::checkUpdates() {
  m_pendingAction = QStringLiteral("check_updates");
  sendRequest(QStringLiteral("check_updates"));
}

void IpcBridge::runTLSDiagnostics() {
  emit backendLogReceived(it(QStringLiteral("⚙ Iniciando diagnóstico TLS (")) +
                          m_serverMode.toUpper() + ")...");
  if (m_serverMode == "rest") {
    QUrl url = IpcBridgeMakeUrl(m_addr, "/tls/trust-status", m_useTLS);
    QNetworkRequest req(url);
    if (!m_token.isEmpty())
      req.setRawHeader("Authorization", "Bearer " + m_token.toUtf8());

    QNetworkReply *reply = m_nam->get(req);
    IpcBridgeAllowLocalTLSErrors(reply, m_addr, m_useTLS);
    connect(reply, &QNetworkReply::finished, this, [this, reply]() {
      if (reply->error() == QNetworkReply::NoError) {
        QByteArray data = reply->readAll();
        emit backendLogReceived(it(QStringLiteral("Diagnóstico TLS recibido vía REST: ")) +
                                QString::fromUtf8(data));
      } else {
        emit backendLogReceived(it(QStringLiteral("Error diagnóstico TLS (REST): ")) +
                                reply->errorString());
      }
      reply->deleteLater();
    });
  } else {
    sendRequest("tls_diagnostics", QVariantMap());
  }
}

void IpcBridge::exportDiagnosticReport() {
  sendRequest("export_diagnostic", QVariantMap());
}

void IpcBridge::clearTLSTrustStore() {
  sendRequest("clear_tls_trust", QVariantMap());
}

void IpcBridge::reinstallBrowserConnectors() {
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
    emit backendLogReceived(it(QStringLiteral("❌ No se encontró el instalador de conectores de navegador.")));
    setStatus(it(QStringLiteral("No se encontraron conectores para reinstalar")));
    return;
  }

  auto *process = new QProcess(this);
  QProcessEnvironment env = ChildProcessEnvironment::sanitized();
  env.insert(QStringLiteral("GRXFIRMA_TARGET_HOME"), home);
  env.insert(QStringLiteral("GRXFIRMA_DESKTOP_ID"), QStringLiteral("grxfirma.desktop"));
  const QString localBridge = home + "/.local/lib/grxfirma/bin/browser-bridge.sh";
  if (QFileInfo::exists(localBridge))
    env.insert(QStringLiteral("GRXFIRMA_BROWSER_BRIDGE"), localBridge);
  const QString localXpi = home + "/.local/lib/grxfirma/extensions/firefox/dipgra-extension-firefox.xpi";
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
            setStatus(ok ? it(QStringLiteral("Conectores de navegador reinstalados. Reinicia el navegador."))
                         : it(QStringLiteral("Error reinstalando conectores de navegador")));
            emit backendLogReceived(ok ? it(QStringLiteral("✅ Conectores de navegador reinstalados."))
                                       : it(QStringLiteral("❌ Error reinstalando conectores de navegador.")));
            process->deleteLater();
          });
  emit backendLogReceived(it(QStringLiteral("⚙ Reinstalando conectores de navegador...")));
  process->start(helper);
#else
  emit backendLogReceived(it(QStringLiteral("La reinstalación de conectores desde la app solo está disponible en Linux por ahora.")));
#endif
}

// ── Gestión del servicio de usuario (via IPC)
// ─────────────────────────────────

void IpcBridge::getServiceStatus() {
  emit backendLogReceived(
      it(QStringLiteral("Consultando estado del servicio...")));
  QJsonObject req;
  req.insert("action", "service_status");
  req.insert("params", QJsonObject());
  QByteArray data = QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n";
  if (!m_socket->isOpen()) {
    emit serviceActionFinished(false,
                               it(QStringLiteral("Sin conexión con el motor")));
    return;
  }
  // La respuesta la procesa onReadyRead → parseServiceResponse
  m_pendingAction = "service_status";
  m_socket->write(data);
}

void IpcBridge::installService() {
  QJsonObject params;
  params.insert("ipcSocket", m_socketPath);
  QJsonObject req;
  req.insert("action", "service_install");
  req.insert("params", params);
  if (!m_socket->isOpen()) {
    emit serviceActionFinished(false, it(QStringLiteral("Sin conexión")));
    return;
  }
  m_pendingAction = "service_install";
  m_socket->write(QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n");
}

void IpcBridge::uninstallService() {
  QJsonObject req;
  req.insert("action", "service_uninstall");
  req.insert("params", QJsonObject());
  if (!m_socket->isOpen()) {
    emit serviceActionFinished(false, it(QStringLiteral("Sin conexión")));
    return;
  }
  m_pendingAction = "service_uninstall";
  m_socket->write(QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n");
}

void IpcBridge::startService() {
  QJsonObject req;
  req.insert("action", "service_start");
  req.insert("params", QJsonObject());
  if (!m_socket->isOpen()) {
    emit serviceActionFinished(false, it(QStringLiteral("Sin conexión")));
    return;
  }
  m_pendingAction = "service_start";
  m_socket->write(QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n");
}

void IpcBridge::stopService() {
  QJsonObject req;
  req.insert("action", "service_stop");
  req.insert("params", QJsonObject());
  if (!m_socket->isOpen()) {
    emit serviceActionFinished(false, it(QStringLiteral("Sin conexión")));
    return;
  }
  m_pendingAction = "service_stop";
  m_socket->write(QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n");
}

void IpcBridge::getSettings() {
  m_pendingAction = "get_settings";
  sendRequest("get_settings");
}

void IpcBridge::sendTokenSettingsRequest(const QString &action, const QVariantMap &params) {
  if (!m_tokenSettingsAction.isEmpty())
    return;
  m_tokenSettingsAction = action;
  sendRequest(action, params);
}

void IpcBridge::getTokenSettings() {
  sendTokenSettingsRequest(QStringLiteral("get_token_settings"));
}

void IpcBridge::diagnoseTokenSettings() {
  sendTokenSettingsRequest(QStringLiteral("diagnose_token_settings"));
}

void IpcBridge::saveTokenSettings(const QVariantMap &settings) {
  sendTokenSettingsRequest(QStringLiteral("save_token_settings"), settings);
}

QString IpcBridge::tokenModuleLocalPath(const QUrl &url) const {
  if (!url.isLocalFile() || !url.host().isEmpty())
    return QString();
  const QString path = url.toLocalFile();
  return QFileInfo(path).isAbsolute() ? path : QString();
}

void IpcBridge::saveSettings(const QVariantMap &settings) {
  m_pendingAction = "save_settings";
  sendRequest("save_settings", settings);
}

void IpcBridge::getProxySecretStoreStatus() {
  m_pendingAction = "proxy_secret_store_status";
  sendRequest("proxy_secret_store_status");
}

void IpcBridge::storeProxyCredentials(const QString &realm,
                                      const QString &username,
                                      QString password) {
  QByteArray passwordBytes = password.toUtf8();
  QVariantMap params;
  params.insert(QStringLiteral("realm"), realm);
  params.insert(QStringLiteral("username"), username);
  // Go decodifica esta cadena Base64 directamente en []byte zeroizable.
  QByteArray encodedPassword = passwordBytes.toBase64();
  params.insert(QStringLiteral("password"),
                QString::fromLatin1(encodedPassword));
  password.fill(QChar('\0'));
  password.clear();
  passwordBytes.fill('\0');
  passwordBytes.clear();
  encodedPassword.fill('\0');
  encodedPassword.clear();

  m_pendingAction = QStringLiteral("proxy_secret_store");
  sendRequest(QStringLiteral("proxy_secret_store"), params);
  params.insert(QStringLiteral("password"), QString());
  params.clear();
}

void IpcBridge::deleteProxyCredentials() {
  m_pendingAction = QStringLiteral("proxy_secret_delete");
  sendRequest(QStringLiteral("proxy_secret_delete"));
}

void IpcBridge::checkCertificateOnline(const QString &certificateId) {
  m_pendingAction = "validate_certificate_online";
  QVariantMap params;
  params.insert(QStringLiteral("certificateId"), certificateId);
  sendRequest("validate_certificate_online", params);
}

void IpcBridge::getPdfPreview(const QString &path, int page,
                              const QString &requestId) {
  const int requestedPage = qMax(1, page);
  const quint64 seq = ++m_requestSeq;
  const QString previewRequestId =
      requestId.trimmed().isEmpty()
          ? ipcNewCorrelationId(QStringLiteral("preview-ipc"), seq)
          : requestId.trimmed();
  const QString traceId =
      ipcNewCorrelationId(QStringLiteral("preview-trace"), seq);
  QJsonObject params;
  params.insert("path", path);
  params.insert("page", requestedPage);

  QJsonObject req;
  req.insert("requestId", previewRequestId);
  req.insert("traceId", traceId);
  req.insert("action", "pdf_preview");
  req.insert("params", params);

  if (!m_socket->isOpen() ||
      m_socket->state() != QLocalSocket::ConnectedState) {
    emit pdfPreviewReceived(previewRequestId, path, requestedPage, false,
                            it(QStringLiteral("Sin conexión IPC")), 0, 0, 0,
                            0);
    return;
  }
  // La GUI solo mantiene una previsualización activa. Conservar únicamente
  // su contexto permite descartar en el propio puente respuestas tardías.
  m_previewRequests.clear();
  m_previewRequests.insert(
      previewRequestId, PreviewRequestContext{path, requestedPage});
  setLastRequestId(previewRequestId);
  setLastTraceId(traceId);
  const QByteArray payload =
      QJsonDocument(req).toJson(QJsonDocument::Compact) + "\n";
  if (m_socket->write(payload) == -1) {
    m_previewRequests.remove(previewRequestId);
    emit pdfPreviewReceived(
        previewRequestId, path, requestedPage, false,
        it(QStringLiteral("No se pudo enviar la petición IPC: ")) +
            m_socket->errorString(),
        0, 0, 0, 0);
    return;
  }
  m_socket->flush();
}

void IpcBridge::importCertificate(const QString &path,
                                  const QString &password) {
  QByteArray data;
  QString error;
  if (!readCredentialFileIPC(path, &data, &error)) {
    emit certificateImportFinished(false, error);
    return;
  }

  QVariantMap params;
  QByteArray encoded = data.toBase64();
  params.insert("p12B64", QString::fromLatin1(encoded));
  params.insert("password", password);
  encoded.fill('\0');
  data.fill('\0');
  data.clear();

  emit backendLogReceived(
      it(QStringLiteral("⚙ Iniciando importación de certificado IPC...")));
  m_pendingAction = "import_certificate";
  sendRequest("import_certificate", params);
  params.insert("p12B64", QString());
  params.insert("password", QString());
}

void IpcBridge::requestCertificateAccessOptions() {
  m_pendingAction = QStringLiteral("certificate_access_options");
  sendRequest(QStringLiteral("certificate_access_options"));
}

void IpcBridge::openCertificateManager(const QString &managerId) {
  QVariantMap params;
  params.insert(QStringLiteral("managerId"), managerId.trimmed());
  m_pendingAction = QStringLiteral("open_certificate_manager");
  sendRequest(QStringLiteral("open_certificate_manager"), params);
}

void IpcBridge::importCertificateToStore(const QString &path,
                                         const QString &password,
                                         const QString &targetId) {
  QByteArray data;
  QString error;
  if (!readCredentialFileIPC(path, &data, &error)) {
    emit certificateImportFinished(false, error);
    return;
  }
  QVariantMap params;
  QByteArray encoded = data.toBase64();
  params.insert(QStringLiteral("credentialB64"), QString::fromLatin1(encoded));
  params.insert(QStringLiteral("password"), password);
  params.insert(QStringLiteral("targetId"), targetId.trimmed());
  encoded.fill('\0');
  data.fill('\0');
  data.clear();
  m_pendingAction = QStringLiteral("import_certificate_to_store");
  sendRequest(QStringLiteral("import_certificate_to_store"), params);
  params.insert(QStringLiteral("credentialB64"), QString());
  params.insert(QStringLiteral("password"), QString());
}

void IpcBridge::useTemporaryCertificate(const QString &path,
                                        const QString &password) {
  QByteArray data;
  QString error;
  if (!readCredentialFileIPC(path, &data, &error)) {
    emit temporaryCertificateFinished(false, error, QVariantMap());
    return;
  }
  QVariantMap params;
  QByteArray encoded = data.toBase64();
  params.insert(QStringLiteral("credentialB64"), QString::fromLatin1(encoded));
  params.insert(QStringLiteral("password"), password);
  encoded.fill('\0');
  data.fill('\0');
  data.clear();
  m_pendingAction = QStringLiteral("use_temporary_certificate");
  sendRequest(QStringLiteral("use_temporary_certificate"), params);
  params.insert(QStringLiteral("credentialB64"), QString());
  params.insert(QStringLiteral("password"), QString());
}

void IpcBridge::removeTemporaryCertificate(const QString &certificateId) {
  QVariantMap params;
  params.insert(QStringLiteral("certificateId"), certificateId.trimmed());
  m_pendingAction = QStringLiteral("remove_temporary_certificate");
  sendRequest(QStringLiteral("remove_temporary_certificate"), params);
}

void IpcBridge::clearTemporaryCertificates() {
  m_pendingAction = QStringLiteral("clear_temporary_certificates");
  sendRequest(QStringLiteral("clear_temporary_certificates"));
}

void IpcBridge::saveTextReport(const QString &outputPath,
                               const QString &content) {
  QString localPath = outputPath;
  if (localPath.startsWith("file://")) {
    localPath = QUrl(outputPath).toLocalFile();
  }
  if (localPath.trimmed().isEmpty()) {
    setStatus(it(QStringLiteral("No se pudo guardar el informe.")));
    return;
  }

  QSaveFile file(localPath);
  if (!file.open(QIODevice::WriteOnly | QIODevice::Text)) {
    setStatus(it(QStringLiteral("No se pudo guardar el informe.")));
    return;
  }
  file.write(content.toUtf8());
  if (!file.commit()) {
    setStatus(it(QStringLiteral("No se pudo guardar el informe.")));
    return;
  }
  setStatus(it(QStringLiteral("Informe guardado correctamente.")));
}

QString IpcBridge::incidentReportsDir() const {
  return ipcIncidentReportsDirPath();
}

QString IpcBridge::defaultIncidentReportPath(const QString &kind) const {
  return ipcDefaultIncidentReportPath(kind);
}

QString IpcBridge::saveIncidentReport(const QVariantMap &report,
                                      const QString &kind) {
  QVariantMap payload = report;
  if (!payload.contains("generatedAt")) {
    payload.insert("generatedAt",
                   QDateTime::currentDateTimeUtc().toString(Qt::ISODate));
  }
  payload = IncidentSanitizePayload(payload, false);
  const QString targetPath = ipcDefaultIncidentReportPath(kind);
  if (targetPath.isEmpty()) {
    setStatus(it(QStringLiteral("No se pudo preparar el directorio privado de incidencias.")));
    emit backendLogReceived(
        it(QStringLiteral("❌ No se pudo preparar el almacenamiento privado de incidencias.")));
    return QString();
  }
  const QString summaryPath = ipcIncidentTextPathForJson(targetPath);
  const QString logTailPath = ipcIncidentLogTailPathForJson(targetPath);
  const QString manifestPath = ipcIncidentManifestPathForJson(targetPath);
  const QString previewPath = ipcIncidentPreviewPathForJson(targetPath);
  QString writeError;
  const QByteArray incidentJSON =
      QJsonDocument(QJsonObject::fromVariantMap(payload))
          .toJson(QJsonDocument::Indented);
  if (!IncidentWritePrivateFile(targetPath, incidentJSON, &writeError)) {
    setStatus(it(QStringLiteral("No se pudo guardar la incidencia.")));
    emit backendLogReceived(
        it(QStringLiteral("❌ No se pudo guardar la incidencia saneada.")));
    return QString();
  }
  const QString summaryText =
      ipcRedactSupportText(payload.value(QStringLiteral("summaryText")).toString().trimmed());
  const bool wroteSummary =
      !summaryText.isEmpty() &&
      IncidentWritePrivateFile(summaryPath,
                               summaryText.toUtf8() + QByteArrayLiteral("\n"));
  const QString logTail = ipcReadLogTailRedacted(ipcGuiLogPath());
  const bool wroteLogTail =
      !logTail.isEmpty() &&
      IncidentWritePrivateFile(logTailPath,
                               logTail.toUtf8() + QByteArrayLiteral("\n"));
  const QByteArray previewJSON =
      QJsonDocument(
          QJsonObject::fromVariantMap(ipcBuildIncidentPreview(payload)))
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
      it(QStringLiteral("Incidencia guardada: ")) +
      QFileInfo(targetPath).fileName();
  setStatus(msg);
  emit backendLogReceived(msg);
  return targetPath;
}

void IpcBridge::sendIncidentReport(const QVariantMap &report,
                                   const QString &endpoint,
                                   const QString &savedIncidentPath) {
  if (!IncidentHasExplicitRemoteConsent(report)) {
    const QString message =
        it(QStringLiteral("El envío requiere previsualización y consentimiento explícito."));
    setStatus(message);
    emit incidentReportSent(false, message, QString());
    return;
  }

  const QString reportsDirectory = ipcIncidentReportsDirPath();
  QVariantMap storedPayload;
  QString sourceError;
  if (reportsDirectory.isEmpty() ||
      !IncidentLoadEligibleSavedReport(savedIncidentPath, reportsDirectory,
                                       &storedPayload, &sourceError)) {
    const QString message = it(QStringLiteral(
        "El envío remoto requiere una incidencia de fallo guardada por la aplicación."));
    setStatus(message);
    emit backendLogReceived(
        it(QStringLiteral("❌ Se rechazó un envío remoto sin incidencia de origen válida.")));
    emit incidentReportSent(false, message, QString());
    return;
  }
  const QString localPath = QFileInfo(savedIncidentPath).canonicalFilePath();

  const QString trimmedEndpoint = endpoint.trimmed();
  if (trimmedEndpoint.isEmpty()) {
    const QString message =
        it(QStringLiteral("No se ha configurado un destino HTTPS para soporte."));
    setStatus(message);
    emit incidentReportSent(false, message, QString());
    return;
  }

  const QUrl url(trimmedEndpoint);
  if (!url.isValid() || url.scheme().toLower() != QStringLiteral("https") ||
      url.host().isEmpty() || !url.userInfo().isEmpty() || url.hasFragment()) {
    const QString message =
        it(QStringLiteral("El envío remoto solo permite destinos HTTPS válidos."));
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
      it(QStringLiteral("⚙ Enviando incidencia por HTTPS.")));

  QNetworkRequest req(url);
  req.setHeader(QNetworkRequest::ContentTypeHeader,
                QStringLiteral("application/json"));
  req.setRawHeader("Accept", "application/json");
  req.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                   QNetworkRequest::ManualRedirectPolicy);

  const QByteArray body =
      QJsonDocument(QJsonObject::fromVariantMap(
                        ipcBuildIncidentUploadEnvelope(safePayload)))
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
              message = it(QStringLiteral("Incidencia enviada correctamente."));
              emit backendLogReceived(
                  it(QStringLiteral("ℹ️ Incidencia remota enviada correctamente.")));
              setStatus(message);
            } else {
              message = it(QStringLiteral(
                  "No se pudo enviar la incidencia por el canal HTTPS configurado."));
              emit backendLogReceived(
                  it(QStringLiteral("❌ El canal HTTPS rechazó la incidencia.")));
              setStatus(message);
            }
            if (!attemptRecorded) {
              emit backendLogReceived(it(QStringLiteral(
                  "❌ No se pudo registrar localmente el resultado del envío remoto.")));
              if (ok) {
                message = it(QStringLiteral(
                    "La incidencia se envió, pero no se pudo registrar el intento localmente."));
                setStatus(message);
              }
            }
            emit incidentReportSent(ok && attemptRecorded, message, localPath);
            reply->deleteLater();
          });
}

void IpcBridge::installPublicRoots() {
  emit backendLogReceived(it(QStringLiteral(
      "⚙ Iniciando instalación de confianza TLS local...")));
  m_pendingAction = "install_public_roots";
  sendRequest("install_public_roots", QVariantMap());
}
