package sign

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/revocation"
	"github.com/digitorus/pdfsign/verify"
	"github.com/mattetti/filebuffer"
)

func loadCertificateAndKey(tb testing.TB) (*x509.Certificate, *rsa.PrivateKey) {
	tb.Helper()

	pkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("generate test signing key: %v", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Pruebas Ficticio"},
			CommonName:   "Pruebas Ficticio",
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &pkey.PublicKey, pkey)
	if err != nil {
		tb.Fatalf("create test signing certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		tb.Fatalf("parse test signing certificate: %v", err)
	}

	return cert, pkey
}

func verifySignedFile(t *testing.T, tmpfile *os.File, originalFileName string) {
	_, err := verify.VerifyFile(tmpfile)
	if err != nil {
		t.Fatalf("%s: %s", tmpfile.Name(), err.Error())

		err2 := os.Rename(tmpfile.Name(), "../testfiles/failed/"+originalFileName)
		if err2 != nil {
			t.Error(err2)
		}
	}
}

func TestReaderCanReadPDF(t *testing.T) {
	files, err := os.ReadDir("../testfiles")
	if err != nil {
		t.Fatalf("%s", err.Error())
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".pdf" {
			continue
		}

		t.Run(f.Name(), func(st *testing.T) {
			st.Parallel()

			input_file, err := os.Open("../testfiles/" + f.Name())
			if err != nil {
				st.Fatalf("%s: %s", f.Name(), err.Error())
			}
			defer func() {
				if err := input_file.Close(); err != nil {
					st.Errorf("Failed to close input_file: %v", err)
				}
			}()

			finfo, err := input_file.Stat()
			if err != nil {
				st.Fatalf("%s: %s", f.Name(), err.Error())
			}
			size := finfo.Size()

			_, err = pdf.NewReader(input_file, size)
			if err != nil {
				st.Fatalf("%s: %s", f.Name(), err.Error())
			}
		})
	}
}

func TestMain(m *testing.M) {
	_ = os.RemoveAll("../testfiles/failed/")
	_ = os.MkdirAll("../testfiles/failed/", 0o777)
	_ = os.RemoveAll("../testfiles/success/")
	_ = os.MkdirAll("../testfiles/success/", 0o777)

	os.Exit(m.Run())
}

func testSignAllFiles(t *testing.T, baseSignData SignData) {
	files, err := os.ReadDir("../testfiles/")
	if err != nil {
		t.Fatalf("%s", err.Error())
	}

	cert, pkey := loadCertificateAndKey(t)

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".pdf" {
			continue
		}

		t.Run(f.Name(), func(st *testing.T) {
			ext := filepath.Ext(f.Name())
			outputName := f.Name()[:len(f.Name())-len(ext)] + "_" + t.Name() + ext
			var outputFile *os.File
			var err error

			if testing.Verbose() {
				outputFile, err = os.Create(filepath.Join("../testfiles/success", outputName))
			} else {
				outputFile, err = os.CreateTemp("", fmt.Sprintf("%s_%s_", t.Name(), f.Name()))
			}

			if err != nil {
				st.Fatalf("%s", err.Error())
			}

			defer func() {
				if err := outputFile.Close(); err != nil {
					st.Errorf("Failed to close outputFile: %v", err)
				}
				if !testing.Verbose() {
					_ = os.Remove(outputFile.Name())
				}
			}()

			signData := baseSignData
			signData.Signer = pkey
			signData.Certificate = cert

			err = SignFile("../testfiles/"+f.Name(), outputFile.Name(), signData)
			if err != nil {
				st.Fatalf("%s: %s", f.Name(), err.Error())
			}
			verifySignedFile(st, outputFile, outputName)
		})
	}
}

func TestSignPDF(t *testing.T) {
	testSignAllFiles(t, SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere",
				Reason:      "Test",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		RevocationData:     revocation.InfoArchival{},
		RevocationFunction: DefaultEmbedRevocationStatusFunction,
	})
}

