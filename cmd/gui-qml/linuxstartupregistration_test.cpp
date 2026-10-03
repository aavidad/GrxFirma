// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "linuxstartupregistration.h"
#include <QFile>
#include <QTemporaryDir>
#include <QtTest>

class LinuxStartupRegistrationTest : public QObject {
  Q_OBJECT
private slots:
  void enablesAndDisablesOnlyOwnEntry() {
    QTemporaryDir config;
    QVERIFY(config.isValid());
    qputenv("XDG_CONFIG_HOME", config.path().toUtf8());
    const QString path = LinuxStartupRegistration::path();
    QVERIFY(!LinuxStartupRegistration::isEnabled());
    QVERIFY(LinuxStartupRegistration::setEnabled(true));
    QVERIFY(LinuxStartupRegistration::isEnabled());
    QFile file(path);
    QVERIFY(file.open(QIODevice::ReadOnly));
    const QByteArray entry = file.readAll();
    QVERIFY(entry.contains(" --start-hidden\n"));
    QVERIFY(entry.contains("X-GrxFirma-Managed=true"));
    QVERIFY(LinuxStartupRegistration::setEnabled(false));
    QVERIFY(!QFile::exists(path));
  }

  void refusesForeignAndSymlinkEntry() {
    QTemporaryDir config;
    QVERIFY(config.isValid());
    qputenv("XDG_CONFIG_HOME", config.path().toUtf8());
    const QString path = LinuxStartupRegistration::path();
    QVERIFY(QDir().mkpath(QFileInfo(path).absolutePath()));
    QFile foreign(path);
    QVERIFY(foreign.open(QIODevice::WriteOnly));
    QVERIFY(foreign.write("[Desktop Entry]\nExec=other\n") > 0);
    foreign.close();
    QVERIFY(!LinuxStartupRegistration::setEnabled(true));
    QVERIFY(!LinuxStartupRegistration::setEnabled(false));
    QVERIFY(foreign.remove());
    QVERIFY(QFile::link(QCoreApplication::applicationFilePath(), path));
    QVERIFY(!LinuxStartupRegistration::setEnabled(true));
  }
};
QTEST_MAIN(LinuxStartupRegistrationTest)
#include "linuxstartupregistration_test.moc"
