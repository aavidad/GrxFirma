<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR-004 — Extensibilidad sin plugins runtime en proceso

Fecha: 2026-07-26
Estado: ACEPTADA
Decisión asociada: `T096`

## Contexto

AutoFirma 1.9 permite instalar JAR y cargarlos mediante `URLClassLoader` dentro
del mismo proceso de la aplicación. La revisión de
`afirma-simple-plugins-manager` y `afirma-simple-plugins` confirma que:

- el plugin comparte proceso, identidad de usuario y memoria con AutoFirma;
- puede intervenir antes o después de la firma, añadir comandos y UI y ejecutar
  acciones de instalación o desinstalación;
- la lista declarativa de permisos decide qué hooks invoca la aplicación, pero
  no constituye un sandbox ni limita las APIs de Java que puede usar el JAR;
- la verificación comprueba integridad y coherencia de los firmantes del JAR,
  pero la UI permite continuar tras encontrar entradas sin firma o una firma
  inválida si el usuario acepta el aviso;
- la confianza final se delega a una confirmación visual del certificado, no a
  una política administrada y fail-closed de publicadores autorizados.

En GrxFirma el proceso maneja documentos, certificados, sesiones locales y
referencias a claves privadas. Ejecutar código aportado por un usuario en ese
proceso concedería en la práctica todos sus privilegios, aunque el paquete
estuviera firmado o declarase permisos reducidos.

Las dos utilidades oficiales relevantes de V1.9, hash y validación de
certificados, ya tienen equivalente integrado en V2. La superficie
`/validator` (`/validador`) reúne verificación rica de firmas, validación X.509
contra raíces del sistema, comprobación OCSP/CRL, hash y exportación de
informes. Por tanto, no es necesario aceptar ejecución arbitraria para
recuperar esas capacidades de producto.

## Decisión

GrxFirma **no tendrá un gestor de plugins runtime ni cargará código aportado
por usuarios dentro de sus procesos**.

La extensibilidad admitida es de compilación:

1. una capacidad se incorpora como código fuente revisado;
2. respeta los límites de la arquitectura hexagonal;
3. incluye tests, i18n, límites de recursos y revisión de seguridad;
4. se distribuye dentro de un artefacto oficial reproducible y firmado;
5. se actualiza o revierte junto con la versión completa del producto.

No se ofrecen directorio de plugins, instalación de módulos, activación,
desactivación ni eliminación individual. Esta ausencia es deliberada y no una
función pendiente.

También quedan prohibidos como atajo:

- `plugin`/`plugin.Open` de la biblioteca estándar de Go;
- `hashicorp/go-plugin`;
- intérpretes Go como `yaegi`;
- runtimes WebAssembly embebidos como `wazero`;
- intérpretes o enlaces Lua.

WebAssembly tampoco se considera automáticamente seguro: necesitaría definir
capacidades, límites, persistencia, actualizaciones, procedencia y un protocolo
que impida alcanzar material criptográfico. No existe hoy ese diseño.

## Lo que no es un plugin de aplicación

Esta decisión no prohíbe componentes controlados por el producto que ya tienen
un contrato y una frontera específicos:

- plugins y módulos QML desplegados por Qt;
- módulos criptográficos PKCS#11 configurados expresamente para hardware;
- extensiones de navegador y `nativehost`, que se ejecutan fuera del proceso
  criptográfico y usan protocolos acotados;
- adaptadores compilados dentro de los binarios oficiales.

Estos componentes mantienen sus propios gates y modelos de amenaza. En
particular, aceptar un driver PKCS#11 no autoriza un sistema general de
extensiones.

## Control automático

`scripts/ci/check_no_runtime_plugins.py` inspecciona código, módulos Go y
manifiestos nativos. Bloquea las APIs y frameworks anteriores sin confundir
Qt/QML ni PKCS#11. Se ejecuta:

- con sus regresiones en el workflow general de tests;
- como paso explícito de CI;
- dentro de `scripts/release/check-release-gate.sh`.

Cambiar el gate exige una ADR posterior que sustituya expresamente esta
decisión.

## Alternativas descartadas

### Replicar el gestor V1.9

Descartada porque la firma del paquete prueba integridad, no aislamiento. Un
JAR válido seguiría teniendo acceso al proceso completo, y la aceptación de
JAR sin firma o inválidos no cumple un baseline de producto seguro.

### Plugins Go nativos

Descartados por ABI frágil, acoplamiento exacto de toolchain y, sobre todo,
ejecución con los privilegios completos del proceso.

### RPC o sidecar genérico de plugins

Descartado para el alcance actual. Reduce el impacto de memoria, pero abre un
ecosistema de instalación, autenticación, actualización, permisos y soporte
que no se justifica para las utilidades conocidas.

### WASM o lenguaje interpretado

Descartado mientras no exista un caso de negocio concreto y una ADR con
capabilities deny-by-default, límites de CPU/memoria, procedencia administrada
y ausencia demostrable de acceso a claves o documentos fuera de la petición.

## Consecuencias

- V2 mantiene una diferencia intencional frente a V1.9, documentada como mejora
  de seguridad y mantenibilidad.
- Las utilidades portadas son funciones normales, homogéneas y traducibles del
  producto.
- No hay textos ni ciclo de vida de plugins que localizar; las superficies
  integradas sí usan el catálogo i18n común.
- Una futura necesidad de terceros debe tratarse como una iniciativa nueva,
  preferentemente fuera de proceso y con protocolo mínimo, no como reapertura
  implícita de `T096`.

## Referencias

- [Extensibilidad y utilidades integradas](EXTENSIBILIDAD_Y_UTILIDADES_INTEGRADAS.md)
- [Arquitectura](ARCHITECTURE.md)
