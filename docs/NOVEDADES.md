<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Novedades de GrxFirma

Estas notas describen los cambios visibles para quienes usan la aplicación.

## 0.0.124 — 2026-10-07

- Nuevo logotipo de GrxFirma: una pluma que escribe la rúbrica, en la barra lateral y en «Acerca de».
- Windows: un selector en Configuración permite elegir si las firmas que piden los portales las atiende GrxFirma o AutoFirma, sin desinstalar ninguno; las actualizaciones respetan la elección.
- Windows: con tarjetas criptográficas o DNIe en el almacén, GrxFirma ya no pide la tarjeta en bucle al listar certificados; solo la usa al firmar, y los certificados de tarjeta aparecen en la lista.
- Windows: la bandeja y los avisos se llaman «GrxFirma»; al actualizar con la app abierta se avisa de que se está reiniciando el servicio en vez de mostrar un error; y la comprobación de versiones funciona con el proxy de la red.

## 0.0.123 — 2026-10-06

- Ayuda de las opciones: el «?» de Operación, Formato y otros selectores muestra todas las opciones a la vez con una frase cada una, y un «+» despliega una explicación más amplia.

## 0.0.122 — 2026-10-06

- El icono GRX aparece en el escritorio, el menú Inicio y la extensión del navegador sin restos de la silueta anterior; el instalador refresca los iconos de Windows al terminar.
- Windows: los botones de ayuda son solo un círculo con la interrogación, sin el recuadro de botón.

## 0.0.121 — 2026-10-06

- Ayuda en cada opción: un botón «?» junto a cada ajuste técnico (cofirma, contrafirma, formatos, perfil B, T, LT o LTA, sellado de tiempo, sello visible, protección…) explica qué es y cuándo usarlo.
- Windows: al actualizar se retiran del escritorio los accesos directos de versiones anteriores de GrxFirma.
- Linux: el perfil de firma por defecto también puede ser Baseline LT o LTA.

## 0.0.120 — 2026-10-06

- Vuelve el icono GRX con el trazo de firma en Windows, Linux, Android y la extensión, y el sello de los PDF recupera su emblema y sus colores.
- Android: el editor del sello muestra todos sus botones, se corrige el tipo de certificado, el botón del DNIe se desactiva de verdad sin NFC y «Acerca de» es más compacto.

## 0.0.119 — 2026-10-05

- Nuevo logotipo de GrxFirma, y la aplicación figura a nombre de su autor.
- El sello visible queda donde se coloca también en páginas giradas, como los escaneos en horizontal, y con cualquier ángulo de giro.
- Más seguridad: un PDF manipulado ya no puede bloquear la aplicación ni agotar la memoria, y solo la extensión de GrxFirma puede conectar con el programa.
- Verificación: los datos del certificado y los errores de la cadena se muestran legibles y traducidos en todas las plataformas.
- Linux: se ve el nombre propuesto al guardar, las firmas seguidas se numeran _001, _002… y los interruptores tienen nombre para los lectores de pantalla.
- La extensión de navegador y la app de Android usan identificadores nuevos; al actualizar se retiran los registros antiguos.

## 0.0.118 — 2026-10-05

- Windows: vuelve a funcionar el sello elegido por la persona en los portales (Canarias y otros): el botón «Firmar con el sello aquí» se activa y, si la vista falla, se puede firmar sin sello o cancelar. La ventana se cierra sola si el portal deja de esperar.
- Windows: «Desproteger» ya no falla ni cierra la aplicación; «Confirmar antes de firmar» funciona; el informe imprimible es la opción principal al verificar; y toda la interfaz se traduce al idioma elegido.
- Veri*Factu: la consulta a la AEAT interpreta bien su respuesta (encontrada, no encontrada o no contrastable) en Windows y Linux, y el informe distingue avisos de errores.
- Linux: buen contraste en todos los temas (también los claros), casillas y campos legibles, la rueda del ratón llega hasta «Firmar ahora» y las pantallas se adaptan a ventanas estrechas.
- Android: el lote indica en qué carpeta se guardaron las firmas, el sello y la vista previa muestran el mismo emisor, y Preferencias y el editor del sello se ven bien con letra grande.
- Windows y Linux: con el sello visible activado por defecto, la vista previa se carga sola antes de firmar. En los portales, el sello queda donde lo coloca la persona aunque lo mueva.
- Windows: GrxFirma ya no termina con error al salir en equipos sin tarjeta gráfica (escritorios remotos, Citrix o máquinas virtuales), y el aviso del portal se cierra solo tras cancelar.
- Portales: al firmar un PDF solo se ofrecen ficheros PDF y, si se elige otro tipo, se explica el motivo en lugar de mostrar un error genérico.
- Informe de verificación: el firmante aparece con su nombre, NIF y emisor, y la fecha indica si procede de un sello de tiempo o del reloj del equipo.
- La opción «Renombrar» ya no reemplaza nunca un fichero existente: el nuevo se guarda con un número (_001) y solo se sobrescribe si se confirma en el diálogo de guardar.
- Linux: el informe de verificación muestra el firmante por su nombre y la fecha de la firma, se puede guardar el informe imprimible y los diálogos salen en el idioma de la aplicación.
- Traducciones revisadas en los 10 idiomas: se corrigen cientos de textos que se habían traducido con un sentido equivocado.

## 0.0.117 — 2026-10-05

