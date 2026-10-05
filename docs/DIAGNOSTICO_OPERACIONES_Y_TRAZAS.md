<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Diagnóstico de Operaciones y Trazas

Estado: documento operativo de producto y soporte.

## Objetivo

Separar claramente dos capas:

1. `Diagnóstico para usuario`
   - lenguaje sencillo
   - pasos visibles de la operación
   - causa probable entendible
   - indicación de responsabilidad:
     - `debe revisarlo el usuario`
     - `podemos corregirlo nosotros`
     - `depende de un servicio externo`

2. `Traza técnica para soporte`
   - eventos finos
   - `requestId` y `traceId` correlados cuando existan
   - request/response IPC
   - estados de reconexión
   - clasificación orientativa del origen del fallo

La app no debe obligar al usuario medio a interpretar logs crudos.

## Estado actual

Ya existe una base útil:

- log persistente de GUI Qt/QML:
  - `~/.local/share/Diputacion de Granada/GrxFirma/logs/gui-qml.log`
- rotación básica del log
- `modo experto` persistente como preferencia de usuario en Qt/QML
- trazas finas de:
  - arranque
  - cambio de idioma
  - guardado de preferencias
  - conexión IPC
  - petición/respuesta IPC
  - acciones diferidas durante reconexión
- correlación por operación en IPC mediante `requestId`
- `traceId` visible en el flujo IPC Qt/QML
- clasificación orientativa cerrada del origen del fallo:
  - `app_local`
  - `certificate_store`
  - `network_proxy`
  - `local_web_service`
  - `remote_service`
  - `government_afirma`
  - `unknown`

Además, en Qt/QML, `/signer` y `grxfirmauri`:

- la validación ya no falla inmediatamente si el motor IPC está reconectando;
  ahora puede diferirse y reintentarse;
- el cambio de idioma ya no muta el locale en el mismo ciclo del `ComboBox`,
  reduciendo el riesgo de reentrancia/crash.
- Qt/QML ya prepara una incidencia privada y saneada desde el asistente
  usuario/experto, con JSON, resumen TXT, previsualización, manifiesto y cola de
  log acotada;
- `/signer` ya permite preparar, copiar y descargar un informe alineado con su
  estado visible y con `GET /diagnostics/report`; además conserva el
  diagnóstico guiado de los errores REST, localiza su categoría y lo incorpora
  saneado al informe usuario/experto;
- `grxfirmauri` ya mantiene fases estables en los modos directo, WebSocket y
  service y, cuando falla, persiste una incidencia protocolaria JSON/TXT y una
  cola de log saneada;
- Qt/QML ya ofrece un envío remoto opt-in solo desde una incidencia de fallo
  previamente guardada. Muestra destino, contenido incluido y omitido, exige
  consentimiento exacto en C++ y rechaza destinos no HTTPS, credenciales en
  URL, fragmentos, redirecciones, errores TLS y respuestas no `2xx`. La capa
  C++ carga el JSON local por ruta canónica y lista positiva; no confía en un
  payload arbitrario preparado por QML.
- Qt/QML ofrece además `Diagnosticar ahora` solo después de un fallo
  registrado. El gesto exige consentimiento, ejecuta sondas acotadas contra el
  motor local y añade el resultado saneado como bloque separado de una nueva
  incidencia correlada.

La interfaz WinUI añade otra superficie, con una frontera deliberadamente
distinta:

- las operaciones conectadas solicitan un diálogo visual cuando reciben un
  fallo tipado;
- el backend IPC genera una cronología únicamente con fases observadas:
  - un fallo de admisión muestra solo `Admisión de la petición`;
  - un resultado de protocolo muestra admisión y protocolo;
  - un resultado de operación muestra admisión, protocolo y operación;
  - una fase desconocida muestra solo `Resultado observado`;
