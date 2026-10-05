// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "ipcbridge.h"
#include "transientsecret.h"

#include <QElapsedTimer>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocalServer>
#include <QLocalSocket>
#include <QUuid>
#include <QtTest>

class IpcBridgeTransientSecretTest : public QObject {
  Q_OBJECT

private slots:
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

QTEST_MAIN(IpcBridgeTransientSecretTest)
#include "ipcbridge_transientsecret_test.moc"
