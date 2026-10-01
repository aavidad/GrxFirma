package sign

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/internal/pkcs7"

	"github.com/mattetti/filebuffer"
)

func SignFile(input string, output string, sign_data SignData) error {
	input_file, err := os.Open(input) // #nosec G304 -- input is the explicit PDF path supplied to the public SignFile API.
	if err != nil {
		return err
	}
	defer func() {
		_ = input_file.Close()
	}()

	output_file, err := os.Create(output) // #nosec G304 -- output is the explicit destination supplied to the public SignFile API.
	if err != nil {
		return err
	}
	defer func() {
		_ = output_file.Close()
	}()

	finfo, err := input_file.Stat()
	if err != nil {
		return err
	}
	size := finfo.Size()

	rdr, err := pdf.NewReader(input_file, size)
	if err != nil {
		return err
	}

	return Sign(input_file, output_file, rdr, size, sign_data)
}

func Sign(input io.ReadSeeker, output io.Writer, rdr *pdf.Reader, size int64, sign_data SignData) (err error) {
	// El lector de PDF aborta con pánicos ante entradas malformadas o con
	// cifrado no soportado: se convierten en error para no cerrar la aplicación.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("PDF no procesable: %v", r)
		}
	}()
	context := SignContext{
		PDFReader:              rdr,
		InputFile:              input,
		OutputFile:             output,
		SignData:               sign_data,
		SignatureMaxLengthBase: signaturePlaceholderBaseLength,
	}

	context.cifrador = nuevoCifrador(rdr)

	// Fetch existing signatures
	existingSignatures, err := context.fetchExistingSignatures()
	if err != nil {
		return err
	}
	context.existingSignatures = existingSignatures

	err = context.SignPDF()
	if err != nil {
		return err
	}

	return nil
}