- una fase anterior solo se marca correcta cuando alcanzar la siguiente
  demuestra que terminó; no se inventan fases posteriores ni se asigna dueño o
  acción a pasos anteriores sin evidencia;
- los estados se expresan con icono y texto:
  - `✓ Correcto`;
  - `✕ Falló`;
  - `— Omitido`;
  - `? No comprobado`;
- la sección `Diagnóstico` puede ejecutar, sin repetir una firma:
  - saludo autenticado ya observado y `ping` real al motor;
  - conteos de certificados utilizables;
  - conteos de gestores y destinos de importación;
  - estado y recuentos de artefactos, certificados y claves del almacén TLS
    local mediante el objeto estructurado `tlsStore`;
  - estado del almacén seguro del proxy;
  - lectura UTC del reloj local y, en Windows 10, estado del servicio
    `W32Time` mediante la API del gestor de servicios, sin ejecutar una shell;
- esas consultas son acotadas y de solo lectura. Se pueden cancelar y se evita
  ejecutar dos diagnósticos simultáneos;
- la lectura local y que `W32Time` esté en ejecución no certifican que el reloj
  esté sincronizado. La interfaz conserva ese estado como `No comprobado`
  mientras no exista una referencia remota autorizada;
- el portal o servidor remoto y `@firma` permanecen como `No comprobado` hasta
  que una operación real aporte evidencia. Una cabecera HTTP de una sede nunca
  se convierte por descarte en evidencia de `@firma`;
- la vista no muestra sujetos, identificadores, huellas, rutas, credenciales,
  certificados completos, payloads ni errores crudos;
- los diagnósticos TLS de REST e IPC tampoco enumeran el directorio ni los
  nombres de sus certificados o claves: solo publican estado y recuentos, de
  forma que copiar un resumen para soporte no revele el perfil local;
- el conteo «utilizable para firmar» exige certificado vigente y una clave
  resoluble, no equivale al total del catálogo;
- instalar o limpiar confianza desde IPC se limita a la CA local inventariada
  por GrxFirma. La limpieza retira primero su confianza del sistema y no
  borra otros `.pem`/`.crt` que puedan coexistir;
- `Ayuda` abre una guía local incluida en el paquete y únicamente destinos
  oficiales fijos. Ninguna URL o ruta recibida por IPC se usa como destino.

Los contratos automatizados de esta superficie no sustituyen su revisión
visual, accesible ni instalada en Windows 10.

### Comparación segura de fecha y hora

La acción IPC `clock_diagnostics` no recibe URL, host ni endpoint. El backend
solo puede consultar una referencia remota cuando una fuente interna le entregue
un origen HTTPS previamente observado en una operación real y autorizado por la
política de confianza. En la arquitectura actual esa fuente compartida todavía
no existe entre `grxfirmauri` y el backend WinUI, por lo que la compilación de
producto muestra honestamente la fase remota como `No comprobado`.

La sonda preparada para integrar esa fuente aplica estas restricciones:

- usa únicamente el origen (`https://host/`), sin ruta, consulta, fragmento,
  usuario ni contraseña;
- acepta solo el puerto HTTPS 443 y rechaza direcciones loopback, privadas,
  locales, multicast o no enrutable, incluida la resolución DNS final;
- no usa proxy de entorno, cookies, `Authorization`, `Origin` ni `Referer`;
- valida TLS con las raíces del sistema, exige TLS 1.2 como mínimo y no sigue
  redirecciones;
- emplea `HEAD`, limita las cabeceras a 32 KiB, lee como máximo un byte del
  cuerpo y aplica un timeout total de cuatro segundos;
- calcula el desfase contra `Date` usando el punto medio entre el envío y la
  recepción para compensar la latencia de ida y vuelta;
- considera compatible un desfase absoluto de hasta cinco segundos. El umbral
  cubre la resolución de un segundo de HTTP `Date` y variación razonable de
  red;
