<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Frontend Qt/QML importado desde V1

Este directorio contiene la base del frontend Qt/QML de la V1 Go, importada
como punto de partida para `GrxFirma`.

Estado actual:

- el backend objetivo es `grxfirma`
- el modo por defecto para escritorio es `IPC`
- el modo `REST` sigue disponible como consola web/API local y para integración experta
- el target de `qmake` es `grxfirma-gui-qml`
- la GUI Qt6 soporta firma visible `PAdES` en:
  - una página concreta
  - rangos como `1,3-5`
  - todas las páginas con `all`
- la GUI Qt6 soporta también metadatos `PAdES` opcionales:
  - motivo
  - ubicación
  - contacto
- la GUI Qt6 soporta además `QR` opcional dentro del sello visible cuando no se usa imagen personalizada
- la GUI Qt6 incluye presets rápidos del sello visible:
  - `Compacto`
  - `Institucional`
  - `Solo logo`
  - `Logo + datos + QR`
- la GUI Qt6 permite restaurar el sello a valores por defecto sin tocar el resto de preferencias
- la GUI Qt6 soporta firma por lote desde la misma pestaña `Firmar`:
  - varios ficheros sueltos
  - o una carpeta completa
  - con carpeta de salida opcional
  - y resultado por fichero dentro de la propia interfaz
- la GUI Qt6 ejecuta además verificación automática del resultado firmado:
  - tras firma simple
  - y por cada salida correcta del lote
- la GUI Qt6 usa guardado explícito de preferencias:
  - `Guardar preferencias`
  - `Descartar cambios`
  - aviso al cerrar si hay cambios sin guardar
- el canal IPC conserva 60 segundos como límite de admisión de un cliente
  lento, pero permite 30 minutos de inactividad una vez conectado:
  - abrir un selector de fichero;
  - revisar la previsualización y colocar el sello;
  - o introducir una credencial temporal
  ya no desconecta el motor al cumplirse un minuto;
- si el backend propietario sigue vivo y el canal se cierra, el frontend
  vuelve a conectarse. Las contraseñas y claves transitorias nunca se guardan
  en una cola para repetirlas automáticamente;
- la REST local opcional del mismo backend publica:
  - `/` como consola técnica local
  - `/signer` como firmador web local con firma múltiple por `/sign-batch`

Objetivo de esta importación:

- reutilizar la UX madura de V1
- conectar la interfaz Qt/QML con los adaptadores V2 ya cerrados (`IPC` y `REST`)
- mantener el frontend separado del core Go y del frontend Fyne

Contrato mínimo ya cubierto en V2:

- `GET /health`
- `GET /certificates`
- `POST /sign` con `inputPath`, `outputPath` y `certificateIndex`
- `POST /sign_batch` con `inputPaths` y/o `directoryPath`
- `POST /sign-batch` para firma de varios ficheros
- `POST /verify` con `inputPath`
- `GET/POST /settings`
- `GET /service/status` y `POST /service/*`
- `POST /pdf/preview`
- `POST /certificates/import`
- `GET /tls/trust-status`
- `GET /diagnostics/report`
- `POST /tls/clear-store`
- `POST /confianza/instalar` / `POST /trust/install-public-roots`
- selector de idioma e i18n de interfaz
- firma y verificación PDF desde la GUI Qt6
- firma visible `PAdES` en una página, rangos o todas las páginas
- metadatos `PAdES` de motivo, ubicación y contacto
- gestión de certificados, importación P12/PFX y diagnóstico local

Compilación rápida en Linux:

- `(cd cmd/gui-qml && qmake6 grxfirma_qt.pro && make -j"$(nproc)")`
- el binario resultante queda en `cmd/gui-qml/grxfirma-gui-qml`
- `grxfirma-gui` ya lo busca también en esa ruta de desarrollo
- `qrc:/...` se mantiene solo como fallback cuando la build lo trae disponible

Instalación local de usuario desde el checkout:

- sin Qt:
  - `packaging/linux/install-user.sh`
- con Qt/QML:
  - `packaging/linux/install-user.sh --with-qt`

La instalación con Qt deja este layout:

