// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "ipcpeer.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QLocalServer>
#include <QLocalSocket>
#include <QTemporaryDir>
#include <QtTest>

#if defined(Q_OS_UNIX)
#include <sys/stat.h>
#endif

// D-01: la GUI solo habla con el backend cuyo PID conoce y nunca usa el
// temporal compartido para el socket.
class IpcPeerTest : public QObject {
  Q_OBJECT

private slots:
  void rejectsWithoutKernelCredentials() {
    IpcPeer::ServerIdentity peer;
    peer.read = false;
    peer.pid = 42;
    QVERIFY(!IpcPeer::acceptServer(peer, 1000, 42));
  }

  void rejectsUnknownExpectedPid() {
    IpcPeer::ServerIdentity peer{true, true, 1000, 42};
    QVERIFY(!IpcPeer::acceptServer(peer, 1000, 0));
  }

  void rejectsOtherUser() {
    IpcPeer::ServerIdentity peer{true, true, 1001, 42};
    QVERIFY(!IpcPeer::acceptServer(peer, 1000, 42));
  }

  void rejectsOtherProcess() {
    IpcPeer::ServerIdentity peer{true, true, 1000, 43};
    QVERIFY(!IpcPeer::acceptServer(peer, 1000, 42));
  }

  void acceptsExpectedProcess() {
    IpcPeer::ServerIdentity peer{true, true, 1000, 42};
    QVERIFY(IpcPeer::acceptServer(peer, 1000, 42));
  }

  void parsesOnlyPositiveDecimalPids() {
    QCOMPARE(IpcPeer::parseProcessId(QStringLiteral("1234")), qint64(1234));
    QCOMPARE(IpcPeer::parseProcessId(QStringLiteral("0")), qint64(0));
    QCOMPARE(IpcPeer::parseProcessId(QStringLiteral("-5")), qint64(0));
    QCOMPARE(IpcPeer::parseProcessId(QStringLiteral("12x")), qint64(0));
    QCOMPARE(IpcPeer::parseProcessId(QStringLiteral("99999999999")),
             qint64(0));
  }

#if defined(Q_OS_LINUX) || defined(Q_OS_MACOS)
  // Lee de verdad las credenciales del núcleo sobre un socket conectado: el
  // servidor es este mismo proceso.
  void readsServerIdentityFromKernel() {
    QTemporaryDir dir;
    QVERIFY(dir.isValid());
    const QString path = dir.filePath(QStringLiteral("p.sock"));
    QLocalServer server;
    QVERIFY(server.listen(path));
    QLocalSocket client;
    client.connectToServer(path);
    QVERIFY(client.waitForConnected(2000));

    const IpcPeer::ServerIdentity peer =
        IpcPeer::readServerIdentity(client.socketDescriptor());
    QVERIFY(peer.read);
    QCOMPARE(peer.pid, QCoreApplication::applicationPid());
    QVERIFY(IpcPeer::acceptServer(peer, IpcPeer::currentUid(),
                                  QCoreApplication::applicationPid()));
    QVERIFY(!IpcPeer::acceptServer(peer, IpcPeer::currentUid(),
                                   QCoreApplication::applicationPid() + 1));
  }

  void runtimeDirWithOpenPermissionsFallsBackToPrivateHomeDir() {
    QTemporaryDir runtime;
    QTemporaryDir home;
    QVERIFY(runtime.isValid() && home.isValid());
    QVERIFY(::chmod(QFile::encodeName(runtime.path()).constData(), 0755) == 0);
    const QByteArray oldRuntime = qgetenv("XDG_RUNTIME_DIR");
    const QByteArray oldHome = qgetenv("HOME");
    qputenv("XDG_RUNTIME_DIR", QFile::encodeName(runtime.path()));
    qputenv("HOME", QFile::encodeName(home.path()));

    const QString socketPath = IpcPeer::newPrivateSocketPath();

    qputenv("XDG_RUNTIME_DIR", oldRuntime);
    qputenv("HOME", oldHome);
    QVERIFY(!socketPath.isEmpty());
    QCOMPARE(QFileInfo(socketPath).absolutePath(),
             QDir(home.path()).absoluteFilePath(QStringLiteral(".local/run")));
    QVERIFY(!socketPath.startsWith(QDir::tempPath() + QStringLiteral("/grxfirma_ipc_")));
    struct stat info = {};
    QVERIFY(::lstat(QFile::encodeName(QFileInfo(socketPath).absolutePath())
                        .constData(),
                    &info) == 0);
    QCOMPARE(info.st_mode & 0777, mode_t(0700));
  }

  void symlinkIsNotAPrivateDirectory() {
    QTemporaryDir base;
    QTemporaryDir target;
    QVERIFY(base.isValid() && target.isValid());
    const QString link = base.filePath(QStringLiteral("enlace"));
    QVERIFY(QFile::link(target.path(), link));
    QVERIFY(!IpcPeer::ensurePrivateDirectory(link, true));
  }

  void noPrivateDirectoryMeansNoPath() {
    const QByteArray oldRuntime = qgetenv("XDG_RUNTIME_DIR");
    const QByteArray oldHome = qgetenv("HOME");
    qunsetenv("XDG_RUNTIME_DIR");
    qputenv("HOME", "/");

    const QString socketPath = IpcPeer::newPrivateSocketPath();

    qputenv("XDG_RUNTIME_DIR", oldRuntime);
    qputenv("HOME", oldHome);
    QVERIFY2(socketPath.isEmpty(), qPrintable(socketPath));
  }

  void eachLaunchGetsANewPath() {
    QTemporaryDir home;
    QVERIFY(home.isValid());
    const QByteArray oldRuntime = qgetenv("XDG_RUNTIME_DIR");
    const QByteArray oldHome = qgetenv("HOME");
    qunsetenv("XDG_RUNTIME_DIR");
    qputenv("HOME", QFile::encodeName(home.path()));
    const QString first = IpcPeer::newPrivateSocketPath();
    const QString second = IpcPeer::newPrivateSocketPath();
    qputenv("XDG_RUNTIME_DIR", oldRuntime);
    qputenv("HOME", oldHome);
    QVERIFY(!first.isEmpty());
    QVERIFY(first != second);
  }
#endif
};

QTEST_GUILESS_MAIN(IpcPeerTest)
#include "ipcpeer_test.moc"