- si se supera el umbral, muestra el desfase pero deja el responsable como
  desconocido: una sola comparación no demuestra si está mal el reloj local o
  el remoto.

No se debe habilitar la sonda remota pasando una URL desde WinUI o IPC. Antes de habilitarla
hace falta un registro efímero y autenticado de orígenes realmente
observados y autorizados, compartido por los procesos sin persistir cookies,
credenciales ni rutas sensibles.

## Modelo objetivo

El modelo objetivo común para las superficies que dispongan de evidencia es:

### Durante la operación

La app debe mostrar pasos como:

- `Preparando operación`
- `Comprobando certificado`
- `Conectando con el motor de firma`
- `Contactando con el servidor remoto`
- `Esperando respuesta del servicio de firma`
- `Aplicando sello de tiempo`
- `Verificando resultado`

### En caso de error

La app debe explicar:

- qué ha fallado;
- dónde parece fallar;
- si el usuario puede hacer algo;
- y si el problema es externo.

Ejemplos esperables:

- `No se puede usar este certificado para firmar. Revisa que siga vigente y que tenga clave privada accesible en este equipo.`
- `La app local no ha podido conectar con el motor de firma. Intenta cerrar y abrir GrxFirma.`
- `La web remota no ha respondido correctamente durante la firma. El problema parece estar en el servicio externo.`
- `El servidor de sello de tiempo no responde o está desalineado con la hora del equipo.`
- `La validación remota no ha podido completarse porque el servicio externo de @firma no responde.`

## Modo usuario frente a modo experto

### Modo usuario

Debe priorizar:

- mensajes cortos;
- pasos visibles;
- causa probable en lenguaje simple;
- responsabilidad;
- siguiente acción recomendada.

Y debe evitar, salvo ampliación explícita:

- jerga como `IPC`, `payload`, `backend`, `trace`, `socket`;
- identificadores técnicos sin contexto;
- mezclar en el mismo bloque el resumen amigable y el detalle experto.

No debe mostrar por defecto:

- payloads JSON;
- rutas internas;
- estados de socket;
- nombres de acciones IPC;
- trazas crudas del bridge.

### Modo experto

Debe permitir además:

- traza técnica saneada y acotada;
- `requestId`;
- acción/protocolo;
- tamaño, tipo y estado de la petición sin su payload;
- categoría, código y tiempos de la respuesta sin cuerpo crudo;
- clasificación técnica;
- detalles mínimos de proxy/TLS/TSA/OCSP/CRL/trifásico si existen, sin
  credenciales, URLs sensibles, certificados completos ni documentos.

El modo experto amplía contexto, no la frontera de privacidad: nunca habilita
payloads, secretos, documentos, rutas completas ni respuestas crudas.

## Modelo recomendado de diagnóstico

Cada operación visible debería acumular una estructura similar a:

- `operation_id`
- `operation_kind`
  - firmar
  - cofirmar
  - contrafirmar
  - validar
  - huella
- `step`
- `step_history`
- `technical_events`
- `user_summary`
- `technical_summary`
- `failure_domain`
  - `local_app`
  - `certificate_or_store`
  - `network_or_proxy`
  - `local_web_service`
  - `remote_service`
  - `government_afirma`
  - `unknown`
- `responsibility`
  - `user_actionable`
  - `our_product`
  - `external_service`
  - `unknown`

## Alcance prioritario

Primero en `Linux/Windows`:

1. WinUI y Qt/QML desktop
2. web local `/` y `/signer`
3. firma/validación remota y trifásica
4. exportación de incidencia en las superficies que la ofrecen, con:
   - resumen para soporte
   - `requestId`
   - `traceId` cuando exista
   - últimos pasos
   - log acotado

## Política para soporte remoto

Si una incidencia local puede enviarse a soporte remoto, la regla de producto
debe mantenerse simple y verificable:

