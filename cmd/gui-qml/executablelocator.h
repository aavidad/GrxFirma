// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef EXECUTABLELOCATOR_H
#define EXECUTABLELOCATOR_H

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QString>
#include <QStringList>
#include <QtEndian>

namespace ExecutableLocator {

inline bool isSafeBaseName(const QString &baseName) {
  if (baseName.isEmpty() || baseName == QStringLiteral(".") ||
      baseName == QStringLiteral("..") ||
      QDir::isAbsolutePath(baseName) ||
      baseName.contains(QLatin1Char('/')) ||
      baseName.contains(QLatin1Char('\\'))) {
    return false;
  }
  return QFileInfo(baseName).fileName() == baseName;
}

// Windows packages always contain PE executables with an explicit .exe
// suffix. Other platforms must keep their native unsuffixed names.
inline QStringList candidatesForPlatform(const QStringList &baseNames,
                                         bool windowsPlatform) {
  QStringList candidates;
  for (const QString &baseName : baseNames) {
    if (!isSafeBaseName(baseName))
      continue;
    if (windowsPlatform && !baseName.endsWith(QStringLiteral(".exe"),
                                              Qt::CaseInsensitive))
      candidates.append(baseName + QStringLiteral(".exe"));
    else
      candidates.append(baseName);
  }
  candidates.removeDuplicates();
  return candidates;
}

inline QStringList candidates(const QStringList &baseNames) {
#ifdef Q_OS_WIN
  return candidatesForPlatform(baseNames, true);
#else
  return candidatesForPlatform(baseNames, false);
#endif
}

inline bool isWithinDirectory(const QString &canonicalDirectory,
                              const QString &canonicalFile) {
  const QString relative = QDir::fromNativeSeparators(
      QDir(canonicalDirectory).relativeFilePath(canonicalFile));
  return !relative.isEmpty() && relative != QStringLiteral("..") &&
         !relative.startsWith(QStringLiteral("../")) &&
         !QDir::isAbsolutePath(relative);
}

inline bool hasWindowsPESignature(const QString &path) {
  constexpr qint64 dosHeaderSize = 64;
  constexpr quint32 maxPEOffset = 16U * 1024U * 1024U;
  constexpr qint64 peAndCoffHeaderSize = 24;
  constexpr qint64 sectionHeaderSize = 40;
  constexpr quint16 maxSections = 96;
  constexpr quint16 imageFileExecutableImage = 0x0002;
  constexpr quint16 imageFileDLL = 0x2000;
  constexpr quint16 machineI386 = 0x014c;
  constexpr quint16 machineAMD64 = 0x8664;
  constexpr quint16 machineARM64 = 0xaa64;
  constexpr quint16 pe32Magic = 0x010b;
  constexpr quint16 pe32PlusMagic = 0x020b;
  constexpr quint16 minPE32OptionalHeader = 96;
  constexpr quint16 minPE32PlusOptionalHeader = 112;

  QFile file(path);
  if (!file.open(QIODevice::ReadOnly))
    return false;
  const qint64 fileSize = file.size();
  if (fileSize < dosHeaderSize + peAndCoffHeaderSize)
    return false;

  const QByteArray dosHeader = file.read(dosHeaderSize);
  if (dosHeader.size() != dosHeaderSize || dosHeader.at(0) != 'M' ||
      dosHeader.at(1) != 'Z') {
    return false;
  }

  const quint32 peOffset = qFromLittleEndian<quint32>(
      reinterpret_cast<const uchar *>(dosHeader.constData() + 0x3c));
  if (peOffset < dosHeaderSize || peOffset > maxPEOffset ||
      static_cast<qint64>(peOffset) >
          fileSize - peAndCoffHeaderSize ||
      !file.seek(static_cast<qint64>(peOffset))) {
    return false;
  }

  const QByteArray peAndCoff = file.read(peAndCoffHeaderSize);
  if (peAndCoff.size() != peAndCoffHeaderSize ||
      peAndCoff.left(4) != QByteArray("PE\0\0", 4)) {
    return false;
  }
  const auto *coff =
      reinterpret_cast<const uchar *>(peAndCoff.constData() + 4);
  const quint16 machine = qFromLittleEndian<quint16>(coff);
  const quint16 sections = qFromLittleEndian<quint16>(coff + 2);
  const quint16 optionalHeaderSize =
      qFromLittleEndian<quint16>(coff + 16);
  const quint16 characteristics =
      qFromLittleEndian<quint16>(coff + 18);
  if ((machine != machineI386 && machine != machineAMD64 &&
       machine != machineARM64) ||
      sections == 0 || sections > maxSections ||
      (characteristics & imageFileExecutableImage) == 0 ||
      (characteristics & imageFileDLL) != 0) {
    return false;
  }

  const qint64 optionalHeaderOffset =
      static_cast<qint64>(peOffset) + peAndCoffHeaderSize;
  const qint64 sectionTableBytes =
      static_cast<qint64>(sections) * sectionHeaderSize;
  if (optionalHeaderSize < 2 ||
      optionalHeaderOffset > fileSize - optionalHeaderSize ||
      optionalHeaderOffset + optionalHeaderSize >
          fileSize - sectionTableBytes) {
    return false;
  }
  const QByteArray optionalHeader = file.read(optionalHeaderSize);
  if (optionalHeader.size() != optionalHeaderSize)
    return false;
  const quint16 optionalMagic = qFromLittleEndian<quint16>(
      reinterpret_cast<const uchar *>(optionalHeader.constData()));
  if (machine == machineI386) {
    return optionalMagic == pe32Magic &&
           optionalHeaderSize >= minPE32OptionalHeader;
  }
  return optionalMagic == pe32PlusMagic &&
         optionalHeaderSize >= minPE32PlusOptionalHeader;
}

inline bool isExecutableCandidate(const QFileInfo &info,
                                  bool windowsPlatform) {
  if (!info.isExecutable())
    return false;
  return !windowsPlatform ||
         hasWindowsPESignature(info.absoluteFilePath());
}

inline QString
bundledExecutableForPlatform(const QString &applicationDir,
                             const QStringList &baseNames,
                             bool windowsPlatform) {
  if (!QDir::isAbsolutePath(applicationDir))
    return QString();
  const QFileInfo applicationDirInfo(applicationDir);
  const QString canonicalApplicationDir =
      applicationDirInfo.canonicalFilePath();
  if (canonicalApplicationDir.isEmpty() || !applicationDirInfo.isDir())
    return QString();

  for (const QString &name :
       candidatesForPlatform(baseNames, windowsPlatform)) {
    const QString candidate =
        QDir(canonicalApplicationDir).absoluteFilePath(name);
    const QFileInfo info(candidate);
    if (!info.exists() || !info.isFile() || info.isSymLink() ||
        !isExecutableCandidate(info, windowsPlatform)) {
      continue;
    }
    const QString canonicalCandidate = info.canonicalFilePath();
    if (!canonicalCandidate.isEmpty() &&
        isWithinDirectory(canonicalApplicationDir, canonicalCandidate)) {
      return canonicalCandidate;
    }
  }
  return QString();
}

inline QString bundledExecutable(const QString &applicationDir,
                                 const QStringList &baseNames) {
#ifdef Q_OS_WIN
  return bundledExecutableForPlatform(applicationDir, baseNames, true);
#else
  return bundledExecutableForPlatform(applicationDir, baseNames, false);
#endif
}

} // namespace ExecutableLocator

#endif // EXECUTABLELOCATOR_H
