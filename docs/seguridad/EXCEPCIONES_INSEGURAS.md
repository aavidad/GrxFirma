<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Excepciones inseguras

Todas las excepciones empiezan desactivadas para cada usuario, también al actualizar desde 0.0.127. No se importa ningún valor antiguo como activado. Leer o verificar material histórico requiere el ajuste correspondiente. Cada uso permitido devuelve la clave de aviso de la operación. Las comprobaciones restantes, como contraseña, autorización de origen o consentimiento por operación, siguen siendo obligatorias.

## Para las personas

Si GrxFirma detiene una operación por seguridad, el aviso explica el motivo. En **Preferencias > Seguridad > Excepciones inseguras** puede leer el riesgo completo, aceptar la excepción o cancelar. El ajuste se puede desactivar en la misma sección. Si la organización lo prohíbe, consulte al administrador. Cada uso permitido muestra un aviso en el resultado.

## Para administradores e integradores

### Configuración y directivas

La vía habitual es `grxfirma -activar-excepcion ID` en el equipo donde se hará la operación. La orden muestra el riesgo y pide aceptación; para un uso automatizado se exige `-acepto-el-aviso`. La CLI guarda el ajuste y el recibo de aceptación en `settings.json`, en el directorio privado de configuración de GrxFirma. En Linux y macOS está bajo `~/.config/grxfirma/`; en Windows, bajo `%APPDATA%\GrxFirma\`.

Un `true` aislado en `config.json` o `settings.json` **no activa** una excepción. El mismo fichero debe contener también un recibo vigente para ese identificador. El ejemplo siguiente muestra la forma de los dos bloques, pero el marcador de fecha impide usarlo como aceptación: la fecha real y el recibo deben proceder de una aceptación expresa de la versión actual del texto.

```json
{
  "excepciones_inseguras": {
    "cms_cbc": true
  },
  "excepciones_inseguras_aceptaciones": {
    "cms_cbc": {
      "id": "cms_cbc",
      "accepted_at": "<fecha UTC emitida tras aceptar el aviso>",
      "text_version": "2026-11-15.3",
      "origin": "aceptado desde el aviso",
      "channel": "cli"
    }
  }
}
```

Si necesita conservar una configuración manual en `config.json`, copie allí **ambos bloques** producidos tras aceptar el aviso con la CLI. Para cada identificador, `settings.json` prevalece sobre `config.json`; una desactivación en `settings.json` no se revierte editando `config.json`. El núcleo lee los nombres exactos `excepciones_inseguras` y `excepciones_inseguras_aceptaciones`, junto con las claves exactas del recibo (`id`, `accepted_at`, `text_version`, `origin`, `channel`). No acepta alias con otras mayúsculas. Si cambia la versión del texto o el recibo falta, la excepción vuelve a estar desactivada hasta una nueva aceptación.

Una directiva de máquina fijada por el administrador prevalece sobre los ficheros de usuario. Si permite expresamente una excepción, ese ajuste de máquina ya es una autorización y no necesita recibo de usuario. La directiva se guarda en `/etc/grxfirma/policy.json` (macOS: `/private/etc/grxfirma/policy.json`) o `HKLM\SOFTWARE\Policies\GrxFirma` en Windows; la plantilla ADMX/ADML presenta los valores de Windows. Una directiva ilegible bloquea la excepción. `prohibir_cifrado_cms_heredado=true` bloquea también la lectura de cifrados CMS antiguos y AES-CBC. Las variables antiguas de entorno no habilitan excepciones.

Si GrxFirma no puede localizar un directorio privado de configuración válido, el bloqueo usa `excepciones.directorio_no_disponible` y no activa nada. En Android, la fachada debe recibir el directorio privado de la aplicación. La sección **Preferencias > Seguridad > Excepciones inseguras** permite revisar y aceptar cada caso desde la aplicación. La CLI mantiene la activación local con confirmación.

| Fila | Identificador | Qué permite | Riesgo | Ajuste de usuario | Directiva de máquina | Clave de aviso | Clave de bloqueo |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 01 | `cms_cifrado_heredado` | Leer sobres CMS con cifrados antiguos | estos cifrados ya no protegen bien el contenido | `excepciones_inseguras.cms_cifrado_heredado` | `permitir_cms_cifrado_heredado`; también `prohibir_cifrado_cms_heredado` | `excepciones.cms_cifrado_heredado.aviso` | `excepciones.cms_cifrado_heredado.bloqueo` |
| 02 | `cms_cbc` | Leer sobres CMS con AES-CBC | AES-CBC no autentica el contenido y puede ocultar cambios | `excepciones_inseguras.cms_cbc` | `permitir_cms_cbc`; también `prohibir_cifrado_cms_heredado` | `excepciones.cms_cbc.aviso` | `excepciones.cms_cbc.bloqueo` |
| 03 | `cms_rsa_v15_crear` | Crear CMS con transporte RSA antiguo | ese transporte de clave es antiguo y menos resistente que RSA-OAEP con SHA-256 | `excepciones_inseguras.cms_rsa_v15_crear` | `permitir_cms_rsa_v15_crear` | `excepciones.cms_rsa_v15_crear.aviso` | `excepciones.cms_rsa_v15_crear.bloqueo` |
| 04 | `cms_rsa_v15_leer` | Leer CMS con transporte RSA antiguo | ese transporte de clave es antiguo y menos resistente que RSA-OAEP con SHA-256 | `excepciones_inseguras.cms_rsa_v15_leer` | `permitir_cms_rsa_v15_leer` | `excepciones.cms_rsa_v15_leer.aviso` | `excepciones.cms_rsa_v15_leer.bloqueo` |
| 05 | `cms_aes_ecb` | Leer CMS AES-ECB de AutoFirma 1.9 | AES-ECB revela patrones del contenido | `excepciones_inseguras.cms_aes_ecb` | `permitir_cms_aes_ecb_legacy` | `excepciones.cms_aes_ecb.aviso` | `excepciones.cms_aes_ecb.bloqueo` |
| 09 | `afp_aad_ambigua` | Abrir archivos AFP v1 con metadatos ambiguos | los saltos de línea permiten interpretar sus metadatos de más de una forma | `excepciones_inseguras.afp_aad_ambigua` | `permitir_afp_aad_ambigua` | `excepciones.afp_aad_ambigua.aviso` | `excepciones.afp_aad_ambigua.bloqueo` |
| 10 | `alto_clave_fichero_crear` | Crear claves de protección alta en fichero | la clave privada queda fuera del almacén de secretos del sistema | `excepciones_inseguras.alto_clave_fichero_crear` | `permitir_alto_clave_fichero_crear` | `excepciones.alto_clave_fichero_crear.aviso` | `excepciones.alto_clave_fichero_crear.bloqueo` |
| 11 | `alto_clave_fichero_leer` | Usar claves de protección alta guardadas en fichero | la clave privada está fuera del almacén de secretos del sistema | `excepciones_inseguras.alto_clave_fichero_leer` | `permitir_alto_clave_fichero_leer` | `excepciones.alto_clave_fichero_leer.aviso` | `excepciones.alto_clave_fichero_leer.bloqueo` |
| 12 | `ca_clave_fichero` | Guardar la clave de la CA local en fichero | quien pueda leer ese fichero podrá usar la clave de la CA local | `excepciones_inseguras.ca_clave_fichero` | `permitir_ca_clave_fichero` | `excepciones.ca_clave_fichero.aviso` | `excepciones.ca_clave_fichero.bloqueo` |
| 13 | `sha1_crear` | Crear firmas SHA-1 | SHA-1 ya no ofrece una resistencia suficiente frente a colisiones | `excepciones_inseguras.sha1_crear` | `permitir_sha1_legacy` | `excepciones.sha1_crear.aviso` | `excepciones.sha1_crear.bloqueo` |
| 14 | `sha1_leer` | Evaluar firmas SHA-1 antiguas | SHA-1 ya no ofrece una resistencia suficiente frente a colisiones | `excepciones_inseguras.sha1_leer` | `permitir_sha1_leer` | `excepciones.sha1_leer.aviso` | `excepciones.sha1_leer.bloqueo` |
| 16 | `sha1_conservar` | Conservar firmas SHA-1 en ENI o cofirmas | SHA-1 ya no ofrece una resistencia suficiente frente a colisiones | `excepciones_inseguras.sha1_conservar` | `permitir_sha1_legacy` | `excepciones.sha1_conservar.aviso` | `excepciones.sha1_conservar.bloqueo` |
| 17 | `xml_c14n_antigua` | Evaluar XML con canonicalización histórica | su interpretación puede diferir del estándar actual | `excepciones_inseguras.xml_c14n_antigua` | `permitir_xml_c14n_antigua` | `excepciones.xml_c14n_antigua.aviso` | `excepciones.xml_c14n_antigua.bloqueo` |
| 18 | `pdf_cifrado_heredado` | Firmar PDF con cifrado antiguo | RC4 y AES-CBC con derivación MD5 mantienen una protección débil | `excepciones_inseguras.pdf_cifrado_heredado` | `permitir_pdf_cifrado_heredado` | `excepciones.pdf_cifrado_heredado.aviso` | `excepciones.pdf_cifrado_heredado.bloqueo` |
| 19 | `rsa_corta` | Firmar con RSA menor de 2048 bits | esa longitud ya no ofrece la seguridad mínima exigida | `excepciones_inseguras.rsa_corta` | `permitir_rsa_corta` | `excepciones.rsa_corta.aviso` | `excepciones.rsa_corta.bloqueo` |
| 06-07 | `des_intercambio` | Usar DES en intercambios AutoFirma 1.x | DES y la clave corta del intercambio son inseguros incluso con HTTPS | `excepciones_inseguras.des_intercambio` | `permitir_des_legacy` | `excepciones.des_intercambio.aviso` | `excepciones.des_intercambio.bloqueo` |
| 48 | `tsa_http` | Consultar una TSA por HTTP | HTTP no protege la conexión con la TSA | `excepciones_inseguras.tsa_http` | `permitir_tsa_http` | `excepciones.tsa_http.aviso` | `excepciones.tsa_http.bloqueo` |
| 49 | `proxy_http` | Enviar credenciales a un proxy HTTP | HTTP no protege las credenciales en el tramo hasta el proxy | `excepciones_inseguras.proxy_http` | `permitir_proxy_http` | `excepciones.proxy_http.aviso` | `excepciones.proxy_http.bloqueo` |
| 50 | `trifasica_http` | Hacer firma trifásica por HTTP | HTTP no protege el intercambio con el servidor | `excepciones_inseguras.trifasica_http` | `permitir_trifasica_http` | `excepciones.trifasica_http.aviso` | `excepciones.trifasica_http.bloqueo` |
| 51 | `ca_instalacion_automatica` | Instalar automáticamente la CA local | la instalación amplía la confianza del navegador | `excepciones_inseguras.ca_instalacion_automatica` | `permitir_ca_instalacion_automatica` | `excepciones.ca_instalacion_automatica.aviso` | `excepciones.ca_instalacion_automatica.bloqueo` |
| 52-53 | `cofirma_sin_original` | Cofirmar o contrafirmar sin el original | no se puede comprobar que la firma anterior corresponda a ese documento | `excepciones_inseguras.cofirma_sin_original` | `permitir_cofirma_sin_original` | `excepciones.cofirma_sin_original.aviso` | `excepciones.cofirma_sin_original.bloqueo` |
| 37 | `origen_vacio` | Aceptar WebSocket sin origen | no se sabe qué web ha iniciado la conexión | `excepciones_inseguras.origen_vacio` | `permitir_origen_vacio` | `excepciones.origen_vacio.aviso` | `excepciones.origen_vacio.bloqueo` |
| 38 | `rutas_directas` | Aceptar rutas locales pedidas por una web | la web puede elegir un destino local en lugar de la persona | `excepciones_inseguras.rutas_directas` | `permitir_rutas_directas_protocolo` | `excepciones.rutas_directas.aviso` | `excepciones.rutas_directas.bloqueo` |
| 39 | `aprobacion_automatica` | Firmar sin confirmación desde la extensión | se omite la aprobación visible de la persona | `excepciones_inseguras.aprobacion_automatica` | `aprobacion_automatica_nativehost` | `excepciones.aprobacion_automatica.aviso` | `excepciones.aprobacion_automatica.bloqueo` |
| 40 | `llamante_desarrollo` | Aceptar extensión de desarrollo en el host nativo | se amplía quién puede pedir operaciones al host | `excepciones_inseguras.llamante_desarrollo` | `permitir_llamante_desarrollo` | `excepciones.llamante_desarrollo.aviso` | `excepciones.llamante_desarrollo.bloqueo` |
| 45 | `pkcs11_experimental` | Usar controladores PKCS#11 experimentales | el controlador nativo puede ejecutar código con los permisos de GrxFirma | `excepciones_inseguras.pkcs11_experimental` | `permitir_pkcs11_experimental` | `excepciones.pkcs11_experimental.aviso` | `excepciones.pkcs11_experimental.bloqueo` |

El lector del sobre CMS firmado y cifrado de AutoFirma 1.9 exige tanto `cms_aes_ecb` como `cms_rsa_v15_leer`. Si la máquina prohíbe cualquiera de las dos, el sobre no se abre.

Para crear una identidad del perfil alto en fichero cuando falta el almacén, se requieren `alto_clave_fichero_crear` y `alto_clave_fichero_leer`: el programa debe poder reabrir esa identidad después de guardarla. La clave visible del ajuste es `excepciones.<id>.ajuste`. Los bloqueos explican el motivo y dan la orden local `grxfirma -activar-excepcion ID`, con confirmación expresa. En verificación de SHA-1 o XML histórico, sin permiso la firma queda sin evaluar y no puede declararse válida. Con permiso, se devuelve la evaluación y el aviso.

### Contrato IPC

`unsafe_exceptions_list` devuelve la lista completa para Qt, WinUI y Android. Cada elemento contiene `id`, `user_key`, `policy_key`, `warning_key`, `block_key`, `setting_key`, `enabled` y `fixed`. `enabled` refleja la decisión efectiva tras aplicar usuario y máquina; `fixed` indica que la directiva de máquina fija el valor. Los nombres son estables y se traducen mediante los catálogos existentes.

`unsafe_exceptions_set` recibe `{"id":"cms_cbc","enabled":false}` para desactivar desde la interfaz local verificada. `enabled:true` se rechaza: la activación exige la aceptación versionada de `unsafe_exceptions_activate`, descrita en el anexo. Guardar preferencias generales conserva estos bloques y no puede activarlos ni borrar una desactivación previa. El error de bloqueo lleva `exceptionId`, `blockKey` y `settingKey`, además de las claves y metadatos del anexo.

El contrato de operaciones incluye `warnings` con claves tipadas. Los adaptadores de IPC, CLI, REST y Android deben trasladarlas cuando la operación pase por ellos; este documento no acredita que una pantalla ya las muestre. El host nativo conserva `Warnings` al firmar y verificar.

### Avisos sin excepción nueva

- Fila 26: iniciar REST con `-verificacion-anclas` mantiene el control existente y añade `excepciones.anclas_locales.aviso`. Las anclas indicadas sustituyen a las raíces del sistema para esa verificación.
- Fila 30: añadir o importar un origen en `trusted-domains.json` mantiene su control y añade `excepciones.origen_usuario.aviso`.
- Filas 46 y 47: delegar `afirma://` en AutoFirma añade `excepciones.autofirma.aviso` para recordar que se aplican sus controles. Si la directiva `manejador_afirma` no se puede leer, se añade `excepciones.politica_ilegible.aviso`.
- Una redirección trifásica a otro origen autorizado añade `excepciones.trifasica_redireccion.aviso`.

