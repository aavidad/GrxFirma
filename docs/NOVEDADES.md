<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Novedades de GrxFirma

Estas notas describen los cambios visibles para quienes usan la aplicación.

## 0.0.109 — 2026-10-03

- GrxFirma comprueba en segundo plano si hay una versión nueva al arrancar y cada 5 horas mientras sigue abierta o en la bandeja. Lo hace aunque el motor local no responda y sin esperas si no hay internet, y avisa con «Descargar e instalar», «Ver novedades» y «Ahora no».
- Windows: exportar el informe de verificación, configurar el sello de tiempo, filtrar certificados por NIF, organización o tipo, controlar el servidor REST y usar el asistente de soporte, como en Linux.
- Linux: generador de facturas FacturaE, estado del lector de tarjetas y arranque automático con la sesión, como en Windows.
- Linux: se retira la pestaña de pruebas internas, que mostraba resultados ficticios, y ya no aparece «Cambios sin guardar» al abrir la aplicación.
- Windows: abrir el lanzador directamente ya no busca la interfaz de Linux.

## 0.0.108 — 2026-10-03

- Cuando una web pide que elijas dónde va la firma, GrxFirma abre el editor del sello sobre la página real del PDF: puedes moverlo, agrandarlo y girarlo, o firmar sin sello. Antes solo había seis posiciones fijas.
- Android: firma con el DNIe por NFC. Introduce el CAN, acerca el DNIe al móvil y escribe el PIN; la clave nunca sale de la tarjeta y ni el CAN ni el PIN se guardan.
- Windows: si la aplicación se cierra por un error inesperado, deja un registro local para poder diagnosticarlo.

## 0.0.107 — 2026-10-03

- Validador en servidor: admite un certificado y una clave TLS propios (`-certificado-tls-rest` y `-clave-tls-rest`) y acepta el nombre heredado `AUTOFIRMAV2_REST_TOKEN` para el token.

## 0.0.106 — 2026-10-02

- Verificación más completa de los PDF con varias firmas: el servicio REST ofrece en `/v2/verify` un informe por cada firma que indica qué parte del documento cubre y si el PDF cambió después de firmarlo.
- Las firmas longevas (PAdES-LT y PAdES-LTA, con datos de validación o sello de tiempo de documento) se comprueban y ya no quedan como «no comprobadas».
- Si no se puede comprobar la revocación de un certificado, la firma ya no aparece como válida.
- Nuevo modo «solo verificación» para usar GrxFirma como validador en un servidor, con TLS 1.3 y token obligatorios.

## 0.0.105 — 2026-10-01

- Corrección de compilación de la versión de Windows; mismo contenido que la 0.0.104.

## 0.0.104 — 2026-10-01

- Android: firma visible con editor del sello (mover con el dedo, agrandar, girar, opacidad, QR y vista previa real).
- Al actualizar, GrxFirma muestra las novedades desde la versión que tenía y se reinicia sola con la versión nueva.
- El menú del icono de la bandeja incluye «Ayuda» con el manual y «Acerca de».
- «Buscar actualizaciones» explica con claridad si no hay versiones publicadas o si falla la conexión.
- Linux: la ventana muestra el icono de GrxFirma en la barra de tareas y «Acerca de» se puede desplazar.

## 0.0.103 — 2026-10-01

- Linux: al actualizar o desinstalar, el instalador cierra GrxFirma y todos sus servicios (bandeja, REST, conexión con el navegador) y, tras actualizar, vuelve a abrir la aplicación a quien la tenía abierta.
- El botón «Asistente» lleva un icono de ayuda y el asistente ya no repite el botón «Abrir ayuda».

## 0.0.102 — 2026-10-01

- Iconos de la barra lateral (Firmar, Verificar, Cifrar…) más grandes.
- Linux: «Acerca de» vuelve a mostrar la versión instalada.

## 0.0.101 — 2026-10-01

- Nuevo tirador con flecha circular para girar el sello arrastrándolo en la vista previa; se ajusta solo a 0°, 90°, 180° y 270°, y con Mayús gira de 15 en 15°.
- Las opciones de imagen y de diseño del sello solo aparecen cuando la firma visible está activada.
- El servidor REST local usa el mismo certificado que los navegadores ya aceptan, y deja de dar error de certificado.