func (context *SignContext) SignPDF() error {
	// set defaults
	if context.SignData.Signature.CertType == 0 {
		context.SignData.Signature.CertType = 1
	}
	if context.SignData.Signature.DocMDPPerm == 0 {
		context.SignData.Signature.DocMDPPerm = 1
	}
	if !context.SignData.DigestAlgorithm.Available() {
		context.SignData.DigestAlgorithm = crypto.SHA256
	}
	if context.SignData.Appearance.Page == 0 && !context.SignData.Appearance.AllPages && len(context.SignData.Appearance.Pages) == 0 {
		context.SignData.Appearance.Page = 1
	}

	context.OutputBuffer = filebuffer.New([]byte{})

	// Copy old file into new buffer.
	_, err := context.InputFile.Seek(0, 0)
	if err != nil {
		return err
	}
	if _, err := io.Copy(context.OutputBuffer, context.InputFile); err != nil {
		return err
	}

	// File always needs an empty line after %%EOF.
	if _, err := context.OutputBuffer.Write([]byte("\n")); err != nil {
		return err
	}

	// Base size for signature.
	if err := context.resetSignaturePlaceholderLength(); err != nil {
		return err
	}

	// If not a timestamp signature
	if context.SignData.Signature.CertType != TimeStampSignature {
		if context.SignData.Certificate == nil {
			return fmt.Errorf("certificate is required")
		}

		switch context.SignData.Certificate.SignatureAlgorithm.String() {
		case "SHA1-RSA":
		case "ECDSA-SHA1":
		case "DSA-SHA1":
			if err := context.addSignatureContentLength(128); err != nil {
				return fmt.Errorf("failed to reserve SHA-1 signature space: %w", err)
			}
		case "SHA256-RSA":
		case "ECDSA-SHA256":
		case "DSA-SHA256":
			if err := context.addSignatureContentLength(256); err != nil {
				return fmt.Errorf("failed to reserve SHA-256 signature space: %w", err)
			}
		case "SHA384-RSA":
		case "ECDSA-SHA384":
			if err := context.addSignatureContentLength(384); err != nil {
				return fmt.Errorf("failed to reserve SHA-384 signature space: %w", err)
			}
		case "SHA512-RSA":
		case "ECDSA-SHA512":
			if err := context.addSignatureContentLength(512); err != nil {
				return fmt.Errorf("failed to reserve SHA-512 signature space: %w", err)
			}
		}

		// Add size of digest algorithm twice (for file digist and signing certificate attribute)
		if err := context.addSignatureContentLength(context.SignData.DigestAlgorithm.Size() * 2); err != nil {
			return fmt.Errorf("failed to reserve digest space: %w", err)
		}

		// Add size for my certificate.
		degenerated, err := pkcs7.DegenerateCertificate(context.SignData.Certificate.Raw)
		if err != nil {
			return fmt.Errorf("failed to degenerate certificate: %w", err)
		}

		if err := context.addSignatureContentLength(len(degenerated)); err != nil {
			return fmt.Errorf("failed to reserve certificate space: %w", err)
		}

		// Add size of the raw issuer which is added by AddSignerChain
		if err := context.addSignatureContentLength(len(context.SignData.Certificate.RawIssuer)); err != nil {
			return fmt.Errorf("failed to reserve certificate issuer space: %w", err)
		}

		// Add size for certificate chain.
		var certificate_chain []*x509.Certificate
		if len(context.SignData.CertificateChains) > 0 && len(context.SignData.CertificateChains[0]) > 1 {
			certificate_chain = context.SignData.CertificateChains[0][1:]
		}

		if len(certificate_chain) > 0 {
			for _, cert := range certificate_chain {
				degenerated, err := pkcs7.DegenerateCertificate(cert.Raw)
				if err != nil {
					return fmt.Errorf("failed to degenerate certificate in chain: %w", err)
				}

				if err := context.addSignatureContentLength(len(degenerated)); err != nil {
					return fmt.Errorf("failed to reserve certificate chain space: %w", err)
				}
			}
		}

		// Fetch revocation data before adding signature placeholder.
		// Revocation data can be quite large and we need to create enough space in the placeholder.
		if err := context.fetchRevocationData(); err != nil {
			return fmt.Errorf("failed to fetch revocation data: %w", err)
		}
	}

	// Add estimated size for TSA.
	// We can't kow actual size of TSA until after signing.
	//
	// Different TSA servers provide different response sizes, we
	// might need to make this configurable or detect and store.
	if context.SignData.TSA.URL != "" {
		if err := context.addSignatureContentLength(9000); err != nil {
			return fmt.Errorf("failed to reserve timestamp space: %w", err)
		}
	}

	// Create the signature object
	var signature_object []byte

	switch context.SignData.Signature.CertType {
	case TimeStampSignature:
		signature_object = context.createTimestampPlaceholder()
	default:
		signature_object = context.createSignaturePlaceholder()
	}

	// Write the new signature object
	context.SignData.objectId, err = context.addObject(signature_object)
	if err != nil {
		return fmt.Errorf("failed to add signature object: %w", err)
	}

	// Create visual signature (visible or invisible based on CertType)
	visible := false
	rectangle := [4]float64{0, 0, 0, 0}
	if context.SignData.Signature.CertType != ApprovalSignature && context.SignData.Appearance.Visible {
		return fmt.Errorf("visible signatures are only allowed for approval signatures")
	} else if context.SignData.Signature.CertType == ApprovalSignature && context.SignData.Appearance.Visible {
		visible = true
		rectangle = [4]float64{
			context.SignData.Appearance.LowerLeftX,
			context.SignData.Appearance.LowerLeftY,
			context.SignData.Appearance.UpperRightX,
			context.SignData.Appearance.UpperRightY,
		}
	}

	if err := context.crearSellosImagen(); err != nil {
		return err
	}
	if nombre := context.SignData.Appearance.FieldName; nombre != "" {
		if !nombreCampoValido(nombre) {
			return fmt.Errorf("nombre de campo de firma no válido")
		}
		if err := context.rellenarCampoFirma(nombre); err != nil {
			return err
		}
	} else if err := context.crearCampoFirma(visible, rectangle); err != nil {
		return err
	}
	if err := context.actualizarPaginasPendientes(); err != nil {
		return err
	}

	// Document Security Store (PAdES B-LT): streams de validación + /DSS,
	// en la misma sección incremental para que el ByteRange los cubra.
	context.dssObjectID, err = context.createDSSObjects()
	if err != nil {
		return fmt.Errorf("failed to create DSS objects: %w", err)
	}

	// Create a new catalog object
	catalog, err := context.createCatalog()
	if err != nil {
		return fmt.Errorf("failed to create catalog: %w", err)
	}

	// Write the new catalog object
	context.CatalogData.ObjectId, err = context.addObject(catalog)
	if err != nil {
		return fmt.Errorf("failed to add catalog object: %w", err)
	}

	// Write xref table
	if err := context.writeXref(); err != nil {
		return fmt.Errorf("failed to write xref: %w", err)
	}

	// Write trailer
	if err := context.writeTrailer(); err != nil {
		return fmt.Errorf("failed to write trailer: %w", err)
	}

	// Update byte range
	if err := context.updateByteRange(); err != nil {
		return fmt.Errorf("failed to update byte range: %w", err)
	}

	// Replace signature
	if err := context.replaceSignature(); err != nil {
		return fmt.Errorf("failed to replace signature: %w", err)
	}

	// Write final output
	if _, err := context.OutputBuffer.Seek(0, 0); err != nil {
		return err
	}
	file_content := context.OutputBuffer.Buff.Bytes()

	if _, err := context.OutputFile.Write(file_content); err != nil {
		return err
	}

	return nil
}

