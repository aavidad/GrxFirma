// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include <QCoreApplication>
#include <QLocalSocket>
#include <QDebug>
#include <QFileInfo>

int main(int argc, char** argv) {
    QCoreApplication app(argc, argv);
    QLocalSocket s;
    s.connectToServer("/tmp/grxfirma_ipc.sock");
    if (s.waitForConnected(500)) {
        qDebug() << "Connected!";
    } else {
        qDebug() << "Failed:" << s.errorString();
    }
    return 0;
}
