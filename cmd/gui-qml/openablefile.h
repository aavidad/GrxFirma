// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef OPENABLEFILE_H
#define OPENABLEFILE_H

#include <QFileInfo>
#include <QString>
#include <QStringList>

// Qué ficheros puede «Abrir» la GUI con el programa asociado del sistema.
//
// Abrir un fichero con el sistema (QDesktopServices, ShellExecute, xdg-open,
// open) ejecuta los tipos ejecutables: .exe, .lnk, .hta, .js, .scr, .bat,
// .desktop... El nombre de un documento desprotegido lo elige quien envió el
// sobre, así que solo se abren tipos documentales de esta lista. Para el resto
// se muestra la carpeta que lo contiene. Es la misma lista que conserva el
// backend al desproteger (handler.go, extensionesDocumentalesDesprotegidas).
namespace OpenableFile {

inline bool isDocumentToOpen(const QString &path) {
  static const QStringList documentSuffixes = {
      QStringLiteral("pdf"),   QStringLiteral("xml"),   QStringLiteral("txt"),
      QStringLiteral("csv"),   QStringLiteral("json"),  QStringLiteral("docx"),
      QStringLiteral("xlsx"),  QStringLiteral("pptx"),  QStringLiteral("odt"),
      QStringLiteral("ods"),   QStringLiteral("odp"),   QStringLiteral("png"),
      QStringLiteral("jpg"),   QStringLiteral("jpeg"),  QStringLiteral("gif"),
      QStringLiteral("tif"),   QStringLiteral("tiff"),  QStringLiteral("xsig"),
      QStringLiteral("csig"),  QStringLiteral("p7s"),   QStringLiteral("p7m"),
      QStringLiteral("asice"), QStringLiteral("asics"),
  };
  // suffix() de "a.txt:x" es "txt:x": no está en la lista.
  const QString suffix = QFileInfo(path).suffix().toLower();
  return !suffix.isEmpty() && documentSuffixes.contains(suffix);
}

} // namespace OpenableFile

#endif // OPENABLEFILE_H
