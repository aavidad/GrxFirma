<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Política de máquina de GrxFirma

La política de máquina recoge las decisiones que **solo puede tomar un
administrador**: la lista de orígenes de confianza de la organización y las
excepciones de compatibilidad que rebajan la seguridad. Un usuario sin
privilegios, una web o un programa que se ejecute con la cuenta del usuario no
pueden modificarla.

## Dónde se lee

| Sistema | Ubicación | Quién puede escribirla |
|---|---|---|
| Windows | `HKLM\SOFTWARE\Policies\GrxFirma` (vista de 64 bits) | Administradores y GPO |
| Linux / macOS | `/etc/grxfirma/policy.json` y `/etc/grxfirma/allowed-domains.json` | root. Se rechaza si el fichero es modificable por grupo u otros (`chmod 644`, propietario root) |

En Windows **no** se leen `C:\etc\...` ni `%ProgramData%`: cualquier usuario
autenticado puede crear carpetas en ellas y plantar una política que afectaría a
todos los usuarios del equipo (por ejemplo, en servidores de escritorio remoto).

## Valores

| Nombre | Tipo en el registro | Efecto |
|---|---|---|
| `allowed_domains` | `REG_MULTI_SZ` | Orígenes permitidos sin preguntar (equivale a `allowed-domains.json`). |
| `dominios_de_confianza` | `REG_MULTI_SZ` | Dominios adicionales de confianza. |
| `tofu_habilitado` | `REG_DWORD` | `0` rechaza, sin diálogo, cualquier origen fuera de la lista. |
| `websocket_habilitado` | `REG_DWORD` | `1` mantiene un servicio WebSocket residente. Sin él, las sedes solo pueden abrir por `afirma://` un canal temporal que se cierra al terminar cada operación. |
| `websocket_permitido` | `REG_DWORD` | `0` impide todo WebSocket local, incluso el canal temporal de `afirma://`; `1` o ausencia permite los canales temporales y, si se habilita aparte, el servicio residente. Prevalece sobre `websocket_habilitado` y el ajuste del usuario. |
| `rest_habilitado` | `REG_DWORD` | `1` activa el servicio REST local; `0` lo impide aunque el usuario lo active. |
| `nivel_log`, `directorio_p12` | `REG_SZ` | Igual que en `policy.json`. |
| `timeout_operacion_segundos`, `max_tamano_documento_bytes` | `REG_DWORD` / `REG_QWORD` | Límites operativos. |

Excepciones de compatibilidad (todas desactivadas por defecto; `1` las activa):

| Nombre | Qué permite | Variable de entorno que no sustituye esta política |
|---|---|---|
| `permitir_des_legacy` | Formato de sesión DES/ECB de AutoFirma 1.9 también fuera de HTTPS (ver nota) | `GRXFIRMA_ENABLE_LEGACY_DES` |
| `permitir_sha1_legacy` | Generar firmas SHA-1 pedidas por portales antiguos | `GRXFIRMA_ENABLE_LEGACY_SHA1` |
| `permitir_cms_aes_ecb_legacy` | Leer sobres `SignedAndEnvelopedData` AES-ECB de 1.9 | `GRXFIRMA_ENABLE_LEGACY_CMS_AES_ECB` |
| `permitir_origen_vacio` | WebSocket sin cabecera `Origin` o con `Origin: null` | `GRXFIRMA_WS_ALLOW_EMPTY_ORIGIN` |
| `permitir_rutas_directas_protocolo` | Rutas locales absolutas en `load`, `save` y `signandsave` | `GRXFIRMA_ENABLE_LEGACY_DIRECT_PATHS` |
| `aprobacion_automatica_nativehost` | Firmar desde la extensión sin diálogo de confirmación | `GRXFIRMA_NATIVEHOST_AUTO_APPROVE` |

**Nota sobre DES.** El intercambio por servidor intermedio de AutoFirma 1.x
(StorageService/RetrieveService, que usan FIRe y el modo de respaldo de
AutoScript 1.9) cifra el paquete de sesión con DES y una clave de 8 dígitos. Sin
esta política, GrxFirma lo admite **solo para ese paquete y solo si todos los
servidores del intercambio son HTTPS**: la confidencialidad real la da TLS y la
clave del protocolo no es más débil con DES. El usuario ve un aviso con el
motivo. DES nunca se usa para firmar. La política solo amplía el permiso a
servidores sin HTTPS (por ejemplo, un servidor local de pruebas).

**Las variables de entorno antiguas ya no tienen efecto.** Cualquier web, guía o
programa podía pedir al usuario que las definiera para rebajar la seguridad.

## Plantillas de directiva de grupo (ADMX)

`packaging/windows/admx/` contiene `GrxFirma.admx` y sus textos en español
(`es-ES`) e inglés (`en-US`). Se copian al almacén central
(`\\<dominio>\SYSVOL\<dominio>\Policies\PolicyDefinitions`) o a
`%WINDIR%\PolicyDefinitions` y aparecen en *Configuración del equipo >
Plantillas administrativas > GrxFirma*, con cada excepción de compatibilidad
marcada como rebaja de seguridad.

## Ejemplo para Windows (`.reg`)

```reg
Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\Policies\GrxFirma]
"tofu_habilitado"=dword:00000000
"allowed_domains"=hex(7):73,00,65,00,64,00,65,00,2e,00,65,00,6a,00,65,00,6d,00,\
  70,00,6c,00,6f,00,2e,00,65,00,73,00,00,00,00,00
```

(`allowed_domains` contiene `sede.ejemplo.es`). Para `REG_MULTI_SZ` también se
admite un `REG_SZ` con valores separados por `;`.

## Ejemplo para Linux

```json
{
  "tofu_habilitado": false,
  "websocket_permitido": false,
  "dominios_de_confianza": ["sede.ejemplo.es"],
  "permitir_des_legacy": false
}
```

## Semántica de los orígenes

- `sede.ejemplo.es` y `*.ejemplo.es` solo autorizan **HTTPS**, en cualquier
  puerto. `http://` nunca queda autorizado por un patrón de dominio.
- `https://sede.ejemplo.es:8443`, `http://localhost:3000` o
  `chrome-extension://<id>` autorizan exactamente ese esquema, host y puerto.
- Por defecto solo se confía en loopback para el firmador local propio
  (`https://127.0.0.1:63118`). Ninguna extensión del navegador ni otro
  servidor local queda autorizado sin declararlo de forma explícita.
