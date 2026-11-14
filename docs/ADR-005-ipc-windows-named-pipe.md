<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR-005 — IPC local mediante named pipe protegido en Windows

- Estado: aceptada
- Fecha: 2026-07-26
- Alcance: GUI de escritorio y backend local en Windows 10 o posterior

## Contexto

La GUI Qt intercambia mensajes JSON delimitados por salto de línea con el
backend Go. En Unix se usa un socket con permisos `0600` y, en Linux, se
comprueba además `SO_PEERCRED`.

Windows no implementa sockets Unix mediante `net.Listen("unix", ...)` para
rutas `\\.\pipe\...`. El listener anterior intentaba usar esa combinación y
el backend terminaba al arrancar, por lo que la GUI no podía listar
certificados ni firmar.

Un named pipe necesita controles equivalentes a los del socket Unix:

- limitarlo a la máquina local;
- autorizar solamente a la sesión de inicio de la aplicación;
- admitir cancelación y cierre limpios;
- mantener el protocolo de flujo que consume `QLocalSocket`.

## Decisión

En Windows se usa `github.com/Microsoft/go-winio` 0.6.2 detrás del adaptador
`internal/adapters/inbound/desktop/ipc`.

El listener:

1. solo acepta rutas con la forma local `\\.\pipe\<nombre>` y rechaza rutas
   remotas o nombres anidados;
2. crea el pipe con una DACL protegida que concede a la sesión actual solo la
   máscara dúplex necesaria (`FILE_GENERIC_READ | FILE_GENERIC_WRITE`) y usa
   `OWNER_RIGHTS` para impedir que otra sesión de la cuenta herede
   implícitamente `WRITE_DAC` por ser propietaria; no autoriza `SYSTEM`,
   `Everyone`, usuarios autenticados, otras sesiones ni administradores en
   general;
3. hereda de `go-winio` `FILE_PIPE_REJECT_REMOTE_CLIENTS`;
4. usa modo byte para conservar el protocolo JSON por líneas;
5. limita los buffers del sistema a 64 KiB; el límite de aplicación sigue
   siendo 4 MiB por mensaje.
6. cierra las conexiones aceptadas al detener el servidor para que ningún
   cliente retenga el objeto del pipe ni bloquee el siguiente arranque.
7. genera un nombre distinto por instancia con PID y 128 bits aleatorios; la
   GUI sondea los named pipes con `QLocalSocket`, no como ficheros, y normaliza
   de forma explícita los nombres simples recibidos por línea de comandos.
8. en el arranque de escritorio, prepara antes del listener un control de
   admisión pendiente y, después de crear el frontend, publica su PID exacto.
   Cada conexión obtiene el PID real mediante
   `GetNamedPipeClientProcessId`; se admiten las reconexiones del mismo proceso
   y se rechaza cualquier PID distinto. Si el frontend no arranca o el PID no
   puede publicarse, el control queda cerrado.

Unix conserva `net.Listen("unix")` y permisos `0600`, y comprueba las
credenciales que da el núcleo: `SO_PEERCRED` en Linux y `LOCAL_PEERCRED` más
`LOCAL_PEERPID` en macOS. La GUI Qt que crea el backend en modo servidor
publica su PID y el servidor lo contrasta con el PID del núcleo, además del
UID, igual que en Windows.

Actualización 13/11/2026 (revisión de seguridad, D-03): se retira la
excepción de `grxfirma-gui --server` sin `--ipc-client-pid`. El modo servidor
exige siempre el PID del frontend y, si falta o la plataforma no permite
comprobar el PID del otro extremo, no abre el canal (falla cerrado). El
lanzador normal tampoco arranca en una plataforma sin esa comprobación. La
unidad de usuario de systemd que escribe el gestor de servicios de Linux no
lleva PID, así que ya no arranca un backend útil (`RestartPreventExitStatus=2`
evita que se reinicie en bucle).

Actualización 14/11/2026 (lote 16c, retrocompatibilidad): lo anterior dejaba
sin arrancar el servicio de usuario de Linux y cualquier `--server` manual que
ya funcionaba, así que el cierre pasa a aviso. `grxfirma-gui --server` sin
`--ipc-client-pid`, o en una plataforma que no permite comprobar el PID del
otro extremo, abre el canal sin vínculo de PID. Lo dice en la salida de error
(`desktop.ipc.server.sin_vinculo_pid`) y en el registro
(`ipc_sin_vinculacion_pid`), y `export_diagnostic` devuelve
`ipcClientBinding: "unbound"`. Siguen la comprobación de mismo UID y el socket
`0600`. En ese modo se firma con los certificados del equipo y la firma remota
CSC se rechaza con `csc_interfaz_no_verificada` (ver FIRMA_REMOTA_CSC.md).
Con `--ipc-client-pid` el vínculo es el de siempre. La unidad de systemd
conserva `RestartPreventExitStatus=2`: el código 2 queda para errores de
configuración (modo de servidor desconocido o PID fuera de rango), que no se
arreglan reiniciando. El lanzador normal sigue sin arrancar en una plataforma
sin comprobación de PID; hoy no hay ninguna publicada (Linux, Windows y macOS
la tienen).

Actualización 13/11/2026 (D-01 y D-04): la comprobación es simétrica. Qt,
como WinUI, solo habla con un backend cuyo PID conoce: el que lanza ella
misma (`QProcess`) o el que le indica `grxfirma-gui` con `--backend-pid`.
Antes de enviar nada lee del sistema el PID del otro extremo
(`SO_PEERCRED`, `getpeereid` + `LOCAL_PEERPID` o
`GetNamedPipeServerProcessId`) y, si no coincide, corta sin enviar datos.
En Unix el socket va siempre en un directorio privado del usuario
(`XDG_RUNTIME_DIR` si es suyo y 0700; si no, `~/.local/run` en 0700), con
nombre nuevo y aleatorio en cada arranque; nunca en el temporal compartido
ni con un nombre fijo. El backend que lanza Qt recibe `--ipc-parent-stdin` y
termina cuando se cierra la entrada estándar que Qt mantiene abierta, así
que no sobrevive a la GUI.

La máscara dúplex incluye `FILE_CREATE_PIPE_INSTANCE`, exigido por la
semántica de instancias de named pipe. La DACL de sesión y el nombre aleatorio
siguen reduciendo la superficie, pero la garantía fuerte del flujo normal es
la comprobación del PID acreditado por Windows.

## Alternativas descartadas

- TCP en `localhost`: amplía la superficie de red y requiere autenticación
  adicional y gestión de puertos.
- Implementar directamente las llamadas Win32: duplica lógica delicada de
  I/O solapado, cancelación, ACL y ciclo de vida ya mantenida por Microsoft.
- DACL predeterminada de Windows: depende del token y del entorno del proceso
  y no expresa el principio de mínimo privilegio del producto.

## Consecuencias y validación

Se incorpora una dependencia directa, exclusiva de Windows, cuya necesidad
queda controlada por el gate de pureza del repositorio.

La validación mínima exige:

- compilación cruzada del paquete y del lanzador;
- prueba nativa en Windows 10 de apertura, conexión y respuesta por el pipe;
- rechazo de un PID diferente al frontend y admisión de sus reconexiones;
- cierre del control de admisión cuando el frontend no llega a arrancar;
- rechazo de rutas que no sean pipes locales;
- prueba real de interoperabilidad con `QLocalSocket`;
- captura de la GUI conectada y con el catálogo de certificados cargado.
