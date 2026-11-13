<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR — Modelo de seguridad desktop: Zero Server por defecto

Fecha: 2026-03-18
Estado: APROBADO
Autor: Oficina de Software Libre de la Diputacion de Granada.

---

## Contexto

AutoFirma V1 (escritorio) abre siempre un servidor WebSocket en `localhost:8080-8089`
al arrancar. Este servidor:

- acepta conexiones de cualquier proceso local,
- no autentica el origen de la solicitud más allá de un check de dominio débil,
- está activo incluso cuando el usuario no está firmando nada,
- produce una superficie de ataque permanente mientras la app está en ejecución.

La arquitectura mobile de GrxFirma (ADR-mobile-security) ya establece el modelo
correcto: sin puertos abiertos, IPC mediado por el OS, clave en TEE.

Se decide aplicar el mismo principio a escritorio.

---

## Decisión

**GrxFirma desktop no abre ningún puerto TCP/UDP por defecto.**

El servidor WebSocket heredado (T063) existe para compatibilidad con webs antiguas,
pero es **opt-in**: desactivado en la configuración por defecto y activado solo cuando
el usuario lo habilita explícitamente en la aplicación.

### Comportamiento por defecto

```
~/.config/grxfirma/config.json
{
  "websocket_habilitado": false,
  "rest_habilitado": false,
  "tofu_habilitado": true
}
```

Con esta configuración, la superficie de ataque de red de GrxFirma es **cero**.

### Comportamiento opt-in

El usuario activa el WebSocket desde la configuración de la app (GUI o fichero).
Al activarlo, la app:

1. Muestra un aviso explícito: "Estás habilitando un servidor local. Cualquier
   aplicación de este equipo podrá solicitar firmas electrónicas."
2. Registra la activación en el log de auditoría con timestamp.
3. Abre el servidor solo mientras la app está en primer plano (o en systray si se
   implementa), no como servicio de sistema.

### Canales de entrada por defecto (siempre activos, sin red)

| Canal | Mecanismo | Red | Por defecto |
|-------|-----------|-----|-------------|
| `afirma://` URI | xdg-open → proceso nuevo | No | ✅ Activo |
| Native Messaging | stdin/stdout, lanzado por browser | No | ✅ Activo |
| CLI directo | argumentos de línea de comandos | No | ✅ Activo |

### Canales opt-in (requieren activación explícita)

| Canal | Mecanismo | Red | Por defecto |
|-------|-----------|-----|-------------|
| WebSocket local | TCP localhost:8080-8089 | Sí, localhost | ❌ Desactivado |
| REST local | TCP localhost:XXXX | Sí, localhost | ❌ Desactivado |

---

## Comparativa de superficie de ataque

| Escenario | V1 Desktop | V2 Desktop (esta ADR) |
|-----------|-----------|----------------------|
| App en reposo | Puerto 8080-8089 abierto | Sin puertos |
| App activa, usuario no firmando | Puerto 8080-8089 abierto | Sin puertos |
| Firma via `afirma://` | Puerto + proceso efímero | Proceso efímero, sin puerto |
| Firma via extensión navegador | Puerto + Native Messaging | Solo Native Messaging |
| Compatibilidad web antigua | Puerto siempre activo | Puerto opt-in, solo si el usuario lo activa |
| Proceso malicioso en el equipo | Puede enviar solicitudes de firma | No puede, no hay puerto; el IPC de escritorio solo atiende al PID del frontend que lanzó el backend (ADR-005) |

---

## Implicaciones para el plan de tareas

### T063 — WebSocket local: opt-in, no opt-out

La tarea T063 implementa el servidor WebSocket, pero con estas restricciones:

- El servicio residente arranca **solo** si `websocket_habilitado: true` en la
  configuración.
- El canal temporal que una sede abre con `afirma://websocket` no depende de
  `websocket_habilitado`: se abre para esa operación, exige `idsession` y se
  cierra al terminar. Para impedirlo también hay que poner
  `websocket_permitido: false`, que corta el residente y los canales temporales
  y prevalece sobre el ajuste del usuario (véase `docs/POLITICA_MAQUINA.md`).
- Si arranca, registra el evento en auditoría.
- Muestra aviso en la GUI cuando está activo (icono o indicador visible).
- Se detiene cuando la ventana principal se cierra (no como demonio de fondo).

### T072 — Sistema de configuración de GrxFirma (tarea nueva)

Para materializar este modelo se necesita un sistema de configuración mínimo:

- Fichero `~/.config/grxfirma/config.json` con valores por defecto seguros.
- Variables de entorno `GRXFIRMA_*` como override (para despliegues gestionados).
- Lectura al arrancar cada binario (`cmd/grxfirmauri`, `cmd/nativehost`).
- Sección en GUI (T038–T040) para modificar la configuración.
- Clave `websocket_habilitado` (bool, defecto: false).
- Clave `rest_habilitado` (bool, defecto: false).
- Clave `tofu_habilitado` (bool, defecto: true; desactivable por política).
- Clave `dominios_de_confianza` (lista, integrada con TrustPolicy T054).
- Clave `directorio_p12` (string, defecto: `~/.config/grxfirma/pkcs12`).
- Clave `nivel_log` (string, defecto: `info`).

