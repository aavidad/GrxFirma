<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Novedades de GrxFirma

Estas notas describen los cambios visibles para quienes usan la aplicación.

## 0.0.128 — 2026-11-15

- Las opciones inseguras vienen desactivadas para todo el mundo, también al actualizar desde una versión anterior. Si una operación necesita una de ellas, GrxFirma la detiene, explica el riesgo y ofrece una ayuda más larga con la alternativa y la forma de deshacerlo.
- Sin el ajuste correspondiente ya no se puede, por ejemplo, firmar en portales antiguos que usan el intercambio DES de AutoFirma 1.x (aunque vayan por HTTPS), crear sobres CMS para destinatarios que solo abren el formato antiguo, firmar con SHA-1 o con claves RSA de menos de 2048 bits, pedir sellos de tiempo por HTTP, enviar el usuario y la contraseña a un proxy por HTTP ni hacer la firma trifásica por HTTP.
- También hace falta el ajuste para abrir o comprobar material antiguo, como los sobres CMS cifrados con DES, 3DES o AES-CBC, las firmas SHA-1 y las firmas XML con la canonicalización antigua. Sin él, esas firmas quedan sin evaluar y no salen válidas, aunque las hubieras aceptado en tu equipo con la 0.0.126.
- Lo mismo ocurre al cofirmar o contrafirmar sin el documento original y al guardar en un fichero la clave del perfil de protección «alto» cuando el equipo no tiene almacén de secretos. En Linux, sin ajuste, GrxFirma no instala por su cuenta el certificado local al abrir un enlace de firma ni guarda su clave en un fichero si falta ese almacén; la instalación desde el instalador o desde la línea de órdenes sigue disponible.
- Para activar una opción, marca la casilla del aviso y pulsa «Activar y continuar», o hazlo en Configuración (Preferencias en Android), en el apartado de seguridad. La activas bajo tu responsabilidad. Una web no puede activarla por ti, y si tu organización la prohíbe, no se puede activar.
- Cada uso de una opción activada muestra un aviso en el resultado. Se desactiva en el mismo apartado. Si cambia el texto que aceptaste, la opción vuelve a quedar desactivada hasta que aceptes el nuevo.
- Para administradores: la directiva de máquina puede prohibir o fijar cada caso y manda sobre la elección de la persona; si la directiva no se puede leer, el caso queda bloqueado. En la línea de órdenes, `grxfirma -activar-excepcion ID` muestra el riesgo y pide confirmación (en un uso automatizado, con `-acepto-el-aviso`), y `grxfirma -desactivar-excepcion ID` la desactiva. Los identificadores y las directivas están en `docs/seguridad/EXCEPCIONES_INSEGURAS.md`, y la plantilla ADMX incluye las nuevas.
- Los sobres CMS cifrados nuevos protegen la clave con RSA-OAEP y SHA-256, también al firmar y cifrar. Puede que AutoFirma no los abra. Si un destinatario solo abre el formato antiguo, hay que activar su ajuste y pedir ese formato, por ejemplo con `-opcion cms_key_transport=pkcs1v15` en la línea de órdenes.
- Extensión del navegador 1.2.0: sale un solo botón «Firmar PDF» por documento, también cuando el PDF va dentro de varias páginas anidadas. La extensión no pide permisos nuevos.
- La extensión solo muestra el botón si la página principal es de un sitio de confianza; no basta con que lo sea el PDF. Esto puede afectar a portales de terceros que muestran dentro de su página un PDF de dipgra.es o savia.net. El botón tampoco aparece fuera de HTTPS por el puerto 443.
- Accesibilidad en Linux: un ajuste nuevo amplía el texto de la aplicación hasta el doble de su tamaño, y las pantallas se adaptan para que quepa todo. Los diálogos retienen el foco del teclado hasta que se cierran, también con Esc, y los mensajes ya no empiezan con pictogramas.
- Accesibilidad en Windows: el lector de pantalla avisa cada diez segundos de que la firma de un lote sigue en curso, y también cuando termina. Los tiradores del sello son más grandes, y los colores se actualizan al cambiar de tema o activar el alto contraste con la aplicación abierta. Si una operación falla, el diálogo del error trae una ayuda con los pasos siguientes y un botón para comprobar los certificados y el reloj del equipo.
- Accesibilidad en Android: las pantallas de las opciones inseguras tienen encabezados y nombres para TalkBack y funcionan con la letra del sistema al doble de tamaño. Los textos que se pueden seleccionar tienen un área táctil mayor, y el teclado y las barras del sistema ya no tapan el contenido.
- Los instaladores de Windows y Linux están ahora en los 11 idiomas de la aplicación. En Windows, el instalador propone el idioma del sistema y deja elegir otro; en Linux se usa el del sistema, y la variable `GRXFIRMA_INSTALLER_LANG` permite fijar otro. También salen traducidos los mensajes que comprueban las bibliotecas al instalar en Linux desde el tar.gz.
- En Linux, retirar el paquete con apt ya no falla si nunca se llegó a crear el certificado local, por ejemplo porque no se abrió ningún enlace de firma. Además, `grxfirma-afirmauri --help` muestra la ayuda y ya no instala ese certificado.
- En Windows, al desinstalar ya no quedan en el registro claves vacías de la conexión entre la extensión del navegador y GrxFirma.
- Al verificar un PDF firmado, un texto del propio PDF o del certificado que se parezca a la estructura de la firma ya no hace fallar una firma correcta.