- `~/.local/bin/grxfirma-gui`
- `~/.local/bin/grxfirma-gui-qml`
- `~/.local/lib/grxfirma/gui-qml/qml`
- `~/.local/lib/grxfirma/gui-qml/assets`
- `~/.local/lib/grxfirma/gui-qml/help` si existe `cmd/gui-qml/help`
- `~/.config/grxfirma/settings.json` para preferencias persistentes de la GUI

Estado actual real de preferencias persistidas:

- ya cubre comportamiento de shell y sesión:
  - idioma
  - tema
  - modo experto
  - autocierre
  - modo de cierre
  - residente
  - recordar certificado
  - autoselección cuando solo hay un certificado
  - preferencia entre certificado predeterminado y último recordado
  - prioridad visual del certificado predeterminado en la lista
  - prioridad visual de certificados aptos para firma
  - prioridad visual de certificados vigentes
  - persistencia opcional del buscador de certificados
  - filtros visibles de certificados (`caducados`, `no utilizables`)
- ya cubre ajustes operativos de uso frecuente:
  - `TSA`
  - proxy básico `host/port`
  - algoritmo de huella por defecto
  - copia automática al portapapeles para huellas simples
  - formato de huella por defecto para ficheros
  - formato de manifiesto por defecto para directorios
  - modo recursivo por defecto para huellas de directorio
  - guardado de informe por defecto en comprobación de directorios
  - acción y formato por defecto de firma
  - política de salida por defecto (`renombrar`, `error`, `forzar`)
  - firma visible `PAdES` por defecto
  - aplicación por defecto del sello visible en todas las páginas
  - conservación por defecto del texto sobre imagen en sello visible
  - rotación por defecto del sello visible
  - motivo de firma por defecto
  - resolución automática de formato por tipo documental (`PDF/OOXML/FacturaE/ODF/XML/binario`)
  - política de firma por defecto (`compatibilidad estricta`); los PDF
    estructuralmente inválidos se rechazan siempre
  - cofirma múltiple guiada
  - sello visible y metadatos `PAdES`
  - política XAdES completa persistida, cuando existe en el documento tipado:
    identificador, digest Base64, algoritmo y qualifier se normalizan y
    alimentan la firma simple y por lotes de IPC/REST; el proyector compartido
    conserva la misma precedencia en la ruta IPC de multifirma, aunque la GUI
    no ofrece XAdES en ese flujo; una opción explícita prevalece y nunca se
    mezcla una política parcial
- sigue sin paridad con `AutoFirma 1.9` en:
  - superficie visual completa para políticas por formato
    `CAdES/XAdES/PAdES/FacturaE`; FacturaE, el subfiltro PAdES y el consumo
    runtime de la política XAdES ya tienen camino real
  - almacenes y filtros de certificados
  - proxy con credenciales y exclusiones
  - seguridad/dominios como superficie de producto cerrada

Si reinstalas una build Linux sin Qt encima de otra que sí lo traía, el
instalador limpia el `grxfirma-gui-qml` antiguo y `gui-qml/` para que el
lanzador no reutilice recursos obsoletos. Las preferencias de usuario no se
borran.

Ese es el layout real que resuelve [main.cpp](./main.cpp):

- primero `../lib/grxfirma/gui-qml/qml/main.qml`
- después `../lib/grxfirma/gui-qml/assets`
- después `../lib/grxfirma/gui-qml/help` para PDFs de ayuda localizada
- y solo si no existen, intenta `qrc:/...` o la ruta de desarrollo del repo

Resolución de ayuda PDF antes del fallback HTML:

- `help/ayuda-<locale>.pdf`
- `help/ayuda-<lang>.pdf`
- `help/ayuda.pdf`
- `help/<locale>/ayuda.pdf`
- `help/<lang>/ayuda.pdf`

Ejemplos válidos:

- `cmd/gui-qml/help/ayuda-es.pdf`
- `cmd/gui-qml/help/ayuda-en.pdf`
- `cmd/gui-qml/help/es/ayuda.pdf`
- `cmd/gui-qml/help/en/ayuda.pdf`
