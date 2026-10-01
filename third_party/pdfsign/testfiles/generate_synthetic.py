#!/usr/bin/env python3
# Copyright (c) 2026 Alberto Avidad Fernández.
# SPDX-License-Identifier: BSD-2-Clause

"""Regenera los PDF de prueba propios con contenido ficticio y sin metadatos personales."""
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent


def pdf_document(page_count, *, outline=False):
    objects = [b'<< /Type /Catalog /Pages 2 0 R' + (b' /Outlines 3 0 R /Version /1.5' if outline else b'') + b' >>']
    pages_start = 4 if outline else 3
    kids = b' '.join(f'{pages_start + 2 * i} 0 R'.encode() for i in range(page_count))
    objects.append(b'<< /Type /Pages /Kids [' + kids + b'] /Count ' + str(page_count).encode() + b' >>')
    if outline:
        objects.append(b'<< /Type /Outlines /Count 0 >>')
    for i in range(page_count):
        page_id = pages_start + 2 * i
        content_id = page_id + 1
        objects.append(f'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 {pages_start + 2 * page_count} 0 R >> >> /Contents {content_id} 0 R >>'.encode())
        message = f'Pruebas Ficticio - pagina {i + 1}'.encode()
        stream = b'BT /F1 18 Tf 72 760 Td (' + message + b') Tj ET\n'
        objects.append(b'<< /Length ' + str(len(stream)).encode() + b' >>\nstream\n' + stream + b'endstream')
    objects.append(b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>')
    data = bytearray(b'%PDF-1.5\n%\xe2\xe3\xcf\xd3\n')
    offsets = [0]
    for index, body in enumerate(objects, 1):
        offsets.append(len(data))
        data.extend(f'{index} 0 obj\n'.encode() + body + b'\nendobj\n')
    start = len(data)
    data.extend(f'xref\n0 {len(offsets)}\n0000000000 65535 f\n'.encode())
    for offset in offsets[1:]:
        data.extend(f'{offset:010} 00000 n\n'.encode())
    data.extend(f'trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{start}\n%%EOF\n'.encode())
    return bytes(data)


if __name__ == '__main__':
    with tempfile.TemporaryDirectory() as directory:
        intermediate = Path(directory) / 'source.pdf'
        intermediate.write_bytes(pdf_document(2, outline=True))
        subprocess.run(['qpdf', '--object-streams=generate', str(intermediate), str(ROOT / 'testfile12.pdf')], check=True)
    (ROOT / 'testfile14.pdf').write_bytes(pdf_document(1))
    (ROOT / 'testfile16.pdf').write_bytes(pdf_document(7))