func TestSignPDFVisibleAll(t *testing.T) {
	testSignAllFiles(t, SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere",
				Reason:      "Visible Signature Test",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible:     true,
			LowerLeftX:  400,
			LowerLeftY:  50,
			UpperRightX: 600,
			UpperRightY: 125,
		},
	})
}

func TestSignPDFFileUTF8(t *testing.T) {
	cert, pkey := loadCertificateAndKey(t)
	signerName := "姓名"
	signerLocation := "位置"
	inputFilePath := "../testfiles/testfile20.pdf"
	originalFileName := filepath.Base(inputFilePath)

	tmpfile, err := os.CreateTemp("", t.Name())
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = tmpfile.Close()
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Errorf("Failed to remove tmpfile: %v", err)
		}
	}()

	err = SignFile(inputFilePath, tmpfile.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        signerName,
				Location:    signerLocation,
				Reason:      "Test with UTF-8",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          pkey,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatalf("%s: %s", originalFileName, err.Error())
	}

	info, err := verify.VerifyFile(tmpfile)
	if err != nil {
		t.Fatalf("%s: %s", tmpfile.Name(), err.Error())
		if err := os.Rename(tmpfile.Name(), "../testfiles/failed/"+originalFileName); err != nil {
			t.Error(err)
		}
	} else if len(info.Signers) == 0 {
		t.Fatalf("no signers found in %s", tmpfile.Name())
	} else {
		if info.Signers[0].Name != signerName {
			t.Fatalf("expected %q, got %q", signerName, info.Signers[0].Name)
		}
		if info.Signers[0].Location != signerLocation {
			t.Fatalf("expected %q, got %q", signerLocation, info.Signers[0].Location)
		}
	}
}

func BenchmarkSignPDF(b *testing.B) {
	cert, pkey := loadCertificateAndKey(b)
	certificateChains := [][]*x509.Certificate{}

	data, err := os.ReadFile("../testfiles/testfile20.pdf")
	if err != nil {
		b.Fatalf("%s", err.Error())
	}

	inputFile := filebuffer.New(data)
	size := int64(len(data))

	rdr, err := pdf.NewReader(inputFile, size)
	if err != nil {
		b.Fatalf("%s: %s", "testfile20.pdf", err.Error())
	}

	for n := 0; n < b.N; n++ {
		if _, err := inputFile.Seek(0, 0); err != nil {
			b.Fatalf("%s: %s", "testfile20.pdf", err.Error())
		}

		err = Sign(inputFile, io.Discard, rdr, size, SignData{
			Signature: SignDataSignature{
				Info: SignDataSignatureInfo{
					Name:        "John Doe",
					Location:    "Somewhere",
					Reason:      "Test",
					ContactInfo: "None",
					Date:        time.Now().Local(),
				},
				CertType:   CertificationSignature,
				DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
			},
			Signer:            pkey,
			Certificate:       cert,
			CertificateChains: certificateChains,
			RevocationData:    revocation.InfoArchival{},
		})
		if err != nil {
			b.Fatalf("%s: %s", "testfile20.pdf", err.Error())
		}
	}
}

func TestSignPDFWithTwoApproval(t *testing.T) {
	cert, pkey := loadCertificateAndKey(t)
	tbsFile := "../testfiles/testfile20.pdf"

	for i := 1; i <= 2; i++ {
		approvalTMPFile, err := os.CreateTemp("", fmt.Sprintf("%s_%d_", t.Name(), i))
		if err != nil {
			t.Fatalf("%s", err.Error())
		}
		defer func() {
			_ = approvalTMPFile.Close()
			if err := os.Remove(approvalTMPFile.Name()); err != nil {
				t.Errorf("Failed to remove approvalTMPFile: %v", err)
			}
		}()

		err = SignFile(tbsFile, approvalTMPFile.Name(), SignData{
			Signature: SignDataSignature{
				Info: SignDataSignatureInfo{
					Name:        fmt.Sprintf("Jane %d Doe", i),
					Location:    "Anywhere",
					Reason:      fmt.Sprintf("Approval Signature %d", i),
					ContactInfo: "None",
					Date:        time.Now().Local(),
				},
				CertType:   ApprovalSignature,
				DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesAndCRUDAnnotationsPerms,
			},
			DigestAlgorithm: crypto.SHA512,
			Signer:          pkey,
			Certificate:     cert,
		})
		if err != nil {
			t.Fatalf("%s: %s", "testfile20.pdf", err.Error())
		}

		verifySignedFile(t, approvalTMPFile, filepath.Base(tbsFile))
		tbsFile = approvalTMPFile.Name()
	}
}

