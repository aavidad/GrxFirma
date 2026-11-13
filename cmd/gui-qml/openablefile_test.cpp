// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "openablefile.h"

#include <QtTest>

// D-02: «Abrir» no entrega al sistema tipos que se ejecutan.
class OpenableFileTest : public QObject {
  Q_OBJECT

private slots:
  void rejectsExecutableAndUnknownTypes_data() {
    QTest::addColumn<QString>("path");
    for (const char *name :
         {"nomina_desprotegido.exe", "a.lnk", "a.hta", "a.js", "a.scr",
          "a.bat", "a.cmd", "a.ps1", "a.vbs", "a.msi", "a.cpl", "a.url",
          "a.desktop", "a.sh", "a.app", "a.jar", "a.txt:evil", "sin_extension",
          "a.afp", "a.enveloped", "a.bin", "a.docm", "a.svg", "a.html"}) {
      QTest::newRow(name) << QString::fromLatin1(name);
    }
  }

  void rejectsExecutableAndUnknownTypes() {
    QFETCH(QString, path);
    QVERIFY(!OpenableFile::isDocumentToOpen(path));
  }

  void acceptsDocumentTypes_data() {
    QTest::addColumn<QString>("path");
    for (const char *name :
         {"a.pdf", "a.PDF", "a.xml", "a.txt", "a.docx", "a.xlsx", "a.odt",
          "a.png", "a.jpg", "firma.xsig", "firma.csig", "firma.p7s",
          "contenedor.asice"}) {
      QTest::newRow(name) << QString::fromLatin1(name);
    }
  }

  void acceptsDocumentTypes() {
    QFETCH(QString, path);
    QVERIFY(OpenableFile::isDocumentToOpen(path));
  }
};

QTEST_GUILESS_MAIN(OpenableFileTest)
#include "openablefile_test.moc"
