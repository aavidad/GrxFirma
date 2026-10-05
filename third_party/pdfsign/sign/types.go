package sign

import (
	"context"
	"crypto"
	"crypto/x509"
	"io"
	"net/http"
	"time"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/revocation"
	"github.com/mattetti/filebuffer"
)

type CatalogData struct {
	ObjectId   uint32
	RootString string
}

type TSA struct {
	URL      string
	Username string
	Password string
	// Context cancels the timestamp request with the signing operation. A nil
	// context still receives the bounded default timeout.
	Context context.Context
	// HTTPClient preserves the application's transport/proxy configuration.
	// Requests always enforce size, time and same-origin redirect limits.
	HTTPClient *http.Client
}

type RevocationFunction func(cert, issuer *x509.Certificate, i *revocation.InfoArchival) error

type SignData struct {
	Signature          SignDataSignature
	Signer             crypto.Signer
	DigestAlgorithm    crypto.Hash
	Certificate        *x509.Certificate
	CertificateChains  [][]*x509.Certificate
	TSA                TSA
	RevocationData     revocation.InfoArchival
	RevocationFunction RevocationFunction
	Appearance         Appearance

	// ValidationData contiene material de validación DER para incrustar en el
	// diccionario /DSS a nivel de documento (PAdES B-LT, "LTV enabled").
	// Se escribe en la misma sección incremental que la firma, por lo que
	// queda cubierto por el ByteRange de esta.
	ValidationData ValidationData

	objectId uint32
}

// ValidationData agrupa los blobs DER del Document Security Store (ISO 32000-2
// §12.8.4.3): certificados de la cadena, respuestas OCSP y CRLs.
type ValidationData struct {
	Certs [][]byte
	OCSPs [][]byte
	CRLs  [][]byte
}

// IsEmpty indica si no hay ningún material de validación que incrustar.
func (v ValidationData) IsEmpty() bool {
	return len(v.Certs) == 0 && len(v.OCSPs) == 0 && len(v.CRLs) == 0
}

// Appearance represents the appearance of the signature
type Appearance struct {
	Visible bool

	Page        uint32
	Pages       []uint32
	AllPages    bool
	LowerLeftX  float64
	LowerLeftY  float64
	UpperRightX float64
	UpperRightY float64
	// PerPage define rectángulo y apariencia propios para cada widget del
	// único campo de firma. Si está vacío se conserva el comportamiento previo.
	PerPage []PageAppearance

	Image              []byte // Image data to use as signature appearance
	ImageAsWatermark   bool   // If true, the text will be drawn over the image
	LogoImage          []byte // Logo layer, drawn separately over Image
	LogoOpacityPercent int    // Deprecated: ignored; use SealOpacityPercent instead.
	SealOpacityPercent *int   // 0–100; nil keeps the appearance fully opaque.

	// Stamps son imágenes que se estampan en el documento dentro de la
	// revisión firmada (parámetro image de AutoFirma Java), como anotaciones
	// de sello bloqueadas.
	Stamps []StampImage

	// FieldName, si no está vacío, firma en el campo de firma vacío con ese
	// nombre que ya contiene el PDF (signatureField de AutoFirma Java), en
	// lugar de crear uno nuevo. La posición es la del campo.
	FieldName string
}

type PageAppearance struct {
	Page      uint32
	Rect      [4]float64
	Image     []byte
	LogoImage []byte
}

// StampImage es una imagen que se estampa en una página.
type StampImage struct {
	Page  uint32
	Rect  [4]float64
	Image []byte
}

type VisualSignData struct {
	pageObjectId   uint32
	objectId       uint32
	annotObjectIDs []uint32
	fieldObjectID  uint32
	// annotsPendientes son anotaciones (sellos de imagen) que se añaden a
	// /Annots de cada página junto con la firma, en una sola actualización.
	annotsPendientes map[uint32][]uint32
}

type InfoData struct {
	ObjectId uint32
}

// The generated fallback must use FormatUint(uint64(i+1));
// TestGeneratedEnumStringsPreserveUnsignedValues guards regeneration.
//
//go:generate stringer -type=CertType
type CertType uint

const (
	CertificationSignature CertType = iota + 1
	ApprovalSignature
	UsageRightsSignature
	TimeStampSignature
)

// The generated fallback must use FormatUint(uint64(i+1));
// TestGeneratedEnumStringsPreserveUnsignedValues guards regeneration.
//
//go:generate stringer -type=DocMDPPerm
type DocMDPPerm uint

const (
	DoNotAllowAnyChangesPerms DocMDPPerm = iota + 1
	AllowFillingExistingFormFieldsAndSignaturesPerms
	AllowFillingExistingFormFieldsAndSignaturesAndCRUDAnnotationsPerms
)

type SignDataSignature struct {
	CertType   CertType
	DocMDPPerm DocMDPPerm
	// FieldMDPAll explicitly locks all form fields after an approval signature.
	// The default approval signature leaves fields available for later signers.
	FieldMDPAll bool
	Info        SignDataSignatureInfo
	// SubFilter selects the PDF signature encoding advertised in the
	// signature dictionary. The zero value is the recommended ETSI CAdES
	// detached profile; Adobe PKCS#7 detached must be requested explicitly.
	SubFilter SignatureSubFilter
}

// SignatureSubFilter is a supported PDF signature dictionary /SubFilter.
type SignatureSubFilter string

const (
	SignatureSubFilterETSICAdESDetached  SignatureSubFilter = "ETSI.CAdES.detached"
	SignatureSubFilterAdobePKCS7Detached SignatureSubFilter = "adbe.pkcs7.detached"
)

type SignDataSignatureInfo struct {
	Name        string
	Location    string
	Reason      string
	ContactInfo string
	Date        time.Time
	// Description es el texto alternativo (/TU) del sello ya compuesto en el
	// idioma del documento. Si está vacío se compone uno genérico.
	Description string
}

type SignContext struct {
	InputFile              io.ReadSeeker
	OutputFile             io.Writer
	OutputBuffer           *filebuffer.Buffer
	SignData               SignData
	CatalogData            CatalogData
	VisualSignData         VisualSignData
	InfoData               InfoData
	PDFReader              *pdf.Reader
	NewXrefStart           int64
	ByteRangeValues        []int64
	SignatureMaxLength     uint32
	SignatureMaxLengthBase uint32

	existingSignatures []SignData
	lastXrefID         uint32
	newXrefEntries     []xrefEntry
	updatedXrefEntries []xrefEntry
	dssObjectID        uint32
	// cifrador cifra los objetos nuevos cuando el PDF original está cifrado.
	cifrador *cifradorObjetos
}
