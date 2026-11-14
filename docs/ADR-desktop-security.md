<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR — Modelo de seguridad desktop: Zero Server por defecto

Fecha: 2026-03-18
Estado: APROBADO
Autoría: Alberto Avidad Fernández

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

El servidor WebSocket heredado existe para compatibilidad con webs antiguas,
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

## Implicaciones de diseño

### WebSocket local: opt-in, no opt-out

El servidor WebSocket se implementa con estas restricciones:

- El servicio residente arranca **solo** si `websocket_habilitado: true` en la
  configuración.
- El canal temporal que una sede abre con `afirma://websocket` no depende de
  `websocket_habilitado`: se abre para esa operación y se cierra al terminar.
  Si el enlace trae `idsession`, todo mensaje debe llevar ese mismo valor; uno
  distinto o ausente cierra el canal. Las sedes antiguas que no mandan
  `idsession` siguen funcionando por compatibilidad: el canal se abre como
  antes, queda un aviso en el registro y el diálogo de aprobación de cada firma
  dice que la web no identifica la sesión. Para impedirlo también hay que poner
  `websocket_permitido: false`, que corta el residente y los canales temporales
  y prevalece sobre el ajuste del usuario (véase `docs/POLITICA_MAQUINA.md`).
- Si arranca, registra el evento en auditoría.
- Muestra aviso en la GUI cuando está activo (icono o indicador visible).
- Se detiene cuando la ventana principal se cierra (no como demonio de fondo).

### Sistema de configuración de GrxFirma

Para materializar este modelo se necesita un sistema de configuración mínimo:

- Fichero `~/.config/grxfirma/config.json` con valores por defecto seguros.
- Variables de entorno `GRXFIRMA_*` como override (para despliegues gestionados).
- Lectura al arrancar cada binario (`cmd/grxfirmauri`, `cmd/nativehost`).
- Sección en la GUI para modificar la configuración.
- Clave `websocket_habilitado` (bool, defecto: false).
- Clave `rest_habilitado` (bool, defecto: false).
- Clave `tofu_habilitado` (bool, defecto: true; desactivable por política).
- Clave `dominios_de_confianza` (lista, integrada con TrustPolicy).
- Clave `directorio_p12` (string, defecto: `~/.config/grxfirma/pkcs12`).
- Clave `nivel_log` (string, defecto: `info`).

### Límites sistémicos: bloqueo de red por defecto

Los límites y tiempos de espera incluyen:

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
- Almacén de certificados de Windows: listar el catálogo nunca abre claves
  privadas. Solo se lee la propiedad `CERT_KEY_PROV_INFO_PROP_ID` del
  certificado; no se usa `CryptFindCertificateKeyProvInfo`, porque recorre
  todos los proveedores, incluido el de tarjeta, y muestra «Seguridad de
  Windows» por cada certificado con la clave en una tarjeta ausente. La clave
  se abre únicamente al firmar, en `adquirirClaveWindows`, y si la tarjeta no
  está o el usuario cancela, el resto de la misma operación no vuelve a
  pedirla. Un contrato en `wincertstore` comprueba ambas reglas.

---

## Llamantes autorizados del host nativo

Este es el único inventario de las extensiones que pueden abrir el host de
Native Messaging (`cmd/nativehost/caller.go` y los manifiestos que escriben los
instaladores de Linux, Windows y macOS). Revisión del 13/11/2026 (hallazgos
C-07 y G-04), actualizada al llevar los arreglos a la 0.0.125.

| Navegador | ID | Host nativo | Titular | Estado |
|---|---|---|---|---|
| Chromium | `pkefjandjcgdmhoonmhnllikibobijgg` | `io.github.aavidad.grxfirma`, `com.grxfirma.native` | Extensión oficial de GrxFirma | En uso |
| Firefox | `grxfirma@aavidad.github.io` | `io.github.aavidad.grxfirma`, `com.grxfirma.native` | Extensión oficial de GrxFirma | PENDIENTE: confirmar reserva en AMO |

En Firefox, un ID con forma de correo pertenece a quien lo registra primero en
AMO; no hace falta tener el dominio. Mientras no conste la reserva, otra
persona podría registrar `grxfirma@aavidad.github.io`, obtener un XPI firmado y
hablar con el host de todos los equipos con GrxFirma (hallazgo JD-02). Para
cerrarlo hay que comprobar en AMO que el ID está en la cuenta del proyecto, la
de `WEB_EXT_API_KEY` del job `build-firefox-extension`, con al menos una
versión firmada. Si no lo está, se sube cuanto antes una versión sin listar
desde esa cuenta. Al confirmarlo se cambia el estado de la fila.
`TestCallerPolicyOnlyDocumentedIDs` (`cmd/nativehost`) falla si esta tabla y
los ID integrados en el binario no coinciden.

Además, el host acepta el ID que el instalador deja en
`extensions/chromium/grxfirma-extension-chromium.id` y los que figuren en un
manifiesto de host instalado con uno de esos dos nombres que apunte a este
mismo ejecutable.

Las extensiones «portafirmas» (`com.dipgra.portafirmas`) ya no son llamantes de
GrxFirma desde la 0.0.125: ni sus ID ni su manifiesto autorizan llamadas
(`TestNativeCaller_RechazaManifiestoEIDsDePortafirmas`).

- Solo la extensión oficial cuenta como «oficial»: los ID de la tabla, el del
  fichero `.id` y los de los manifiestos anteriores. Un ID admitido solo por el
  modo de desarrollo no lo es.
- La política de máquina `aprobacion_automatica_nativehost` solo se aplica a la
  extensión oficial. Con cualquier otro llamante el host pide siempre
  confirmación, porque el origen solicitante que declara no se puede comprobar:
  la extensión oficial lo calcula a partir de la URL que da el navegador.

### Portales que pueden pedir pruebas de identidad

La extensión acepta `proveIdentity` desde cualquier subdominio de `dipgra.es` y
de `savia.net`. Es una decisión expresa de Alberto del 13/11/2026 (hallazgo
C-08): se mantienen todos los subdominios. La lista está en
`FACTORY_PORTAL_PATTERNS` (`background.js`), `FACTORY_SITES`
(`trusted_sites.js`) y en `content_scripts` y `host_permissions` de los
`manifest.json` de Chromium y Firefox.
