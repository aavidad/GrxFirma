// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "ipcbridge.h"
#include "transientsecret.h"
#include "translatorbridge.h"

#include <QElapsedTimer>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <QTemporaryFile>
#include <QLocalServer>
#include <QLocalSocket>
#include <QUuid>
#include <QtTest>

namespace {
// Al conectar, el puente pide su estado de arranque (local_tls_startup_status).
// Las pruebas leen la siguiente petición que no sea ese aviso de arranque.
QJsonObject nextRequest(QLocalSocket *peer) {
  QElapsedTimer timer;
  timer.start();
  while (timer.elapsed() < 5000) {
    while (peer->canReadLine()) {
      const auto request = QJsonDocument::fromJson(peer->readLine()).object();
      if (request.value("action").toString() != QStringLiteral("local_tls_startup_status"))
        return request;
    }
    QTest::qWait(10);
  }
  return {};
}
} // namespace

class IpcBridgeTransientSecretTest : public QObject {
  Q_OBJECT

private slots:
  void unsafeVersionedActivation();
  void unsafeActivationOriginIsClosed();
  void unsafeBlockAndRetryRouting();
  void unsafeBlockUsesInterfaceLanguageAndNoDiagnostic();
  void unsafeSignBlockThenWarningsAfterActivation();
  void unsafeDisconnectedNeverDeferred();
  void disconnectedProtectIsNotDeferred();
  void disconnectedUnprotectIsNotDeferred();
  void disconnectedRemoteProtectSignIsNotDeferred();
  void remoteSecretsTravelOnlyWithProtectSign();
  void droppedProtectResponseFailsExactlyOnce();
  void droppedUnprotectResponseFailsExactlyOnce();
  void droppedVerifyResponseFailsExactlyOnce();
  void tokenSettingsDisconnectedNeverDeferred();
  void tokenSettingsCorrelatesAndFinishesOnDisconnect();
  void tokenSettingsRejectsRemoteModuleURLs();
  void tokenSettingsTimeoutIgnoresLateReply();
  void tokenSettingsUsesFixedErrorCodeNotMessage();
  void settingsAccessUsesInterfaceCatalogAndOwnDialog();
  void protectionRecipientsUserRequestSurvivesAutomaticLoad();
  void unexpectedServerReceivesNothing();
  void expectedServerReceivesDeferredRequest();
  void secretFrameZeroesInputs();
  void importCertificateSendsPasswordB64();

private:
  QLocalSocket *connectPeer(IpcBridge &bridge, QLocalServer &server);
};

void IpcBridgeTransientSecretTest::unsafeVersionedActivation() {
  IpcBridge bridge;
  QLocalServer server;
  auto *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  peer->readAll();
  QSignalSpy finished(&bridge, &IpcBridge::unsafeExceptionsFinished);
  bridge.activateUnsafeException("cms_cbc", "2026-11-15.1", "aviso", "fixture-token");
  const auto sent = nextRequest(peer);
  QVERIFY(!sent.isEmpty());
  QCOMPARE(sent.value("action").toString(), QStringLiteral("unsafe_exceptions_activate"));
  const auto params = sent.value("params").toObject();
  QCOMPARE(params.value("acceptanceVersion").toString(), QStringLiteral("2026-11-15.1"));
  QCOMPARE(params.value("accepted").toBool(), true);
  QCOMPARE(params.value("retryId").toString(), QStringLiteral("fixture-token"));
  QCOMPARE(params.value("origin").toString(), QStringLiteral("aviso"));
  auto reply = [&](const QString &id) {
    peer->write(QJsonDocument(QJsonObject{{"action", "unsafe_exceptions_activate"},
        {"requestId", id}, {"ok", true}, {"data", QJsonObject{}}}).toJson(QJsonDocument::Compact) + '\n');
    peer->flush();
    QTest::qWait(25);
  };
  reply("unrelated");
  QCOMPARE(finished.count(), 0);
  reply(sent.value("requestId").toString());
  QCOMPARE(finished.count(), 1);
  reply(sent.value("requestId").toString());
  QCOMPARE(finished.count(), 1);
}

// Lote 39f: el puente solo envía los dos orígenes que admite el motor y,
// con un reintento, solo el aviso. Lo demás termina en fallo sin enviar nada.
void IpcBridgeTransientSecretTest::unsafeActivationOriginIsClosed() {
  IpcBridge bridge;
  QLocalServer server;
  auto *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  peer->readAll();
  QSignalSpy finished(&bridge, &IpcBridge::unsafeExceptionsFinished);
  bridge.activateUnsafeException("cms_cbc", "2026-11-15.1", "ajustes");
  const auto sent = nextRequest(peer);
  QCOMPARE(sent.value("params").toObject().value("origin").toString(), QStringLiteral("ajustes"));
  QVERIFY(!sent.value("params").toObject().contains("retryId"));
  const QStringList rejected{QString(), QStringLiteral("linea_ordenes"), QStringLiteral("Aviso"),
                             QStringLiteral("aceptado desde el aviso")};
  for (const auto &origin : rejected) {
    finished.clear();
    bridge.activateUnsafeException("cms_cbc", "2026-11-15.1", origin);
    QCOMPARE(finished.count(), 1);
    QVERIFY(!finished.at(0).at(1).toBool());
  }
  finished.clear();
  bridge.activateUnsafeException("cms_cbc", "2026-11-15.1", "ajustes", "fixture-token");
  QCOMPARE(finished.count(), 1);
  QVERIFY(!finished.at(0).at(1).toBool());
  QTest::qWait(50);
  while (peer->canReadLine()) {
    const auto request = QJsonDocument::fromJson(peer->readLine()).object();
    QVERIFY(request.value("action").toString() != QStringLiteral("unsafe_exceptions_activate"));
  }
}

