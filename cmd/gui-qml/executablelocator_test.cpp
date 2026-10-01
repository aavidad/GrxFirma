// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "executablelocator.h"

#include <QDir>
#include <QFile>
#include <QTemporaryDir>
#include <QtTest>
#include <QtEndian>

class ExecutableLocatorTest : public QObject {
  Q_OBJECT

private slots:
  void windowsAddsOnlyExe();
  void windowsLookupSelectsPackagedExe();
  void windowsDoesNotDuplicateExistingExeSuffix();
  void nonWindowsPreservesCandidateNames();
  void lookupRejectsDirectories();
  void lookupSkipsNonExecutablePlaceholder();
  void windowsPEValidation_data();
  void windowsPEValidation();
  void lookupRejectsOutsidePathsAndSymlinks();
  void lookupRequiresAbsoluteApplicationDir();
  void currentPlatformPolicy();
};

static QByteArray minimalPEExecutable() {
  constexpr int peOffset = 64;
  constexpr int peAndCoffHeaderSize = 24;
  constexpr int optionalHeaderSize = 240;
  constexpr int sectionHeaderSize = 40;
  QByteArray minimalPE(
      peOffset + peAndCoffHeaderSize + optionalHeaderSize +
          sectionHeaderSize,
      '\0');
  minimalPE[0] = 'M';
  minimalPE[1] = 'Z';
  qToLittleEndian<quint32>(
      peOffset, reinterpret_cast<uchar *>(minimalPE.data() + 0x3c));
  minimalPE[peOffset] = 'P';
  minimalPE[peOffset + 1] = 'E';
  qToLittleEndian<quint16>(
      0x8664,
      reinterpret_cast<uchar *>(minimalPE.data() + peOffset + 4));
  qToLittleEndian<quint16>(
      1, reinterpret_cast<uchar *>(minimalPE.data() + peOffset + 6));
  qToLittleEndian<quint16>(
      optionalHeaderSize,
      reinterpret_cast<uchar *>(minimalPE.data() + peOffset + 20));
  qToLittleEndian<quint16>(
      0x0002,
      reinterpret_cast<uchar *>(minimalPE.data() + peOffset + 22));
  qToLittleEndian<quint16>(
      0x020b,
      reinterpret_cast<uchar *>(minimalPE.data() + peOffset +
                                peAndCoffHeaderSize));
  return minimalPE;
}

static bool createFile(const QString &path, bool executable) {
  QFile file(path);
  if (!file.open(QIODevice::WriteOnly))
    return false;
  if (executable && path.endsWith(QStringLiteral(".exe"),
                                  Qt::CaseInsensitive)) {
    file.write(minimalPEExecutable());
  } else {
    file.write("test");
  }
  file.close();
  QFileDevice::Permissions permissions =
      QFileDevice::ReadOwner | QFileDevice::WriteOwner;
  if (executable)
    permissions |= QFileDevice::ExeOwner;
  return QFile::setPermissions(path, permissions);
}

void ExecutableLocatorTest::windowsAddsOnlyExe() {
  const QStringList candidates = ExecutableLocator::candidatesForPlatform(
      {QStringLiteral("grxfirma"), QStringLiteral("grxfirma-gui")},
      true);

  QCOMPARE(candidates,
           QStringList({QStringLiteral("grxfirma.exe"),
                        QStringLiteral("grxfirma-gui.exe")}));
}

void ExecutableLocatorTest::windowsLookupSelectsPackagedExe() {
  QTemporaryDir temporaryDir;
  QVERIFY(temporaryDir.isValid());
  const QString packagedExe = QDir(temporaryDir.path())
                                  .filePath(QStringLiteral("grxfirma.exe"));
  QVERIFY(createFile(packagedExe, true));

  const QString found = ExecutableLocator::bundledExecutableForPlatform(
      temporaryDir.path(), {QStringLiteral("grxfirma")}, true);
  QCOMPARE(found, QFileInfo(packagedExe).canonicalFilePath());
  QVERIFY(QDir::isAbsolutePath(found));
}

void ExecutableLocatorTest::windowsDoesNotDuplicateExistingExeSuffix() {
  const QStringList candidates = ExecutableLocator::candidatesForPlatform(
      {QStringLiteral("grxfirma.exe"),
       QStringLiteral("GRXFIRMA.EXE")},
      true);

  QCOMPARE(candidates,
           QStringList({QStringLiteral("grxfirma.exe"),
                        QStringLiteral("GRXFIRMA.EXE")}));
}

void ExecutableLocatorTest::nonWindowsPreservesCandidateNames() {
  const QStringList candidates = ExecutableLocator::candidatesForPlatform(
      {QStringLiteral("grxfirma"), QStringLiteral("grxfirma-gui")},
      false);

  QCOMPARE(candidates,
           QStringList({QStringLiteral("grxfirma"),
                        QStringLiteral("grxfirma-gui")}));
}

void ExecutableLocatorTest::lookupRejectsDirectories() {
  QTemporaryDir temporaryDir;
  QVERIFY(temporaryDir.isValid());
  const QString directoryCandidate =
      QDir(temporaryDir.path()).filePath(QStringLiteral("grxfirma"));
  QVERIFY(QDir().mkpath(directoryCandidate));

  QCOMPARE(ExecutableLocator::bundledExecutableForPlatform(
               temporaryDir.path(), {QStringLiteral("grxfirma")}, false),
           QString());
}

