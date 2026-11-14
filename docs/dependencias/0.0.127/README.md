<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Expediente técnico de dependencias — 0.0.127

Revisión técnica: **14/11/2026**. Estado: inventario generado y triage local;
**aceptación de Alberto pendiente**, expediente contractual abierto.

## Procedencia y alcance

El [inventario generado](inventario.json) relaciona **690 identidades y versiones**
con sus ocurrencias, uso, licencia, evidencia legal y procedencia. Sus 38 fuentes
llevan SHA-256. El [procedimiento](../README.md) permite regenerarlo y controlar
las altas sin red, reutilizando los generadores y filtros existentes.

Se ha trabajado sobre `729c47a0679af9e90b7c46041717265dda44a18c`, que contiene
los lotes locales previos. El tag publicado `0.0.127` es
`b8902d63bf7b110bb7777a38f4e5af43c54b371c` (también `origin/main` consultado).
[fuentes.json](evidencias/fuentes.json) compara cada fuente con ese tag: todos
los manifiestos de dependencias coinciden; `legal/licensing.toml` amplía en este
lote la cobertura central de las evidencias generadas. La comparación debe
regenerarse si se cambia cualquier fuente.

No se han descargado ni examinado binarios publicados por plataforma. Este es
un expediente de fuentes asociado a esa versión, no una nueva atestación de
los assets de la release. Queda adjuntar sus SPDX oficiales y sus hashes,
construcciones por plataforma, avisos de redistribución y aceptación humana.
La enumeración completa nativa de Syft está comprimida en
[evidencias/syft-local.json.gz](evidencias/syft-local.json.gz).

| Ecosistema | Identidades y versiones | Uso y evidencia |
|---|---:|---|
| Go | 52 | Requisitos raíz y de las copias PDF, hashes go.sum, licencias Syft/caché y avisos locales; conservar diferencias de MVS |
| npm | 316 | Herramientas de construcción de extensión; versiones, licencias, URL de origen e integridad del lockfile |
| Maven | 276 | Configuraciones Gradle clasificadas usando el filtro existente de runtime; licencias de POM locales, con padres y hashes |
| NuGet | 30 | WinUI/auxiliares de pruebas, versiones resueltas y contentHash; licencia exacta pendiente en todos los casos por falta de metadatos locales |
| Qt | 8 | Módulos declarados en proyectos; uso de ejecución o pruebas; versión del kit y modalidad de licencia pendientes |
| Toolchain | 2 | Go 1.26.11 y Gradle 9.4.1, construcción; no promete soporte del SDK |
| Copias upstream y avisos legales | 6 | Bases PDF, pdfsign, pkcs7 y wrapper; licencias y notices preservados; se solapan con sus ocurrencias Go, no son seis paquetes adicionales distribuidos |

La lista congelada de deuda inicial contiene **38** identidades Qt/NuGet, sin
licencia acreditada. El control ordinario las muestra y rechaza altas nuevas
sin licencia/uso; `--strict` falla hasta cerrarlas. Los textos de licencia de
fuentes gráficas que Syft identifica solo por hash también requieren examen
humano. La modalidad LGPL/GPL/comercial del kit Qt y los términos de cada
paquete WinUI no se deducen de su nombre ni se dan por aceptados.

## Herramientas del sistema y pérdida de funcionalidad

Son requisitos de borde o de prueba; no implica que se empaqueten sus binarios.
Las versiones siguientes describen **este equipo de ensayo**. Al distribuir
por plataforma hay que inventariar las versiones y avisos reales de sus paquetes.

