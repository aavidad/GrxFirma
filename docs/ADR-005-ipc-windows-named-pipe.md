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

Unix conserva `net.Listen("unix")`, permisos `0600` y la comprobación de
credenciales disponible en Linux. En Linux, la GUI Qt que crea el backend en
modo servidor publica también su PID y el servidor lo contrasta con el PID de
`SO_PEERCRED`, además del UID.

La ejecución manual o gestionada de `grxfirma-gui --server` sin
`--ipc-client-pid` se mantiene por compatibilidad con el gestor de servicios
de Linux. En ese modo no existe vinculación a un proceso concreto: se conserva
la validación del mismo UID y los permisos `0600`, y cualquier proceso ya
comprometido de ese usuario permanece dentro del límite de confianza. El
arranque normal de escritorio en Windows y Linux y el backend iniciado por Qt
no usan esa excepción.

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
