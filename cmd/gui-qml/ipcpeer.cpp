// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#include "ipcpeer.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QRandomGenerator>

#if defined(Q_OS_WIN)
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#elif defined(Q_OS_UNIX)
#include <cerrno>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/un.h>
#include <unistd.h>
#endif

namespace IpcPeer {

ServerIdentity readServerIdentity(qintptr descriptor) {
  ServerIdentity identity;
  if (descriptor == -1)
    return identity;
#if defined(Q_OS_LINUX)
  struct ucred credentials = {};
  socklen_t length = sizeof(credentials);
  if (::getsockopt(static_cast<int>(descriptor), SOL_SOCKET, SO_PEERCRED,
                   &credentials, &length) != 0 ||
      length != sizeof(credentials))
    return identity;
  identity.read = true;
  identity.uidKnown = true;
  identity.uid = static_cast<quint32>(credentials.uid);
  identity.pid = static_cast<qint64>(credentials.pid);
#elif defined(Q_OS_MACOS)
  uid_t uid = 0;
  gid_t gid = 0;
  if (::getpeereid(static_cast<int>(descriptor), &uid, &gid) != 0)
    return identity;
  pid_t pid = 0;
  socklen_t length = sizeof(pid);
  if (::getsockopt(static_cast<int>(descriptor), SOL_LOCAL, LOCAL_PEERPID,
                   &pid, &length) != 0 ||
      length != sizeof(pid))
    return identity;
  identity.read = true;
  identity.uidKnown = true;
  identity.uid = static_cast<quint32>(uid);
  identity.pid = static_cast<qint64>(pid);
#elif defined(Q_OS_WIN)
  // En Windows QLocalSocket::socketDescriptor() es el HANDLE del pipe. La
  // DACL del pipe ya limita el acceso a la sesión; aquí se comprueba el PID.
  ULONG pid = 0;
  if (!::GetNamedPipeServerProcessId(reinterpret_cast<HANDLE>(descriptor),
                                     &pid))
    return identity;
  identity.read = true;
  identity.pid = static_cast<qint64>(pid);
#endif
  return identity;
}

quint32 currentUid() {
#if defined(Q_OS_UNIX)
  return static_cast<quint32>(::geteuid());
#else
  return 0;
#endif
}

qint64 parseProcessId(const QString &text) {
  bool ok = false;
  const qint64 pid = text.trimmed().toLongLong(&ok, 10);
  if (!ok || pid <= 0 || pid > 0xFFFFFFFFLL)
    return 0;
  return pid;
}

bool ensurePrivateDirectory(const QString &path, bool fixPermissions) {
#if defined(Q_OS_UNIX)
  if (path.isEmpty() || !QDir::isAbsolutePath(path))
    return false;
  const QByteArray native = QFile::encodeName(QDir::cleanPath(path));
  struct stat info = {};
  if (::lstat(native.constData(), &info) != 0)
    return false;
  // lstat no sigue enlaces: un enlace simbólico no es S_ISDIR.
  if (!S_ISDIR(info.st_mode) || info.st_uid != ::geteuid())
    return false;
  if ((info.st_mode & 077) != 0) {
    if (!fixPermissions || ::chmod(native.constData(), 0700) != 0)
      return false;
  }
  return true;
#else
  Q_UNUSED(path);
  Q_UNUSED(fixPermissions);
  return false;
#endif
}

QString privateSocketDirectory() {
#if defined(Q_OS_UNIX)
  const QString runtimeDir = qEnvironmentVariable("XDG_RUNTIME_DIR");
  if (QDir::isAbsolutePath(runtimeDir) &&
      ensurePrivateDirectory(runtimeDir, false))
    return QDir::cleanPath(runtimeDir);

  const QString home = QDir::homePath();
  if (home.isEmpty() || !QDir::isAbsolutePath(home) ||
      QDir::cleanPath(home) == QDir::rootPath())
    return QString();
  const QString localDir = QDir(home).absoluteFilePath(QStringLiteral(".local"));
  const QString runDir = QDir(localDir).absoluteFilePath(QStringLiteral("run"));
  if (!QDir().mkpath(localDir))
    return QString();
  const QByteArray nativeRun = QFile::encodeName(runDir);
  if (::mkdir(nativeRun.constData(), 0700) != 0 && errno != EEXIST)
    return QString();
  if (!ensurePrivateDirectory(runDir, true))
    return QString();
  return runDir;
#else
  return QString();
#endif
}

QString newPrivateSocketPath() {
  const QString dir = privateSocketDirectory();
  if (dir.isEmpty())
    return QString();
  const quint64 nonce = QRandomGenerator::system()->generate64();
  return QDir(dir).absoluteFilePath(
      QStringLiteral("grxfirma_ipc_%1_%2.sock")
          .arg(QCoreApplication::applicationPid())
          .arg(nonce, 16, 16, QLatin1Char('0')));
}

} // namespace IpcPeer