| Herramienta / plataforma | Uso | Si falta | Procedencia legal/versión local |
|---|---|---|---|
| certutil, pk12util / NSS Linux | Ejecución opcional: consulta, importación y confianza NSS; certutil también aísla pdfsig en pruebas | No funcionan esas operaciones NSS ni la prueba pdfsig aislada | libnss3-tools 3.120-1ubuntu2.2, paquete Ubuntu; avisos MPL-2.0/Zlib/BSD según fichero en `/usr/share/doc/libnss3-tools/copyright` |
| openssl / Linux y auxiliar P12 | Ejecución opcional: rutas auxiliares P12/NSS y compatibilidad de importación; pruebas CMS/TSA | Fallan las rutas que lo invocan, no se promete esa importación auxiliar; sin verificación independiente CMS/TSA | OpenSSL 3.5.5 (paquete 3.5.5-1ubuntu3.7), Apache-2.0 y avisos del paquete en `/usr/share/doc/openssl/copyright` |
| pdfinfo, pdftoppm / PDF | Ejecución opcional: metadatos y previsualización/rasterización PDF | Sin esa previsualización ni lectura QR desde PDF que requiera rasterización | poppler-utils 26.01.0-2ubuntu0.3; licencias por fichero GPL-2/GPL-3 y avisos de `/usr/share/doc/poppler-utils/copyright` |
| notify-send / Linux | Ejecución opcional: aviso de escritorio | Se pierde la notificación; la operación ya completada conserva su resultado | Herramienta del sistema; versión y avisos del despliegue pendientes |
| secret-tool / Linux | Ejecución opcional: guardar/recuperar credenciales de proxy en Secret Service | No se puede persistir ni recuperar el secreto por ese backend | libsecret-tools 0.21.7-2build1 local; conservar copyright/licencia del paquete al distribuir |
| systemctl --user / Linux | Ejecución opcional: gestionar el servicio del usuario | Sin gestión mediante systemd del agente | Versión y avisos del despliegue pendientes |
| security, open / macOS | Ejecución por plataforma: confianza, llavero y apertura | Sin instalación/consulta de confianza por esa ruta ni apertura del gestor/navegador | Herramientas incluidas en macOS; no instaladas ni ensayadas aquí, términos del sistema pendientes |
| cmd/certmgr.msc, rundll32 / Windows | Ejecución por plataforma: apertura de gestores, enlaces y navegador CSC | Sin esa apertura/integración del sistema | Herramientas Windows; no ensayadas aquí, términos del sistema pendientes |
| xdg-open y gestores/navegadores detectados (Seahorse, Kleopatra, GCR, Firefox, Chromium/Chrome, Edge, Brave) | Ejecución opcional: abrir enlaces CSC o gestor del usuario | No se puede abrir el gestor o completar esa interacción por navegador desde la app | Allowlist en `certaccess/certaccess.go` y `csc/navegador.go`; versiones/licencias según instalación, pendientes por despliegue |
| bwrap / Linux | Opcional experimental: aislamiento del trabajador PKCS#11 | El trabajador rechaza operar; no se habilita una ruta sin aislamiento | bubblewrap 0.11.1-1ubuntu0.3 local; PKCS#11 sigue sin ofertarse como producción |
| pdfsig / prueba | Validar firma PAdES | Sin evidencia independiente de Poppler | poppler-utils 26.01.0-2ubuntu0.3; no valida aquí confianza cualificada |
| qpdf / prueba | Comprobar estructura PDF | Sin comprobación estructural independiente | qpdf 12.3.2-1; avisos en `/usr/share/doc/qpdf/copyright` |
| xmlsec1 / prueba | Validar XMLDSig de XAdES/FacturaE | Sin verificación XML independiente | No instalado; versión/licencia de futura instalación pendientes |
| runner DSS / prueba | Conformidad con política y confianza de laboratorio | Sin evidencia DSS/TOTAL_PASSED | No disponible; montaje descrito en la guía de conformidad |

Las referencias de código de estas rutas son `pkcs12importer`, `nssstore`,
`localtlstrust`, `certaccess`, `pdfpreview`, `proxysecretstore`, `desktopnotify`,
`servicemanager`, `pkcs11worker` y `csc/navegador.go`, bajo los adaptadores de
`internal/`. Se han leído, sin modificarlos. Esta tabla amplía el inventario
operativo; los requisitos de bibliotecas gráficas nativas, SDK y herramientas
no disponibles deben acreditarse sobre el empaquetado de cada plataforma.

## Revisión de vulnerabilidades local

[evidencias/revision.json](evidencias/revision.json) guarda alcance, códigos de
salida, versiones y hashes de las bases Go/npm; [SHA256SUMS](evidencias/SHA256SUMS)
identifica los informes y logs conservados. No se han enviado fuentes ni datos
a servicios. Se han usado herramientas instaladas, sin descargar herramientas,
módulos ni bases nuevos.