### Alcance

Se mantienen `*.dipgra.es` y `*.savia.net` como sitios de fábrica. Fijar el certificado local exacto sigue siendo una comprobación más estricta que confiar en cualquier cadena. La comprobación previa de revocación antes de firmar queda para otro lote. Las preferencias `secureConnections` y `secureDomainsList` están pendientes de confirmar como controles efectivos de transporte.

La fachada móvil expone `UnsafeExceptionsJSON()` y `SetUnsafeExceptionJSON(request)`, con el mismo esquema de consulta y cambio, vinculados al directorio privado recibido en `NewAndroidFacade` o `NewIOSFacade`. Estos métodos permiten que la aplicación móvil consulte y cambie las excepciones mediante su directorio privado.

### Recibos y activación local

El núcleo conserva `AcceptanceVersion = "2026-11-15.3"`. La versión `2026-11-15.3` sustituye a la `.2` porque cambian los textos que se aceptan: `proxy_http` cubre también SOCKS5 y lo dice, la ruta para desactivar es «el apartado Seguridad» en todas las plataformas y los nueve idiomas que eran copia del castellano tienen traducción propia. Quien aceptó con la `.2` vuelve a ver el aviso una vez. Una aceptación local guarda `origin` con el valor exacto «aceptado desde el aviso», `accepted_at` en UTC distinto de cero, `channel` (`cli`, `desktop` o `android`; `protocol_dialog` se conserva solo para leer recibos históricos) y `text_version`. El recibo se lee por sus nombres exactos, sin alias con otras mayúsculas. Si cambia la versión del texto, el ajuste de usuario deja de activarse hasta que se lea y acepte el texto nuevo. La directiva de máquina prevalece también después de una aceptación local.

