<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

Las pantallas ENI de Qt y WinUI usan los códigos del motor Go, muestran sus
descripciones traducidas y permiten elegir las fechas mediante calendario.
La generación comprueba el XML antes de entregarlo. La operación
`-operacion validar-eni -entrada <fichero.xml>` y la acción IPC `validate_eni`
informan de los problemas de estructura y valores.

Los catálogos canónicos están en `internal/adapters/outbound/common/eni/catalogos.go`.
`go generate ./internal/adapters/outbound/common/eni` regenera sus proyecciones
C# y JavaScript. Los contratos Python de las dos interfaces comprueban que esas
proyecciones coinciden exactamente con Go y que las pantallas las utilizan.
Las descripciones están al final de los once catálogos con claves `eni.codigo.*`.
Las coincidencias lingüísticas están justificadas individualmente en
`internal/adapters/outbound/common/localizador/testdata/eni_coincidencias.md`.

El identificador usa `ES_<DIR3>_<AAAA>_<id>`; su último tramo admite de 1 a 30
caracteres. El expediente generado conserva `EXP_`, seguido de 26 caracteres,
para respetar ese límite. DIR3 exige una letra mayúscula y ocho cifras. La
clasificación admite un código SIA numérico o `<DIR3>_PRO_<id>`. Las copias
EE02, EE03 y EE04 necesitan un identificador ENI del documento de origen.

La validación nativa comprueba el perfil ENI 1.0 que genera el motor: espacios
de nombres, secuencias, cardinalidades, metadatos, contenido binario, índice y
envolturas de firma. No comprueba la integridad criptográfica de XMLDSig ni
sustituye la validación con los XSD oficiales. Rechaza DTD, referencias externas,
atributos inesperados y referencias a identificadores ausentes. Admite un BOM
UTF-8. Limita el XML a unos 137 MiB (el Base64 de los 100 MiB de contenido y
firmas, más 4 MiB para metadatos y envolturas), la profundidad a 64, los elementos a 20 000,
los atributos por elemento a 64 y el informe a 256 incidencias. Al alcanzar
el límite del informe devuelve una incidencia de límite, sin ocultar que se
ha alcanzado. El contenido y los datos de firma suman como máximo 100 MiB;
los expedientes admiten hasta 128 documentos y 256 MiB de entradas.

Las fechas se envían en RFC 3339 con zona horaria. WinUI utiliza
`CalendarDatePicker` y `TimePicker`; Qt utiliza `MonthGrid` y `DayOfWeekRow`
con imports sin versión y conserva la hora al cambiar el día. Los campos se
validan al abandonar el foco y cuando se corrige un error. Generar valida todo
el formulario, enfoca el primer error y lo desplaza a la vista antes de abrir
el diálogo de guardado. La validación se repite tras elegir la salida.

Resultados del 4 de octubre de 2026, en este entorno restringido:

| Puerta | Resultado exacto |
| --- | --- |
| `GOFLAGS=-buildvcs=false go vet ./...` | Código 0, sin diagnósticos. |
| `GOFLAGS=-buildvcs=false go test ./...` | Código 1: 83 paquetes pasan y 17 fallan; detalles debajo. |
| Go: paquetes ENI, CLI y localizador | Código 0 en los tres. Incluye validación de documento y expediente desde CLI, estructura, entradas inválidas e i18n. |
| Go: pruebas IPC ENI y contrato de acciones | Código 0. |
| `python3 -m unittest discover -s cmd/gui-qml/tests` | Código 0: 133 pruebas, OK. |
| `python3 -m unittest discover -s cmd/gui-winui/tests` | Código 0: 212 pruebas, OK. |
| `qmllint` de EniPanel, EniDateField y EniValidatedField | Código 0, sin diagnósticos. |
| qmake6 y make, fuera del árbol de fuentes | Código 0; binario `/tmp/eni-qt-build/grxfirma-gui-qml`. |
| Arranque Qt desde la raíz, HOME temporal, offscreen/software, timeout 15 | Código 124: el proceso sigue vivo hasta el timeout. |
| qmltestrunner de `cmd/gui-qml/tests/qml` | Código 0: 17 pruebas pasan; 0 fallan; 0 omitidas. Incluye apertura del calendario, cambio de fecha, desplegables y foco real en un campo inválido. |
| gopls check de los archivos Go ENI y validar_eni | Código 0, sin diagnósticos. |
| Semgrep, cinco reglas locales Go/JS/C#/QML, métricas y comprobación de versión desactivadas | Código 0: 18 archivos, 0 hallazgos, 0 errores. Dos archivos de pruebas excluidos por `.semgrepignore`. |
| Prueba opcional XSD/xmllint | Dos subpruebas omitidas mediante `t.Skip`: no están indicadas las rutas de los XSD. |
| Compilación y pruebas C# | Sin ejecutar: no hay dotnet en este entorno. Se añaden pruebas de lógica con `Assert.ThrowsExactly`. |

Go utiliza `GOCACHE=/tmp/eni-go-cache` porque la caché del directorio personal
es de solo lectura, y `GOPROXY=off` para impedir descargas. La revisión focal
de seguridad siguió la skill local `security-audit`; no se enviaron código ni
datos a servicios externos.

Para la prueba XSD opcional, indique las rutas completas de los esquemas
oficiales locales mediante `GRXFIRMA_ENI_XSD_DOCUMENTO` y
`GRXFIRMA_ENI_XSD_EXPEDIENTE`. La prueba utiliza `xmllint --nonet` y no descarga
esquemas. No se ha acreditado la conformidad XSD en esta sesión.