| Informe | Resultado revisado | Decisión técnica |
|---|---|---|
| govulncheck 1.6.0, aplicación | Salida 0; GO-2026-5932 en x/crypto 0.58.0, traza limitada al módulo | No comunica paquete/símbolo alcanzable en linux/amd64; conservar aviso, sin excepción nueva |
| govulncheck 1.6.0, pdfsign aislado | Salida 0; mismo aviso en x/crypto 0.56.0 | No comunica paquete/símbolo alcanzable; root selecciona 0.58.0 al construir la app |
| OSV 2.3.8, Go y npm | Salida 1: GO-2026-5932 en ambos go.mod; deepmerge filtrado por excepción existente | No declarar escaneo limpio; conservar triage y revisar cambios de imports/plataforma |
| OSV Maven/NuGet | Salida 127: faltan ambas bases offline | No comprobado; pendiente disponer de bases locales, sin activar consultas online |
| Grype sobre Syft fuente | Salida 1: base construida el 13/10/2026, rechazada por edad máxima de cinco días | No comprobado; no se rebaja el control de antigüedad ni se equipara al gate de runtime |

Se exportaron **4.820** entradas oficiales `GO-*.json` del ZIP Go de OSV para
govulncheck, sin alterarlas. La fecha de modificación que comunica esa base es
13/11/2026 19:12:57 UTC. Es una instantánea local; la fecha del fichero ZIP no
se interpreta como publicación del contenido.

`GO-2026-5932` declara inseguro y sin mantenimiento `x/crypto/openpgp` y sus
subpaquetes. El aviso sitúa la corrección del módulo en
`0.58.1-0.20261009181718-2f1f834330b2`. El análisis local de símbolos no encuentra
uso alcanzable, lo cual no sustituye el análisis de otras plataformas ni una
aceptación de riesgo. No se modifica go.mod en este lote; la actualización y
su regresión deben encargarse dentro de un lote autorizado. Alberto debe
asignar responsable y decidir si acepta temporalmente este resultado.

## Excepción deepmerge: revisión, sin prórroga

Excepción existente: `GHSA-gcqx-w48m-w2h4`, caduca **2027-02-14**. Se conserva la
fecha exacta. Alberto verificó en el registro npm el **14/11/2026**:

- deepmerge sigue en **4.3.1**, última publicación de 2023, sin versión corregida;
- web-ext **10.8.0** es la última versión comprobada;
- la cadena es `web-ext 10.8.0 > addons-linter 10.14.0 > deepmerge 4.3.1`.

El lockfile local acredita esas versiones y el reason registra la revisión y
su atribución. No se repite la consulta al registro desde este entorno. Es una
herramienta de construcción/validación de la extensión, que mezcla nuestra
configuración y manifiesto; no se instala en el equipo usuario. El riesgo
residual persiste si entran manifiestos/configuraciones ajenos no confiables.
La continuación de la excepción existente se documenta por encargo; no se
inventa una aceptación formal ni un responsable. Confirmación de Alberto:
pendiente, antes de caducar.

No se identifica con la evidencia local un sustituto mantenido y compatible
que permita eliminarla con seguridad. Un alias `overrides` cambia qué paquete
recibe addons-linter bajo el nombre deepmerge: debe conservar API, estrategia
de arrays, clone/customMerge y tratamiento de claves de prototipo. Antes de
aplicarlo hacen falta pruebas de esas semánticas, casos maliciosos, validación
y construcción real de la extensión, además de licencia y escaneos nuevos.
No se cambia package.json/lockfile ni se aplica un override sin esas pruebas.

## Histórico y pendientes de aceptación

| Versión/fecha | Corrección o decisión | Evidencia / límite |
|---|---|---|
| 0.0.127, 14/11/2026 | Go 1.26.11; x/net 0.62.0, x/crypto 0.58.0, x/sys 0.49.0, x/term 0.47.0 y x/text 0.43.0 | go.mod y [novedades](../../NOVEDADES.md#00127--2026-11-14): actualización por fallos conocidos. No se inventan CVE individuales ni backports; tampoco cierra GO-2026-5932 |
| Revisión local, 14/11/2026 | Se mantiene deepmerge con caducidad 14/02/2027; no se cambia dependencia | Reason, lockfile e informe OSV conservados; aceptación formal pendiente |
| Revisión local, 14/11/2026 | Evidencia PAdES básico/B-T y CAdES con herramientas independientes; XAdES/FacturaE internos | [Guía y alcance](../../../test/conformance/README.md); xmlsec1 y DSS pendientes |

Pendiente de Alberto: responsable de triage y licencias, aceptación o rechazo
de riesgos con fecha, cierre Qt/NuGet, modalidad de distribución Qt/WinUI,
compromisos de respuesta, versiones soportadas, backports y archivo de las
licencias/artefactos de cada plataforma. No se prometen SLA, LTS ni certificación.
