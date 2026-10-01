<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Frontend nativo WinUI 3

Frontend nativo de GrxFirma para Windows y opción recomendada en la Suite.
Convive con la GUI Qt/QML, que se mantiene como alternativa opcional y
soportada sobre el mismo backend Go. Esta preferencia de interfaz no declara
por sí sola una release terminada ni cierra los gates propios de Qt.

## Alcance actual

- WinUI 3 `unpackaged`, `self-contained` y `x64`.
- `.NET 10`, Windows 10 1809 (`10.0.17763.0`) o posterior.
- Windows App SDK `2.3.1`.
- `NavigationView` adaptativa con páginas iniciales de Firma, Verificación,
  Huellas, Protección, Certificados, Configuración, Diagnóstico y Ayuda.
- recursos de tema claro, oscuro y alto contraste;
- nombres accesibles, mensajes mediante `InfoBar` y estado expresado mediante
  icono y texto, no solo color;
- MVVM mínimo sin incorporar lógica criptográfica a la interfaz;
- cliente genérico IPC NDJSON en el proyecto `Core`;
- diálogo de diagnóstico de una operación con causa sencilla, responsable,
  acción sugerida, pasos observados y detalle técnico plegado.

Cada página habilita únicamente las operaciones que el motor anuncia tras el
saludo IPC autenticado. Firma, cofirma y contrafirma simples, verificación,
huellas, protección, configuración, diagnóstico y consulta de certificados usan
ya el backend real; ningún botón devuelve resultados ficticios. La cofirma
múltiple guiada usa `sign_multicosign` para `PAdES`, `ODF` y `OOXML`, exige al
menos un certificado adicional distinto y no se mezcla con contrafirma. La
firma por lotes usa `sign_batch`: admite varios ficheros y una carpeta de
primer nivel, una carpeta de salida explícita y un resumen por documento que
conserva los resultados parciales confirmados. En Certificados también se puede
comprobar online la revocación del elemento seleccionado y abrir el gestor de
Windows descubierto por la lista cerrada del backend. La misma página permite
establecer y quitar el certificado predeterminado mediante el snapshot tipado
de configuración compartida.

El lote aplica límites locales antes de enviar la petición: hasta 128
documentos, 100 MB por documento y 256 MB en conjunto. La selección de
carpeta no es recursiva. Al cancelar, los elementos pendientes quedan marcados
como cancelados y solo se presentan como correctas las salidas que el backend
haya confirmado; una cancelación o una respuesta incoherente no genera falsos
éxitos.

La página de Firma conecta además el sello PDF visible con preview real:
página, lista/rango o todas las páginas; posición y tamaño normalizados;
rotaciones de 0, 90, 180 o 270 grados; texto sobre imagen y metadatos
opcionales. El usuario puede mover la zona arrastrándola sobre la página y
redimensionarla desde el tirador inferior derecho. El código transforma los
desplazamientos del `Viewbox` a porcentajes normalizados, limita toda la
geometría al interior de la página y mantiene los `NumberBox`
X/Y/ancho/alto como modelo autoritativo y alternativa accesible mediante
teclado. Después de guardar, solicita al backend la verificación de la firma
cuando este anuncia esa capacidad y distingue el resultado de firma del
resultado de validación posterior.

En Windows la previsualización usa primero `Windows.Data.Pdf.PdfDocument` y
renderiza la página a PNG dentro del propio proceso WinUI. Así, la instalación
autocontenida no depende de `pdfinfo`, `pdftoppm` ni Poppler. La ruta nativa
rechaza puntos de reanálisis, limita el PDF de entrada a 100 MiB, acota la
imagen resultante y devuelve las dimensiones reales de `MediaBox`. El cliente
IPC de preview del backend se conserva como alternativa para otros hosts y
pruebas de contrato.

Protección admite `EncryptedData` con una clave AES-256 aleatoria expresada en
Base64 canónico. La captura y la confirmación se realizan mediante diálogos
Win32 nativos: el secreto no se convierte en `string` administrado, pasa a
buffers borrables, se envía como `secretB64` binario y las copias controladas se
sobrescriben al salir del flujo. La clave no se persiste ni se registra y no es
una contraseña derivada mediante KDF.

Certificados aplica el mismo principio a P12/PFX. El usuario puede cargar la
credencial solo para la sesión —opción recomendada— o elegir uno de los
destinos persistentes tipados que anuncia el backend. La contraseña se captura
con el diálogo Win32 nativo sin materializarse como `string`. Si Windows no
puede crear esa plantilla, el cliente usa como respaldo el diálogo CredUI del
sistema en modo de solo contraseña, siempre visible y no persistente. CredUI
limita esta ruta de respaldo a 256 caracteres. En ambos casos el secreto pasa
directamente a un buffer nativo borrable; fichero y contraseña viajan como
`byte[]` en `credentialB64` y `passwordB64`. El cliente rechaza credenciales
vacías o mayores de 2 MiB y contraseñas mayores de 4 KiB, y sobrescribe las
copias controladas después de cada intento.

