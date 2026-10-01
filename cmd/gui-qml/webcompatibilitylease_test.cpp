// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "webcompatibilitylease.h"

#include <QSignalSpy>
#include <QtTest>

class WebCompatibilityLeaseTest final : public QObject {
  Q_OBJECT

private slots:
  void expiresWithoutLongSleeps();
  void rejectsInvalidDurationsFailClosed();
  void restartReplacesPreviousExpiry();
  void cancelPreventsExpiry();
  void expiryStopsOnlyTheOwnedWebServer();
};

void WebCompatibilityLeaseTest::expiresWithoutLongSleeps() {
  int stops = 0;
  WebCompatibilityLease lease([&stops]() { ++stops; });
  QSignalSpy expired(&lease, &WebCompatibilityLease::expired);

  QVERIFY(lease.startMilliseconds(20));
  QVERIFY(lease.active());
  QTRY_COMPARE_WITH_TIMEOUT(expired.count(), 1, 250);
  QVERIFY(!lease.active());
  QCOMPARE(stops, 1);
  QTest::qWait(30);
  QCOMPARE(stops, 1);
}

void WebCompatibilityLeaseTest::rejectsInvalidDurationsFailClosed() {
  WebCompatibilityLease lease([]() {});
  QVERIFY(lease.startMilliseconds(100));

  QVERIFY(!lease.startMilliseconds(0));
  QVERIFY(!lease.active());
  QVERIFY(!lease.startMilliseconds(-1));
  QVERIFY(lease.startMilliseconds(240 * 60 * 1000));
  QVERIFY(lease.active());
  lease.cancel();
  QVERIFY(!lease.startMilliseconds(240 * 60 * 1000 + 1));
}

void WebCompatibilityLeaseTest::restartReplacesPreviousExpiry() {
  int stops = 0;
  WebCompatibilityLease lease([&stops]() { ++stops; });
  QSignalSpy expired(&lease, &WebCompatibilityLease::expired);

  QVERIFY(lease.startMilliseconds(15));
  QTest::qWait(5);
  QVERIFY(lease.startMilliseconds(80));
  QTest::qWait(25);
  QCOMPARE(expired.count(), 0);
  QVERIFY(lease.active());
  QTRY_COMPARE_WITH_TIMEOUT(expired.count(), 1, 250);
  QCOMPARE(stops, 1);
}

void WebCompatibilityLeaseTest::cancelPreventsExpiry() {
  int stops = 0;
  WebCompatibilityLease lease([&stops]() { ++stops; });
  QSignalSpy expired(&lease, &WebCompatibilityLease::expired);

  QVERIFY(lease.startMilliseconds(20));
  lease.cancel();
  QVERIFY(!lease.active());
  QTest::qWait(40);
  QCOMPARE(expired.count(), 0);
  QCOMPARE(stops, 0);
}

void WebCompatibilityLeaseTest::expiryStopsOnlyTheOwnedWebServer() {
  bool primaryBackendRunning = true;
  bool ownedWebServerRunning = true;
  int webStops = 0;
  WebCompatibilityLease lease([&]() {
    ownedWebServerRunning = false;
    ++webStops;
  });
  QSignalSpy expired(&lease, &WebCompatibilityLease::expired);

  QVERIFY(lease.startMilliseconds(20));
  QTRY_COMPARE_WITH_TIMEOUT(expired.count(), 1, 250);
  QVERIFY(primaryBackendRunning);
  QVERIFY(!ownedWebServerRunning);
  QCOMPARE(webStops, 1);
}

QTEST_MAIN(WebCompatibilityLeaseTest)

#include "webcompatibilitylease_test.moc"
