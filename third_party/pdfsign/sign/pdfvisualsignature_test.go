package sign

import (
	"os"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

func TestVisualSignature(t *testing.T) {
	input_file, err := os.Open("../testfiles/testfile20.pdf")
	if err != nil {
		t.Errorf("Failed to load test PDF")
		return
	}

	finfo, err := input_file.Stat()
	if err != nil {
		t.Errorf("Failed to load test PDF")
		return
	}
	size := finfo.Size()

	rdr, err := pdf.NewReader(input_file, size)
	if err != nil {
		t.Errorf("Failed to load test PDF")
		return
	}

	timezone, _ := time.LoadLocation("Europe/Tallinn")
	now := time.Date(2017, 9, 23, 14, 39, 0, 0, timezone)

	sign_data := SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere",
				Reason:      "Test",
				ContactInfo: "None",
				Date:        now,
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
	}

	sign_data.objectId = uint32(rdr.XrefInformation.ItemCount) + 3

	context := SignContext{
		PDFReader: rdr,
		InputFile: input_file,
		SignData:  sign_data,
	}

	expected_visual_signature := "<<\n  /Type /Annot\n  /Subtype /Widget\n  /Rect [0 0 0 0]\n  /P 4 0 R\n  /F 132\n  /NM (SignatureWidget 1)\n  /M (D:20170923143900+03'00')\n  /FT /Sig\n  /T (Signature 1)\n  /TU (Firma digital | Firmante: John Doe | Motivo: Test | Ubicacion: Somewhere | Fecha: 2017-09-23 14:39)\n  /Contents (Firma digital | Firmante: John Doe | Motivo: Test | Ubicacion: Somewhere | Fecha: 2017-09-23 14:39)\n  /V 13 0 R\n>>\n"

	visual_signature, _, err := context.createVisualSignature(false, 1, [4]float64{0, 0, 0, 0}, 1)
	if err != nil {
		t.Errorf("%s", err.Error())
		return
	}

	if string(visual_signature) != expected_visual_signature {
		t.Errorf("Visual signature mismatch, expected\n%q\nbut got\n%q", expected_visual_signature, visual_signature)
	}
}
