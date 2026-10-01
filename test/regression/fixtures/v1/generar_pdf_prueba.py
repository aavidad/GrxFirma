#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Genera el PDF multipágina sintético de regresión sin dependencias externas."""

from pathlib import Path
import sys
import zlib


DESTINO = Path(__file__).resolve().parent / "samples" / "2.pdf"


def objeto_stream(datos: bytes, atributos: str = "") -> bytes:
    comprimido = zlib.compress(datos, level=9)
    return (
        f"<< {atributos} /Length {len(comprimido)} /Filter /FlateDecode >>\n"
        .encode("ascii")
        + b"stream\n"
        + comprimido
        + b"\nendstream"
    )


def imagen_sintetica(variante: int) -> bytes:
    """Dibuja píxeles geométricos propios; no contiene fotos ni metadatos."""
    pixeles = bytearray()
    for y in range(72):
        for x in range(108):
            cuadro = (x // (18 + variante * 9) + y // 18) % 2
            circulo = (x - 54 + variante * 12) ** 2 + (y - 36) ** 2 < 22**2
            if circulo:
                pixeles.extend((242, 175 - variante * 45, 73 + variante * 70))
            elif cuadro:
                pixeles.extend((41 + variante * 75, 109, 143))
            else:
                pixeles.extend((221, 237, 236))
    return bytes(pixeles)


def texto_pdf(valor: str) -> str:
    return valor.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")


def generar() -> bytes:
    objetos: list[bytes] = [b""]

    def anadir(contenido: bytes) -> int:
        objetos.append(contenido)
        return len(objetos) - 1

    catalogo = anadir(b"")
    paginas = anadir(b"")
    fuente = anadir(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
    imagen_1 = anadir(
        objeto_stream(
            imagen_sintetica(0),
            "/Type /XObject /Subtype /Image /Width 108 /Height 72 "
            "/ColorSpace /DeviceRGB /BitsPerComponent 8",
        )
    )
    imagen_2 = anadir(
        objeto_stream(
            imagen_sintetica(1),
            "/Type /XObject /Subtype /Image /Width 108 /Height 72 "
            "/ColorSpace /DeviceRGB /BitsPerComponent 8",
        )
    )
    ids_paginas = []
    for numero in range(1, 16):
        ancho, alto = (594, 816) if numero == 2 else (612, 792)
        lineas = [
            "GrxFirma - documento sintetico de prueba",
            f"Pagina {numero} de 15. Serie ficticia: ENSAYO-{numero:02d}.",
            "Contenido creado para pruebas de firma y visualizacion de PDF.",
            "El grafico muestra solo formas y colores generados por codigo.",
            "No representa personas, empresas, lugares ni sistemas reales.",
        ]
        instrucciones = [
            "q 230 0 0 153 55 385 cm /Im1 Do Q",
            "q 230 0 0 153 320 385 cm /Im2 Do Q",
        ]
        for indice, linea in enumerate(lineas):
            y = alto - 76 - indice * 28
            instrucciones.append(
                f"BT /F1 13 Tf 55 {y} Td ({texto_pdf(linea)}) Tj ET"
            )
        instrucciones.append(
            f"BT /F1 10 Tf 55 65 Td (Muestra {numero:02d} - uso exclusivo de pruebas) Tj ET"
        )
        contenido = anadir(objeto_stream("\n".join(instrucciones).encode("ascii")))
        pagina = anadir(
            (
                f"<< /Type /Page /Parent {paginas} 0 R /MediaBox [0 0 {ancho} {alto}] "
                f"/Resources << /Font << /F1 {fuente} 0 R >> "
                f"/XObject << /Im1 {imagen_1} 0 R /Im2 {imagen_2} 0 R >> >> "
                f"/Contents {contenido} 0 R >>"
            ).encode("ascii")
        )
        ids_paginas.append(pagina)

    objetos[catalogo] = f"<< /Type /Catalog /Pages {paginas} 0 R >>".encode()
    hijos = " ".join(f"{indice} 0 R" for indice in ids_paginas)
    objetos[paginas] = (
        f"<< /Type /Pages /Count {len(ids_paginas)} /Kids [{hijos}] >>"
    ).encode()

    salida = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = [0]
    for indice, contenido in enumerate(objetos[1:], 1):
        offsets.append(len(salida))
        salida.extend(f"{indice} 0 obj\n".encode())
        salida.extend(contenido)
        salida.extend(b"\nendobj\n")
    posicion_xref = len(salida)
    salida.extend(f"xref\n0 {len(objetos)}\n0000000000 65535 f \n".encode())
    for offset in offsets[1:]:
        salida.extend(f"{offset:010d} 00000 n \n".encode())
    salida.extend(
        (
            f"trailer\n<< /Size {len(objetos)} /Root {catalogo} 0 R >>\n"
            f"startxref\n{posicion_xref}\n%%EOF\n"
        ).encode()
    )
    return bytes(salida)


if __name__ == "__main__":
    destino = Path(sys.argv[1]) if len(sys.argv) > 1 else DESTINO
    destino.write_bytes(generar())