- nunca hay envío automático;
- siempre hay previsualización explícita;
- siempre hay consentimiento explícito del usuario;
- el contenido sale ya saneado y minimizado;
- el transporte debe ir por `TLS/HTTPS` con validación estricta;
- y no se añade, por ahora, una capa extra de cifrado del paquete.

La decisión actual no es “enviar artefactos completos porque ya hay TLS”, sino
la contraria: enviar menos, mejor saneado y solo cuando el usuario lo apruebe.

## Diagnóstico activo posterior al fallo

La implementación Qt mantiene una frontera distinta del envío remoto:

- no se ejecuta ninguna sonda durante una operación correcta;
- `Diagnosticar ahora` solo aparece mientras existe un fallo registrado;
- el diálogo explica alcance, datos omitidos y exige una casilla de
  consentimiento;
- el runner solo admite `127.0.0.1` o `localhost`, resuelto de nuevo a
  loopback antes de conectar;
- observa la configuración de proxy y, si procede, prueba DNS, TCP y TLS con
  timeouts separados;
- el TLS local se acepta por validación normal o por coincidencia exacta con
  el pin SHA-256 del certificado esperado;
- cancelar o registrar un fallo nuevo invalida sockets, lookup y resultado
  anterior, sin señales tardías;
- el resultado usa códigos cerrados de causa, responsabilidad y acción; no
  incluye URL, host, puerto, IP, credenciales, certificado ni error crudo.

No se sondean destinos remotos configurables por el usuario. Portal,
`@firma`, trifásico y TSA solo podrán incorporarse mediante un catálogo
cerrado ligado al contexto real de la operación. Es una decisión de seguridad
para evitar SSRF y diagnósticos engañosos.

La sección `Diagnóstico` de WinUI no está limitada a un fallo anterior ni pide
consentimiento de red porque no abre conexiones remotas: ejecuta solamente las
acciones IPC locales y tipadas enumeradas en `Estado actual`. Tampoco reutiliza
el exportador o el envío remoto de Qt. Si en el futuro incorpora red, deberá
adoptar consentimiento, catálogo cerrado y correlación con la operación antes
de ejecutarla.

## Capa crítica adicional en flujo protocolario

El frente `grxfirmauri` no puede quedarse en “log de debug y ya”.

En la práctica de producto, cuando falla una firma en `VALIDe` o en otra sede,
el usuario y soporte necesitan saber si el problema parece estar en:

- la app local;
- el certificado o el almacén;
- la red, el proxy o la confianza TLS local;
- el navegador u origen de la petición;
- el portal o servicio remoto del gobierno;
- o si no hay evidencia suficiente todavía.

Para eso, el flujo `afirma://` mantiene una `sesión diagnóstica` por
invocación, con:

- fase visible actual;
- última fase completada;
- últimos eventos redactados;
- clasificación guiada del fallo;
- y una incidencia exportable propia del flujo protocolario.

### Fases mínimas recomendadas

- `startup`
- `config_load`
- `launch_parse`
- `runtime_bootstrap`
- `channel_prepare`
- `waiting_browser`
- `browser_connected`
- `document_pick`
- `certificate_catalog_load`
- `certificate_select`
- `sign_execute`
- `save_execute`
- `session_complete`

### Artefacto mínimo de incidencia protocolaria

Ruta utilizada en Linux:

- `~/.local/state/grxfirma/incidents/`

Formatos:

- `JSON` estructurado para soporte
- `TXT` legible para revisión rápida

Contenido mínimo:

- versión y SO
- modo `directo/websocket/service`
- `origin` cuando exista
- `sessionId` truncado
- tipo de operación
- fase actual y última completada
- clasificación del fallo
- responsable probable
- acción sugerida
- últimos eventos redactados

Este modelo sustituye la necesidad de exponer un log crudo: conserva únicamente
la información saneada necesaria para reproducir, clasificar o escalar.

## Envío remoto a soporte

La incidencia local ya puede usar `Enviar incidencia`, con límites estrictos:

- nunca automático;
- solo desde un JSON de fallo ya guardado por la aplicación en su directorio
  privado;
- siempre con consentimiento explícito;
- siempre con vista previa;
- y siempre con saneado previo.

El usuario debe ver:

- qué se va a enviar;
- a qué destino;
- qué datos se omiten;
- y poder cancelar.

El artefacto remoto no debe incluir:

- claves;
- secretos;
- `dat=` o blobs completos;
- documentos;
- rutas completas del perfil del usuario;
- ni payloads íntegros.

Debe incluir, como máximo:

- incidencia estructurada;
- log acotado y redactado;
- clasificación del fallo;
- pasos recientes;
- versión, SO y contexto técnico mínimo útil.

Después de cada intento, la incidencia local se reemplaza atómicamente
añadiendo solo la fecha UTC y `sent`/`failed`. No se persisten destino, IP,
respuesta, cuerpo remoto ni texto de error.

### Canal de envío

Para esta primera fase:

- `TLS/HTTPS` es obligatorio;
- el envío debe rechazar `HTTP` o canales inseguros;
- la validación del certificado del servidor debe ser estricta;
- no se añade todavía cifrado adicional del paquete con clave pública.

La razón es pragmática:

- el control principal aquí es `minimización + saneado + consentimiento + TLS`;
- el cifrado adicional de paquete aporta endurecimiento, pero también
  introduce rotación, despliegue y soporte de claves;
- no debe bloquear el primer flujo útil y seguro de soporte remoto.

## Asistente guiado de ayuda

Además del diagnóstico visible, el producto encaja bien con un asistente simple
de ayuda operativa.

No debe ser un chat genérico. Debe ser un asistente acotado a:

- `quiero firmar`
- `quiero validar`
- `no encuentro mi certificado`
- `ha fallado la firma`
- `ha fallado la validación`
- `quiero preparar una incidencia para soporte`

### Qué debe hacer

- leer el contexto real de la operación actual;
- detectar el paso en el que está el usuario;
- proponer acciones concretas;
  - resumir el fallo en lenguaje sencillo;
  - en `modo experto`, permitir ampliar:
    - `requestId`
    - `traceId`
    - últimos pasos
    - clasificación del fallo
    - exportación de incidencia

### Qué no debe hacer

- no inventar causas sin evidencia;
- no abrir conversación generalista;
- no reemplazar el log técnico;
- no ocultar incertidumbre cuando no se sepa la causa.

### Forma recomendada

Un panel o diálogo guiado con árbol de decisión:

- `Qué quieres hacer`
- `En qué paso estás`
- `Qué ha fallado`
- `Qué puedes probar ahora`
- `Si sigue fallando, preparar incidencia`

Eso mantiene la app utilizable por usuario medio y aprovecha la infraestructura
de diagnóstico guiado, trazas correlacionadas y exportación de incidencias.

El asistente guiado de Qt está implantado sin chat libre y con entradas
cerradas, entre ellas:

- `Quiero firmar`
- `Quiero validar`
- `Ha fallado`

Cada una debe abrir sobre contexto real ya conocido por la app y no pedir al
usuario que vuelva a describir la operación actual.

WinUI ofrece por ahora una guía local, recomendaciones breves y accesos fijos a
manual, carpeta, proyecto y contacto privado. Es un centro de ayuda seguro,
pero todavía no es el asistente contextual completo descrito en este apartado.

## Criterios de calidad

El diagnóstico cumple su función cuando:

- un usuario no experto pueda entender por qué ha fallado una firma o validación;
- la app indique si el problema es nuestro, del certificado, del equipo, de la
  red o de un servicio externo cuando haya evidencia, y muestre
  `No comprobado` cuando no la haya;
- soporte pueda correlacionar la incidencia completa con un `requestId`;
- y el modo experto siga permitiendo diagnóstico fino sin contaminar la experiencia básica.