`unsafe_exceptions_activate` recibe `{"id":"cms_cbc","accepted":true,"acceptanceVersion":"2026-11-15.3"}`. Puede incluir `retryId` cuando el núcleo haya ofrecido un reintento seguro. `unsafe_operation_retry` recibe `{"retryId":"..."}` y solo reintenta una operación local pendiente y vigente. El núcleo valida el identificador, la versión, la aceptación y la directiva antes de activar. La interfaz presenta el riesgo antes de aceptar y permite cancelar la operación.

La lista y los bloqueos exponen `title_key`, `risk_key`, `acceptance_key`, `help_key` y `organization_key`, además de `activable`. Cuando `activable` es falso por una directiva, la interfaz muestra el texto de organización y remite al administrador.

### Vuelta 2: firmar y cifrar con transporte seguro

«Firmar y cifrar», incluido Android, conserva el formato SignedAndEnvelopedData y usa por defecto RSA-OAEP con SHA-256 para proteger la clave del contenido, AES-256-GCM para el contenido y SHA-256 para la firma. No exige activar `cms_rsa_v15_crear`. La compatibilidad PKCS#1 v1.5 requiere tanto `cms_key_transport=pkcs1v15` como el ajuste expreso ya documentado. El lector propio reconoce OAEP con sus parámetros y conserva las comprobaciones de integridad, vigencia y confianza. No se ha añadido descifrado mediante dispositivos de claves no exportables; sigue usando el material PKCS#8 que admitía antes.

