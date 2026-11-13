// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#ifndef IPCPEER_H
#define IPCPEER_H

#include <QString>
#include <QtGlobal>

// Identidad del motor de firma al otro lado del canal IPC local.
//
// La GUI Qt solo habla con el backend que ella misma lanzó (su PID es el de
// QProcess) o con el que le indica el lanzador grxfirma-gui por
// --backend-pid. Antes de enviar nada comprueba con el sistema operativo que
// el otro extremo del socket o del named pipe es ese proceso: SO_PEERCRED en
// Linux, getpeereid + LOCAL_PEERPID en macOS y GetNamedPipeServerProcessId en
// Windows, igual que WinUI (ADR-005). En Unix el socket vive en un directorio
// privado del usuario (0700), nunca en el temporal compartido.
namespace IpcPeer {

// Tope de una línea de respuesta: el mayor fichero que admite el backend
// (100 MiB) en Base64 más margen para el JSON que lo envuelve.
constexpr qint64 kMaxResponseLineBytes = 192LL * 1024 * 1024;

struct ServerIdentity {
  bool read = false; // el sistema devolvió las credenciales del otro extremo
  bool uidKnown = false;
  quint32 uid = 0;
  qint64 pid = 0;
};

// Decisión pura: se acepta solo si el sistema dio las credenciales, el PID
// coincide con el esperado (que debe ser conocido) y, cuando el sistema da el
// UID, es el del usuario actual.
inline bool acceptServer(const ServerIdentity &peer, quint32 ownUid,
                         qint64 expectedPid) {
  if (!peer.read || expectedPid <= 0 || peer.pid <= 0)
    return false;
  if (peer.uidKnown && peer.uid != ownUid)
    return false;
  return peer.pid == expectedPid;
}

// Lee del sistema la identidad del proceso al otro lado del descriptor de
// QLocalSocket::socketDescriptor(). Devuelve read=false si no puede.
ServerIdentity readServerIdentity(qintptr descriptor);

// UID efectivo del proceso actual (0 en Windows, donde no se usa).
quint32 currentUid();

// PID decimal positivo de --backend-pid; 0 si no es válido.
qint64 parseProcessId(const QString &text);

// Unix: exige un directorio real del usuario actual sin permisos para grupo
// ni otros. Con fixPermissions, uno propio más abierto se deja en 0700.
bool ensurePrivateDirectory(const QString &path, bool fixPermissions);

// Unix: XDG_RUNTIME_DIR si es privado y, si no, ~/.local/run (0700). Cadena
// vacía si no hay ninguno; nunca el directorio temporal compartido.
QString privateSocketDirectory();

// Unix: ruta nueva y aleatoria en privateSocketDirectory(); vacía si falla.
QString newPrivateSocketPath();

} // namespace IpcPeer

#endif // IPCPEER_H
