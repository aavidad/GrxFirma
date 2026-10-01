// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "processarguments.h"
#include "ipcsocketpath.h"

#include <QtTest>

class ProcessArgumentsTest : public QObject {
  Q_OBJECT

private slots:
  void rejectsDirectSensitiveNames_data();
  void rejectsDirectSensitiveNames();
  void rejectsGenericOptionBypasses_data();
  void rejectsGenericOptionBypasses();
  void permitsQtAndGrxFirmaOptions_data();
  void permitsQtAndGrxFirmaOptions();
  void platformSpecificSlashPolicy();
  void normalizesWindowsPipePaths_data();
  void normalizesWindowsPipePaths();
};

void ProcessArgumentsTest::rejectsDirectSensitiveNames_data() {
  QTest::addColumn<QStringList>("arguments");

  QTest::newRow("password-inline")
      << QStringList({QStringLiteral("--password=REDACTED")});
  QTest::newRow("contrasena")
      << QStringList({QStringLiteral("-contrasena"),
                      QStringLiteral("REDACTED")});
  QTest::newRow("contrasena-unicode")
      << QStringList({QStringLiteral("--contraseña=REDACTED")});
  QTest::newRow("rest-token")
      << QStringList({QStringLiteral("--rest-token=REDACTED")});
  QTest::newRow("protection-secret")
      << QStringList(
             {QStringLiteral("--protection-secret-b64=REDACTED")});
  QTest::newRow("authorization")
      << QStringList({QStringLiteral("--authorization=REDACTED")});
  QTest::newRow("bearer")
      << QStringList({QStringLiteral("--bearer=REDACTED")});
  QTest::newRow("api-key")
      << QStringList({QStringLiteral("--api-key=REDACTED")});
  QTest::newRow("private-key")
      << QStringList({QStringLiteral("--private-key=REDACTED")});
  QTest::newRow("pin")
      << QStringList({QStringLiteral("--certificate-pin=REDACTED")});
  QTest::newRow("credential")
      << QStringList({QStringLiteral("--credential=REDACTED")});
  QTest::newRow("safe-source-with-inline-secret")
      << QStringList(
             {QStringLiteral("--p12-password-stdin=REDACTED")});
  QTest::newRow("profile-is-not-a-file-source")
      << QStringList({QStringLiteral("--password-profile"),
                      QStringLiteral("REDACTED")});
}

void ProcessArgumentsTest::rejectsDirectSensitiveNames() {
  QFETCH(QStringList, arguments);
  QVERIFY(ProcessArguments::containsSensitiveOptionName(arguments));
}

void ProcessArgumentsTest::rejectsGenericOptionBypasses_data() {
  QTest::addColumn<QStringList>("arguments");

  QTest::newRow("opcion-next-secret")
      << QStringList({QStringLiteral("-opcion"),
                      QStringLiteral("secret_b64=REDACTED")});
  QTest::newRow("opcion-inline-authorization")
      << QStringList(
             {QStringLiteral("--opcion=authorization=REDACTED")});
  QTest::newRow("option-next-bearer")
      << QStringList({QStringLiteral("-option"),
                      QStringLiteral("bearer=REDACTED")});
  QTest::newRow("opcion-next-api-key")
      << QStringList({QStringLiteral("--opcion"),
                      QStringLiteral("api_key=REDACTED")});
  QTest::newRow("option-inline-private-key")
      << QStringList(
             {QStringLiteral("--option=private_key=REDACTED")});
  QTest::newRow("opcion-next-pin")
      << QStringList(
             {QStringLiteral("-opcion"), QStringLiteral("pin=REDACTED")});
  QTest::newRow("option-next-credential")
      << QStringList({QStringLiteral("-option"),
                      QStringLiteral("credential=REDACTED")});
}

void ProcessArgumentsTest::rejectsGenericOptionBypasses() {
  QFETCH(QStringList, arguments);
  QVERIFY(ProcessArguments::containsSensitiveOptionName(arguments));
}

