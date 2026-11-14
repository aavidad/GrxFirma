// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "translatorbridge.h"
#include <QtTest>

class TranslatorAccessibilityTest : public QObject {
  Q_OBJECT
private slots:
  void navigationIncludesAllLanguages() {
    TranslatorBridge translator;
    const QStringList labels = translator.navigationLabels();
    QVERIFY(labels.contains(QStringLiteral("VERSCHLÜSSELN")));
    QVERIFY(labels.contains(QStringLiteral("DOKUMENTUAK")) ||
            labels.contains(QStringLiteral("ENI DOKUMENTUAK")));
    QVERIFY(labels.contains(QStringLiteral("DOCUMENTOS ENI")));
    QVERIFY(labels.contains(QStringLiteral("签名")));
    const QStringList before = labels;
    translator.setLocale(QStringLiteral("eu"));
    QCOMPARE(translator.navigationLabels(), before);
  }
  void legacyKeysRemainReadableInEveryLanguage() {
    TranslatorBridge translator;
    for (const QString &locale : {"es", "en", "fr", "de", "it", "pt", "gl",
                                  "eu", "ca", "va", "zh"}) {
      translator.setLocale(locale);
      const QString clean = translator.t(QStringLiteral("Firmando..."));
      QCOMPARE(translator.t(QStringLiteral("⌛ Firmando...")), clean);
      QVERIFY(!clean.startsWith(QStringLiteral("⌛")));
      QVERIFY(!translator.t(QStringLiteral("qt.accessibility.text_size"))
                   .startsWith(QStringLiteral("qt.")));
    }
  }
  void fallbackRemovesOnlyTheDecorativePrefix() {
    TranslatorBridge translator;
    QCOMPARE(translator.t(QStringLiteral("⚙️ Mensaje técnico sin catálogo")),
             QStringLiteral("Mensaje técnico sin catálogo"));
    QCOMPARE(translator.t(QStringLiteral("🔑 Credencial: 123")),
             QStringLiteral("Credencial: 123"));
    QCOMPARE(translator.t(QStringLiteral("Texto con ✓ dentro")),
             QStringLiteral("Texto con ✓ dentro"));
    QCOMPARE(translator.t(QStringLiteral("+%1")), QStringLiteral("+%1"));
  }
};

QTEST_GUILESS_MAIN(TranslatorAccessibilityTest)
#include "translatorbridge_test.moc"
