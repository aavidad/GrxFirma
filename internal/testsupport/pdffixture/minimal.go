// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package pdffixture builds small, structurally valid PDF inputs for tests and
// development sample generators.
package pdffixture

import (
	"bytes"
	"fmt"
)

// Marker is embedded in Minimal so callers can assert that incremental
// signing preserved the original document.
const Marker = "% GrxFirma valid PDF fixture\n"

// Minimal returns a valid one-page PDF 1.4 document with an empty content
// stream and a classic cross-reference table.
func Minimal() []byte {
	var pdf bytes.Buffer
	offsets := make([]int, 5)
	writeObject := func(number int, body string) {
		offsets[number] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", number, body)
	}

	pdf.WriteString("%PDF-1.4\n")
	pdf.WriteString("%\xC2\xE2\xCF\xD3\n")
	pdf.WriteString(Marker)
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Count 1 /Kids [3 0 R] >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << >> /Contents 4 0 R >>")
	writeObject(4, "<< /Length 0 >>\nstream\n\nendstream")

	xref := pdf.Len()
	pdf.WriteString("xref\n0 5\n")
	pdf.WriteString("0000000000 65535 f \n")
	for object := 1; object <= 4; object++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[object])
	}
	pdf.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\n")
	fmt.Fprintf(&pdf, "startxref\n%d\n%%%%EOF\n", xref)
	return pdf.Bytes()
}
