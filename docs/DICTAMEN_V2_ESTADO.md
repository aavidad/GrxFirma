<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Dictamen de verificación v2

**Contrato:** `autofirmav2.dictamen-verificacion.v2` (esquema 2.0.0).
**Esquema cerrado:** [dictamen-verificacion-v2.schema.json](schema/dictamen-verificacion-v2.schema.json).
**SHA-256 del fichero de esquema:** `e57e5cc0f47ad1ffdde30ff5ae8df3e0f1f93671f4714416ddce425e6142db6e`.

`POST /v2/verify` y `POST /verify` con `contrato_solicitado` v2 devuelven la misma envoltura `ok`, `valid`, `reason`, `details`, `signers`, `dictamen`. Sin el selector, `/verify` conserva la v1. VEC debe leer `dictamen`, exigir el valor exacto de `contrato` y recalcular su decisión. El campo PDF `campo` se omite deliberadamente: puede contener datos personales y no está autenticado por sí solo. Los atributos `asunto`, `emisor` y `serie` proceden del certificado de cada firmante.

Petición mínima:

```json
{"content_base64":"<PDF>","contrato_solicitado":"autofirmav2.dictamen-verificacion.v2"}
```

`original_content_base64` es opcional. Si se aporta, debe ser exactamente la primera revisión sin firma, y la primera actualización debe añadir solo la primera firma. Las dos huellas de cada firma tienen significados distintos: `revisionHuellaSHA256` resume **todos** los bytes `pdf[:revisionLongitud]`; `contenidoFirmadoHuellaSHA256` resume la concatenación de segmentos de `byteRange`, sin `/Contents`. `cubreDocumentoCompletoHastaAqui` es falso si el `ByteRange` acaba antes del `%%EOF` de su propia revisión.

La lectura abre el xref efectivo de cada revisión y sigue `/Prev`, incluso al alternar tabla, flujo xref y actualización híbrida `/XRefStm`. Compara entradas activas, generaciones, objetos liberados, raíz, `/ID`, `/Encrypt`, `/Info` y cuerpos asociados. Solo clasifica `permitidos` las actualizaciones de firma cuyo campo `/FT /Sig`, `/V`, widget y `ByteRange` están ligados a la firma activa y cuyo catálogo cambia dentro de la lista inspeccionada. Detecta contenido de página modificado antes de otra firma o después de la última. DocMDP nivel 1 impide una firma posterior; los niveles 2 y 3 se tratan según los cambios realmente clasificados. FieldMDP `/All` impide modificar otro campo. FieldMDP con listas de campos, apariencia compleja y objetos comprimidos quedan `no_comprobados` cuando no se pueden demostrar.

Una actualización posterior de DSS/VRI recibe `permitidos` y `dss_anadido` solo cuando el catálogo cambia exclusivamente en `/DSS`, `/Extensions` o `/Version`, y todas las entradas xref nuevas o cambiadas pertenecen al catálogo, DSS, VRI, sus arrays y flujos de certificados, CRL u OCSP, o al propio xref. Se comprueba que los flujos sean DER de su categoría y que no se reescriban páginas, contenido, campos ni anotaciones. Se admiten como máximo 128 flujos, 1 MiB decodificado por flujo, 8 MiB en conjunto y 128 entradas VRI. Un DSS con material ilegible u objetos ajenos queda `no_comprobados`.

Un campo de sello `/ETSI.RFC3161` aparece en `firmas` con `tipoFirma=sello_tiempo_documento`, orden, `byteRange` y ambas huellas. El token se verifica sobre el contenido de su `ByteRange`: firma CMS, algoritmo e impronta. La TSA usa las anclas, la vigencia y la revocación locales del resto de firmas. Su cambio se promociona a `permitidos` solo con todos esos aspectos concluyentes; la falta de confianza o de revocación lo deja `indeterminada`, y una impronta falsa produce `no_valida`. Un sello sin token autenticable nunca se acepta.

El agregado es estricto: cada firma necesita integridad, cadena, vigencia y revocación concluyentes; todo cambio debe ser `ninguno` o `permitidos`; y el original, si se aportó, debe quedar `acreditado`. `no_permitidos` produce `no_valida`, `no_comprobados` o cualquier comprobación necesaria pendiente produce `indeterminada`. Si un CMS no puede comprobarse, su registro conserva `orden`, `byteRange` y revisión con `integridad=parcial`; se omite la huella del certificado desconocido y el agregado queda `indeterminada`. El atributo `valid` de la envoltura sigue exactamente `dictamen.estado == "valida"`.

Límites por defecto: cuerpo HTTP 100 MiB (413 si se supera), PDF decodificado 72 MiB, 20 firmas, 20 revisiones y 50 000 entradas xref por revisión. El servicio permite configurar máximos menores o mayores hasta los topes de 20 firmas, 20 revisiones y 100 MiB de PDF mediante `-verificacion-v2-max-firmas`, `-verificacion-v2-max-revisiones`, `-verificacion-v2-max-pdf-mib` y `-verificacion-v2-max-cuerpo-mib` (hasta 150 MiB). Superar firmas, revisiones o PDF produce `indeterminada/limite_excedido`; un cuerpo grande se rechaza antes con HTTP 413. JSON inválido, claves duplicadas, claves desconocidas y contratos desconocidos se rechazan con HTTP 400. La profundidad se limita en el decodificador y en el esquema de respuesta. El modo de solo verificación usa TLS 1.3 y Bearer; no publica mTLS propio.

El [corpus sintético](../testdata/dictamen-v2/README.md) contiene los ocho casos pedidos por VEC, su PDF original y dictamen esperado, más xref stream, híbrido, FieldMDP, DSS, DocTimeStamp, DocMDP niveles 2 y 3 y cinco casos LT/LTA. Incluye una PKI de prueba con dos firmantes, una TSA y CRL buena/revocada. Los certificados y CRL del corpus tienen vigencia de 2025 a 2035: fuera de ella cambian los aspectos temporales. Las mismas entradas, anclas, CRL, política, versión y fecha de referencia producen el mismo dictamen.