// crearCampoFirma crea el campo y los widgets de una firma nueva.
func (context *SignContext) crearCampoFirma(visible bool, rectangle [4]float64) error {
	pageNumbers, err := context.resolveAppearancePages()
	if err != nil {
		return fmt.Errorf("failed to resolve visual signature pages: %w", err)
	}

	if visible && (len(pageNumbers) > 1 || len(context.SignData.Appearance.PerPage) > 0) {
		// PAdES exige una única firma: un solo campo /FT /Sig (dueño de /V) cuyos
		// /Kids son un widget por página. La ruta anterior emitía createVisualSignature
		// por página, generando N campos de firma completos que apuntaban al mismo /V,
		// lo que producía un PDF con múltiples firmas ("multiple signatures covering
		// entire document. Impossible").

		// 1) Objetos de apariencia (uno por página).
		appearanceIDs := make([]uint32, len(pageNumbers))
		originalAppearance := context.SignData.Appearance
		for i := range pageNumbers {
			pageRect := rectangle
			if len(originalAppearance.PerPage) > 0 {
				placement := originalAppearance.PerPage[i]
				pageRect = placement.Rect
				context.SignData.Appearance.Image = placement.Image
				context.SignData.Appearance.LogoImage = placement.LogoImage
			}
			appearance, err := context.createAppearance(pageRect)
			if err != nil {
				return fmt.Errorf("failed to create appearance: %w", err)
			}
			appearanceID, err := context.addObject(appearance)
			if err != nil {
				return fmt.Errorf("failed to add appearance object: %w", err)
			}
			appearanceIDs[i] = appearanceID
		}
		context.SignData.Appearance = originalAppearance

		// 2) El campo padre se escribe tras los widgets; reservamos su ID para que
		// cada widget pueda referenciarlo como /Parent.
		fieldObjectID, err := context.getNextObjectID()
		if err != nil {
			return fmt.Errorf("failed to reserve visual signature field object id: %w", err)
		}
		fieldObjectID, err = checkedAddUint32(fieldObjectID, len(pageNumbers))
		if err != nil {
			return fmt.Errorf("failed to reserve visual signature field object id: %w", err)
		}

		widgetIDs := make([]uint32, len(pageNumbers))
		pageIDs := make([]uint32, len(pageNumbers))
		for i, pageNumber := range pageNumbers {
			pageRect := rectangle
			if len(originalAppearance.PerPage) > 0 {
				pageRect = originalAppearance.PerPage[i].Rect
			}
			widget, pageObjectID, err := context.createVisualSignatureWidget(pageNumber, pageRect, appearanceIDs[i], fieldObjectID)
			if err != nil {
				return fmt.Errorf("failed to create visual signature widget: %w", err)
			}
			widgetObjectID, err := context.addObject(widget)
			if err != nil {
				return fmt.Errorf("failed to add visual signature widget object: %w", err)
			}
			widgetIDs[i] = widgetObjectID
			pageIDs[i] = pageObjectID
		}

		// 3) Campo de firma padre: dueño de /V y con los widgets como /Kids.
		fieldName := "Signature " + strconv.Itoa(len(context.existingSignatures)+1)
		field, err := context.createVisualSignatureField(fieldName, widgetIDs)
		if err != nil {
			return fmt.Errorf("failed to create visual signature field: %w", err)
		}
		addedFieldID, err := context.addObject(field)
		if err != nil {
			return fmt.Errorf("failed to add visual signature field object: %w", err)
		}
		if addedFieldID != fieldObjectID {
			return fmt.Errorf("visual signature field object id mismatch: reserved %d, wrote %d", fieldObjectID, addedFieldID)
		}
		context.VisualSignData.fieldObjectID = fieldObjectID

		// 4) Actualización incremental de cada página para añadir su widget a /Annots.
		for i, pageNumber := range pageNumbers {
			incPageUpdate, err := context.createIncPageUpdate(pageNumber, widgetIDs[i])
			if err != nil {
				return fmt.Errorf("failed to create incremental page update: %w", err)
			}
			if err := context.updateObject(pageIDs[i], incPageUpdate); err != nil {
				return fmt.Errorf("failed to add incremental page update object: %w", err)
			}
		}
	} else {
		visualSignature, pageObjectID, err := context.createVisualSignature(visible, pageNumbers[0], rectangle, len(context.existingSignatures)+1)
		if err != nil {
			return fmt.Errorf("failed to create visual signature: %w", err)
		}

		context.VisualSignData.objectId, err = context.addObject(visualSignature)
		if err != nil {
			return fmt.Errorf("failed to add visual signature object: %w", err)
		}
		context.VisualSignData.pageObjectId = pageObjectID

		if context.SignData.Appearance.Visible {
			incPageUpdate, err := context.createIncPageUpdate(pageNumbers[0], context.VisualSignData.objectId)
			if err != nil {
				return fmt.Errorf("failed to create incremental page update: %w", err)
			}
			err = context.updateObject(context.VisualSignData.pageObjectId, incPageUpdate)
			if err != nil {
				return fmt.Errorf("failed to add incremental page update object: %w", err)
			}
		}
	}

	return nil
}
