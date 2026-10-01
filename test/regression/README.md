<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

## Corpus de regresión

`fixtures/v1/samples/2.pdf` es un documento de prueba propio de 15 páginas.
Contiene texto ficticio y un gráfico de píxeles geométricos generado por código;
no incorpora capturas, datos de personas ni enlaces externos. Para regenerarlo de
forma determinista:

```bash
python3 test/regression/fixtures/v1/generar_pdf_prueba.py
qpdf --check test/regression/fixtures/v1/samples/2.pdf
```

El manifiesto enumera los documentos y las firmas CAdES/XAdES que acompañan al
árbol público. Las firmas PAdES anteriores del PDF retirado no son válidas para
el nuevo documento y se han eliminado.

### Regenerar las muestras firmadas

El responsable, con su certificado **FNMT de pruebas autorizado**, debe:

1. Firmar `fixtures/v1/samples/2.pdf` con formato PAdES y guardar el resultado
   como `fixtures/v1/samples/2_signed.pdf`.
2. Verificar ese resultado con `pdfsig` y `qpdf --check`.
3. Si se necesita la prueba de doble firma, cofirmar `2_signed.pdf` y guardar
   `2_signed_signed.pdf`; verificar ambas firmas con `pdfsig`.
4. Añadir al manifiesto los ficheros firmados incorporados al repositorio.

Las muestras `2_txt_signed.csig` y `2_txt_signed_signed.csig` representan una
firma CAdES de `2.txt` y otra firma de la primera muestra; las
`2_xml_signed.xsig` y `2_xml_signed_signed.xsig` siguen esa secuencia en XAdES
con `2.xml`.
Si se sustituyen, repetir esa secuencia con el certificado FNMT de pruebas,
comprobarlas con las pruebas de regresión y actualizar el manifiesto. La prueba
de `2_xml_signed.xsig` detecta expresamente un formato histórico no estándar:
actualizar también ese caso si se sustituye la muestra.

No incorporar al repositorio el contenedor P12/PFX, la clave privada ni copias
temporales de estos materiales. Las pruebas que necesitan `2_signed.pdf`
indican mediante *skip* cómo completarlo cuando falta.

### Ejecutar las pruebas

```bash
go test ./test/regression/...
go test -tags regression_v1 ./test/regression/...
```

La segunda orden compara las muestras no firmadas que proceden de una fuente
externa solo si se configura `GRXFIRMA_V1_SAMPLES`. El PDF propio `2.pdf` queda
fuera de esa comparación.