Las credenciales temporales pueden retirarse individualmente o limpiarse en
bloque. Ambas acciones requieren confirmación visible y solo actualizan la
interfaz cuando el backend confirma el resultado. Su inventario vive en la
sesión IPC compartida: navegar a otra pantalla no convierte una credencial
temporal activa en una credencial invisible o imposible de retirar.

Configuración permite crear, rotar y retirar credenciales de proxy mediante
las operaciones tipadas del almacén seguro. Usuario y contraseña solo se
mantienen durante la captura y la petición; la contraseña usa diálogo Win32 y
buffers borrables, y `settings.json` conserva únicamente la referencia opaca y
el realm no sensible. En Windows el backend usa DPAPI ligado al usuario y
publica el blob limitado de forma atómica con DACL privada. La lectura y el
borrado no siguen enlaces ni puntos de reanálisis. No hay `PasswordBox`,
portapapeles, argumentos ni logging del secreto.

Diagnóstico ejecuta `ping` e inventarios locales tipados, incluida la
disponibilidad del almacén seguro de proxy. La sección TLS consulta únicamente
estado y recuentos saneados. Cuando el backend Windows anuncia las acciones
gestionadas, ofrece dos gestos separados y confirmados: instalar la confianza
local de GrxFirma y retirar solo esa confianza y sus artefactos
inventariados. La reparación es reversible y no enumera ni elimina
certificados ajenos. En otros sistemas esas acciones no se anuncian. El
asistente no crea una conexión ficticia con la sede ni con `@firma`.

## Proyectos

```text
GrxFirma.WinUI.sln
src/
  GrxFirma.WinUI.Core/  DTO, presentación de diagnósticos y cliente IPC
  GrxFirma.WinUI/       aplicación WinUI 3
tests/
  GrxFirma.WinUI.Core.Tests/
```

El ensamblado publicable se llama `grxfirma-winui.exe`, como espera el
lanzador Go.

## Contrato de arranque

El lanzador entrega los dos parámetros como una pareja inseparable:

```text
--ipc-socket \\.\pipe\PIPE_ASSIGNED_BY_LAUNCHER
--backend-pid 1234
```

Los valores anteriores son marcadores, no una ruta ni un PID de producción.
La aplicación también puede abrirse sin ambos argumentos para revisar la
interfaz; en ese caso todas las operaciones permanecen desconectadas.

Cuando existe endpoint, el orden es:

1. validar que la ruta representa un named pipe local;
2. conectar con tiempo límite;
3. consultar mediante `GetNamedPipeServerProcessId` el PID propietario del
   handle conectado;
4. comparar con `--backend-pid` y fallar cerrado si no coincide;
5. enviar `hello` con `protocol=desktop-ipc-v1`;
6. verificar protocolo, correlación, acciones y límites anunciados antes de
   devolver el cliente a la interfaz.

No se acepta una ruta IPC sin PID esperado. No se reflejan rutas ni valores de
argumentos en los mensajes de error.

## Seguridad del cliente IPC

- `requestId` y `traceId` usan 128 bits independientes de
  `RandomNumberGenerator`;
- una única petición queda en vuelo por conexión;
- cada respuesta debe repetir protocolo, acción, `requestId` y `traceId`;
- el lector NDJSON aplica UTF-8 estricto y un máximo absoluto de 4 MiB;
- el límite de petición se reduce al anunciado por `hello`;
- los eventos solo contienen acción, correlación, resultado y código estable;
- nunca reciben `params`, `data`, el error bruto del backend, rutas o secretos;
- los frames de petición se limpian después de enviarse;
- los secretos de `EncryptedData` se capturan fuera de los controles XAML
  mediante diálogo Win32 nativo, se codifican sin crear texto administrado y
  se transportan como `byte[]`/`secretB64`;
- credencial y contraseña P12 se mantienen en buffers borrables y se
  transportan como `byte[]`/`credentialB64` y `byte[]`/`passwordB64`;
- la contraseña del proxy se captura mediante diálogo Win32 nativo, viaja como
  `byte[]` y se borra tras crear o rotar la entrada del almacén seguro;
- los diagnósticos de proxy/TLS muestran disponibilidad, estados y recuentos,
  no credenciales, huellas, rutas ni nombres de artefactos;
- mensajes y diagnósticos se limitan y eliminan caracteres de control;
- estados desconocidos del diagnóstico se conservan como «No comprobado».
- el preview visible solo acepta PNG real, dimensiones positivas y límites
  anunciados por el backend;
