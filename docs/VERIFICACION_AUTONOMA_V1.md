<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Verificación autónoma v1

**Estado:** dictamen explícito, anclas y CRL locales, verificación de sellos
RFC 3161 y modo de servicio de solo verificación implementados con pruebas
automáticas sobre material sintético. No se ha desplegado ni probado con
certificados, CRL o sellos de prestadores reales.

Principio: la verificación no depende de otras aplicaciones ni servicios. Las
anclas de confianza y las CRL son ficheros locales fijados por configuración;
el almacén raíz del sistema no cuenta como ancla. OCSP y validación remota de
sellos existen solo como puntos de extensión, desactivados por defecto.

## Contrato de respuesta

`POST /verify` conserva todos los campos anteriores (`ok`, `valid`, `reason`,
`details`, `signers`, `result`) y añade `dictamen`, con contrato
`autofirmav2.dictamen-verificacion.v1`. Solo `estado=valida` con
`motivo=verificada` acredita la firma.

```json
"dictamen": {
  "contrato": "autofirmav2.dictamen-verificacion.v1",
  "estado": "valida",
  "motivo": "verificada",
  "formato": "PAdES",
  "comprobadoEn": "2026-09-25T10:00:00Z",
  "integridad": {"estado": "valida"},
  "cadena": {"estado": "valida", "fuente": "anclas_locales"},
  "certificado": {"estado": "vigente", "fecha": "2028-01-31T23:59:59Z"},
  "revocacion": {"estado": "vigente", "fuente": "crl_local", "fecha": "2026-09-25T10:00:00Z"},
  "selloTiempo": {"estado": "no_presente"},
  "vinculoOriginal": {"estado": "acreditado", "fuente": "pades_revision"},
  "huellaFirmadoSHA256": "…64 hex…",
  "huellaOriginalSHA256": "…64 hex…",
  "certificadoHuellaSHA256": "…64 hex…",
  "firmantes": [{
    "certificadoHuellaSHA256": "…", "serie": "…", "asunto": "…", "emisor": "…",
    "cadena": {…}, "certificado": {…}, "revocacion": {…}, "selloTiempo": {…}
  }],
  "extensiones": {"revocacionRemota": "desactivada", "selloTiempoRemoto": "desactivada"}
}
```

Cada aspecto es `{estado, motivo?, fuente?, fecha?}` con valores cerrados:

| Aspecto | Estados |
|---|---|
| `integridad` | `valida`, `parcial` (contenido no cubierto), `no_valida` |
| `cadena` | `valida`, `no_valida`, `no_comprobada` (sin anclas o sin ruta hasta ellas) |
| `certificado` | `vigente`, `no_vigente`, `uso_no_permitido`, `no_comprobado` |
| `revocacion` | `vigente`, `revocado`, `no_comprobada` (con `motivo`) |
| `selloTiempo` | `no_presente`, `valido`, `no_valido`, `no_comprobado` |
| `vinculoOriginal` | `acreditado`, `no_acreditado`, `no_aportado` |

Veredicto global, por precedencia:

1. `no_valida`: `integridad_no_valida`, `certificado_no_valido` (caducado, sin
   uso de firma o revocado), `confianza_no_valida` (ruta defectuosa).
2. `indeterminada`: `integridad_parcial`, `firmante_no_identificado`,
   `certificado_no_acreditado`, `confianza_no_acreditada`,
   `revocacion_no_acreditada`, `sello_tiempo_no_acreditado` (sello presente y
   `no_valido`), `vinculo_original_no_acreditado`.
3. `valida`/`verificada` en otro caso.

El sello no es obligatorio: `no_presente` y `no_comprobado` no impiden
`valida`. Varios firmantes se agregan por el peor estado.

## Qué comprueba

- **Cadena**: `x509.Verify` contra las anclas locales, con los certificados
  embebidos como intermedios. Sin ruta hasta un ancla: `no_comprobada`
  (equivale a NO_CERTIFICATE_CHAIN_FOUND); ruta defectuosa: `no_valida`.
- **Revocación**: cada certificado de la ruta salvo el ancla. Fuentes, en
  orden: CRL del directorio local, CRL/OCSP embebidas en la firma (CAdES/PAdES
  LT) y, solo si se compone en código, la extensión remota. Toda evidencia se
  autentica contra el emisor y se exige vigente (`thisUpdate`/`nextUpdate`).
  Se descartan CRL delta, indirectas, restringidas por motivos o de otra
  partición (IDP que no coincide con los puntos de distribución del
  certificado). Revocado prevalece; `vigente` exige evidencia para todos.