func TestSignPDFWithCertificationApprovalAndTimeStamp(t *testing.T) {
	tsaURL := os.Getenv("PDFSIGN_TEST_TSA_URL")
	if tsaURL == "" {
		t.Skip("requiere PDFSIGN_TEST_TSA_URL con un servidor TSA de pruebas; la batería local no usa la red")
	}
	cert, pkey := loadCertificateAndKey(t)
	tbsFile := "../testfiles/testfile20.pdf"

	tmpfile, err := os.CreateTemp("", t.Name())
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = tmpfile.Close()
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Errorf("Failed to remove tmpfile: %v", err)
		}
	}()

	err = SignFile(tbsFile, tmpfile.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere",
				Reason:      "Certification Test",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesAndCRUDAnnotationsPerms,
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          pkey,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatalf("%s: %s", filepath.Base(tbsFile), err.Error())
	}

	verifySignedFile(t, tmpfile, filepath.Base(tbsFile))
	tbsFile = tmpfile.Name()

	for i := 1; i <= 2; i++ {
		approvalTMPFile, err := os.CreateTemp("", fmt.Sprintf("%s_approval_%d_", t.Name(), i))
		if err != nil {
			t.Fatalf("%s", err.Error())
		}
		defer func() {
			_ = approvalTMPFile.Close()
			if err := os.Remove(approvalTMPFile.Name()); err != nil {
				t.Errorf("Failed to remove approvalTMPFile: %v", err)
			}
		}()

		err = SignFile(tbsFile, approvalTMPFile.Name(), SignData{
			Signature: SignDataSignature{
				Info: SignDataSignatureInfo{
					Name:        fmt.Sprintf("Jane %d Doe", i),
					Location:    "Anywhere",
					Reason:      fmt.Sprintf("Approval Signature %d", i),
					ContactInfo: "None",
					Date:        time.Now().Local(),
				},
				CertType:   ApprovalSignature,
				DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesAndCRUDAnnotationsPerms,
			},
			DigestAlgorithm: crypto.SHA512,
			Signer:          pkey,
			Certificate:     cert,
		})
		if err != nil {
			t.Fatalf("%s: %s", filepath.Base(tbsFile), err.Error())
		}

		verifySignedFile(t, approvalTMPFile, filepath.Base(tbsFile))
		tbsFile = approvalTMPFile.Name()
	}

	timeStampTMPFile, err := os.CreateTemp("", fmt.Sprintf("%s_timestamp_", t.Name()))
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = timeStampTMPFile.Close()
		if err := os.Remove(timeStampTMPFile.Name()); err != nil {
			t.Errorf("Failed to remove timeStampTMPFile: %v", err)
		}
	}()

	err = SignFile(tbsFile, timeStampTMPFile.Name(), SignData{
		Signature: SignDataSignature{
			CertType: TimeStampSignature,
		},
		DigestAlgorithm: crypto.SHA512,
		TSA: TSA{
			URL: tsaURL,
		},
	})
	if err != nil {
		t.Fatalf("%s: %s", filepath.Base(tbsFile), err.Error())
	}
	verifySignedFile(t, timeStampTMPFile, "testfile20.pdf")
}