## Ayudas para las personas

Cada ayuda explica qué se ha detenido, el riesgo y cómo continuar o desactivar la excepción desde Preferencias, Seguridad. Estos son los textos completos del catálogo español. El catálogo inglés tiene traducción propia; los otros nueve idiomas muestran temporalmente el castellano para estas claves.

### `afp_aad_ambigua`

Qué ha pasado: GrxFirma ha detenido esta operación: abrir un archivo AFP v1 con metadatos ambiguos. Un archivo .afp guarda contenido protegido y metadatos como el nombre.

Por qué es inseguro: Los saltos de línea permiten interpretar sus metadatos de más de una forma. Por ejemplo, un salto de línea en el nombre puede cambiar cómo se interpreta el nombre y el tipo del archivo.

Cuándo tiene sentido: Solo si necesita abrir un archivo .afp antiguo y conoce su procedencia.

Alternativa: Pida una copia nueva o guarde de nuevo el contenido en el formato .afp actual.

Qué cambia: al activar «Abrir archivos AFP v1 con metadatos ambiguos», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Abrir archivos AFP v1 con metadatos ambiguos» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `alto_clave_fichero_crear`

Qué ha pasado: GrxFirma ha detenido esta operación: crear una clave de protección alta en un fichero sin almacén seguro. La identidad del perfil alto necesita una clave privada, que normalmente guarda el almacén de secretos del sistema.

