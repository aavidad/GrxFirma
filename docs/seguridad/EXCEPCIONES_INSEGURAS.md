# Excepciones inseguras

Todas las excepciones empiezan desactivadas para cada usuario, también al actualizar desde 0.0.127. No se importa ningún valor antiguo como activado. Leer o verificar material histórico requiere el ajuste correspondiente. Cada uso permitido devuelve la clave de aviso de la operación. Las comprobaciones restantes, como contraseña, autorización de origen o consentimiento por operación, siguen siendo obligatorias.

## Ajustes y directivas

El ajuste operativo hoy está en `config.json`: en Linux y macOS, `~/.config/grxfirma/config.json`; en Windows, `%APPDATA%\GrxFirma\config.json`. Es un mapa JSON:

```json
{
  "excepciones_inseguras": {
    "cms_cbc": true
  }
}
```

Al editar un `config.json` existente, añada la clave dentro del objeto sin borrar los demás ajustes. Las preferencias de escritorio admiten el mismo mapa en `settings.json`. Para cada identificador, si existe un valor en `settings.json`, prevalece sobre `config.json`. La consulta IPC lee ambos ficheros y el cambio por IPC escribe solo `settings.json`. La sección **Preferencias > Seguridad > Excepciones inseguras** se preparará en el lote siguiente; aún no está disponible. La directiva de máquina se guarda en `/etc/grxfirma/policy.json` (macOS: `/private/etc/grxfirma/policy.json`) o en `HKLM\SOFTWARE\Policies\GrxFirma` en Windows; la plantilla ADMX/ADML presenta los valores de Windows. Una directiva fijada prevalece sobre el usuario. La lectura fallida o un valor ilegible bloquea la excepción. `prohibir_cifrado_cms_heredado=true` bloquea también la lectura de cifrados CMS antiguos y AES-CBC. Las variables antiguas de entorno no habilitan excepciones.

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

Para crear una identidad del perfil alto en fichero cuando falta el almacén, se requieren `alto_clave_fichero_crear` y `alto_clave_fichero_leer`: el programa debe poder reabrir esa identidad después de guardarla. La clave visible del ajuste es `excepciones.<id>.ajuste`. Los bloqueos explican el motivo y señalan la ruta actual de `config.json`. En verificación de SHA-1 o XML histórico, sin permiso la firma queda sin evaluar y no puede declararse válida. Con permiso, se devuelve la evaluación y el aviso.

## Contrato IPC

`unsafe_exceptions_list` devuelve la lista completa para Qt, WinUI y Android. Cada elemento contiene `id`, `user_key`, `policy_key`, `warning_key`, `block_key`, `setting_key`, `enabled` y `fixed`. `enabled` refleja la decisión efectiva tras aplicar usuario y máquina; `fixed` indica que la directiva de máquina fija el valor. Los nombres son estables y se traducen mediante los catálogos existentes.

`unsafe_exceptions_set` recibe `{"id":"cms_cbc","enabled":true}` y guarda solo el ajuste de usuario. El backend valida el identificador y conserva intactos los demás ajustes. Tras guardar, devuelve la lista de estados efectivos, de modo que una interfaz futura pueda explicar una prohibición de máquina. El error de bloqueo lleva `exceptionId`, `blockKey` y `settingKey` además del texto localizado. La interfaz podrá ofrecer «Abrir preferencias» usando `exceptionId` cuando exista la nueva sección.

El contrato de operaciones incluye `warnings` con claves tipadas. Los adaptadores de IPC, CLI, REST y Android deben trasladarlas cuando la operación pase por ellos; este documento no acredita que una pantalla ya las muestre. El host nativo conserva `Warnings` al firmar y verificar.

## Avisos sin excepción nueva

- Fila 26: iniciar REST con `-verificacion-anclas` mantiene el control existente y añade `excepciones.anclas_locales.aviso`. Las anclas indicadas sustituyen a las raíces del sistema para esa verificación.
- Fila 30: añadir o importar un origen en `trusted-domains.json` mantiene su control y añade `excepciones.origen_usuario.aviso`.
- Filas 46 y 47: delegar `afirma://` en AutoFirma añade `excepciones.autofirma.aviso` para recordar que se aplican sus controles. Si la directiva `manejador_afirma` no se puede leer, se añade `excepciones.politica_ilegible.aviso`.
- Una redirección trifásica a otro origen autorizado añade `excepciones.trifasica_redireccion.aviso`.

## Alcance

Se mantienen `*.dipgra.es` y `*.savia.net` como sitios de fábrica. Fijar el certificado local exacto sigue siendo una comprobación más estricta que confiar en cualquier cadena. La comprobación previa de revocación antes de firmar queda para otro lote. Las preferencias `secureConnections` y `secureDomainsList` están pendientes de confirmar como controles efectivos de transporte.

La fachada móvil expone `UnsafeExceptionsJSON()` y `SetUnsafeExceptionJSON(request)`, con el mismo esquema de consulta y cambio, vinculados al directorio privado recibido en `NewAndroidFacade` o `NewIOSFacade`. Estos métodos preparan la integración del panel de preferencias; este lote no incorpora ese panel.