## 0.0.127 — 2026-11-14

- Actualización de seguridad: GrxFirma se compila ahora con Go 1.26.11 y con bibliotecas de red más recientes, que corrigen fallos conocidos en las conexiones seguras, en la comprobación de certificados y en el tratamiento de las peticiones web. Conviene instalarla cuanto antes.

## 0.0.126 — 2026-11-14

- Al verificar, ahora se puede aceptar en tu equipo una firma que GrxFirma no da por válida solo por uno de estos motivos: la autoridad del certificado no es conocida, la clave RSA tiene menos de 2048 bits, es un certificado de servidor, no se pudo comprobar la revocación o la firma usa SHA-1. En la aplicación de escritorio aparece el botón «Aceptar en este equipo», que muestra un aviso antes de guardar nada; en la línea de órdenes, `-aceptar-confianza`. Desde entonces esas firmas salen válidas con el aviso «Se da por válida porque lo aceptaste en la configuración». Lo aceptado se quita en Configuración o con `-quitar-confianza-aceptada`. Las webs no pueden aceptar nada por ti, y un administrador puede impedirlo con la política `prohibir_confianza_forzada`.
- Sin conexión a internet, algunas firmas que antes salían válidas ahora salen «no concluyente», porque GrxFirma no puede comprobar si el certificado está revocado. Puedes aceptar ese caso en tu equipo como se explica en el punto anterior.
- Los registros Veri*Factu firmados con SHA-1 se comprueban: si no han cambiado, el aviso dice que la firma usa SHA-1 en lugar de decir que no es del tipo que exige la AEAT. Si aceptas SHA-1 en tu equipo, salen válidos con un aviso.
- Si la CRL que publica la autoridad no cubre el certificado firmante, la firma longeva (LT) se sigue creando con esa CRL y GrxFirma te avisa al firmar. Al verificarla, la revocación sale como no comprobada.
- Al firmar desde una web (afirma://), el selector de certificado ya no muestra «Recordar esta selección». La casilla no hacía nada: en la firma desde una web GrxFirma pide el certificado cada vez, igual que antes.
- GrxFirma vuelve a abrir los archivos cifrados (sobres CMS) con algoritmos antiguos como DES o 3DES. Los abre y avisa de que el cifrado ya no se considera seguro; un administrador puede impedirlo con la directiva `prohibir_cifrado_cms_heredado`. El aviso se ve también en la aplicación, en la línea de órdenes y en la consola REST.
- Los archivos `.afp` que protege esta versión se abren también con las versiones anteriores de GrxFirma. El formato más nuevo sigue disponible si se pide expresamente.
- Al firmar desde una web (afirma://), la ventana de aprobación dice qué web lo pide, qué documentos se firman y a qué servidores irá la firma. Avisa también si la web usa un sistema antiguo que no identifica la sesión, si algún servidor usa una conexión sin cifrar (http) o si el servidor de la web puede pedir varias firmas con una sola confirmación. «Confiar esta vez» vale durante cinco minutos, y si un portal abre varias peticiones a la vez se pregunta una sola vez. El lote que pide una web pasa por el selector de certificado, y el servidor de la web solo puede redirigir a otro servidor autorizado, nunca de https a http.
- Cuando una web pide guardar un fichero, los tipos habituales (PDF, XML, firmas, ZIP o texto) se guardan como siempre. Para los demás, GrxFirma pide confirmación y, si el tipo puede llevar macros o código que se ejecuta al abrirlo (por ejemplo .docm, .xlsm, .iso o .chm), te lo advierte. Los ejecutables (.exe, .bat, .js…) siguen sin guardarse. En Windows, lo guardado queda marcado como descargado de internet.
- Al cofirmar o contrafirmar una firma CAdES que no lleva dentro el documento, la aplicación de escritorio pregunta antes. Puedes elegir el documento original, y GrxFirma comprueba que es el que firmaron los demás, o bien firmar de todos modos o cancelar; la respuesta vale solo para ese intento. En Android, la firma pide el original con «Añadir el documento original» y el lote pide confirmación antes de cofirmar sin ver los documentos. La línea de órdenes firma como antes, con un aviso. Si aportas el original, también se pueden cofirmar firmas antiguas hechas solo con SHA-1.
- Después de firmar, la aplicación de escritorio, Android, la línea de órdenes y la consola REST muestran los avisos de la firma, como el de haber cofirmado sin el documento original. En un lote, cada documento enseña los suyos.
- Ahora se pueden verificar firmas que antes daban error: CAdES separadas sin el documento (se comprueba el resto y se pide el original para el contenido), CMS en codificación BER, XAdES-T con el sello de tiempo en XML, PDF firmados con los formatos antiguos adbe.x509.rsa_sha1 y adbe.pkcs7.sha1, FacturaE 3.0, 3.1 y 3.2, documentos ENI con firma XAdES y firmas XML de versiones anteriores de GrxFirma, que salen como compatibilidad histórica con un aviso. Si una XAdES apunta a documentos de fuera del fichero, esa parte queda sin comprobar, porque GrxFirma no descarga nada.
- Las firmas con SHA-1 se tratan igual en todos los formatos (XAdES, CAdES, PAdES, Office, OpenDocument y FacturaE): si no han cambiado, salen íntegras pero no válidas, con un aviso que da el algoritmo como motivo. El informe HTML para imprimir lo dice igual y sale entero en tu idioma.
- La verificación es más estricta. Una firma XML, de Office o de OpenDocument que cubre solo una parte del documento ya no sale como completa, y se dice qué queda fuera; los metadatos que Office y el ENI dejan fuera de la firma por norma aparecen en un aviso. Un PDF con una firma que GrxFirma no sabe comprobar ya no sale válido. Si un sello de tiempo no se puede comprobar, su fecha aparece como «fecha que declara un sello de tiempo que no se ha podido comprobar» y no salva un certificado caducado.
- Al crear un documento ENI a partir de una firma, GrxFirma comprueba que la firma corresponde al documento y rechaza la que no corresponde. Si corresponde pero usa SHA-1, el documento se crea con un aviso; un administrador puede impedirlo poniendo la directiva `permitir_sha1_legacy` en «Deshabilitada».
- Archivos cifrados: un sobre CMS dirigido también a destinatarios de un tipo que GrxFirma no admite se abre con tu certificado y un aviso. Al desproteger, GrxFirma limpia el nombre que puso el remitente: si la extensión no es de un documento, el fichero se guarda como .bin y, al abrirlo, se abre su carpeta en lugar del fichero. La clave del perfil de protección «alto» pasa al almacén de secretos del sistema la primera vez que se usa. Si el equipo no tiene ese almacén, el perfil alto sigue funcionando con la clave guardada en un fichero de este equipo, y GrxFirma te avisa al abrir un archivo protegido con esa clave.
- Firma con certificado remoto (CSC): el cuadro del PIN dice a qué servicio se envían el PIN y el código, y la línea de órdenes también lo dice. La firma remota solo funciona desde la aplicación que arrancó el motor de firma. Si arrancas el motor a mano con `--server` sin `--ipc-client-pid`, funciona con un aviso y firma con los certificados del equipo, pero sin firma remota. Si el prestador no indica cuándo caduca el acceso, caduca a las 24 horas.
- Línea de órdenes: al verificar, el programa termina con el código 2 si la firma no es válida, con 0 si es válida y con 1 si no se pudo comprobar. Las preguntas de sí o no salen en el idioma elegido y aceptan su respuesta, por ejemplo «o» en francés, «j» en alemán o «b» en euskera. Si el fichero de salida es un enlace simbólico, la orden falla en vez de escribir en el destino del enlace.
- Consola REST: el firmador web pide el token de acceso (Bearer), igual que la consola, y ninguno de los dos muestra ya los campos de ruta de fichero, que el servidor no aceptaba. El inicio de sesión con certificado rechaza las peticiones que llegan de otras webs. El certificado local del servidor se renueva solo antes de caducar, y las respuestas de firma llevan los avisos.
- Para administradores: la directiva nueva `firma_remota_csc_servicios_permitidos` limita los prestadores de firma remota a los que se puede conectar. Las webs ya no pueden usar una autoridad de sellado de tiempo (TSA) de la red interna salvo que figure en la directiva nueva `tsa_permitidas`; en la consola REST, si la lista existe, solo valen las que figuran en ella. Las dos están en la plantilla ADMX. `aprobacion_automatica_nativehost` vale ya solo para la extensión oficial. Un ajuste mal escrito en la política de máquina solo anula ese ajuste y, si es de los que limitan (REST, WebSocket o confianza al primer uso), lo deja desactivado. Si el fichero de política existe pero no se puede leer, se desactivan REST, WebSocket y la firma remota.
- Al instalar en Windows o en Linux, GrxFirma avisa de que los enlaces afirma:// se abrirán con GrxFirma en lugar de AutoFirma, salvo que en Configuración elijas AutoFirma. En Linux, al desinstalar vuelve el programa que abría esos enlaces antes, si no lo has cambiado tú. En macOS, el desinstalador quita del llavero el certificado local de GrxFirma.
- Si tienes AutoFirma instalado, en Configuración, «Programa para firmar en las webs», eliges con qué programa firmar cuando una web lo pide: GrxFirma, AutoFirma o «Preguntar cada vez». Con esta última, GrxFirma te pregunta en cada firma y puede recordar tu respuesta. El instalador pregunta lo mismo si encuentra AutoFirma, y la instalación silenciosa mantiene lo que había. Los administradores pueden fijarlo con `/AFIRMA=` en Windows, `--afirma-handler=` en Linux o la directiva `manejador_afirma`. GrxFirma encuentra ahora AutoFirma aunque se instalara solo para tu usuario o con otro nombre de lanzador; antes, en esos casos, el ajuste decía que AutoFirma no estaba instalado.
- Más seguridad: la aplicación de escritorio Qt muestra como texto, sin interpretar marcado, los nombres de documentos, certificados y servidores, y el motor de firma que arranca se cierra con ella aunque la aplicación termine de forma inesperada. En Android, un documento dañado ya no cierra la aplicación: sale un error. La línea de órdenes, la consola REST y la extensión guardan ahora el registro de auditoría, que conserva 90 días.
- En Android, al seleccionar un documento se ven los primeros caracteres de su huella SHA-256. Si la app de la que viene lo cambia antes de firmar, GrxFirma no lo firma y te pide que lo vuelvas a seleccionar. En un lote, el fichero que ha cambiado sale como no firmado y el resto se firma.
- Si el nombre de un certificado o de su emisor lleva caracteres invisibles que cambian cómo se lee, como los que invierten el orden del texto, la aplicación de escritorio y la línea de órdenes los muestran como «?».
- En Windows, la instalación y las actualizaciones guardan los programas en `%LOCALAPPDATA%\Programs\GrxFirma`. Los datos de uso y la configuración permanecen en sus carpetas de usuario al desinstalar.

## 0.0.125 — 2026-10-07

- Accesibilidad revisada según WCAG 2.1 AA en las tres plataformas: mejor contraste, foco visible y orden de tabulación, avisos para lectores de pantalla y errores señalados en su campo.
- Linux: un selector en Configuración permite elegir si las firmas de los portales las atiende GrxFirma o AutoFirma, como en Windows; si AutoFirma no está instalado, se ofrece su web de descarga.
- Valenciano revisado según la normativa de la AVL, y la extensión de navegador muestra un solo botón «Firmar PDF» cuando el PDF va dentro de otra página.

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