Por qué es inseguro: La clave privada queda fuera del almacén de secretos del sistema. Por ejemplo, otro programa con acceso a su cuenta podría copiar el fichero de la clave privada.

Cuándo tiene sentido: Solo en un equipo controlado que no dispone de almacén de secretos.

Alternativa: Habilite el almacén de secretos del sistema antes de crear la identidad.

Qué cambia: al activar «Crear claves de protección alta en fichero», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Crear claves de protección alta en fichero» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `alto_clave_fichero_leer`

Qué ha pasado: GrxFirma ha detenido esta operación: usar una clave antigua de protección alta guardada en un fichero. La identidad del perfil alto usa una clave privada guardada antes en un fichero.

Por qué es inseguro: La clave privada está fuera del almacén de secretos del sistema. Por ejemplo, un programa con acceso al fichero podría copiar la clave antigua mientras se usa.

Cuándo tiene sentido: Solo para recuperar documentos protegidos con esa identidad antigua.

Alternativa: Active el almacén de secretos para que GrxFirma migre la clave cuando sea posible.

Qué cambia: al activar «Usar claves de protección alta guardadas en fichero», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Usar claves de protección alta guardadas en fichero» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `aprobacion_automatica`

Qué ha pasado: GrxFirma ha detenido esta operación: firmar desde la extensión sin confirmar cada operación. La extensión es el complemento del navegador que solicita la firma a GrxFirma.

Por qué es inseguro: Se omite la aprobación visible de la persona. Por ejemplo, una petición de firma de la extensión podría continuar sin el diálogo de confirmación.

Cuándo tiene sentido: Solo en un puesto dedicado, controlado y administrado.

Alternativa: Mantenga la confirmación de cada firma para revisar documento y solicitante.

Qué cambia: al activar «Firmar sin confirmación desde la extensión», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Firmar sin confirmación desde la extensión» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `ca_clave_fichero`

Qué ha pasado: GrxFirma ha detenido esta operación: guardar la clave privada de la CA local en un fichero sin almacén seguro. La autoridad local crea el certificado que GrxFirma usa para sus conexiones locales.

Por qué es inseguro: Quien pueda leer ese fichero podrá usar la clave de la CA local. Por ejemplo, un programa de su cuenta que copie la clave podría crear certificados locales falsos.

Cuándo tiene sentido: Solo si necesita el canal local y el equipo no tiene almacén de secretos.

Alternativa: Habilite el almacén de secretos del sistema para guardar la clave de la autoridad local.

Qué cambia: al activar «Guardar la clave de la CA local en fichero», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Guardar la clave de la CA local en fichero» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `ca_instalacion_automatica`

Qué ha pasado: GrxFirma ha detenido esta operación: instalar automáticamente la CA local al abrir un enlace de firma en Linux. La autoridad local emite certificados para la conexión entre GrxFirma y el navegador.

Por qué es inseguro: La instalación amplía la confianza del navegador. Por ejemplo, el navegador confiará en la autoridad local para las conexiones que esta pueda emitir.

Cuándo tiene sentido: Solo en un equipo propio donde acepta instalar la autoridad local al abrir el protocolo.

Alternativa: Instale la autoridad de forma expresa con el instalador o la orden de GrxFirma.

Qué cambia: al activar «Instalar automáticamente la CA local», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Instalar automáticamente la CA local» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `cms_aes_ecb`

Qué ha pasado: GrxFirma ha detenido esta operación: abrir un sobre CMS AES-ECB de AutoFirma 1.9. CMS es un formato de archivo cifrado. AES-ECB cifra bloques iguales de forma que dejan patrones visibles.

Por qué es inseguro: AES-ECB revela patrones del contenido. Por ejemplo, dos partes iguales del documento dejan patrones iguales en el cifrado.

Cuándo tiene sentido: Solo para recuperar sobres antiguos de AutoFirma 1.9.

Alternativa: Pida una copia protegida con un cifrado actual que oculte patrones y compruebe cambios.

Qué cambia: al activar «Leer CMS AES-ECB de AutoFirma 1.9», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Leer CMS AES-ECB de AutoFirma 1.9» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `cms_cbc`

Qué ha pasado: GrxFirma ha detenido esta operación: abrir un sobre CMS cifrado con AES-CBC. CMS es un formato de archivo cifrado. AES-CBC es un modo de cifrado que no comprueba por sí mismo si el contenido se cambió.

Por qué es inseguro: AES-CBC no autentica el contenido y puede ocultar cambios. Por ejemplo, un cambio en el archivo cifrado puede no detectarse al abrirlo.

Cuándo tiene sentido: Solo para recuperar un sobre antiguo del que conoce el origen.

Alternativa: Pida una copia cifrada con un modo que compruebe también la integridad, como AES-GCM.

Qué cambia: al activar «Leer sobres CMS con AES-CBC», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Leer sobres CMS con AES-CBC» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `cms_cifrado_heredado`

Qué ha pasado: GrxFirma ha detenido esta operación: abrir un sobre CMS cifrado con DES, 3DES u otro cifrado antiguo. CMS es un formato de archivo cifrado para uno o varios destinatarios.