- **Sello (CMS)**: firma del token, algoritmo SHA-256/384/512,
  `messageImprint` sobre el valor de firma, instante no futuro, TSA con uso
  `timeStamping` y ruta hasta las anclas locales en el instante del sello.
  XAdES-T se declara `no_comprobado` (`formato_sin_evaluacion_de_sello`).
- **Vínculo**: CMS, si la huella del contenido cuyo `messageDigest` se
  verificó coincide con el original (encapsulado o separado); PAdES, si el
  original es un prefijo exacto del PDF firmado y alguna firma cubre todos sus
  bytes (revisión incremental). XML: `no_acreditado`.
- **Huellas de eco**: SHA-256 del firmado y del original recibidos, y del
  certificado firmante cuando hay uno solo.

Vigencia, cadena y revocación se evalúan en el instante de la comprobación;
todavía no se aplica validación a largo plazo con prueba de existencia.

## Configuración

```sh
# Servicio de solo verificación: v1 por defecto y v2 optativa.
GRXFIRMA_REST_TOKEN=<secreto> grxfirma -rest-solo-verificacion \
  -verificacion-anclas ANCLAS \
  -verificacion-crl CRL \
  -direccion-rest 127.0.0.1:63118
```

- `-verificacion-anclas` (alias `-verify-anchors`): fichero PEM con uno o
  varios certificados, DER, o directorio no recursivo con `.pem`, `.crt`,
  `.cer`, `.der`. Obligatorio en modo solo verificación.
- `-verificacion-crl` (alias `-verify-crl-dir`): directorio con `.crl`, `.pem`
  o `.der`. Se relee en cada petición (con caché por fecha y tamaño), así que
  basta con sustituir los ficheros para actualizarlas. Debe contener la CRL de
  cada CA de la ruta, incluida la ARL de la raíz.
- `-rest-solo-verificacion` (alias `-rest-verify-only`): no carga
  certificados, claves ni configuración de firma; sin rutas de fichero, sin
  autenticación por certificado y sin ninguna conexión saliente. El listener
  exige TLS 1.3 y cada petición a `/health` o `/verify` exige token Bearer.
  Si no se aporta un token en loopback, se genera uno y se entrega mediante
  un fichero privado. Las rutas no publicadas responden 404 sin redirección.
  `POST /v2/verify` publica el [dictamen v2](DICTAMEN_V2_ESTADO.md).
  Fuera de loopback se exige un token configurado explícitamente.

En el modo REST completo (`-rest`) las mismas banderas sustituyen el almacén
del sistema por las anclas locales y añaden el dictamen; sin ellas, el
dictamen informa `cadena: no_comprobada / sin_anclas_locales`.

## Puntos de extensión (desactivados)

`verificacionlocal.Configuracion` admite `RevocacionRemota`
(`ports.RevocationProvider`, compatible con `revocationclient`) y
`SelloRemoto` (`ports.ValidadorSelloTiempoRemoto`). No hay banderas para
activarlos: requieren composición explícita en código y el dictamen declara
en `extensiones` si intervinieron.

## Cambio en el resultado heredado

`result.certificate` ya no es `valid` sin revocación concluyente: queda
`unknown` si no se comprobó y `warning` si no concluyó. Antes, una consulta
no concluyente se reescribía como `valid` al cerrar la comprobación de la
cadena. El efecto en cada entrada es:

| Entrada | Resultado visible |
|---|---|
| Escritorio (IPC, WinUI y página web) | `certificate.status` pasa a `unknown` o `warning`; la página web ya no muestra «confiable» cuando falta acreditación. |
| CLI | `resultado.certificado.estado` muestra el estado no concluyente. |
| REST normal | `result.certificate.status` muestra el estado no concluyente; `dictamen.estado` queda `indeterminada` cuando falta revocación. |
| Native host | `result.certificateStatus` expone ahora ese estado. |
| Móvil | `certificate_status` recibe `unknown` o `warning`; iOS no muestra el resultado en verde si falta acreditación. |

El campo heredado `valid` sigue expresando la integridad técnica del formato;
puede ser `true` con revocación no acreditada. Para tomar decisiones de
confianza se debe usar `dictamen.estado` o comprobar todos los aspectos.