Los fallos de la suite Go completa afectan a las pruebas de sockets y al
entorno PKCS#11, incluido `bubblewrap_unsafe_permissions` y tres casos de rutas
administradas de módulos. No se han modificado esas pruebas ni sus requisitos.
El paquete IPC completo falla en las pruebas de sockets; las pruebas ENI de
ese paquete pasan por separado. Los paquetes fallidos son:

- `grxfirma/cmd/dssrunner`
- `grxfirma/cmd/grxfirma-pkcs11-worker`
- `grxfirma/cmd/grxfirmauri`
- `grxfirma/internal/adapters/inbound/common/rest`
- `grxfirma/internal/adapters/inbound/desktop/ipc`
- `grxfirma/internal/adapters/inbound/desktop/padesviewer`
- `grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase`
- `grxfirma/internal/adapters/inbound/legacy/websocket`
- `grxfirma/internal/adapters/outbound/common/intermediatehttp`
- `grxfirma/internal/adapters/outbound/common/revocationclient`
- `grxfirma/internal/adapters/outbound/common/signer`
- `grxfirma/internal/adapters/outbound/common/tsaclient`
- `grxfirma/internal/adapters/outbound/common/updatecheck`
- `grxfirma/internal/adapters/outbound/desktop/isolatedtokenstore`
- `grxfirma/internal/adapters/outbound/desktop/pkcs11worker`
- `grxfirma/internal/adapters/outbound/desktop/signer`
- `grxfirma/internal/testsupport/pkcs11sandbox`

Los cambios permanecen en el worktree. Su archivo `.git` apunta a
`/home/alberto/Trabajo/GrxFirma/.git/worktrees/wt-eni`, que es de solo lectura
para esta sesión. Por ello los commits se han creado en un repositorio
auxiliar local: `/tmp/eni-commits/git`, con la misma base y la rama
`feat/eni-catalogos`. No se ha hecho push ni se ha movido la referencia de la
rama registrada en el repositorio principal.

La entrega incluye `/tmp/eni-entrega/eni-catalogos.bundle` y parches por commit.
En un terminal con escritura en los metadatos Git, tras comprobar que el
worktree conserva exactamente estos cambios, puede incorporar los commits
sin reemplazar sus archivos:

```bash
git fetch /tmp/eni-entrega/eni-catalogos.bundle feat/eni-catalogos
git reset --mixed FETCH_HEAD
```

`reset --mixed` actualiza la rama y el índice, y conserva los archivos del
worktree. La base del bundle está en `/tmp/eni-entrega/base.txt`. Los registros
de cada puerta están en `/tmp/eni-entrega`.

Archivos del cambio:

- `cmd/gui-qml/qml.qrc`
- `cmd/gui-qml/qml/EniPanel.qml`
- `cmd/gui-winui/src/GrxFirma.WinUI/Views/EniPage.xaml.cs`
- `cmd/gui-winui/tests/test_winui_localizer_contract.py`
- `docs/schemas/desktop-ipc-v1.schema.json`
- `internal/adapters/inbound/common/cli/adapter.go`
- `internal/adapters/inbound/common/cli/generar_expediente_test.go`
- `internal/adapters/inbound/desktop/ipc/eni_invoice.go`
- `internal/adapters/inbound/desktop/ipc/eni_invoice_test.go`
- `internal/adapters/inbound/desktop/ipc/handler.go`
- `internal/adapters/inbound/desktop/ipc/protocol.go`
- `internal/adapters/outbound/common/eni/documento.go`
- `internal/adapters/outbound/common/eni/expediente.go`
- `internal/adapters/outbound/common/localizador/locales/ca.json`
- `internal/adapters/outbound/common/localizador/locales/de.json`
- `internal/adapters/outbound/common/localizador/locales/en.json`
- `internal/adapters/outbound/common/localizador/locales/es.json`
- `internal/adapters/outbound/common/localizador/locales/eu.json`
- `internal/adapters/outbound/common/localizador/locales/fr.json`
- `internal/adapters/outbound/common/localizador/locales/gl.json`
- `internal/adapters/outbound/common/localizador/locales/it.json`
- `internal/adapters/outbound/common/localizador/locales/pt.json`
- `internal/adapters/outbound/common/localizador/locales/va.json`
- `internal/adapters/outbound/common/localizador/locales/zh.json`
- `internal/adapters/outbound/common/localizador/testdata/i18n_iguales_permitidas.json`
- `cmd/gui-qml/qml/EniCatalog.js`
- `cmd/gui-qml/qml/EniDateField.qml`
- `cmd/gui-qml/qml/EniValidatedField.qml`
- `cmd/gui-qml/qml/EniValidation.js`
- `cmd/gui-qml/tests/qml/tst_eni.qml`
- `cmd/gui-qml/tests/test_eni_catalogs_contract.py`
- `cmd/gui-qml/tests/test_eni_validation.py`
- `cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/EniCatalog.cs`
- `cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/EniValidation.cs`
- `cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests/EniValidationTests.cs`
- `cmd/gui-winui/tests/test_eni_catalogs_contract.py`
- `docs/ENI_CATALOGOS_VALIDACION.md`
- `internal/adapters/inbound/common/cli/validar_eni.go`
- `internal/adapters/inbound/common/cli/validar_eni_test.go`
- `internal/adapters/outbound/common/eni/catalogos.go`
- `internal/adapters/outbound/common/eni/validacion.go`
- `internal/adapters/outbound/common/eni/validacion_test.go`
- `internal/adapters/outbound/common/localizador/testdata/eni_coincidencias.md`
- `scripts/generar_catalogos_eni.py`