Por qué es inseguro: Estos cifrados ya no protegen bien el contenido. Por ejemplo, un archivo antiguo puede usar DES y revelar más información a quien lo intercepte.

Cuándo tiene sentido: Solo si necesita recuperar un archivo antiguo de una fuente conocida.

Alternativa: Pida que vuelvan a cifrar el documento con un formato actual y una clave nueva.

Qué cambia: al activar «Leer sobres CMS con cifrados antiguos», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Leer sobres CMS con cifrados antiguos» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `cms_rsa_v15_crear`

Qué ha pasado: GrxFirma ha detenido esta operación: crear un sobre CMS con transporte RSA PKCS#1 v1.5. CMS es un formato de archivo cifrado. RSA PKCS#1 v1.5 es una forma antigua de cifrar la clave del contenido.

Por qué es inseguro: Ese transporte de clave es antiguo y menos resistente que RSA-OAEP con SHA-256. Por ejemplo, un programa antiguo que procesa el sobre puede revelar información sobre la clave mediante sus respuestas.

Cuándo tiene sentido: Solo si el destinatario no puede abrir el transporte actual de clave.

Alternativa: Use el transporte RSA-OAEP con SHA-256 que GrxFirma elige por defecto.

Qué cambia: al activar «Crear CMS con transporte RSA antiguo», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Crear CMS con transporte RSA antiguo» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `cms_rsa_v15_leer`

Qué ha pasado: GrxFirma ha detenido esta operación: abrir un sobre CMS con transporte RSA PKCS#1 v1.5. CMS es un formato de archivo cifrado. RSA PKCS#1 v1.5 es una forma antigua de cifrar la clave del contenido.

Por qué es inseguro: Ese transporte de clave es antiguo y menos resistente que RSA-OAEP con SHA-256. Por ejemplo, un sobre preparado para programas antiguos puede exponer información sobre la clave al probar distintas respuestas.

Cuándo tiene sentido: Solo para recuperar sobres de un remitente conocido.

Alternativa: Pida al remitente un nuevo sobre con RSA-OAEP y SHA-256.

Qué cambia: al activar «Leer CMS con transporte RSA antiguo», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Leer CMS con transporte RSA antiguo» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `cofirma_sin_original`

Qué ha pasado: GrxFirma ha detenido esta operación: cofirmar o contrafirmar sin el documento original. Una cofirma añade una firma a otra existente; una contrafirma firma una firma previa.

Por qué es inseguro: No se puede comprobar que la firma anterior corresponda a ese documento. Por ejemplo, podría firmar sobre un resumen sin ver el documento que firmaron las otras personas.

Cuándo tiene sentido: Solo si no puede obtener el original y conoce bien el alcance de la firma.

Alternativa: Aporte el documento original para que GrxFirma compruebe que corresponde a la firma anterior.

Qué cambia: al activar «Cofirmar o contrafirmar sin el original», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Cofirmar o contrafirmar sin el original» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `des_intercambio`

Qué ha pasado: GrxFirma ha detenido esta operación: usar DES en un intercambio AutoFirma 1.x. DES es un algoritmo antiguo de cifrado usado por ese intercambio de AutoFirma.

Por qué es inseguro: DES y la clave corta del intercambio son inseguros incluso con HTTPS. Por ejemplo, la clave corta del paquete puede averiguarse aunque el servidor use HTTPS.

Cuándo tiene sentido: Solo para un intercambio AutoFirma 1.x imprescindible y con destino autorizado.

Alternativa: Pida al portal un intercambio actual que no use DES.

Qué cambia: al activar «Usar DES en intercambios AutoFirma 1.x», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Usar DES en intercambios AutoFirma 1.x» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `llamante_desarrollo`

Qué ha pasado: GrxFirma ha detenido esta operación: aceptar una extensión de desarrollo como llamante del host nativo. El host nativo comunica una extensión del navegador con GrxFirma.

Por qué es inseguro: Se amplía quién puede pedir operaciones al host. Por ejemplo, una extensión de pruebas podría pedir operaciones al host nativo.

Cuándo tiene sentido: Solo durante pruebas locales de la extensión en una compilación de desarrollo.

Alternativa: Use la extensión oficial en una instalación de producción.

Qué cambia: al activar «Aceptar extensión de desarrollo en el host nativo», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Aceptar extensión de desarrollo en el host nativo» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `origen_vacio`

Qué ha pasado: GrxFirma ha detenido esta operación: aceptar una conexión WebSocket sin origen identificado. El origen de una conexión WebSocket identifica la página que la abrió.

Por qué es inseguro: No se sabe qué web ha iniciado la conexión. Por ejemplo, una petición sin origen no permite distinguir qué página abrió la conexión.

Cuándo tiene sentido: Solo en un entorno controlado donde conoce el programa que abre la conexión.

Alternativa: Use un portal que envíe su origen y autorícelo de forma normal.

Qué cambia: al activar «Aceptar WebSocket sin origen», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Aceptar WebSocket sin origen» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `pdf_cifrado_heredado`