func TestTimestampPDFFile(t *testing.T) {
	tsaURL := os.Getenv("PDFSIGN_TEST_TSA_URL")
	if tsaURL == "" {
		t.Skip("requiere PDFSIGN_TEST_TSA_URL con un servidor TSA de pruebas; la batería local no usa la red")
	}
	tmpfile, err := os.CreateTemp("", t.Name())
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = tmpfile.Close()
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Errorf("Failed to remove tmpfile: %v", err)
		}
	}()

	err = SignFile("../testfiles/testfile20.pdf", tmpfile.Name(), SignData{
		Signature: SignDataSignature{
			CertType: TimeStampSignature,
		},
		DigestAlgorithm: crypto.SHA512,
		TSA: TSA{
			URL: tsaURL,
		},
	})
	if err != nil {
		t.Fatalf("%s: %s", "testfile20.pdf", err.Error())
	}

	verifySignedFile(t, tmpfile, "testfile20.pdf")
}

// TestSignPDFWithImage tests signing a PDF with an image in the signature
func TestSignPDFWithImage(t *testing.T) {
	cert, pkey := loadCertificateAndKey(t)
	inputFilePath := "../testfiles/testfile12.pdf"
	originalFileName := filepath.Base(inputFilePath)

	// Read the signature image file
	signatureImage, err := os.ReadFile("../testfiles/pdfsign-signature.jpg")
	if err != nil {
		t.Fatalf("Failed to read signature image: %s", err.Error())
	}

	tmpfile, err := os.CreateTemp("", t.Name())
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = tmpfile.Close()
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Errorf("Failed to remove tmpfile: %v", err)
		}
	}()

	err = SignFile(inputFilePath, tmpfile.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere",
				Reason:      "Test with visible signature and image",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible:     true,
			LowerLeftX:  400,
			LowerLeftY:  50,
			UpperRightX: 600,
			UpperRightY: 125,
			Image:       signatureImage, // Use the signature image
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          pkey,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatalf("%s: %s", originalFileName, err.Error())
	}

	verifySignedFile(t, tmpfile, originalFileName)
}

// TestSignPDFWithTwoImages tests signing a PDF with two different signatures with images
func TestSignPDFWithTwoImages(t *testing.T) {
	cert, pkey := loadCertificateAndKey(t)
	tbsFile := "../testfiles/testfile12.pdf"

	// Read the signature image file
	signatureImage, err := os.ReadFile("../testfiles/pdfsign-signature.jpg")
	if err != nil {
		t.Fatalf("Failed to read signature image: %s", err.Error())
	}

	// First signature
	firstSignature, err := os.CreateTemp("", fmt.Sprintf("%s_first_", t.Name()))
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = firstSignature.Close()
		if err := os.Remove(firstSignature.Name()); err != nil {
			t.Errorf("Failed to remove firstSignature: %v", err)
		}
	}()

	err = SignFile(tbsFile, firstSignature.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere",
				Reason:      "First signature with image",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible:     true,
			LowerLeftX:  50,
			LowerLeftY:  50,
			UpperRightX: 250,
			UpperRightY: 125,
			Image:       signatureImage,
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          pkey,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatalf("First signature failed: %s", err.Error())
	}

	verifySignedFile(t, firstSignature, filepath.Base(tbsFile))

	// Second signature
	secondSignature, err := os.CreateTemp("", fmt.Sprintf("%s_second_", t.Name()))
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = secondSignature.Close()
		if err := os.Remove(secondSignature.Name()); err != nil {
			t.Errorf("Failed to remove secondSignature: %v", err)
		}
	}()

	err = SignFile(firstSignature.Name(), secondSignature.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "Jane Doe",
				Location:    "Elsewhere",
				Reason:      "Second signature with image",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible:     true,
			LowerLeftX:  300,
			LowerLeftY:  50,
			UpperRightX: 500,
			UpperRightY: 125,
			Image:       signatureImage,
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          pkey,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatalf("Second signature failed: %s", err.Error())
	}

	verifySignedFile(t, secondSignature, filepath.Base(tbsFile))
}