## 0.0.100 — 2026-10-01

- Firma visible: la opacidad se aplica a todo el sello, para que se vea el documento debajo.
- Al girar el sello, gira entero como una tarjeta y conserva su tamaño, sin deformarse ni encogerse, con cualquier ángulo.
- Linux: el sello se puede agrandar arrastrando la esquina sin que se suelte solo, y en la lista de certificados solo se resalta el que está bajo el ratón.

## 0.0.99 — 2026-10-01

- Firma visible: el sello se puede colocar hoja por hoja (posición, tamaño y giro distintos en cada una), o en todas a la vez con «Aplicar a todas las hojas». Sigue siendo una sola firma.
- Nuevo ajuste de opacidad del logo del sello, para que se vea el fondo del documento.
- El código QR de verificación se activa con una casilla; la dirección puede escribirse sin «https://» y solo se admiten direcciones seguras.
- Linux: la vista previa del sello girado coincide con el documento firmado y la lista de certificados separa cada certificado.
- Windows: la desinstalación silenciosa ya no se queda parada esperando un aviso, y la instalación cierra antes los programas de GrxFirma abiertos.

## 0.0.98 — 2026-09-30

- Al firmar un documento aparece un aviso verde de «Firmado» con un botón para verlo.
- Los certificados que no sirven para firmar se muestran en rojo como «No válido», con el motivo debajo.
- El panel de certificados queda plegado; se abre con «Ver todos los certificados», junto al desplegable.
- Nuevo «Compartir mi certificado»: exporta tu certificado público para que otra persona pueda protegerte archivos.
- Los botones «Examinar» quedan alineados con su campo en todas las pantallas.
- En Windows, las ventanas que piden una acción durante la firma desde un portal salen al frente, y la instalación silenciosa ya no se queda parada.
- Extensión del navegador 1.1.0: apartado «Sitios de confianza» para añadir otros portales donde firmar PDF; el botón Firmar solo responde a clics reales; los PDF grandes abren el firmador sin bloquearse. En Linux, la extensión vuelve a conectar con GrxFirma.

## 0.0.97 — 2026-09-30

- Linux: la firma con certificados de Firefox y la comprobación de revocación ya no fallan cuando Firefox está abierto; si falta el certificado del emisor, se descarga de la dirección indicada en el propio certificado.
- Linux: al abrir GrxFirma por primera vez se instala la confianza del canal local también en Firefox (normal, snap y flatpak), para que los portales puedan conectar.
- Firma desde portales: la ventana de GrxFirma ya no tapa la web; se sitúa en una esquina y se retira al entregar la firma, avisando de forma discreta.
- Seguridad: el certificado local que usa el navegador para hablar con GrxFirma queda restringido a este equipo, se renueva cada poco tiempo y su clave se guarda protegida; los administradores pueden desactivar ese canal.
- Mensajes de error de firma más claros, con la causa concreta.

## 0.0.96 — 2026-09-30

- En Windows, el instalador vuelve a crear el acceso a GrxFirma en el menú Inicio y, por defecto, en el escritorio.
- Nuevo icono de GrxFirma.
- GrxFirma se queda en la bandeja del sistema al cerrar la ventana (se puede desactivar en Ajustes), con opción de iniciarse con Windows.
- La aplicación de Linux incorpora las novedades de Windows: vista previa real del sello, estilos, QR, páginas y giro libre; estado y caducidad de los certificados con acceso a la renovación FNMT; guía para DNIe y tarjetas; destinatarios públicos en Proteger; y novedades en Acerca de.
## 0.0.95 — 2026-09-28

- En Windows, los programas se instalan en %LOCALAPPDATA%\Programs\GrxFirma y los datos del usuario se guardan aparte en %LOCALAPPDATA%\GrxFirma, de modo que reinstalar o actualizar ya no falla después de haber usado la aplicación.

## Próxima versión (sin publicar)

- En Windows, la instalación y las actualizaciones guardan los programas en `%LOCALAPPDATA%\Programs\GrxFirma`. Los datos de uso y la configuración permanecen en sus carpetas de usuario al desinstalar.

## 0.0.94 — 2026-09-28