Qué ha pasado: GrxFirma ha detenido esta operación: firmar un PDF que conserva RC4 o AES-CBC con derivación MD5. PDF es el formato del documento. RC4 y AES-CBC son formas antiguas de mantenerlo cifrado; MD5 interviene al obtener la clave.

Por qué es inseguro: RC4 y AES-CBC con derivación MD5 mantienen una protección débil. Por ejemplo, una revisión nueva seguiría protegida por RC4 o por una clave derivada con MD5.

Cuándo tiene sentido: Solo si debe firmar el PDF antiguo sin poder cambiar su cifrado.

Alternativa: Pida una copia reprotegida con cifrado actual antes de firmarla.

Qué cambia: al activar «Firmar PDF con cifrado antiguo», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Firmar PDF con cifrado antiguo» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `pkcs11_experimental`

Qué ha pasado: GrxFirma ha detenido esta operación: cargar un controlador PKCS#11 experimental. PKCS#11 es una interfaz para usar tarjetas y dispositivos criptográficos mediante un controlador.

Por qué es inseguro: El controlador nativo puede ejecutar código con los permisos de GrxFirma. Por ejemplo, un controlador defectuoso puede fallar o actuar con los permisos de GrxFirma.

Cuándo tiene sentido: Solo para probar un dispositivo compatible con la versión experimental.

Alternativa: Use un certificado del almacén del sistema o un controlador ya admitido.

Qué cambia: al activar «Usar controladores PKCS#11 experimentales», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Usar controladores PKCS#11 experimentales» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `proxy_http`

Qué ha pasado: GrxFirma ha detenido el envío de un usuario y una contraseña a un servidor intermedio de red sin una conexión cifrada. El proxy puede usar HTTP o SOCKS5.

Por qué es inseguro: HTTP y SOCKS5 no cifran las credenciales en el tramo hasta el proxy. Otro equipo en esa red puede capturar el usuario y la contraseña.

Cuándo tiene sentido: Solo en una red administrada donde el proxy requerido no tenga una conexión cifrada.

Alternativa: Use un proxy HTTPS o uno que no pida credenciales.

Qué cambia: al activar «Enviar credenciales a un proxy sin cifrar», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Enviar credenciales a un proxy sin cifrar» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `rsa_corta`

Qué ha pasado: GrxFirma ha detenido esta operación: firmar con una clave RSA de menos de 2048 bits. RSA es el algoritmo de la clave pública que se usa para firmar.

Por qué es inseguro: Esa longitud ya no ofrece la seguridad mínima exigida. Por ejemplo, una clave de 1024 bits ofrece menos resistencia que el mínimo de 2048 bits.

Cuándo tiene sentido: Solo para un trámite antiguo que no admita otra identidad y cuyo riesgo haya aceptado.

Alternativa: Use un certificado con clave RSA de al menos 2048 bits.

Qué cambia: al activar «Firmar con RSA menor de 2048 bits», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Firmar con RSA menor de 2048 bits» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `rutas_directas`

Qué ha pasado: GrxFirma ha detenido esta operación: usar una ruta local absoluta indicada por una web. Una ruta absoluta señala un archivo o carpeta concretos del equipo.

Por qué es inseguro: La web puede leer o sustituir archivos locales sin que usted elija cada uno. Puede exponer claves de acceso o cambiar archivos que afectan al inicio de otros programas.

Cuándo tiene sentido: Solo para un portal autorizado cuyo flujo necesite una ruta fija.

Alternativa: Elija el archivo o la carpeta personalmente en el diálogo de GrxFirma.

Qué cambia: al activar «Aceptar rutas locales pedidas por una web», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Aceptar rutas locales pedidas por una web» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `sha1_conservar`

Qué ha pasado: GrxFirma ha detenido esta operación: conservar una firma SHA-1 antigua al crear un ENI o una cofirma. SHA-1 es un algoritmo antiguo que calcula la huella de la firma.

Por qué es inseguro: SHA-1 ya no ofrece una resistencia suficiente frente a colisiones. Por ejemplo, el documento resultante conservaría una firma cuya huella ya no ofrece la resistencia actual.

Cuándo tiene sentido: Solo si debe conservar una firma histórica que corresponde al original.

Alternativa: Pida una firma nueva con SHA-256 o superior y aporte el documento original.

Qué cambia: al activar «Conservar firmas SHA-1 en ENI o cofirmas», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Conservar firmas SHA-1 en ENI o cofirmas» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `sha1_crear`

Qué ha pasado: GrxFirma ha detenido esta operación: crear una firma SHA-1. SHA-1 es un algoritmo que calcula la huella usada en la firma.

Por qué es inseguro: SHA-1 ya no ofrece una resistencia suficiente frente a colisiones. Por ejemplo, dos documentos distintos pueden llegar a compartir la misma huella de ese algoritmo.

Cuándo tiene sentido: Solo si una sede antigua exige SHA-1 y no acepta una firma actual.

Alternativa: Pida a la sede que admita SHA-256 o superior.

Qué cambia: al activar «Crear firmas SHA-1», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Crear firmas SHA-1» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `sha1_leer`

Qué ha pasado: GrxFirma ha detenido esta operación: evaluar una firma SHA-1 antigua. SHA-1 es un algoritmo antiguo que calcula la huella de la firma.

