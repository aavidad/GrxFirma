<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Extensibilidad y utilidades integradas

Fecha de revisión: 2026-07-26.
Decisión canónica:
[ADR-004](ADR-004-extensibilidad-sin-plugins-runtime.md).

## Modelo de producto

GrxFirma se amplía incorporando capacidades revisadas y compiladas en sus
artefactos oficiales. No instala ni ejecuta plugins de usuario.

Esto permite que todas las capacidades compartan:

- política criptográfica y validación X.509;
- límites de tamaño, timeouts y tratamiento de errores;
- localización en los 11 catálogos del producto;
- tests de regresión y gates de release;
- actualización, firma y rollback del producto completo.

## Utilidades de V1.9 integradas

| Capacidad heredada | Implementación V2 | Superficies |
|---|---|---|
| Hash de fichero | Creación y comprobación con `SHA-1`, `SHA-256`, `SHA-384` y `SHA-512` | CLI, REST, consola web, `/validator`, Qt/QML |
| Hash de directorio | Manifiestos XML/TXT y `.hashreport`, lectura legacy defensiva | CLI, REST, consola web, `/validator`, Qt/QML |
| Validación de firma | Resultado rico, documento original opcional y confianza X.509 | CLI, REST, Qt/QML, `/signer`, `/validator` |
| Validación de certificado | Vigencia, `DigitalSignature`, cadena contra raíces del sistema y estado online OCSP/CRL | REST y `/validator` |
| Informes | Firma, certificado y hash exportables como JSON | `/validator` |

La utilidad dedicada está disponible en `/validator` y su alias
`/validador` cuando se arranca la REST local. Usa el mismo token autenticado
que el resto de la web local y lo conserva solo en memoria. Permite:

1. verificar una firma y, si es separada, aportar el documento original;
2. validar localmente el certificado seleccionado sin confiar en una raíz
   presentada por el propio fichero;
3. consultar OCSP/CRL de forma explícita;
4. crear o comprobar huellas;
5. exportar tres informes JSON independientes.

Los ficheros se comprueban antes de leerlos: 64 MiB por fichero y 72 MiB para
la combinación firma más original. Una respuesta antigua no puede sobrescribir
la selección o el resultado más reciente.

## Cómo añadir una capacidad

Una propuesta de nueva utilidad debe:

1. describir el caso de uso y su modelo de amenaza;
2. colocar reglas de negocio en `internal/domain` o `internal/application`;
3. definir puertos mínimos en `internal/ports`;
4. implementar adaptadores de entrada/salida sin saltarse el núcleo;
5. incorporar límites, cancelación, i18n y tests;
6. actualizar matrices, operación, seguridad y release;
7. pasar el gate completo y distribuirse en la siguiente versión oficial.

No se acepta como mecanismo de extensión:

- copiar binarios o scripts a un directorio observado por la aplicación;
- cargar módulos Go con `plugin.Open`;
- introducir frameworks RPC de plugins;
- embeber Go, WebAssembly o Lua para ejecutar código de terceros;
- usar un módulo Qt o PKCS#11 como puente para un gestor genérico.

El control `scripts/ci/check_no_runtime_plugins.py` hace fallar CI y release si
aparece una de esas superficies.

## Ciclo de vida

No existe ciclo de vida individual `instalar/activar/desactivar/quitar` para
extensiones porque no existen extensiones runtime.

| Acción deseada | Operación soportada |
|---|---|
| Incorporar una capacidad | Revisión de código y nueva versión oficial |
| Activarla | Disponible como función integrada según configuración/política |
| Desactivarla | Política o configuración tipada, si el caso de uso la requiere |
| Retirarla | Cambio revisado y nueva versión oficial |
| Revertirla | Rollback del artefacto completo firmado |

Las extensiones de navegador, módulos Qt y drivers PKCS#11 tienen instaladores
y ciclos propios porque son componentes de plataforma con contratos definidos;
no forman un API de plugins de GrxFirma.

## Si aparece un requisito de terceros

No debe desactivarse el gate de forma directa. Primero hace falta una ADR nueva
con, como mínimo:

- ejecución fuera del proceso criptográfico;
- protocolo versionado y deny-by-default;
- ninguna entrega de claves privadas ni handles reutilizables;
- autorización explícita por operación;
- procedencia y actualización administradas;
- límites de CPU, memoria, tiempo y tamaño;
- apagado, auditoría y revocación;
- tests de escape y recuperación ante fallo.

Hasta que ese expediente exista y sustituya ADR-004, la respuesta de producto
es integrar la capacidad o mantenerla fuera de GrxFirma.
