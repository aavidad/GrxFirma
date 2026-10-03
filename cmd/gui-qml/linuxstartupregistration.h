// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef LINUXSTARTUPREGISTRATION_H
#define LINUXSTARTUPREGISTRATION_H

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QSaveFile>
#include <QStandardPaths>

class LinuxStartupRegistration {
public:
  static QString path() {
    const QString config = QStandardPaths::writableLocation(QStandardPaths::GenericConfigLocation);
    if (config.isEmpty() || !QDir::isAbsolutePath(config))
      return {};
    return QDir(config).filePath(QStringLiteral("autostart/grxfirma.desktop"));
  }

  static QByteArray expectedEntry() {
    const QString binary = QCoreApplication::applicationFilePath();
    if (!QFileInfo(binary).isAbsolute() || binary.contains(QLatin1Char('\n')) ||
        binary.contains(QLatin1Char('\r')) || binary.contains(QLatin1Char('"')) ||
        binary.contains(QLatin1Char('\\')) || binary.contains(QLatin1Char('%')))
      return {};
    // Desktop Entry Exec usa comillas para conservar espacios en la ruta.
    return QStringLiteral("[Desktop Entry]\nType=Application\nName=GrxFirma\n"
                          "Exec=\"%1\" --start-hidden\n"
                          "Terminal=false\nX-GNOME-Autostart-enabled=true\n"
                          "X-GrxFirma-Managed=true\n")
        .arg(binary).toUtf8();
  }

  static bool isManaged() {
    if (path().isEmpty()) return false;
    const QFileInfo info(path());
    if (!info.isFile() || info.isSymLink() || info.size() > 4096)
      return false;
    QFile file(path());
    return file.open(QIODevice::ReadOnly) &&
           file.readAll().contains("\nX-GrxFirma-Managed=true\n");
  }

  static bool isEnabled() {
    if (!isManaged()) return false;
    QFile file(path());
    return file.open(QIODevice::ReadOnly) && file.readAll() == expectedEntry();
  }

  static bool setEnabled(bool enabled) {
    const QString target = path();
    if (target.isEmpty()) return false;
    const QFileInfo config(QStandardPaths::writableLocation(QStandardPaths::GenericConfigLocation));
    const QFileInfo autostart(QFileInfo(target).absolutePath());
    const QFileInfo existing(target);
    if (config.isSymLink() || autostart.isSymLink() || existing.isSymLink() ||
        (existing.exists() && !isManaged()))
      return false;
    if (!enabled)
      return !existing.exists() || QFile::remove(target);
    const QByteArray entry = expectedEntry();
    if (entry.isEmpty())
      return false;
    if (!QDir().mkpath(QFileInfo(target).absolutePath()))
      return false;
    QSaveFile file(target);
    file.setDirectWriteFallback(false);
    if (!file.open(QIODevice::WriteOnly))
      return false;
    if (!file.setPermissions(QFileDevice::ReadOwner | QFileDevice::WriteOwner))
      return false;
    return file.write(entry) == entry.size() && file.commit();
  }
};

#endif