Por qué es inseguro: SHA-1 ya no ofrece una resistencia suficiente frente a colisiones. Por ejemplo, una firma antigua puede tener un resumen que no ofrezca la protección esperada hoy.

Cuándo tiene sentido: Solo para examinar una firma histórica y entender sus límites.

Alternativa: Pida una copia firmada de nuevo con SHA-256 o superior si necesita una firma actual.

Qué cambia: al activar «Evaluar firmas SHA-1 antiguas», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Evaluar firmas SHA-1 antiguas» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `trifasica_http`

Qué ha pasado: GrxFirma ha detenido esta operación: hacer una firma trifásica por HTTP. La firma trifásica intercambia datos con un servidor en varios pasos. HTTP no cifra ese trayecto.

Por qué es inseguro: HTTP no protege el intercambio con el servidor. Por ejemplo, alguien en la red podría ver o modificar datos intercambiados con el servidor de firma.

Cuándo tiene sentido: Solo si el portal autorizado exige HTTP y no ofrece HTTPS.

Alternativa: Pida al portal una dirección HTTPS para la firma en varias fases.

Qué cambia: al activar «Hacer firma trifásica por HTTP», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Hacer firma trifásica por HTTP» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `tsa_http`

Qué ha pasado: GrxFirma ha detenido esta operación: enviar el resumen del documento a una TSA por HTTP. Una TSA es un servicio de sellado de tiempo. HTTP es una conexión de red que no cifra el trayecto.

Por qué es inseguro: HTTP no protege la conexión con la TSA. Por ejemplo, alguien en la red puede observar o cambiar la petición enviada al servicio de sellado.

Cuándo tiene sentido: Solo si la entidad exige una dirección HTTP que no tiene alternativa segura.

Alternativa: Configure la dirección HTTPS del servicio de sellado de tiempo.

Qué cambia: al activar «Consultar una TSA por HTTP», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Consultar una TSA por HTTP» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

### `xml_c14n_antigua`

Qué ha pasado: GrxFirma ha detenido esta operación: evaluar una firma XML con canonicalización histórica o no declarada. XML es un formato de documento. La canonicalización fija qué bytes exactos se comprueban en su firma.

Por qué es inseguro: Su interpretación puede diferir del estándar actual. Por ejemplo, dos programas pueden reconstruir de forma distinta el texto exacto que se firmó.

Cuándo tiene sentido: Solo para estudiar una firma XML histórica de procedencia conocida.

Alternativa: Pida una nueva firma XML que declare el método de normalización actual.

Qué cambia: al activar «Evaluar XML con canonicalización histórica», GrxFirma permite esta operación. Cada uso muestra un aviso en el resultado.

Cómo deshacerlo: desactive «Evaluar XML con canonicalización histórica» en Preferencias, Seguridad.

Bloqueo de la organización: si la organización lo prohíbe, usted no puede activarla. Consulte al administrador.

El diálogo propio de afirma:// no tiene todavía una operación de activación implementada. Se ha retirado su emisor de autoridad sin llamadores. Hasta conectar ese diálogo mediante IPC local, la activación se hace en Preferencias, Seguridad; ningún parámetro del protocolo puede crear una autoridad local.

### Texto de bloqueo por canal (vuelta 6)

Las claves `excepciones.<id>.bloqueo` explican la operación, el riesgo y el nombre del ajuste en Preferencias, Seguridad. La persona puede aceptar el aviso en GrxFirma cuando la interfaz ofrezca esa acción, o abrir esa sección. Android, escritorio y los portales reciben el mismo texto sin instrucciones de terminal. Si la organización lo prohíbe o su directiva es ilegible, el bloqueo remite al administrador y no ofrece activación.

La CLI añade `excepciones.cli.como_activar`, sustituyendo `{id}` por el identificador. La orden es `grxfirma -activar-excepcion <id>` con confirmación interactiva; para automatizarla se necesita `-acepto-el-aviso`. El texto de aceptación y su versión `2026-11-15.3` no cambian.

La barrera de rutas compara la identidad de cada antecesor existente con las carpetas protegidas mediante `os.SameFile`. Así se comprueban también los alias por mayúsculas y las uniones de compatibilidad de Windows. Las pruebas específicas de esos sistemas deben ejecutarse allí; compilarlas en Linux no acredita ese resultado.

### Desactivar desde la CLI (vuelta 9)

`grxfirma -desactivar-excepcion ID` guarda el ajuste desactivado sin pedir aceptación. Su alias es `-disable-unsafe-exception`. La elección y la fecha quedan en la auditoría local. Si una directiva fija el valor, hay que dirigirse al administrador. Una directiva ilegible mantiene el cierre de seguridad y permite guardar la renuncia local, que seguirá desactivada cuando vuelva a poder leerse la directiva y no haya un valor fijado.

La excepción `proxy_http` y su directiva conservan el identificador. Protegen las credenciales que se envían sin cifrar hasta un proxy HTTP, SOCKS5 o SOCKS5H; un proxy HTTPS no necesita esta excepción. Se mantienen intactos los textos de aceptación y su versión.
