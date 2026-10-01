// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "transientsecret.h"

#include <QtTest>

class TransientSecretTest : public QObject {
  Q_OBJECT

private slots:
  void acceptsCanonicalAES256Key();
  void rejectsMalformedOrWrongSizedKeys();
  void normalizesEncryptedDataOutputSuffix();
  void zeroizesOwnedBuffers();
};

void TransientSecretTest::acceptsCanonicalAES256Key() {
  const QString encoded =
      QString::fromLatin1(QByteArray(TransientSecret::AES256KeyBytes, '\x5a')
                              .toBase64());
  QVERIFY(TransientSecret::isCanonicalAES256Base64(encoded));
}

void TransientSecretTest::rejectsMalformedOrWrongSizedKeys() {
  QVERIFY(!TransientSecret::isCanonicalAES256Base64(QString()));
  QVERIFY(!TransientSecret::isCanonicalAES256Base64(
      QString::fromLatin1(QByteArray(31, '\x5a').toBase64())));
  QVERIFY(!TransientSecret::isCanonicalAES256Base64(
      QString::fromLatin1(QByteArray(33, '\x5a').toBase64())));
  QVERIFY(!TransientSecret::isCanonicalAES256Base64(
      QStringLiteral("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!=")));

  QString nonCanonical =
      QString::fromLatin1(QByteArray(TransientSecret::AES256KeyBytes, '\x5a')
                              .toBase64());
  nonCanonical[42] = QLatin1Char('b');
  QVERIFY(!TransientSecret::isCanonicalAES256Base64(nonCanonical));
}

void TransientSecretTest::normalizesEncryptedDataOutputSuffix() {
  QCOMPARE(TransientSecret::normalizedEncryptedDataOutputPath(QString()),
           QString());
  QCOMPARE(TransientSecret::normalizedEncryptedDataOutputPath(
               QStringLiteral(" /tmp/documento ")),
           QStringLiteral("/tmp/documento.encrypted.p7m"));
  QCOMPARE(TransientSecret::normalizedEncryptedDataOutputPath(
               QStringLiteral("/tmp/documento.encrypted.p7m")),
           QStringLiteral("/tmp/documento.encrypted.p7m"));
  QCOMPARE(TransientSecret::normalizedEncryptedDataOutputPath(
               QStringLiteral("/tmp/documento.ENCRYPTED.P7M")),
           QStringLiteral("/tmp/documento.ENCRYPTED.P7M"));
}

void TransientSecretTest::zeroizesOwnedBuffers() {
  QString text = QStringLiteral("sensitive");
  QByteArray bytes("sensitive");
  TransientSecret::zeroize(text);
  TransientSecret::zeroize(bytes);
  QVERIFY(text.isEmpty());
  QVERIFY(bytes.isEmpty());
}

QTEST_MAIN(TransientSecretTest)
#include "transientsecret_test.moc"
