<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR — Modelo de seguridad mobile: Zero Server

Fecha: 2026-03-18

Última revisión: 2026-07-29

Estado: aceptado como principio arquitectónico

Autoría: Alberto Avidad Fernández

## Contexto

El protocolo de escritorio heredado puede necesitar un listener local para
integrarse con navegadores. En mobile, abrir un puerto TCP o UDP ampliaría la
superficie de ataque a otras aplicaciones del dispositivo. El producto mobile
debe usar los canales de intercambio mediados por el sistema operativo.

## Decisión

GrxFirma mobile no abre puertos TCP/UDP ni implementa servidores locales.
Android usa intents y URI `content://` con permisos SAF de alcance transitorio.
La APK actual:

- acepta un documento mediante `ACTION_SEND` o `ACTION_VIEW`;
- permite escoger entrada y destino mediante SAF;
- no declara un esquema `afirma://`, App Links ni `SEND_MULTIPLE`;
- no devuelve todavía la firma a otra aplicación mediante `setResult()`;
- no solicita `INTERNET` y, por tanto, no ejecuta protocolo trifásico ni
  comprobaciones de revocación en línea.

Un intent explícito o una concesión de URI reducen la exposición, pero no
justifican afirmar que cualquier custom scheme futuro sea exclusivo o
ininterceptable. Una integración app-a-app deberá definir autenticación,
destinatario, límites y devolución del resultado en un ADR específico.

## Controles obligatorios

| Control | Implementación Android actual |
|---|---|
| Sin listener mobile | Prohibidos `net.Listen`, `http.ListenAndServe` y equivalentes |
| Entrada documental | URI `content://`; sin resolver rutas de almacenamiento compartido |
| Permisos SAF | Solo transitorios; al arrancar se liberan concesiones persistentes heredadas |
| Temporales PAdES | Directorio privado bajo `noBackupFilesDir`, permisos `0700` y limpieza |
| Logs | Nunca contienen documento, firma, PKCS#12 ni contraseña |
| Sesión | Una identidad PKCS#12 en memoria; reemplazo, `clearSession()` o fin del proceso |
| Capturas | `FLAG_SECURE` en producción |
| Red | Sin permiso `INTERNET` y sin actualizador propio |

## Identidad y memoria

La implementación Android actual importa un PKCS#12 y mantiene su clave privada
en el heap del proceso Go durante la sesión. No usa Android Keystore, hardware
TEE ni biometría, y la documentación o la interfaz no deben afirmar lo
contrario.

El adaptador reduce copias y ventanas de exposición:

- el PKCS#12 cruza Kotlin/Go como `byte[]`, sin una copia Base64;
- los buffers mutables controlados se sobrescriben al terminar;
- la acción visible «Olvidar certificado» llama a `clearSession()`;
- el resultado pendiente se sobrescribe al guardarlo o descartarlo y, al
  descartarlo, también se limpia la identidad de sesión;
- no se persisten documentos, certificados ni secretos en preferencias.

`gobind` obliga actualmente a entregar la contraseña como `String`. Como las
cadenas JVM son inmutables y el recolector gestiona el heap, estos controles son
de mejor esfuerzo: no equivalen a una garantía de borrado físico completo.

Una futura integración con Android Keystore/TEE será una capacidad distinta:
deberá operar con una referencia no exportable real y aportar pruebas en
dispositivo. La abstracción `ports.SigningKey` por sí sola no acredita TEE.

## Flujo Android implementado

```text
Selector SAF o aplicación remitente
        |
        | content:// + permiso temporal
        v
GrxFirma
        |
        | importar PKCS#12 en sesión
        | confirmar acción nativa
        | firmar/verificar localmente
        v
Selector SAF de destino
```

Mientras una operación está en curso o existe un resultado pendiente de
guardar, la actividad rechaza nuevos intents y cambios de selección. Esto evita
mezclar el documento o la sesión activa con una segunda solicitud.

## Semántica de verificación

La integridad criptográfica no equivale por sí sola a confianza completa. La UI
presenta por separado:

1. integridad de la firma;
2. vigencia/estado del certificado;
3. confianza de la cadena;
4. revocación y disponibilidad de la comprobación.

El núcleo mobile no usa actualmente las anclas de confianza del sistema y solo
evalúa evidencias de revocación embebidas. Por ello la confianza se informa
como desconocida y la pantalla nunca resume el resultado como «firma válida»
global.

## Tráfico saliente futuro

Un cliente trifásico HTTPS saliente no contradice por definición Zero Server,
porque no crea un listener. Sin embargo, no forma parte de la APK Android
actual: añadirlo exigiría permiso de red, política TLS, autenticación, modelo de
amenazas, consentimiento y pruebas propias. Esta ADR no lo autoriza ni lo
declara implementado.

## Relación con escritorio

T063–T065 (WebSocket y REST local) son capacidades exclusivas de escritorio por
compatibilidad. No deben entrar en el camino mobile.

## Consecuencias

- El producto Android mantiene una superficie local pequeña y no necesita
  servicios de fondo ni permiso de red.
- La interoperabilidad web/app-a-app avanzada queda fuera hasta disponer de un
  protocolo explícito y verificable.
- Las afirmaciones de Keystore, TEE, biometría o revocación en línea solo podrán
  incorporarse cuando exista implementación real y evidencia de prueba.