void IpcBridgeTransientSecretTest::unsafeBlockAndRetryRouting() {
  IpcBridge bridge;
  QLocalServer server;
  auto *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  peer->readAll();
  QSignalSpy blocked(&bridge, &IpcBridge::unsafeExceptionBlocked);
  QSignalSpy verified(&bridge, &IpcBridge::verificationFinished);
  QSignalSpy warnings(&bridge, &IpcBridge::unsafeOperationWarnings);
  bridge.verifyFile("/tmp/fixture.pdf");
  const auto sent = nextRequest(peer);
  QVERIFY(!sent.isEmpty());
  const QJsonObject block{{"action", "verify"}, {"requestId", sent.value("requestId")},
      {"ok", false}, {"exceptionId", "cms_cbc"}, {"activable", false},
      {"acceptanceVersion", "2026-11-15.1"}, {"retryId", "fixture-token"},
      {"exception", QJsonObject{{"id", "cms_cbc"}}}};
  peer->write(QJsonDocument(block).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(blocked.count(), 1);
  QVERIFY(!blocked.at(0).at(0).toMap().value("activable").toBool());
  QCOMPARE(verified.count(), 1);
  bridge.retryUnsafeOperation("fixture-token");
  const auto retry = nextRequest(peer);
  QVERIFY(!retry.isEmpty());
  QCOMPARE(retry.value("action").toString(), QStringLiteral("unsafe_operation_retry"));
  const QJsonObject reply{{"action", "unsafe_operation_retry"}, {"requestId", retry.value("requestId")},
      {"ok", true}, {"warnings", QJsonArray{"excepciones.cms_cbc.aviso"}},
      {"data", QJsonObject{{"valid", true}}}};
  peer->write(QJsonDocument(reply).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(verified.count(), 2);
  QVERIFY(verified.at(1).at(0).toBool());
  QCOMPARE(warnings.last().at(0).toStringList(), QStringList{"excepciones.cms_cbc.aviso"});
}

// Lote 39a, vuelta 2: una firma con RSA corta. Sin el ajuste, el bloqueo
// llega antes que el fin de la firma (la tarjeta lo pinta como bloqueo, no
// como avería). Tras activar, la firma reenviada trae el aviso de uso, que
// sale por la señal del aviso común y por lastSigningWarnings.
void IpcBridgeTransientSecretTest::unsafeSignBlockThenWarningsAfterActivation() {
  IpcBridge bridge;
  QLocalServer server;
  auto *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  peer->readAll();
  QList<QString> order;
  connect(&bridge, &IpcBridge::unsafeExceptionBlocked, &bridge,
          [&order](const QVariantMap &) { order << QStringLiteral("blocked"); });
  connect(&bridge, &IpcBridge::signingFinished, &bridge,
          [&order](bool ok, const QString &, const QString &) {
            order << (ok ? QStringLiteral("signed") : QStringLiteral("failed"));
          });
  QSignalSpy blocked(&bridge, &IpcBridge::unsafeExceptionBlocked);
  QSignalSpy warnings(&bridge, &IpcBridge::unsafeOperationWarnings);
  bridge.signFileAdvanced("/tmp/fixture.pdf", "/tmp/fixture_firmado.pdf", 0,
                          QVariantMap{{"format", "pades"}});
  const auto sent = nextRequest(peer);
  QVERIFY(!sent.isEmpty());
  const QJsonObject block{{"action", sent.value("action")}, {"requestId", sent.value("requestId")},
      {"ok", false}, {"errorCode", "unsafe_exception_blocked"}, {"exceptionId", "rsa_corta"},
      {"activable", true}, {"acceptanceVersion", "2026-11-15.3"},
      {"error", QStringLiteral("blocked")},
      {"exception", QJsonObject{{"id", "rsa_corta"}, {"block_key", "excepciones.rsa_corta.bloqueo"}}}};
  peer->write(QJsonDocument(block).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(order.size(), 2);
  QCOMPARE(order, (QList<QString>{QStringLiteral("blocked"), QStringLiteral("failed")}));
  QCOMPARE(blocked.at(0).at(0).toMap().value("operationAction").toString(),
           sent.value("action").toString());
  QCOMPARE(warnings.count(), 0);
  bridge.signFileAdvanced("/tmp/fixture.pdf", "/tmp/fixture_firmado.pdf", 0,
                          QVariantMap{{"format", "pades"}});
  const auto again = nextRequest(peer);
  QVERIFY(!again.isEmpty());
  const QJsonObject signedReply{{"action", again.value("action")}, {"requestId", again.value("requestId")},
      {"ok", true}, {"warnings", QJsonArray{"excepciones.rsa_corta.aviso"}},
      {"data", QJsonObject{{"OutputPath", "/tmp/fixture_firmado.pdf"}}}};
  peer->write(QJsonDocument(signedReply).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(order.size(), 3);
  QCOMPARE(order.last(), QStringLiteral("signed"));
  QCOMPARE(warnings.count(), 1);
  QCOMPARE(warnings.at(0).at(0).toStringList(), QStringList{"excepciones.rsa_corta.aviso"});
  QCOMPARE(bridge.lastSigningWarnings(), QStringList{"excepciones.rsa_corta.aviso"});
}

// Lote 39a: el motor da el texto del bloqueo en el idioma del proceso. Qt lo
// pinta desde las claves con su catálogo, no guarda el diagnóstico de avería
// y marca la verificación como «sin evaluar» para no pintarla «No válida».
void IpcBridgeTransientSecretTest::unsafeBlockUsesInterfaceLanguageAndNoDiagnostic() {
  TranslatorBridge translator;
  translator.setLocale(QStringLiteral("en"));
  const QString english = translator.t(QStringLiteral("excepciones.sha1_leer.bloqueo"));
  QVERIFY(english != QStringLiteral("excepciones.sha1_leer.bloqueo"));
  IpcBridge bridge;
  QLocalServer server;
  auto *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  peer->readAll();
  QSignalSpy blocked(&bridge, &IpcBridge::unsafeExceptionBlocked);
  QSignalSpy verified(&bridge, &IpcBridge::verificationFinished);
  bridge.verifyFile("/tmp/fixture.csig");
  const auto sent = nextRequest(peer);
  QVERIFY(!sent.isEmpty());
  const QJsonObject exception{{"id", "sha1_leer"}, {"block_key", "excepciones.sha1_leer.bloqueo"},
      {"risk_key", "excepciones.sha1_leer.riesgo"}, {"organization_key", "excepciones.sha1_leer.organizacion"},
      {"activable", true}};
  const QJsonObject block{{"action", "verify"}, {"requestId", sent.value("requestId")},
      {"ok", false}, {"errorCode", "unsafe_exception_blocked"}, {"exceptionId", "sha1_leer"},
      {"blockKey", "excepciones.sha1_leer.bloqueo"}, {"activable", true},
      {"acceptanceVersion", "2026-11-15.3"}, {"retryId", "fixture-token"},
      {"error", QStringLiteral("La app no permite esto por seguridad.")},
      {"diagnostic", QJsonObject{{"category", "certificate_store"}, {"likelyOwner", "local_certificate"}}},
      {"exception", exception}};
  peer->write(QJsonDocument(block).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(verified.count(), 1);
  QCOMPARE(blocked.count(), 1);
  QCOMPARE(verified.at(0).at(0).toBool(), false);
  QCOMPARE(verified.at(0).at(1).toString(), english);
  QVERIFY(verified.at(0).at(2).toMap().value("unsafeBlocked").toBool());
  QVERIFY(bridge.lastDiagnostic().isEmpty());
}

void IpcBridgeTransientSecretTest::unsafeDisconnectedNeverDeferred() {
  IpcBridge bridge;
  QSignalSpy finished(&bridge, &IpcBridge::unsafeExceptionsFinished);
  bridge.activateUnsafeException("cms_cbc", "2026-11-15.1", "ajustes");
  QCOMPARE(finished.count(), 1);
  QVERIFY(!finished.at(0).at(1).toBool());
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  bridge.disableUnsafeException("cms_cbc");
  QCOMPARE(finished.count(), 2);
}

static QString validTransientSecret() {
  return QString::fromLatin1(
      QByteArray(TransientSecret::AES256KeyBytes, '\x5a').toBase64());
}

void IpcBridgeTransientSecretTest::tokenSettingsDisconnectedNeverDeferred() {
  IpcBridge bridge;
  QSignalSpy finished(&bridge, &IpcBridge::tokenSettingsFinished);
  bridge.getTokenSettings();
  bridge.diagnoseTokenSettings();
  bridge.saveTokenSettings({{QStringLiteral("confirmed"), true}});
  QCOMPARE(finished.count(), 3);
  for (const auto &args : finished)
    QVERIFY(!args.at(1).toBool());
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  QVERIFY(bridge.m_tokenSettingsAction.isEmpty());
}

void IpcBridgeTransientSecretTest::tokenSettingsCorrelatesAndFinishesOnDisconnect() {
  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  QSignalSpy finished(&bridge, &IpcBridge::tokenSettingsFinished);
  bridge.getTokenSettings();
  const QString requestId = bridge.m_tokenSettingsRequestId;
  QVERIFY(!requestId.isEmpty());
  auto reply = [&](const QString &id) {
    QJsonObject value{{"action", "get_token_settings"}, {"requestId", id},
                      {"ok", true}, {"data", QJsonObject{{"available", false}}}};
    peer->write(QJsonDocument(value).toJson(QJsonDocument::Compact) + '\n');
    peer->flush();
    QTest::qWait(25);
  };
  reply(QStringLiteral("unsolicited"));
  QCOMPARE(finished.count(), 0);
  reply(QString());
  QCOMPARE(finished.count(), 0);
  reply(requestId);
  QTRY_COMPARE(finished.count(), 1);
  QVERIFY(finished.at(0).at(1).toBool());
  reply(requestId);
  QCOMPARE(finished.count(), 1);
  bridge.diagnoseTokenSettings();
  // Other read requests must not steal the token operation's terminal signal.
  bridge.getSettings();
  peer->abort();
  QTRY_COMPARE(finished.count(), 2);
  QVERIFY(!finished.at(1).at(1).toBool());
  QCoreApplication::processEvents();
  QCOMPARE(finished.count(), 2);
}

void IpcBridgeTransientSecretTest::tokenSettingsRejectsRemoteModuleURLs() {
  IpcBridge bridge;
  QCOMPARE(bridge.tokenModuleLocalPath(QUrl("file:///usr/lib/test%20module.so")),
           QStringLiteral("/usr/lib/test module.so"));
  QVERIFY(bridge.tokenModuleLocalPath(QUrl("https://example.test/module.so")).isEmpty());
  QVERIFY(bridge.tokenModuleLocalPath(QUrl("file://server/module.so")).isEmpty());
  QVERIFY(bridge.tokenModuleLocalPath(QUrl("relative.so")).isEmpty());
}

void IpcBridgeTransientSecretTest::tokenSettingsTimeoutIgnoresLateReply() {
  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  QSignalSpy finished(&bridge, &IpcBridge::tokenSettingsFinished);
  bridge.getTokenSettings();
  const QString requestId = bridge.m_tokenSettingsRequestId;
  QTRY_COMPARE_WITH_TIMEOUT(finished.count(), 1, 23000);
  QVERIFY(!finished.at(0).at(1).toBool());
  QVERIFY(bridge.m_tokenSettingsRequestId.isEmpty());
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  const QJsonObject reply{{"action", "get_token_settings"}, {"requestId", requestId},
                          {"ok", true}, {"data", QJsonObject{{"available", true}}}};
  peer->write(QJsonDocument(reply).toJson(QJsonDocument::Compact) + '\n');
  peer->flush();
  QTest::qWait(50);
  QCOMPARE(finished.count(), 1);
}

void IpcBridgeTransientSecretTest::tokenSettingsUsesFixedErrorCodeNotMessage() {
  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  QSignalSpy finished(&bridge, &IpcBridge::tokenSettingsFinished);
  for (const QString &code : {QStringLiteral("token_settings_unsafe"), QString(), QStringLiteral("untrusted")}) {
    bridge.getTokenSettings();
    const QJsonObject reply{{"action", "get_token_settings"}, {"requestId", bridge.m_tokenSettingsRequestId},
                            {"ok", false}, {"errorCode", code}, {"error", "UNTRUSTED_DRIVER_TEXT"}};
    peer->write(QJsonDocument(reply).toJson(QJsonDocument::Compact) + '\n');
    peer->flush();
    QTRY_COMPARE(finished.count(), 1);
    const auto args = finished.takeFirst();
    QVERIFY(!args.at(1).toBool());
    QCOMPARE(args.at(3).toString(), code == "token_settings_unsafe" ? code : QStringLiteral("token_settings_failed"));
    QVERIFY(!args.at(3).toString().contains("UNTRUSTED_DRIVER_TEXT"));
  }
}

// Lote 39g: ajustes sin permiso o de otra cuenta. Qt pinta el texto de su
// catálogo (con la orden entera, sin [PATH]) en su propio diálogo; leerlos no
// se presenta como una firma fallida y guardar sigue avisando del resultado.
void IpcBridgeTransientSecretTest::settingsAccessUsesInterfaceCatalogAndOwnDialog() {
  TranslatorBridge translator;
  translator.setLocale(QStringLiteral("en"));
#ifdef Q_OS_WIN
  const QString noPermissionKey = QStringLiteral("error.settings_sin_permiso");
  const QString command = QStringLiteral("icacls \"%APPDATA%\\GrxFirma\" /grant \"%USERNAME%:(OI)(CI)F\" /T");
#else
  const QString noPermissionKey = QStringLiteral("error.settings_sin_permiso_unix");
  const QString command = QStringLiteral("sudo chown -R \"$USER\" ~/.config/grxfirma");
#endif
  const QString noPermission = translator.t(noPermissionKey);
  const QString otherUser = translator.t(QStringLiteral("error.settings_otro_usuario"));
  QVERIFY(noPermission != noPermissionKey);
  QVERIFY(noPermission.endsWith(QLatin1Char('\n') + command));
  IpcBridge bridge;
  QLocalServer server;
  auto *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  peer->readAll();
  QSignalSpy access(&bridge, &IpcBridge::settingsAccessFailed);
  QSignalSpy signing(&bridge, &IpcBridge::signingFinished);
  QSignalSpy saved(&bridge, &IpcBridge::settingsSaved);
  const QJsonObject diagnostic{{"category", "app_local"}, {"failureCode", "SETTINGS_ACCESS"},
      {"userMessage", "texto del motor"}, {"expertMessage", noPermissionKey}};

  bridge.getSettings();
  auto sent = nextRequest(peer);
  QCOMPARE(sent.value("action").toString(), QStringLiteral("get_settings"));
  QJsonObject reply{{"action", "get_settings"}, {"requestId", sent.value("requestId")},
      {"ok", false}, {"errorCode", "settings_no_permission"}, {"outcome", "failure"},
      {"error", QStringLiteral("texto del motor en otro idioma /home/maria/.config/grxfirma")},
      {"diagnostic", diagnostic}};
  peer->write(QJsonDocument(reply).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(access.count(), 1);
  QCOMPARE(access.at(0).at(0).toString(), QStringLiteral("get_settings"));
  QCOMPARE(access.at(0).at(1).toString(), noPermission);
  QVERIFY(!access.at(0).at(1).toString().contains(QStringLiteral("[PATH]")));
  QCOMPARE(signing.count(), 0);

  bridge.saveSettings(QVariantMap{{"tema", "oscuro"}});
  sent = nextRequest(peer);
  QCOMPARE(sent.value("action").toString(), QStringLiteral("save_settings"));
  reply = QJsonObject{{"action", "save_settings"}, {"requestId", sent.value("requestId")},
      {"ok", false}, {"errorCode", "settings_other_user"}, {"outcome", "failure"},
      {"error", QStringLiteral("texto del motor")}, {"diagnostic", diagnostic}};
  peer->write(QJsonDocument(reply).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(access.count(), 2);
  QCOMPARE(access.at(1).at(0).toString(), QStringLiteral("save_settings"));
  QCOMPARE(access.at(1).at(1).toString(), otherUser);
  QTRY_COMPARE(saved.count(), 1);
  QVERIFY(!saved.at(0).at(0).toBool());
  QCOMPARE(signing.count(), 0);

  // Otro código de fallo sigue el camino de siempre.
  bridge.saveSettings(QVariantMap{{"tema", "claro"}});
  sent = nextRequest(peer);
  reply = QJsonObject{{"action", "save_settings"}, {"requestId", sent.value("requestId")},
      {"ok", false}, {"errorCode", "operation_failed"}, {"error", QStringLiteral("Configuración de usuario no disponible")}};
  peer->write(QJsonDocument(reply).toJson(QJsonDocument::Compact) + '\n'); peer->flush();
  QTRY_COMPARE(saved.count(), 2);
  QCOMPARE(access.count(), 2);
}

static QByteArray readRequestsUntil(QLocalSocket *peer,
                                    const QByteArray &needle) {
  QByteArray requests = peer->readAll();
  QElapsedTimer timer;
  timer.start();
  while (!requests.contains(needle) && timer.elapsed() < 3000) {
    if (peer->waitForReadyRead(100))
      requests += peer->readAll();
  }
  return requests;
}

QLocalSocket *
IpcBridgeTransientSecretTest::connectPeer(IpcBridge &bridge,
                                          QLocalServer &server) {
  const QString serverName =
      QStringLiteral("grxfirma-secret-test-%1")
          .arg(QUuid::createUuid().toString(QUuid::WithoutBraces));
  if (!server.listen(serverName))
    return nullptr;
  // El servidor de la prueba es este mismo proceso: es el PID esperado.
  bridge.m_expectedServerPid = QCoreApplication::applicationPid();
  bridge.m_socket->connectToServer(serverName);
  if (!bridge.m_socket->waitForConnected(3000) ||
      !server.waitForNewConnection(3000)) {
    return nullptr;
  }
  return server.nextPendingConnection();
}

void IpcBridgeTransientSecretTest::disconnectedProtectIsNotDeferred() {
  IpcBridge bridge;
  QSignalSpy finished(&bridge, &IpcBridge::protectionFinished);

  bridge.protectEncryptedDataFile(QStringLiteral("/tmp/documento.pdf"),
                                  QString(), validTransientSecret());

  QCOMPARE(finished.count(), 1);
  QVERIFY(!finished.takeFirst().at(0).toBool());
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  QVERIFY(bridge.m_deferredAction.isEmpty());
}

void IpcBridgeTransientSecretTest::disconnectedUnprotectIsNotDeferred() {
  IpcBridge bridge;
  QSignalSpy finished(&bridge, &IpcBridge::unprotectionFinished);

  bridge.unprotectEncryptedDataFile(
      QStringLiteral("/tmp/documento.pdf.encrypted.p7m"), QString(),
      validTransientSecret());

  QCOMPARE(finished.count(), 1);
  QVERIFY(!finished.takeFirst().at(0).toBool());
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  QVERIFY(bridge.m_deferredAction.isEmpty());
}

static QVariantMap remoteProtectOptions(bool signToo) {
  return {{QStringLiteral("profile"), QStringLiteral("compat")},
          {QStringLiteral("signToo"), signToo},
          {QStringLiteral("certificateId"), QString(64, QLatin1Char('a'))},
          {QStringLiteral("remotePin"), QStringLiteral("1234")},
          {QStringLiteral("remoteOtp"), QStringLiteral("567890")}};
}

void IpcBridgeTransientSecretTest::disconnectedRemoteProtectSignIsNotDeferred() {
  IpcBridge bridge;
  QSignalSpy finished(&bridge, &IpcBridge::protectionFinished);

  bridge.protectFileAdvanced(QStringLiteral("/tmp/documento.pdf"), QString(),
                             {QStringLiteral("destinatario")}, 0,
                             remoteProtectOptions(true));

  // El PIN/OTP es de una sola firma: nunca se repite tras reconectar.
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  QVERIFY(bridge.m_deferredAction.isEmpty());
  QCOMPARE(finished.count(), 1);
  QVERIFY(!finished.takeFirst().at(0).toBool());
}

// Devuelve la siguiente petición de protección; el bridge envía otras al
// conectar (estado del arranque TLS, por ejemplo) que aquí no interesan.
static QJsonObject readProtectRequest(QLocalSocket *peer, QByteArray &buffer) {
  QElapsedTimer timer;
  timer.start();
  while (timer.elapsed() < 3000) {
    const qsizetype end = buffer.indexOf('\n');
    if (end < 0) {
      if (peer->bytesAvailable() == 0)
        peer->waitForReadyRead(100);
      buffer += peer->readAll();
      continue;
    }
    const QJsonObject request =
        QJsonDocument::fromJson(buffer.left(end)).object();
    buffer.remove(0, end + 1);
    if (request.value(QStringLiteral("action")).toString().startsWith(
            QStringLiteral("protect")))
      return request;
  }
  return {};
}

void IpcBridgeTransientSecretTest::remoteSecretsTravelOnlyWithProtectSign() {
  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  QByteArray buffer;

  bridge.protectFileAdvanced(QStringLiteral("/tmp/documento.pdf"), QString(),
                             {QStringLiteral("destinatario")}, 0,
                             remoteProtectOptions(true));
  const QJsonObject signed_ = readProtectRequest(peer, buffer);
  QCOMPARE(signed_.value(QStringLiteral("action")).toString(),
           QStringLiteral("protect_sign"));
  const QJsonObject signParams =
      signed_.value(QStringLiteral("params")).toObject();
  QCOMPARE(signParams.value(QStringLiteral("remotePin")).toString(),
           QString::fromLatin1(QByteArray("1234").toBase64()));
  QCOMPARE(signParams.value(QStringLiteral("remoteOtp")).toString(),
           QString::fromLatin1(QByteArray("567890").toBase64()));
  QCOMPARE(signParams.value(QStringLiteral("certificateId")).toString(),
           QString(64, QLatin1Char('a')));

  bridge.protectFileAdvanced(QStringLiteral("/tmp/documento.pdf"), QString(),
                             {QStringLiteral("destinatario")}, 0,
                             remoteProtectOptions(false));
  const QJsonObject plain = readProtectRequest(peer, buffer);
  QCOMPARE(plain.value(QStringLiteral("action")).toString(),
           QStringLiteral("protect"));
  const QJsonObject plainParams =
      plain.value(QStringLiteral("params")).toObject();
  QVERIFY(!plainParams.contains(QStringLiteral("remotePin")));
  QVERIFY(!plainParams.contains(QStringLiteral("remoteOtp")));
  QVERIFY(!plainParams.contains(QStringLiteral("certificateId")));
}

void IpcBridgeTransientSecretTest::droppedProtectResponseFailsExactlyOnce() {
  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  QSignalSpy finished(&bridge, &IpcBridge::protectionFinished);

  bridge.protectEncryptedDataFile(
      QStringLiteral("/tmp/documento.pdf"), QStringLiteral("/tmp/salida"),
      validTransientSecret());

  QCOMPARE(finished.count(), 0);
  QCOMPARE(bridge.m_pendingAction, QStringLiteral("protect"));
  const QByteArray request =
      readRequestsUntil(peer, QByteArrayLiteral("\"action\":\"protect\""));
  QVERIFY(request.contains("\"action\":\"protect\""));
  QVERIFY(request.contains(
      "\"outputPath\":\"/tmp/salida.encrypted.p7m\""));

  peer->abort();
  QTRY_COMPARE(finished.count(), 1);
  QVERIFY(!finished.at(0).at(0).toBool());
  QVERIFY(bridge.m_pendingAction.isEmpty());
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  QVERIFY(bridge.m_deferredAction.isEmpty());
  QCoreApplication::processEvents();
  QCOMPARE(finished.count(), 1);
}

void IpcBridgeTransientSecretTest::droppedUnprotectResponseFailsExactlyOnce() {
  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  QSignalSpy finished(&bridge, &IpcBridge::unprotectionFinished);

  bridge.unprotectEncryptedDataFile(
      QStringLiteral("/tmp/documento.encrypted.p7m"), QString(),
      validTransientSecret());

  QCOMPARE(finished.count(), 0);
  QCOMPARE(bridge.m_pendingAction, QStringLiteral("unprotect"));
  QVERIFY(readRequestsUntil(peer, QByteArrayLiteral("\"action\":\"unprotect\""))
              .contains("\"action\":\"unprotect\""));

  peer->abort();
  QTRY_COMPARE(finished.count(), 1);
  QVERIFY(!finished.at(0).at(0).toBool());
  QVERIFY(bridge.m_pendingAction.isEmpty());
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  QVERIFY(bridge.m_deferredAction.isEmpty());
  QCoreApplication::processEvents();
  QCOMPARE(finished.count(), 1);
}

void IpcBridgeTransientSecretTest::droppedVerifyResponseFailsExactlyOnce() {
  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  QSignalSpy finished(&bridge, &IpcBridge::verificationFinished);

  bridge.verifyFile(QStringLiteral("/tmp/documento.pdf"));

  QCOMPARE(finished.count(), 0);
  QCOMPARE(bridge.m_pendingAction, QStringLiteral("verify"));
  QVERIFY(readRequestsUntil(peer, QByteArrayLiteral("\"action\":\"verify\""))
              .contains("\"action\":\"verify\""));

  peer->abort();
  QTRY_COMPARE(finished.count(), 1);
  QVERIFY(!finished.at(0).at(0).toBool());
  QVERIFY(bridge.m_pendingAction.isEmpty());
  QCoreApplication::processEvents();
  QCOMPARE(finished.count(), 1);
}

// D-01: si el otro extremo no es el proceso esperado, la GUI cierra sin
// enviar nada, tampoco la acción que tenía diferida.
// Lote p3: la carga automática de destinatarios y la petición de la persona
// (crear la clave propia o recargar la lista) se cruzan. Cada respuesta se
// juzga por su propia petición: la automática no abre el aviso de seguridad
// y la de la persona lo abre aunque ya no sea la última en vuelo.
void IpcBridgeTransientSecretTest::protectionRecipientsUserRequestSurvivesAutomaticLoad() {
  IpcBridge bridge;
  QLocalServer server;
  auto *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  peer->readAll();
  QSignalSpy blocked(&bridge, &IpcBridge::unsafeExceptionBlocked);
  QSignalSpy unavailable(&bridge, &IpcBridge::protectionOwnIdentityUnavailable);
  const auto blockReply = [](const QJsonObject &sent) {
    return QJsonObject{{"action", "protection_recipients"}, {"requestId", sent.value("requestId")},
        {"ok", false}, {"exceptionId", "alto_clave_fichero_crear"}, {"activable", true},
        {"acceptanceVersion", "2026-11-15.3"},
        {"exception", QJsonObject{{"id", "alto_clave_fichero_crear"}}}};
  };
  const auto write = [peer](const QJsonObject &reply) {
    peer->write(QJsonDocument(reply).toJson(QJsonDocument::Compact) + '\n');
    peer->flush();
  };

  // Automática y luego la persona; responde primero la automática.
  bridge.loadProtectionRecipients(false);
  const auto automatic = nextRequest(peer);
  bridge.loadProtectionRecipients(true);
  const auto user = nextRequest(peer);
  QCOMPARE(automatic.value("action").toString(), QStringLiteral("protection_recipients"));
  QCOMPARE(user.value("action").toString(), QStringLiteral("protection_recipients"));
  write(blockReply(automatic));
  QTRY_COMPARE(unavailable.count(), 1);
  QCOMPARE(blocked.count(), 0);
  write(blockReply(user));
  QTRY_COMPARE(blocked.count(), 1);
  QCOMPARE(blocked.at(0).at(0).toMap().value("operationAction").toString(),
           QStringLiteral("protection_recipients"));

  // La persona y luego otra automática; la de la persona ya no es la última.
  bridge.loadProtectionRecipients(true);
  const auto userFirst = nextRequest(peer);
  bridge.loadProtectionRecipients(false);
  const auto automaticLater = nextRequest(peer);
  write(blockReply(userFirst));
  QTRY_COMPARE(blocked.count(), 2);
  write(blockReply(automaticLater));
  QTRY_COMPARE(unavailable.count(), 4);
  QCOMPARE(blocked.count(), 2);
}

void IpcBridgeTransientSecretTest::unexpectedServerReceivesNothing() {
  IpcBridge bridge;
  QLocalServer server;
  const QString serverName =
      QStringLiteral("grxfirma-peer-test-%1")
          .arg(QUuid::createUuid().toString(QUuid::WithoutBraces));
  QVERIFY(server.listen(serverName));
  bridge.m_socketPath = server.fullServerName();
  bridge.m_expectedServerPid = QCoreApplication::applicationPid() + 1;
  bridge.getSettings();
  QVERIFY(server.waitForNewConnection(3000));
  QLocalSocket *peer = server.nextPendingConnection();
  QVERIFY(peer);
  QTest::qWait(300);
  QByteArray received = peer->readAll();
  if (peer->waitForReadyRead(200))
    received += peer->readAll();
  QVERIFY2(received.isEmpty(), received.constData());
  QVERIFY(bridge.m_serverRejected);
  QVERIFY(bridge.m_deferredRequest.isEmpty());
  QVERIFY(bridge.m_socket->state() != QLocalSocket::ConnectedState);
}

void IpcBridgeTransientSecretTest::expectedServerReceivesDeferredRequest() {
  IpcBridge bridge;
  QLocalServer server;
  const QString serverName =
      QStringLiteral("grxfirma-peer-test-%1")
          .arg(QUuid::createUuid().toString(QUuid::WithoutBraces));
  QVERIFY(server.listen(serverName));
  bridge.m_socketPath = server.fullServerName();
  bridge.m_expectedServerPid = QCoreApplication::applicationPid();
  bridge.getSettings();
  QVERIFY(server.waitForNewConnection(3000));
  QLocalSocket *peer = server.nextPendingConnection();
  QVERIFY(peer);
  const QByteArray requests =
      readRequestsUntil(peer, QByteArrayLiteral("\"get_settings\""));
  QVERIFY(requests.contains("\"get_settings\""));
  QVERIFY(!bridge.m_serverRejected);
}

// D-06: la trama con secretos se construye sobre QByteArray y los búferes de
// entrada quedan a cero.
void IpcBridgeTransientSecretTest::secretFrameZeroesInputs() {
  QByteArray p12("\x30\x82\x01\x02contenido-p12", 17);
  QByteArray password("contraseña-ñ");
  const qsizetype p12Size = p12.size();
  const qsizetype passwordSize = password.size();
  QJsonObject header;
  header.insert(QStringLiteral("action"), QStringLiteral("import_certificate"));
  header.insert(QStringLiteral("requestId"), QStringLiteral("r1"));
  QJsonObject plain;
  plain.insert(QStringLiteral("targetId"), QStringLiteral("nss"));
  const QByteArray expectedP12 = p12.toBase64();
  const QByteArray expectedPassword = password.toBase64();

  const QByteArray frame = TransientSecret::buildSecretRequestFrame(
      header, plain,
      {{QStringLiteral("p12B64"), &p12},
       {QStringLiteral("passwordB64"), &password}});

  QCOMPARE(p12.size(), p12Size);
  QCOMPARE(password.size(), passwordSize);
  QCOMPARE(p12, QByteArray(p12Size, '\0'));
  QCOMPARE(password, QByteArray(passwordSize, '\0'));
  QVERIFY(frame.endsWith("\n"));
  QJsonParseError parseError;
  const QJsonDocument doc = QJsonDocument::fromJson(frame, &parseError);
  QCOMPARE(parseError.error, QJsonParseError::NoError);
  const QJsonObject obj = doc.object();
  QCOMPARE(obj.value("action").toString(), QStringLiteral("import_certificate"));
  QCOMPARE(obj.value("requestId").toString(), QStringLiteral("r1"));
  const QJsonObject params = obj.value("params").toObject();
  QCOMPARE(params.value("targetId").toString(), QStringLiteral("nss"));
  QCOMPARE(params.value("p12B64").toString().toLatin1(), expectedP12);
  QCOMPARE(params.value("passwordB64").toString().toLatin1(), expectedPassword);
  QVERIFY(!params.contains("password"));

  // Sin parámetros normales ni cabecera la trama sigue siendo JSON válido.
  QByteArray only("x");
  const QByteArray minimal = TransientSecret::buildSecretRequestFrame(
      QJsonObject(), QJsonObject(), {{QStringLiteral("passwordB64"), &only}});
  QCOMPARE(minimal, QByteArrayLiteral("{\"params\":{\"passwordB64\":\"eA==\"}}\n"));
}

// D-06: import_certificate envía passwordB64 (nunca "password" en claro) y
// sin conexión no deja nada diferido.
void IpcBridgeTransientSecretTest::importCertificateSendsPasswordB64() {
  QTemporaryFile p12File;
  QVERIFY(p12File.open());
  p12File.write("p12-falso");
  p12File.flush();

  IpcBridge offline;
  QSignalSpy offlineFinished(&offline, &IpcBridge::certificateImportFinished);
  offline.importCertificate(p12File.fileName(), QStringLiteral("clave"));
  QCOMPARE(offlineFinished.count(), 1);
  QVERIFY(!offlineFinished.at(0).at(0).toBool());
  QVERIFY(offline.m_deferredRequest.isEmpty());

  IpcBridge bridge;
  QLocalServer server;
  QLocalSocket *peer = connectPeer(bridge, server);
  QVERIFY(peer);
  bridge.importCertificate(p12File.fileName(), QStringLiteral("clave"));
  const QByteArray request =
      readRequestsUntil(peer, QByteArrayLiteral("\"import_certificate\""));
  QJsonObject params;
  for (const QByteArray &line : request.split('\n')) {
    const QJsonObject obj = QJsonDocument::fromJson(line).object();
    if (obj.value("action").toString() == QStringLiteral("import_certificate"))
      params = obj.value("params").toObject();
  }
  QCOMPARE(params.value("p12B64").toString(),
           QString::fromLatin1(QByteArray("p12-falso").toBase64()));
  QCOMPARE(params.value("passwordB64").toString(),
           QString::fromLatin1(QByteArray("clave").toBase64()));
  QVERIFY(!params.contains("password"));
  QCOMPARE(bridge.m_pendingAction, QStringLiteral("import_certificate"));
}

QTEST_MAIN(IpcBridgeTransientSecretTest)
#include "ipcbridge_transientsecret_test.moc"
