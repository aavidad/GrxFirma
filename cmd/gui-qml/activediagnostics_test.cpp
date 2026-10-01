// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "activediagnostics.h"

#include <QJsonDocument>
#include <QJsonObject>
#include <QSignalSpy>
#include <QTcpServer>
#include <QtTest>

class ActiveDiagnosticsTest final : public QObject {
  Q_OBJECT

private slots:
  void neverRunsWithoutExplicitConsent();
  void requiresPostFailureContext();
  void rejectsNonLoopbackTargetWithoutLeakingIt();
  void preservesClosedPassiveCategories();
  void classifiesUnavailableLocalChannel();
  void probesBoundedLocalTCPService();
  void tlsTimeoutEmitsOneResult();
  void cancellationCannotEmitLateResult();
};

void ActiveDiagnosticsTest::neverRunsWithoutExplicitConsent() {
  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(QStringLiteral("127.0.0.1:63118"), true, QStringList(), true,
               QVariantMap{{QStringLiteral("postFailure"), true}}, false);
  QCOMPARE(spy.size(), 1);
  const QVariantMap result = spy.first().at(1).toMap();
  QCOMPARE(result.value(QStringLiteral("errorCode")).toString(),
           QStringLiteral("consent_required"));
  QVERIFY(!runner.running());
}

void ActiveDiagnosticsTest::requiresPostFailureContext() {
  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(QStringLiteral("127.0.0.1:63118"), true, QStringList(), true,
               QVariantMap(), true);
  QCOMPARE(spy.size(), 1);
  const QVariantMap result = spy.first().at(1).toMap();
  QCOMPARE(result.value(QStringLiteral("errorCode")).toString(),
           QStringLiteral("post_failure_required"));
}

void ActiveDiagnosticsTest::rejectsNonLoopbackTargetWithoutLeakingIt() {
  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(
      QStringLiteral("example.test:443"), true, QStringList(), true,
      QVariantMap{{QStringLiteral("postFailure"), true}},
      true);
  QCOMPARE(spy.size(), 1);
  const QVariantMap result = spy.first().at(1).toMap();
  const QByteArray serialized =
      QJsonDocument(QJsonObject::fromVariantMap(result))
          .toJson(QJsonDocument::Compact);
  QVERIFY(!serialized.contains("example.test"));
  QVERIFY(!serialized.contains("dns_resolution"));
  QVERIFY(!serialized.contains("tcp_reachability"));
  QCOMPARE(result.value(QStringLiteral("probableCause")).toString(),
           QStringLiteral("insufficient_evidence"));
}

void ActiveDiagnosticsTest::preservesClosedPassiveCategories() {
  for (const QString &category :
       {QStringLiteral("local_web_service"),
        QStringLiteral("remote_service"),
        QStringLiteral("government_afirma")}) {
    ActiveDiagnosticsRunner runner;
    QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
    runner.start(
        QString(), false, QStringList(), true,
        QVariantMap{{QStringLiteral("postFailure"), true},
                    {QStringLiteral("failureCategory"), category}},
        true);
    QCOMPARE(spy.size(), 1);
    QCOMPARE(
        spy.first().at(1).toMap().value(QStringLiteral("passiveCategory"))
            .toString(),
        category);
  }

  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(
      QString(), false, QStringList(), true,
      QVariantMap{{QStringLiteral("postFailure"), true},
                  {QStringLiteral("failureCategory"),
                   QStringLiteral("arbitrary_remote_target")}},
      true);
  QCOMPARE(spy.size(), 1);
  QCOMPARE(
      spy.first().at(1).toMap().value(QStringLiteral("passiveCategory"))
          .toString(),
      QStringLiteral("unknown"));
}

void ActiveDiagnosticsTest::classifiesUnavailableLocalChannel() {
  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(
      QString(), false, QStringList(), false,
      QVariantMap{{QStringLiteral("postFailure"), true},
                  {QStringLiteral("failureCategory"),
                   QStringLiteral("app_local")}},
      true);
  QCOMPARE(spy.size(), 1);
  const QVariantMap result = spy.first().at(1).toMap();
  QCOMPARE(result.value(QStringLiteral("probableCause")).toString(),
           QStringLiteral("local_backend_unavailable"));
  QCOMPARE(result.value(QStringLiteral("likelyOwner")).toString(),
           QStringLiteral("app_local"));
}

void ActiveDiagnosticsTest::probesBoundedLocalTCPService() {
  QTcpServer server;
  QVERIFY(server.listen(QHostAddress::LocalHost));

  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(
      QStringLiteral("127.0.0.1:%1").arg(server.serverPort()), false,
      QStringList(), true,
      QVariantMap{{QStringLiteral("postFailure"), true}}, true);
  QVERIFY(spy.wait(4000));
  const QVariantMap result = spy.first().at(1).toMap();
  QCOMPARE(result.value(QStringLiteral("completed")).toBool(), true);
  const QByteArray serialized =
      QJsonDocument(QJsonObject::fromVariantMap(result))
          .toJson(QJsonDocument::Compact);
  QVERIFY(serialized.contains("tcp_reachability"));
  QVERIFY(serialized.contains("\"state\":\"ok\""));
  QVERIFY(!serialized.contains("127.0.0.1"));
  QVERIFY(!serialized.contains(QByteArray::number(server.serverPort())));
}

void ActiveDiagnosticsTest::tlsTimeoutEmitsOneResult() {
  QTcpServer server;
  QVERIFY(server.listen(QHostAddress::LocalHost));

  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(
      QStringLiteral("127.0.0.1:%1").arg(server.serverPort()), true,
      QStringList(), true,
      QVariantMap{{QStringLiteral("postFailure"), true}}, true);
  QVERIFY(spy.wait(4500));
  QCOMPARE(spy.size(), 1);
  const QByteArray serialized =
      QJsonDocument(QJsonObject::fromVariantMap(spy.first().at(1).toMap()))
          .toJson(QJsonDocument::Compact);
  QVERIFY(serialized.contains("\"kind\":\"tls_handshake\""));
  QVERIFY(serialized.contains("\"state\":\"failed\""));
}

void ActiveDiagnosticsTest::cancellationCannotEmitLateResult() {
  QTcpServer server;
  QVERIFY(server.listen(QHostAddress::LocalHost));

  ActiveDiagnosticsRunner runner;
  QSignalSpy spy(&runner, &ActiveDiagnosticsRunner::finished);
  runner.start(
      QStringLiteral("localhost:%1").arg(server.serverPort()), true,
      QStringList{QString(64, QLatin1Char('a'))}, true,
      QVariantMap{{QStringLiteral("postFailure"), true}}, true);
  QVERIFY(runner.running());
  runner.cancel();
  QVERIFY(!runner.running());
  QTest::qWait(3200);
  QCOMPARE(spy.size(), 0);
}

QTEST_MAIN(ActiveDiagnosticsTest)

#include "activediagnostics_test.moc"
