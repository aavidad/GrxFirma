<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Corpus sintético del dictamen v2

Cada carpeta contiene `original.pdf` sin firmar, `firmado.pdf` y `dictamen-esperado.json` completo. Los casos 01 a 08 corresponden, en el mismo orden, a los casos de referencia que pidió la aplicación integradora. La prueba `TestDictamenV2_CorpusSintetico` compara los dictámenes completos salvo los instantes de comprobación, y además fija los estados esenciales de cada caso.

| Caso | Estado | Evidencia que decide |
|---|---|---|
| 01, una firma | `valida` | CRL local buena, original exacto |
| 02, dos firmas | `valida` | Dos actualizaciones de firma permitidas |
| 03, página entre firmas | `no_valida` | `cambiosDesdeAnterior=no_permitidos` en la segunda |
| 04, página posterior | `no_valida` | `cambiosPosteriores=no_permitidos` |
| 05, DocMDP nivel 1 | `no_valida` | La segunda firma infringe el nivel 1 |
| 06, original ajeno | `indeterminada` | `vinculoOriginal=no_acreditado` |
| 07, ByteRange incompleto | `no_valida` | `cubreDocumentoCompletoHastaAqui=false` |
| 08, firmante B revocado | `no_valida` | CRL sintética revoca solo B |
| 09, xref stream | `indeterminada` | Revisión estructural posterior sin firma |
| 10, xref híbrido | `indeterminada` | Revisión híbrida posterior sin firma |
| 11, FieldMDP `/All` | `no_valida` | Bloquea la segunda firma |
| 12, DSS vacío | `valida` | DSS añadido sin otros objetos ni material ilegible |
| 13, DocTimeStamp mal formado | `indeterminada` | Sello sin campo ni token autenticable |
| 14, DocMDP nivel 2 | `valida` | Permite una firma de aprobación posterior |
| 15, DocMDP nivel 3 | `valida` | También permite una firma de aprobación posterior |
| 16, DSS válido | `valida` | Certificado, CRL, OCSP y VRI DER; `dss_anadido` |
| 17, DSS con objeto ajeno | `indeterminada` | Anotación no perteneciente al DSS; `no_comprobados` |
| 18, DocTimeStamp válido | `valida` | Token RFC 3161 sobre su ByteRange, TSA anclada y CRL local |
| 19, impronta ajena | `no_valida` | Token firmado con `messageImprint` distinto |
| 20, LTA completo | `valida` | Firma, DSS válido y sello de documento posterior |

`pki/*.key.pem` son **claves de prueba públicas**: nunca deben utilizarse para datos reales. La PKI, certificados y CRL son sintéticos, no contienen identidades reales y no requieren red. La TSA del generador responde mediante transporte HTTP en memoria, sin sockets. El generador está en [generar_dictamen_v2.go](../../scripts/generar_dictamen_v2.go); se ejecuta desde la raíz con `GOFLAGS=-buildvcs=false go run scripts/generar_dictamen_v2.go`. Las claves se conservan para repetir la generación; el generador vuelve a emitir certificados, CRL, PDF y dictámenes esperados. Los PDF generados pueden variar en bytes por atributos temporales del CMS, por lo que el generador actualiza las huellas y los resultados esperados conjuntamente.