- Corregida la conexión con los portales web en equipos donde nunca se había usado la aplicación: el certificado local que usa el navegador para comunicarse con GrxFirma no quedaba como de confianza. Ahora el instalador lo instala por la vía normal de Windows, que pide una única confirmación.

## 0.0.93 — 2026-09-28

- GrxFirma deja de usar el nombre «AutoFirma» también en sus carpetas de instalación, programas, configuración y extensiones del navegador. Sigue funcionando con los portales de la Administración que firman mediante el protocolo afirma://.

## 0.0.92 — 2026-09-26

- En la firma desde portales web, la cuenta atrás de cierre muestra los segundos que faltan y los mensajes con datos (canal, tiempos del diagnóstico) se muestran completos.

## 0.0.91 — 2026-09-26

- En Windows, el instalador ya no deja accesos directos duplicados ni con el nombre anterior en el escritorio o el menú Inicio.
- Los ficheros de instalación se llaman ahora GrxFirma con su número de versión (por ejemplo, GrxFirma-0.0.91-windows-amd64-setup.exe).

## 0.0.90 — 2026-09-26

- La aplicación pasa a llamarse GrxFirma. Se han actualizado los nombres visibles y los accesos directos.
- En Windows, «Usar DNIe o tarjeta» guía la detección del lector y la tarjeta, actualiza los certificados y explica cómo instalar el controlador o introducir el PIN al firmar.
- En Windows, el sello visible de los PDF ofrece un nuevo diseño con emblema original, nombre del firmante, fecha y emisor. Puede elegirse un estilo institucional, solo texto o una imagen propia.
- En Windows, el sello puede incluir un código QR con una dirección HTTPS de verificación. Puede colocarse en la primera página, la última, todas o páginas concretas, y girarse libremente. La vista previa muestra el sello real antes de firmar.
- En Windows, la ficha del certificado muestra más información y avisa cuando se acerca su caducidad. Si es un certificado FNMT, ofrece acceso a la página oficial de renovación.
- En Firma y Certificados, las tarjetas muestran de un vistazo la fecha de vencimiento y el emisor; al elegir un certificado próximo a caducar aparece un aviso con la opción de renovarlo cuando corresponde.
- En Proteger, la aplicación Windows permite importar certificados públicos de destinatarios para que puedan abrir el documento protegido. No hace falta importar sus claves privadas.
- El logotipo de la aplicación se ha sustituido por un emblema original.

## Histórico: nombre anterior AutoFirmaV2, versiones 2.0.x

### 2.0.3 — 2026-09-26

- En Windows, «Usar DNIe o tarjeta» guía la detección del lector y la tarjeta, actualiza los certificados y explica cómo instalar el controlador o introducir el PIN al firmar.

### 2.0.2 — 2026-09-26

- En Windows, el sello visible de los PDF ofrece un nuevo diseño con emblema original, nombre del firmante, fecha y emisor. Puede elegirse un estilo institucional, solo texto o una imagen propia.
- En Windows, el sello puede incluir un código QR con una dirección HTTPS de verificación. Puede colocarse en la primera página, la última, todas o páginas concretas, y girarse libremente. La vista previa muestra el sello real antes de firmar.
- En Windows, la ficha del certificado muestra más información y avisa cuando se acerca su caducidad. Si es un certificado FNMT, ofrece acceso a la página oficial de renovación.
- En Firma y Certificados, las tarjetas muestran de un vistazo la fecha de vencimiento y el emisor; al elegir un certificado próximo a caducar aparece un aviso con la opción de renovarlo cuando corresponde.
- En Proteger, la aplicación Windows permite importar certificados públicos de destinatarios para que puedan abrir el documento protegido. No hace falta importar sus claves privadas.
- El logotipo de la aplicación se ha sustituido por un emblema original.

### 2.0.1 — 2026-09-26

- Se amplió la compatibilidad de firma desde sedes electrónicas y navegadores, incluidos los flujos de firma de Aragón, Canarias, MITES, Castilla y León, VALIDe y Navarra probados durante la campaña.
- Se mejoraron la firma y verificación de PDF, XML y otros documentos, la protección de archivos y los informes de validación.
- El sello visible permite elegir su posición en el PDF desde la aplicación y en los portales que solicitan esa elección.
- Se reforzó la lectura de PDF cifrados o dañados para evitar cierres inesperados.