### T058 — Límites sistémicos: incluir bloqueo de red por defecto

La tarea T058 (límites y timeouts) debe incluir:

- Verificación al arrancar de que no se abre ningún puerto residente si la
  configuración indica `websocket_habilitado: false`, y de que no se abre
  ninguno, ni siquiera temporal, con `websocket_permitido: false`.
- Test que arranca el binario con configuración por defecto y verifica que no
  hay ningún socket en estado LISTEN.

---

## Relación con el modelo mobile

Ambos modelos comparten el mismo principio. La diferencia es de mecanismo:

| | Mobile | Desktop |
|--|--------|---------|
| Canales de entrada | Intent / URL Scheme (OS IPC) | afirma:// + Native Messaging |
| Puerto TCP | Nunca | Solo opt-in |
| Clave privada | TEE hardware cuando la plataforma lo permite | Referencia del almacén del SO cuando este lo permite; PKCS#11/DNIe no está disponible en producción |
| Documento en disco | Solo sandbox privado | Solo sandbox privado |
| Resultado en log | Hash solamente | Hash solamente |

---

## Notas de implementación

- El fichero `config.json` debe tener un esquema JSON validable. Si el fichero
  no existe, se crea con los valores por defecto seguros al primer arranque.
- Los valores por defecto deben estar codificados en Go, no dependiendo de que
  el fichero exista.
- Una organización puede desplegar una política de configuración en
  `/etc/grxfirma/policy.json` que sobreescriba ciertos valores (por ejemplo,
  forzar `websocket_habilitado: false` o `tofu_habilitado: false` en todos los
  equipos corporativos).
- La allowlist del sistema usada por `TrustPolicy` se carga además desde
  `/etc/grxfirma/allowed-domains.json`; en despliegues gestionados esta
  ruta debe considerarse el baseline de confianza, dejando TOFU solo como
  fallback para escritorios no gestionados.

---

## Llamantes autorizados del host nativo

Este es el único inventario de las extensiones que pueden abrir el host de
Native Messaging (`cmd/nativehost/caller.go` y los manifiestos que escriben los
instaladores de Linux, Windows y macOS). Revisión del 13/11/2026 (hallazgos
C-07 y G-04).

| Navegador | ID | Host nativo | Titular | Estado |
|---|---|---|---|---|
| Chromium | `pkefjandjcgdmhoonmhnllikibobijgg` | `com.dipgra.grxfirma`, `com.grxfirma.native` | Extensión oficial de GrxFirma | En uso |
| Firefox | `extension@dipgra.es` | `com.dipgra.grxfirma`, `com.grxfirma.native` | Extensión oficial de GrxFirma | Falta confirmar que el ID está reservado en AMO a nombre de la Diputación |
| Chromium | `ipkpimgjhkjibkbhfdhggjldlaetbcoa` | `com.dipgra.portafirmas` | PENDIENTE: falta confirmar quién la publica | No es un ID de Chromium válido (lleva una «t»; solo valen las letras a-p), así que el host ya lo descarta |
| Chromium | `knldjmfmopnpolahpmmgbagdohdnhkik` | `com.dipgra.portafirmas` | PENDIENTE: falta confirmar quién la publica | Autorizada |
| Firefox | `portafirmas@dipgra.es` | `com.dipgra.portafirmas` | PENDIENTE: falta confirmar quién la publica y si el ID está reservado en AMO | Autorizada |

Además, el host acepta el ID que el instalador deja en
`extensions/chromium/dipgra-extension-chromium.id` y los que figuren en un
manifiesto de host instalado que apunte a este mismo ejecutable.

Hasta que se confirme el titular de las extensiones portafirmas, la lista no se
cambia. Mientras tanto se aplica lo siguiente, que no depende de ese dato:

- Solo la extensión oficial cuenta como «oficial». Son los ID oficiales de la
  tabla, el del fichero `.id` y los que figuran en los manifiestos
  `com.dipgra.grxfirma` y `com.grxfirma.native`. Un ID que solo aparece en
  `com.dipgra.portafirmas` no lo es.
- La política de máquina `aprobacion_automatica_nativehost` solo se aplica a la
  extensión oficial. Con cualquier otro llamante el host pide siempre
  confirmación, porque el origen solicitante que declara no se puede comprobar:
  la extensión oficial lo calcula a partir de la URL que da el navegador, pero el
  código de las portafirmas no está en este repositorio.

Queda por decidir cuando se sepa el titular: qué acciones se permiten a las
portafirmas (por ejemplo, `getCertificates`) y si sus ID se quitan del binario
para aceptarlos solo con un manifiesto `com.dipgra.portafirmas` instalado.

### Portales que pueden pedir pruebas de identidad

La extensión acepta `proveIdentity` desde cualquier subdominio de `dipgra.es` y
de `savia.net`. Es una decisión expresa de Alberto del 13/11/2026 (hallazgo
C-08): se mantienen todos los subdominios. La lista está en
`FACTORY_PORTAL_PATTERNS` (`background.js`), `FACTORY_SITES`
(`trusted_sites.js`) y en `content_scripts` y `host_permissions` de los
`manifest.json` de Chromium y Firefox.
