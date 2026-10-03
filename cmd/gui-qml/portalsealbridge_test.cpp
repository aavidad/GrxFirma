// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "portalsealbridge.h"
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QtTest>

class PortalSealBridgeTest final : public QObject {
  Q_OBJECT
private slots:
  void rejectsUnexpectedPaths();
  void writesExplicitDecision();
  void writesPlacementAppearance();
};

static QString writeRequest(const QString &dir, const QString &result) {
  const QString pdf = dir + QStringLiteral("/document.pdf");
  QFile pdfFile(pdf);
  if (!pdfFile.open(QIODevice::WriteOnly)) return {};
  pdfFile.write("%PDF-1.4\n");
  pdfFile.close();
  const QString request = dir + QStringLiteral("/request.json");
  QFile requestFile(request);
  if (!requestFile.open(QIODevice::WriteOnly)) return {};
  QJsonObject content{{QStringLiteral("documentPath"), pdf},
                      {QStringLiteral("resultPath"), result},
                      {QStringLiteral("signerName"), QStringLiteral("Test signer")}};
  requestFile.write(QJsonDocument(content).toJson(QJsonDocument::Compact));
  return request;
}

void PortalSealBridgeTest::rejectsUnexpectedPaths() {
  QTemporaryDir privateDir;
  QTemporaryDir otherDir;
  QVERIFY(privateDir.isValid());
  QVERIFY(otherDir.isValid());
  const QString request = writeRequest(privateDir.path(),
                                       otherDir.path() + QStringLiteral("/result.json"));
  QVERIFY(!request.isEmpty());
  PortalSealBridge bridge;
  QVERIFY(!bridge.load(request));
}

void PortalSealBridgeTest::writesExplicitDecision() {
  QTemporaryDir privateDir;
  QVERIFY(privateDir.isValid());
  const QString result = privateDir.path() + QStringLiteral("/result.json");
  const QString request = writeRequest(privateDir.path(), result);
  PortalSealBridge bridge;
  QVERIFY(bridge.load(request));
  QVERIFY(bridge.active());
  QCOMPARE(bridge.signerName(), QStringLiteral("Test signer"));
  QVERIFY(!bridge.submit(QStringLiteral("place"), {}, {}));
  QVERIFY(bridge.submit(QStringLiteral("without"), {}, {}));
  QFile resultFile(result);
  QVERIFY(resultFile.open(QIODevice::ReadOnly));
  const auto decision = QJsonDocument::fromJson(resultFile.readAll()).object();
  QCOMPARE(decision.value(QStringLiteral("action")).toString(),
           QStringLiteral("without"));
  QVERIFY(!bridge.submit(QStringLiteral("cancel"), {}, {}));
}

void PortalSealBridgeTest::writesPlacementAppearance() {
  QTemporaryDir privateDir;
  QVERIFY(privateDir.isValid());
  const QString result = privateDir.path() + QStringLiteral("/result.json");
  PortalSealBridge bridge;
  QVERIFY(bridge.load(writeRequest(privateDir.path(), result)));
  const QVariantMap rect{{QStringLiteral("x"), 0.2},
                         {QStringLiteral("y"), 0.1},
                         {QStringLiteral("w"), 0.3},
                         {QStringLiteral("h"), 0.2}};
  const QVariantMap placement{{QStringLiteral("page"), 1},
                              {QStringLiteral("rect"), rect},
                              {QStringLiteral("rotation"), 45}};
  const QVariantMap appearance{{QStringLiteral("logo"), QStringLiteral("institutional")},
                               {QStringLiteral("opacityPercent"), 40.0}};
  QVERIFY(bridge.submit(QStringLiteral("place"), {placement}, appearance));
  QFile resultFile(result);
  QVERIFY(resultFile.open(QIODevice::ReadOnly));
  const QByteArray encoded = resultFile.readAll();
  QVERIFY(encoded.contains("\"opacityPercent\":40"));
  QCOMPARE(QJsonDocument::fromJson(encoded).object()
               .value(QStringLiteral("visibleSealPlacements"))
               .toArray().at(0).toObject().value(QStringLiteral("rotation")).toInt(), 45);
}

QTEST_GUILESS_MAIN(PortalSealBridgeTest)
#include "portalsealbridge_test.moc"
