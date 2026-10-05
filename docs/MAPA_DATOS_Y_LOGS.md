<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Mapa de datos, logs y evidencias — GrxFirma

Fecha inicial: 2026-03-20.

Última revisión técnica: 2026-07-29.
Autoría: Alberto Avidad Fernández
Estado: mapa operativo vivo.

---

## Objetivo

Tener una visión práctica de:

- qué datos trata la app,
- dónde pueden aparecer en memoria, logs o evidencias,
- qué ya está saneado,
- y qué hay que revisar antes de declarar cumplimiento `RGPD/LOPDGDD` o `ENS`.

---

## Flujos principales

| Flujo | Datos potenciales | Riesgo principal |
|-------|-------------------|------------------|
| Selección de certificado | sujeto, emisor, huella, identificador del certificado | exposición excesiva en UI o logs |
| Firma de documento | documento, hash, formato, certificado, origen | trazas con nombres, hashes o payloads innecesarios |
| Verificación | documento firmado, certificado, estado de validez | exceso de detalle en errores o auditoría |
| `afirma://` legado | URI completa, endpoints, request ID, parámetros de sesión | fuga de URLs, IDs o payloads en debug |
| Native host / bridges | certificados disponibles, operaciones solicitadas | exposición de metadatos del usuario |

---

## Qué ya está bien orientado en el código

### Auditoría saneada

El caso de uso de auditoría:

- [audit.go](../internal/application/audit.go)

ya aplica medidas razonables:

- sanea `Origin`
- sanea `DocumentName`
- limita longitud
- elimina caracteres no imprimibles
- usa hash del documento (`sha256`) en vez de persistir el contenido
- serializa registro JSON saneado antes de enviarlo a `EvidenceLogger`

Esto es un buen punto de partida para `RGPD` y `ENS`.

### Logger estructurado con modo debug explícito

El logger común:

- [logger.go](../internal/adapters/outbound/common/logging/logger.go)

ya tiene:

- `GRXFIRMA_DEBUG`
- `GRXFIRMA_LOG_FILE`
- niveles de log
- salida texto/JSON según entorno
- redacción central de atributos, errores, rutas, orígenes, endpoints,
  identificadores, credenciales, cuerpos, cabeceras, PEM y datos codificados
- clasificación estable de errores sin persistir el texto crudo

Las trazas legacy de WebSocket, batch y `grxfirmauri` reutilizan esa política o
un vocabulario cerrado. Los paquetes oficiales añaden la etiqueta de build
`production`, que impide elevar el nivel a `DEBUG`, y no instalan lanzadores de
depuración.

---

## Riesgos detectados

| Riesgo | Estado | Observación |
|--------|--------|-------------|
| Logs debug demasiado ricos | `MITIGADO_EN_CODIGO` | Hay redacción y límites compartidos; los paquetes `production` no admiten DEBUG y la política técnica fija retención y borrado |
| Persistencia de payloads sensibles | `MITIGADO_EN_CODIGO` | Las trazas legacy e incidencias usan listas positivas o vocabularios cerrados y no guardan payloads crudos |
| URLs y endpoints en logs | `MITIGADO_EN_CODIGO` | Solo se conservan esquema/host/categoría cuando aportan diagnóstico; se eliminan credenciales, query y fragmentos |
| Certificados y huellas visibles | `PARCIAL` | Necesarios funcionalmente, pero deben limitarse a los casos imprescindibles |
| Temporales y P12 | `MITIGADO_EN_CODIGO` | El uso temporal vive solo en memoria, se purga al ocultar/apagar y la importación persistente exige una acción separada |
| Incidencias locales | `MITIGADO_EN_CODIGO` | Directorio 0700, ficheros 0600/atómicos, nombres aleatorios, rechazo de symlink final, colas acotadas, 30 días y máximo 50 grupos |
| Envío remoto de incidencias | `PARCIAL` | Transporte y consentimiento están endurecidos; faltan endpoint operativo, responsables y retención contractual |

---

## Variables y puntos sensibles localizados

Variables y configuraciones relevantes observadas:

- `GRXFIRMA_DEBUG`
- `GRXFIRMA_LOG_FILE`
- `GRXFIRMA_LOG_LEVEL`
- `GRXFIRMA_ENV`
- `GRXFIRMA_SUCCESS_GRACE_MS`
- `GRXFIRMA_PKCS12_DIR`
- `GRXFIRMA_PKCS12_PASSWORD`
- `GRXFIRMA_REST_TOKEN`
- `GRXFIRMA_PROTECTION_SECRET_B64`

