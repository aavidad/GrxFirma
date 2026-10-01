// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "processenvironment.h"

#include <QByteArray>
#include <QMap>
#include <QtTest>

class ProcessEnvironmentTest : public QObject {
  Q_OBJECT

private slots:
  void sanitizedEnvironmentRemovesSecrets();
  void restEnvironmentReinjectsOnlyExplicitToken();
  void emptyRestTokenStaysAbsent();
  void currentProcessCanBeScrubbed();
};

class EnvironmentRestore {
public:
  explicit EnvironmentRestore(const QStringList &names) {
    for (const QString &name : names) {
      const QByteArray encodedName = name.toLatin1();
      m_values.insert(name, qgetenv(encodedName.constData()));
      m_wasSet.insert(name, qEnvironmentVariableIsSet(encodedName.constData()));
    }
  }

  ~EnvironmentRestore() {
    for (auto it = m_values.constBegin(); it != m_values.constEnd(); ++it) {
      const QByteArray encodedName = it.key().toLatin1();
      if (m_wasSet.value(it.key()))
        qputenv(encodedName.constData(), it.value());
      else
        qunsetenv(encodedName.constData());
    }
  }

private:
  QMap<QString, QByteArray> m_values;
  QMap<QString, bool> m_wasSet;
};

static QProcessEnvironment environmentWithFixtures() {
  QProcessEnvironment environment;
  environment.insert(QStringLiteral("GRXFIRMA_PKCS12_PASSWORD"),
                     QStringLiteral("fixture-p12"));
  environment.insert(QStringLiteral("GRXFIRMA_REST_TOKEN"),
                     QStringLiteral("fixture-token"));
  environment.insert(QStringLiteral("GRXFIRMA_PROTECTION_SECRET_B64"),
                     QStringLiteral("fixture-protection"));
  environment.insert(QStringLiteral("GRXFIRMA_BENIGN"),
                     QStringLiteral("preserved"));
  return environment;
}

void ProcessEnvironmentTest::sanitizedEnvironmentRemovesSecrets() {
  const QProcessEnvironment sanitized =
      ChildProcessEnvironment::sanitized(environmentWithFixtures());

  for (const QString &name :
       ChildProcessEnvironment::sensitiveVariableNames())
    QVERIFY(!sanitized.contains(name));
  QCOMPARE(sanitized.value(QStringLiteral("GRXFIRMA_BENIGN")),
           QStringLiteral("preserved"));
}

void ProcessEnvironmentTest::restEnvironmentReinjectsOnlyExplicitToken() {
  const QProcessEnvironment rest = ChildProcessEnvironment::forRestToken(
      QStringLiteral("explicit-token"), environmentWithFixtures());

  QCOMPARE(rest.value(QStringLiteral("GRXFIRMA_REST_TOKEN")),
           QStringLiteral("explicit-token"));
  QVERIFY(!rest.contains(QStringLiteral("GRXFIRMA_PKCS12_PASSWORD")));
  QVERIFY(
      !rest.contains(QStringLiteral("GRXFIRMA_PROTECTION_SECRET_B64")));
  QCOMPARE(rest.value(QStringLiteral("GRXFIRMA_BENIGN")),
           QStringLiteral("preserved"));
}

void ProcessEnvironmentTest::emptyRestTokenStaysAbsent() {
  const QProcessEnvironment rest = ChildProcessEnvironment::forRestToken(
      QString(), environmentWithFixtures());
  QVERIFY(!rest.contains(QStringLiteral("GRXFIRMA_REST_TOKEN")));
}

void ProcessEnvironmentTest::currentProcessCanBeScrubbed() {
  EnvironmentRestore restore(
      ChildProcessEnvironment::sensitiveVariableNames());
  for (const QString &name :
       ChildProcessEnvironment::sensitiveVariableNames()) {
    const QByteArray encodedName = name.toLatin1();
    QVERIFY(qputenv(encodedName.constData(), QByteArrayLiteral("fixture")));
  }

  QVERIFY(ChildProcessEnvironment::scrubCurrentProcess());

  for (const QString &name :
       ChildProcessEnvironment::sensitiveVariableNames()) {
    const QByteArray encodedName = name.toLatin1();
    QVERIFY(!qEnvironmentVariableIsSet(encodedName.constData()));
  }
}

QTEST_MAIN(ProcessEnvironmentTest)
#include "processenvironment_test.moc"