void ExecutableLocatorTest::lookupSkipsNonExecutablePlaceholder() {
  QTemporaryDir temporaryDir;
  QVERIFY(temporaryDir.isValid());
  const QDir appDir(temporaryDir.path());
  QVERIFY(createFile(appDir.filePath(QStringLiteral("placeholder.exe")),
                     false));
  const QString executable =
      appDir.filePath(QStringLiteral("grxfirma.exe"));
  QVERIFY(createFile(executable, true));

  QCOMPARE(ExecutableLocator::bundledExecutableForPlatform(
               temporaryDir.path(),
               {QStringLiteral("placeholder"),
                QStringLiteral("grxfirma")},
               true),
           QFileInfo(executable).canonicalFilePath());
}

void ExecutableLocatorTest::windowsPEValidation_data() {
  QTest::addColumn<QByteArray>("contents");
  QTest::addColumn<bool>("accepted");

  const QByteArray valid = minimalPEExecutable();
  QTest::newRow("valid-pe32-plus") << valid << true;

  QByteArray pe32 = valid;
  qToLittleEndian<quint16>(
      0x014c, reinterpret_cast<uchar *>(pe32.data() + 64 + 4));
  qToLittleEndian<quint16>(
      0x010b, reinterpret_cast<uchar *>(pe32.data() + 64 + 24));
  QTest::newRow("valid-pe32-i386") << pe32 << true;

  QByteArray malformed = valid;
  malformed[0] = 'N';
  QTest::newRow("bad-dos-signature") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint32>(
      32, reinterpret_cast<uchar *>(malformed.data() + 0x3c));
  QTest::newRow("pe-offset-before-dos-header") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint32>(
      16U * 1024U * 1024U + 1U,
      reinterpret_cast<uchar *>(malformed.data() + 0x3c));
  QTest::newRow("pe-offset-over-limit") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint32>(
      4096, reinterpret_cast<uchar *>(malformed.data() + 0x3c));
  QTest::newRow("pe-offset-after-eof") << malformed << false;

  malformed = valid;
  malformed[64] = 'X';
  QTest::newRow("bad-pe-signature") << malformed << false;
  QTest::newRow("truncated-coff") << valid.left(75) << false;
  QTest::newRow("truncated-optional-header") << valid.left(120)
                                             << false;

  malformed = valid;
  qToLittleEndian<quint16>(
      0, reinterpret_cast<uchar *>(malformed.data() + 64 + 6));
  QTest::newRow("no-sections") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint16>(
      97, reinterpret_cast<uchar *>(malformed.data() + 64 + 6));
  QTest::newRow("too-many-sections") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint16>(
      0x9999,
      reinterpret_cast<uchar *>(malformed.data() + 64 + 4));
  QTest::newRow("unsupported-machine") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint16>(
      0x2002,
      reinterpret_cast<uchar *>(malformed.data() + 64 + 22));
  QTest::newRow("dll-image") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint16>(
      0x010b,
      reinterpret_cast<uchar *>(malformed.data() + 64 + 24));
  QTest::newRow("wrong-optional-magic") << malformed << false;

  malformed = valid;
  qToLittleEndian<quint16>(
      64, reinterpret_cast<uchar *>(malformed.data() + 64 + 20));
  QTest::newRow("optional-header-too-small") << malformed << false;
}

void ExecutableLocatorTest::windowsPEValidation() {
  QFETCH(QByteArray, contents);
  QFETCH(bool, accepted);

  QTemporaryDir temporaryDir;
  QVERIFY(temporaryDir.isValid());
  const QString path =
      QDir(temporaryDir.path()).filePath(QStringLiteral("candidate.exe"));
  QFile file(path);
  QVERIFY(file.open(QIODevice::WriteOnly));
  QCOMPARE(file.write(contents), static_cast<qint64>(contents.size()));
  file.close();
  QCOMPARE(ExecutableLocator::hasWindowsPESignature(path), accepted);
}

void ExecutableLocatorTest::lookupRejectsOutsidePathsAndSymlinks() {
  QTemporaryDir temporaryDir;
  QVERIFY(temporaryDir.isValid());
  const QDir root(temporaryDir.path());
  QVERIFY(root.mkpath(QStringLiteral("app")));
  const QString appDir = root.filePath(QStringLiteral("app"));
  const QString outside = root.filePath(QStringLiteral("outside"));
  QVERIFY(createFile(outside, true));

  QCOMPARE(ExecutableLocator::bundledExecutableForPlatform(
               appDir, {QStringLiteral("../outside")}, false),
           QString());
  QCOMPARE(ExecutableLocator::bundledExecutableForPlatform(
               appDir, {outside}, false),
           QString());

  const QString symlink =
      QDir(appDir).filePath(QStringLiteral("grxfirma"));
  if (!QFile::link(outside, symlink))
    QSKIP("El sistema de archivos no permite crear el enlace de prueba");
  QCOMPARE(ExecutableLocator::bundledExecutableForPlatform(
               appDir, {QStringLiteral("grxfirma")}, false),
           QString());
}

void ExecutableLocatorTest::lookupRequiresAbsoluteApplicationDir() {
  QCOMPARE(ExecutableLocator::bundledExecutableForPlatform(
               QStringLiteral("."), {QStringLiteral("grxfirma")}, false),
           QString());
}

void ExecutableLocatorTest::currentPlatformPolicy() {
  const QStringList candidates =
      ExecutableLocator::candidates({QStringLiteral("grxfirma")});
#ifdef Q_OS_WIN
  QCOMPARE(candidates, QStringList({QStringLiteral("grxfirma.exe")}));
#else
  QCOMPARE(candidates, QStringList({QStringLiteral("grxfirma")}));
#endif
}

QTEST_MAIN(ExecutableLocatorTest)
#include "executablelocator_test.moc"
