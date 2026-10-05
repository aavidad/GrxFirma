// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef GRXFIRMA_SAVEDIALOGNAMES_H
#define GRXFIRMA_SAVEDIALOGNAMES_H

#include <QFileInfo>
#include <QGuiApplication>
#include <QObject>
#include <QString>
#include <QUrl>
#include <QWindow>

// El diálogo de guardar propio de Qt Quick (el que se usa cuando el sistema
// no ofrece uno nativo) solo copia al campo «Nombre del fichero» los nombres
// de ficheros que ya existen. Un nombre propuesto nuevo, como
// «contrato_firmado.pdf», queda elegido pero el campo sale vacío. Esta ayuda
// lo escribe en el campo justo después de abrir el diálogo, antes de que la
// persona teclee nada; no cambia la ruta
// elegida ni toca los diálogos nativos.
class SaveDialogNames : public QObject {
  Q_OBJECT

public:
  using QObject::QObject;

  Q_INVOKABLE int showProposedNames() const {
    int filled = 0;
    const auto windows = QGuiApplication::topLevelWindows();
    for (QWindow *window : windows) {
      const auto objects = window->findChildren<QObject *>();
      for (QObject *dialog : objects) {
        if (!dialog->inherits("QQuickFileDialogImpl") ||
            !dialog->property("visible").toBool())
          continue;
        QObject *field = dialog->findChild<QObject *>(QStringLiteral("fileNameTextField"));
        if (!field || !field->property("visible").toBool())
          continue;
        const QString name = proposedName(dialog->property("selectedFile").toUrl());
        if (name.isEmpty() || field->property("text").toString() == name)
          continue;
        field->setProperty("text", name);
        ++filled;
      }
    }
    return filled;
  }

  static QString proposedName(const QUrl &selected) {
    if (!selected.isValid() || !selected.isLocalFile())
      return QString();
    const QFileInfo info(selected.toLocalFile());
    if (info.isDir())
      return QString();
    return info.fileName();
  }
};

#endif