// TestSignPDFWithWatermarkImageJPG tests signing a PDF with a JPG image and text above
func TestSignPDFWithWatermarkImageJPG(t *testing.T) {
	cert, pkey := loadCertificateAndKey(t)
	inputFilePath := "../testfiles/testfile12.pdf"
	originalFileName := filepath.Base(inputFilePath)

	// Read the signature image file
	signatureImage, err := os.ReadFile("../testfiles/pdfsign-signature-watermark.jpg")
	if err != nil {
		t.Fatalf("Failed to read signature image: %s", err.Error())
	}

	tmpfile, err := os.CreateTemp("", t.Name())
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = tmpfile.Close()
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Errorf("Failed to remove tmpfile: %v", err)
		}
	}()

	err = SignFile(inputFilePath, tmpfile.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "James SuperSmith",
				Location:    "Somewhere",
				Reason:      "Test with visible signature and watermark image",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible:          true,
			LowerLeftX:       400,
			LowerLeftY:       50,
			UpperRightX:      600,
			UpperRightY:      125,
			Image:            signatureImage, // Use the signature image
			ImageAsWatermark: true,           // Set the image as a watermark
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          pkey,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatalf("%s: %s", originalFileName, err.Error())
	}

	verifySignedFile(t, tmpfile, originalFileName)
}

// TestSignPDFWithWatermarkImage tests signing a PDF with a PNG image and text above
func TestSignPDFWithWatermarkImagePNG(t *testing.T) {
	cert, pkey := loadCertificateAndKey(t)
	inputFilePath := "../testfiles/testfile12.pdf"
	originalFileName := filepath.Base(inputFilePath)

	// Read the signature image file
	signatureImage, err := os.ReadFile("../testfiles/pdfsign-signature-watermark.png")
	if err != nil {
		t.Fatalf("Failed to read signature image: %s", err.Error())
	}

	tmpfile, err := os.CreateTemp("", t.Name())
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	defer func() {
		_ = tmpfile.Close()
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Errorf("Failed to remove tmpfile: %v", err)
		}
	}()

	err = SignFile(inputFilePath, tmpfile.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "James SuperSmith",
				Location:    "Somewhere",
				Reason:      "Test with visible signature and watermark image",
				ContactInfo: "None",
				Date:        time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible:          true,
			LowerLeftX:       400,
			LowerLeftY:       50,
			UpperRightX:      600,
			UpperRightY:      125,
			Image:            signatureImage, // Use the signature image
			ImageAsWatermark: true,           // Set the image as a watermark
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          pkey,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatalf("%s: %s", originalFileName, err.Error())
	}

	verifySignedFile(t, tmpfile, originalFileName)
}

func TestVisualSignLastPage(t *testing.T) {
	cert, pkey := loadCertificateAndKey(t)
	inputFilePath := "../testfiles/testfile16.pdf"
	input_file, err := os.Open(inputFilePath)
	originalFileName := filepath.Base(inputFilePath)
	if err != nil {
		t.Fail()
	}
	defer func() {
		if err := input_file.Close(); err != nil {
			t.Errorf("Failed to close input_file: %v", err)
		}
	}()

	tmpfile, err := os.CreateTemp("", t.Name())
	if err != nil {
		t.Fail()
	}
	defer func() {
		_ = tmpfile.Close()
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Errorf("Failed to remove tmpfile: %v", err)
		}
	}()

	finfo, err := input_file.Stat()
	if err != nil {
		t.Fail()
	}
	size := finfo.Size()

	rdr, err := pdf.NewReader(input_file, size)
	if err != nil {
		t.Fail()
	}
	lastPage := rdr.NumPage()
	t.Logf("pdf total pages: %d", lastPage)
	err = Sign(input_file, tmpfile, rdr, size, SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere on the globe",
				Reason:      "My season for signing this document",
				ContactInfo: "How you like",
				Date:        time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Signer:          pkey,          // crypto.Signer
		DigestAlgorithm: crypto.SHA256, // hash algorithm for the digest creation
		Certificate:     cert,          // x509.Certificate
		Appearance: Appearance{
			Visible:     true,
			LowerLeftX:  400,
			LowerLeftY:  50,
			UpperRightX: 600,
			UpperRightY: 125,
			Page:        uint32(lastPage),
		},
		RevocationData:     revocation.InfoArchival{},
		RevocationFunction: DefaultEmbedRevocationStatusFunction,
	})
	if err != nil {
		t.Fatal(err)
	}

	verifySignedFile(t, tmpfile, originalFileName)
}
