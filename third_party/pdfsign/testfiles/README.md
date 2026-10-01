# PDF de prueba sintéticos

`testfile12.pdf`, `testfile14.pdf` y `testfile16.pdf` son documentos propios con texto ficticio, sin metadatos de personas. Conservan tres variantes usadas por las pruebas: referencias comprimidas y tabla de referencias, con una o varias páginas.

Para regenerarlos, instala `qpdf` y ejecuta `python3 generate_synthetic.py` desde cualquier directorio. El script produce los tres PDF sin conexión de red. Los demás PDF del directorio cubren formatos o firmas distintos.