- una guarda de fichero y comparaciones SHA-256 en tiempo constante detectan
  cambios del PDF antes y después del preview y de la firma;
- los buffers temporales del preview, hashes y peticiones sensibles se limpian
  cuando dejan de ser necesarios.

El detalle técnico se muestra únicamente en el `Expander` secundario del
diálogo y no se escribe en los eventos del cliente.

## Compilar y probar en Windows

El repositorio fija el SDK mediante `global.json`. Desde una terminal de
desarrollo de Windows:

```powershell
dotnet restore .\cmd\gui-winui\src\GrxFirma.WinUI\GrxFirma.WinUI.csproj --use-lock-file
dotnet build .\cmd\gui-winui\GrxFirma.WinUI.sln -c Debug -p:Platform=x64
dotnet test .\cmd\gui-winui\tests\GrxFirma.WinUI.Core.Tests\GrxFirma.WinUI.Core.Tests.csproj -c Debug
```

Los tres `packages.lock.json` están versionados. Las restauraciones de CI y
empaquetado deben usar siempre `--locked-mode`.

La publicación técnica aislada se realiza mediante la infraestructura de
`packaging/windows/build-desktop-winui.ps1`.

## Pruebas incluidas

Las pruebas de `Core` cubren:

- saludo versionado y correlación;
- rechazo de respuestas con ID diferente;
- ausencia de parámetros, datos y error bruto en eventos;
- generación de IDs no repetidos;
- validación de argumentos y nombres de pipe local;
- mapeo de responsable a equipo, navegador, portal, @firma o desconocido;
- representación accesible de `success`, `failure`, `skipped` y `unknown`;
- estado desconocido futuro, fallo sin pasos y saneamiento de texto.

La navegación principal ofrece teclas de acceso únicas para sus nueve
destinos. Tras navegar, la ventana mueve el foco programático a la primera
acción disponible de la página cargada, evitando que teclado y lector de
pantalla permanezcan en el elemento de navegación anterior. El tema de alto
contraste usa `SystemColorWindowColor` y `SystemColorWindowTextColor`, por lo
que respeta la paleta de contraste elegida por el usuario en vez de imponer
negro y blanco.

Los contratos Python de `cmd/gui-winui/tests` cubren además la conexión de las
páginas, la configuración tipada, el sello visible, su edición por puntero con
límites normalizados, las guardas anti-TOCTOU y la validación posterior. Los
contratos posteriores cubren también la cofirma
múltiple, la captura/borrado del secreto binario de `EncryptedData`, la firma
por lotes y las credenciales temporales/persistentes. En `894c751`, con el
backend binario de `5db7991`, se verificaron:

- 98 pruebas de contrato Python;
- 68 pruebas de `GrxFirma.WinUI.Core.Tests` en Windows 10;
- publicación `self-contained`, compilación del backend Go y
  generación/validación de la stage y el ZIP técnico de WinUI.

Estas pruebas verifican el código y el artefacto técnico. Como evidencia
histórica separada, en el corte `05c5411` se ejecutaron:

- 82 pruebas de contrato Python, todas correctas;
- 60 pruebas de `GrxFirma.WinUI.Core.Tests`, todas correctas;
- restauración bloqueada y compilación x64 Debug de la solución en Windows 10
  Pro 22H2, con cero advertencias y cero errores.

El 29-07-2026 el estado exacto posterior se publicó con .NET SDK `10.0.302`,
superó `100/100` pruebas Core y se integró en una Suite NSIS instalada sobre
Windows 10 19045. La ejecución
`20260729T034537032Z-a73efbdd` cargó el P12 oficial de pruebas de la FNMT,
previsualizó el PDF sin Poppler, cambió posición y tamaño, aplicó rotación de
90 grados, firmó el PAdES y verificó de nuevo su integridad. `pdfsig` confirmó
la firma y que cubre el documento completo. La confianza de la cadena QA no se
presenta como confianza de producción.

Este recorrido cierra la instalación y el sello PAdES de Windows, pero no
sustituye la validación manual completa de accesibilidad, la matriz de
hardware/certificados admitidos, Authenticode/SmartScreen, los portales
externos ni el envío real a FACe.

En Windows, el backend no anuncia `service_status`, `service_install`,
`service_start`, `service_stop` ni `service_uninstall`: esas acciones no tienen
un adaptador implementado en esta plataforma y no forman parte del alcance
operativo actual de WinUI.

El cierre de distribución sigue separado: Authenticode/timestamp, el XPI
Firefox firmado, el canal gestionado de Chrome/Edge, la accesibilidad manual y
las pruebas de portales no se satisfacen con la instalación y firma local
correctas.