- Firma remota: un lote completo con un solo PIN o código, si el prestador lo admite; «Proteger y firmar» con certificado remoto; la sesión se renueva sola y se listan todas las credenciales.
- El sello visible y el informe de verificación salen en el idioma de la aplicación, con la hora local y su zona. En Configuración se puede fijar un idioma del sello, por ejemplo el español para documentos de la Administración.
- Android: lectura del QR tributario con la cámara o desde una imagen, varios certificados abiertos a la vez, resultado con un veredicto claro y detalles técnicos plegados, y editor del sello más cómodo.
- Android: textos más claros y sin tecnicismos, el informe imprimible como opción principal y el cierre automático del certificado ya no salta mientras se elige un fichero ni con una firma sin guardar.

## 0.0.116 — 2026-10-05

- Android: pantalla por pasos, resultado visible al terminar, confirmación antes de descartar una firma, cierre automático del certificado tras unos minutos en segundo plano y textos más claros.
- Android: datos y caducidad del certificado con comprobación de revocación, preferencias, diagnóstico, búsqueda de versiones nuevas, tema claro u oscuro e informe de verificación imprimible.
- Android: expediente ENI, lote con sello visible o cofirma, y DNIe en lote y en «proteger y firmar» pidiendo el PIN una sola vez. El DNIe por NFC ya admite el certificado de firma de la tarjeta.
- Windows y Linux: firma remota con prestadores compatibles con CSC desde la propia aplicación, si la organización la permite; si la prohíbe, se indica claramente.
- Veri*Factu: el QR tributario también se lee desde una imagen o un PDF, y la comprobación rechaza registros con contenido añadido dentro de la firma.
- ENI: antes de crear el documento se comprueba que la firma corresponde al original elegido.
- Se leen bien las configuraciones guardadas con el Bloc de notas antiguo o con PowerShell 5 (con marca BOM).
- Windows y Linux: pantallas de facturas, Veri*Factu y ENI más claras y accesibles: Veri*Factu tiene su propia sección, la fecha se elige sin formatos técnicos, mejor contraste en todos los temas y el sello se dibuja también con teclado.

## 0.0.115 — 2026-10-05

- Android: firma por lotes, crear y comprobar huellas, y cifrar y descifrar ficheros, como en escritorio.
- Android: firma en XMLDSig, ODF, OOXML, FacturaE, ASiC-XAdES y registros Veri*Factu; leyenda CSV y sello con posición distinta en cada página; crear y comprobar documentos ENI.
- Firma remota con certificados de prestadores compatibles con la API CSC, desde la línea de órdenes. Viene desactivada: solo funciona si la organización la habilita en su política o en la configuración. Aún no se ha probado con prestadores reales.

## 0.0.114 — 2026-10-05

- Veri*Factu: firma los registros de facturación que entrega un programa de facturación con el formato que exige la AEAT, y comprueba registros ya firmados: huella encadenada, firma y estructura. GrxFirma no genera, guarda ni envía registros.
- Veri*Factu: lee la URL del QR tributario de una factura y muestra sus datos; solo si lo pides, la coteja con el servicio público de la AEAT.
- Sello visible: con «Dibujar área» marcas su posición y tamaño arrastrando sobre la página, como en AutoFirma. También con teclado. En Windows y Linux, y en el editor que abren los portales.
- Android: cofirma y contrafirma, perfiles con sello de tiempo, verificación automática tras firmar, verificación detallada con informe exportable, los once idiomas con selector propio y pantalla «Acerca de».

## 0.0.113 — 2026-10-04

- Documentos y expedientes ENI: el estado de elaboración, el tipo documental y el estado del expediente se eligen de una lista con los códigos oficiales y su descripción.
- ENI: las fechas de captura y de apertura se eligen en un calendario.
- ENI: GrxFirma comprueba que el documento o expediente generado cumple la estructura de la norma técnica (NTI) y avisa de cada problema. También valida uno existente con «grxfirma -operacion validar-eni -entrada <fichero>».
- Las direcciones de verificación del QR y de la leyenda CSV admiten dominios con acentos o eñe, por ejemplo «https://sede.almuñécar.es/verificar».
- Linux: los diálogos se leen bien con cualquier tema; el texto ya no sale claro sobre fondo claro.

## 0.0.112 — 2026-10-04

- Toda la aplicación aparece en el idioma elegido: se han traducido más de mil textos que seguían en español o en inglés en los diez idiomas disponibles.
- Windows: al cambiar de idioma en Configuración cambian también los textos fijos de todas las pantallas.
- Si un campo tiene un error, se marca en rojo con el motivo debajo y, al firmar o guardar, GrxFirma te lleva a ese campo; el diagnóstico y el asistente ofrecen «Corregir» para ir directamente. En Windows y Linux.
- Leyenda CSV: cada problema tiene su propio mensaje (falta el código, la URL no es válida…) y la dirección puede escribirse sin https://.
- Windows: se admiten servidores de sello de tiempo con dirección http://, como el de la FNMT, igual que en Linux y en AutoFirma Java.

## 0.0.111 — 2026-10-04

- Windows: actualizar con GrxFirma abierta ya funciona. El instalador cierra la aplicación, instala la versión nueva y la vuelve a abrir; antes fallaba con «La instalación PowerShell de la suite ha fallado con código 1».

## 0.0.110 — 2026-10-03

- Windows: si la conexión con el motor local se interrumpe, GrxFirma vuelve a conectar sola en la siguiente operación. Antes la aplicación podía quedarse sin motor («falla todo») hasta cerrarla del todo.
- Validar facturas FacturaE, UBL y CII desde la aplicación, con informe exportable, en Windows y Linux.
- Crear documentos y expedientes ENI desde la aplicación, en Windows y Linux.
- Leyenda CSV de cotejo en el sello PAdES (código y URL de la Administración), en Windows y Linux.

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
