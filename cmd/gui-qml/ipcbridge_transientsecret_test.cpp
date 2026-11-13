// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "ipcbridge.h"
#include "transientsecret.h"

#include <QElapsedTimer>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTemporaryFile>
#include <QLocalServer>
#include <QLocalSocket>
#include <QUuid>
#include <QtTest>

class IpcBridgeTransientSecretTest : public QObject {
  Q_OBJECT

private slots:
  void disconnectedProtectIsNotDeferred();
  void disconnectedUnprotectIsNotDeferred();
  void droppedProtectResponseFailsExactlyOnce();
  void droppedUnprotectResponseFailsExactlyOnce();
  void droppedVerifyResponseFailsExactlyOnce();
  void tokenSettingsDisconnectedNeverDeferred();
  void tokenSettingsCorrelatesAndFinishesOnDisconnect();
  void tokenSettingsRejectsRemoteModuleURLs();
  void tokenSettingsTimeoutIgnoresLateReply();
  void tokenSettingsUsesFixedErrorCodeNotMessage();
  void unexpectedServerReceivesNothing();
  void expectedServerReceivesDeferredRequest();
  void secretFrameZeroesInputs();
  void importCertificateSendsPasswordB64();

private:
  QLocalSocket *connectPeer(IpcBridge &bridge, QLocalServer &server);
};

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
