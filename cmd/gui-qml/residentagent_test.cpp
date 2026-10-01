// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "residentagent.h"

#include <QApplication>
#include <QtTest>

class ResidentAgentTest final : public QObject {
  Q_OBJECT

private slots:
  void keepsOnlyBoundedNonSensitiveEvents();
  void disablingClearsEphemeralState();
  void cannotHideWithoutNativeTrayAndWindow();
  void shutdownRejectsNewResidentState();
};

void ResidentAgentTest::keepsOnlyBoundedNonSensitiveEvents() {
  ResidentAgent agent;
  agent.setEnabled(true);
  for (int i = 0; i < 30; ++i)
    agent.recordOperation(QStringLiteral("/home/usuario-prueba/secret-%1.pdf").arg(i),
                          i % 2 == 0);

  const QVariantList events = agent.recentEvents();
  QCOMPARE(events.size(), 24);
  for (const QVariant &value : events) {
    const QVariantMap event = value.toMap();
    QCOMPARE(event.value(QStringLiteral("operation")).toString(),
             QStringLiteral("other"));
    QVERIFY(!event.contains(QStringLiteral("path")));
    QVERIFY(!event.contains(QStringLiteral("message")));
    QVERIFY(!event.value(QStringLiteral("occurredAt")).toString().isEmpty());
  }
}

void ResidentAgentTest::disablingClearsEphemeralState() {
  ResidentAgent agent;
  agent.setEnabled(true);
  agent.recordOperation(QStringLiteral("sign"), true);
  QCOMPARE(agent.recentEvents().size(), 1);

  agent.setEnabled(false);
  QVERIFY(agent.recentEvents().isEmpty());
  QVERIFY(!agent.enabled());
}

void ResidentAgentTest::cannotHideWithoutNativeTrayAndWindow() {
  ResidentAgent agent;
  agent.setEnabled(true);
  QVERIFY(!agent.hideMainWindow());
  QVERIFY(!agent.hidden());
}

void ResidentAgentTest::shutdownRejectsNewResidentState() {
  ResidentAgent agent;
  agent.setEnabled(true);
  agent.recordOperation(QStringLiteral("verify"), false);
  agent.shutdown();
  agent.setEnabled(true);
  agent.recordOperation(QStringLiteral("sign"), true);

  QVERIFY(!agent.enabled());
  QVERIFY(!agent.hidden());
  QVERIFY(agent.recentEvents().isEmpty());
}

int main(int argc, char **argv) {
  qputenv("QT_QPA_PLATFORM", QByteArrayLiteral("offscreen"));
  QApplication app(argc, argv);
  ResidentAgentTest test;
  return QTest::qExec(&test, argc, argv);
}

#include "residentagent_test.moc"