Estas tres variables contienen secretos y solo se conservan como
compatibilidad para automatización gestionada. La CLI usa stdin sin eco como
canal normal; nunca deben incluirse valores secretos en argv, logs o
diagnósticos.

El token REST autogenerado no se imprime. Se entrega como configuración de
`curl` en `config/run/rest-auth-<pid>.curl`, protegida con `0600` o DACL privada
y eliminada al cerrar el servidor. La ruta puede aparecer en consola; no
contiene material secreto.
- `GRXFIRMA_TSA_URL`

Esto obliga a documentar claramente:

- cuáles son solo de soporte/diagnóstico
- cuáles son operativas
- y cuáles no deben activarse en producción pública sin control

---

## Qué debería revisarse antes de declarar cumplimiento

1. si alguna superficie nueva vuelve a introducir URI, payload, ruta o error
   crudo fuera de los saneadores comunes
2. si se persisten certificados, documentos o nombres completos donde no haga
   falta
3. si algún empaquetador nuevo omite la etiqueta `production` o vuelve a
   instalar un lanzador de depuración
4. si el despliegue necesita una retención menor o archivar evidencias antes
   del borrado técnico predeterminado
5. qué responsable y base contractual rigen el endpoint remoto de soporte

---

## Recomendaciones operativas inmediatas

| Medida | Prioridad |
|--------|-----------|
| Mantener el gate de `production` y ausencia de lanzadores debug | Alta |
| Aprobar contractualmente la retención del endpoint de soporte | Alta |
| Tabla `dato -> dónde aparece -> por qué -> cuánto dura` | Alta |
| Validación real del endpoint remoto y su certificado | Alta |

## Envío remoto de incidencias

La GUI Qt/QML incorpora envío remoto de incidencias acotadas con estas reglas:

- debe ser siempre `opt-in`;
- debe existir previsualización explícita de lo que se va a enviar;
- el transporte debe ser `TLS/HTTPS` obligatorio;
- el saneado debe ocurrir antes del empaquetado y del envío;
- no se enviarán rutas completas del perfil del usuario, secretos, `dat=` ni documentos.

Decisión inicial de producto:

- sí a `TLS` estricto;
- no, por ahora, a cifrado adicional del paquete con clave pública.
- sí a consentimiento explícito y previsualización previa del artefacto.

Esto no reduce la obligación de minimización: el canal cifrado no justifica
enviar más datos de los necesarios.

Interpretación operativa de esta decisión:

- la seguridad del envío remoto se apoya en `TLS` en tránsito;
- la privacidad se apoya en saneado y minimización previos;
- y el control de producto se apoya en preview + consentimiento.

Por tanto, mientras no exista un modelo justificado de claves y receptor remoto,
no se introduce cifrado extra del paquete como sustituto de esas tres medidas.

Implementación verificable:

- el botón solo aparece con un fallo cuya incidencia ya está guardada;
- C++ carga ese JSON desde el directorio privado y rechaza rutas externas,
  symlinks, nombre, esquema, tipo o fecha no válidos;
- la incidencia local no se pierde al cancelar o fallar el envío;
- la previsualización enumera datos incluidos y omitidos;
- la capa C++ vuelve a exigir consentimiento y vuelve a sanear el paquete;
- solo se aceptan destinos `https` sin credenciales ni fragmento;
- se usa validación TLS estricta, no se siguen redirecciones y solo una
  respuesta `2xx` se considera éxito;
- los documentos, claves, certificados completos, secretos, rutas del perfil,
  payloads y respuestas remotas crudas no forman parte del paquete.
- cada intento añade atómicamente solo hora UTC y resultado `sent`/`failed` a
  la copia local, sin URL, IP, cuerpo, respuesta ni error remoto.

---

## Conclusión actual

El saneado y la retención técnica local están implantados y probados, pero el
expediente de privacidad todavía no está cerrado:

- falta aprobar por despliegue si los máximos técnicos de 30/90 días deben
  reducirse o si alguna evidencia debe archivarse;
- falta cerrar roles, base jurídica y acceso de soporte;
- falta validar el endpoint remoto real y documentar su operación;
- y sigue siendo necesaria una revisión periódica para impedir regresiones al
  añadir nuevos campos o integraciones.