void ProcessArgumentsTest::permitsQtAndGrxFirmaOptions_data() {
  QTest::addColumn<QStringList>("arguments");

  QTest::newRow("qt-platform")
      << QStringList({QStringLiteral("-platform"),
                      QStringLiteral("windows")});
  QTest::newRow("qt-platform-plugin-path")
      << QStringList({QStringLiteral("-platformpluginpath"),
                      QStringLiteral("C:/Qt/plugins")});
  QTest::newRow("qt-style")
      << QStringList(
             {QStringLiteral("-style"), QStringLiteral("Fusion")});
  QTest::newRow("qt-qml-debugger")
      << QStringList(
             {QStringLiteral("-qmljsdebugger=port:3768,block")});
  QTest::newRow("grxfirma-rest")
      << QStringList({QStringLiteral("--rest")});
  QTest::newRow("grxfirma-ipc")
      << QStringList({QStringLiteral("--ipc")});
  QTest::newRow("grxfirma-ipc-socket")
      << QStringList(
             {QStringLiteral("--ipc-socket=/tmp/password-in-path.sock")});
  QTest::newRow("grxfirma-ipc-socket-separate")
      << QStringList({QStringLiteral("--ipc-socket"),
                      QStringLiteral("/tmp/password-in-path.sock")});
  QTest::newRow("safe-password-source")
      << QStringList({QStringLiteral("--p12-password-stdin")});
  QTest::newRow("safe-protection-file-source")
      << QStringList({QStringLiteral("--protection-secret-file"),
                      QStringLiteral("C:/qa/password.pdf")});
  QTest::newRow("windows-path-with-sensitive-word")
      << QStringList({QStringLiteral("-platformpluginpath"),
                      QStringLiteral("C:/Qt/password-plugins.dll")});
  QTest::newRow("harmless-unknown")
      << QStringList({QStringLiteral("--theme=contrast")});
  QTest::newRow("sensitive-looking-positional-name")
      << QStringList({QStringLiteral("secret-document.pdf")});
  QTest::newRow("positional-opcion-does-not-wrap-next-value")
      << QStringList({QStringLiteral("opcion"),
                      QStringLiteral("secret-document.pdf")});
  QTest::newRow("generic-option-harmless-key")
      << QStringList(
             {QStringLiteral("-opcion"),
              QStringLiteral("color=secret-in-value-is-not-a-name")});
}

void ProcessArgumentsTest::permitsQtAndGrxFirmaOptions() {
  QFETCH(QStringList, arguments);
  QVERIFY(!ProcessArguments::containsSensitiveOptionName(arguments));
}

void ProcessArgumentsTest::platformSpecificSlashPolicy() {
  QVERIFY(ProcessArguments::containsSensitiveOptionNameForPlatform(
      {QStringLiteral("/REST-TOKEN=REDACTED")}, true));
  QVERIFY(!ProcessArguments::containsSensitiveOptionNameForPlatform(
      {QStringLiteral("/password.pdf")}, false));
  QVERIFY(ProcessArguments::containsSensitiveOptionNameForPlatform(
      {QStringLiteral("/password.pdf")}, true));
  QVERIFY(!ProcessArguments::containsSensitiveOptionNameForPlatform(
      {QStringLiteral("--protection-secret-file"),
       QStringLiteral("/password.pdf")},
      false));
}

void ProcessArgumentsTest::normalizesWindowsPipePaths_data() {
  QTest::addColumn<QString>("candidate");
  QTest::addColumn<QString>("expected");

  const QString prefix = QStringLiteral("\\\\.\\pipe\\");
  QTest::newRow("simple") << QStringLiteral("grxfirma_123")
                          << prefix + QStringLiteral("grxfirma_123");
  QTest::newRow("full") << prefix + QStringLiteral("grxfirma_456")
                        << prefix + QStringLiteral("grxfirma_456");
  QTest::newRow("full-prefix-case")
      << QStringLiteral("\\\\.\\PIPE\\grxfirma_789")
      << prefix + QStringLiteral("grxfirma_789");
  QTest::newRow("empty") << QString() << QString();
  QTest::newRow("empty-full") << prefix << QString();
  QTest::newRow("remote") << QStringLiteral("\\\\server\\pipe\\grxfirma")
                           << QString();
  QTest::newRow("nested") << prefix + QStringLiteral("dir\\grxfirma")
                           << QString();
  QTest::newRow("control") << prefix + QStringLiteral("auto\nfirma")
                            << QString();
  QTest::newRow("too-long")
      << prefix + QString(300, QLatin1Char('a')) << QString();
}

void ProcessArgumentsTest::normalizesWindowsPipePaths() {
  QFETCH(QString, candidate);
  QFETCH(QString, expected);
  QCOMPARE(IpcSocketPath::normalizeWindows(candidate), expected);
}

QTEST_MAIN(ProcessArgumentsTest)
#include "processarguments_test.moc"
