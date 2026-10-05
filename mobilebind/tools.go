// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// Mensajes cerrados de las herramientas. La UI Android los traduce por
// coincidencia exacta; cualquier otro detalle del núcleo queda en genérico.
const (
	mobileHashFileInvalidMessage      = "El fichero de huella no es válido o usa un algoritmo no admitido."
	mobileProtectionKeyInvalidMessage = "La clave de EncryptedData debe ser AES-256 en Base64 canónico (44 caracteres)."
	mobileProtectionRecipientMessage  = "El certificado del destinatario no permite cifrar. Use un certificado público RSA vigente de al menos 2048 bits con cifrado de clave."
	mobileUnprotectFailedMessage      = "No se pudo desproteger el fichero. Compruebe que va dirigido a su certificado o que la clave es correcta."
	mobileProtectSignIdentityMessage  = "Para proteger y firmar hace falta un certificado PKCS#12 importado en la sesión."
)

const (
	maxStoredHashBytes        = 4 << 10
	maxProtectionRecipients   = 16
	maxBatchItems             = 16
	maxBatchInputBytes        = maxDocumentBytes
	maxBatchOutputBytes       = 64 << 20
	protectionSecretTextBytes = 44
	protectionSecretBytes     = 32
)

type hashCreateRequest struct {
	ContentBase64 string `json:"content_base64"`
	Algorithm     string `json:"algorithm"`
	Format        string `json:"format"`
}

type hashCreateResponse struct {
	Algorithm    string `json:"algorithm"`
	Format       string `json:"format"`
	Hash         string `json:"hash"`
	OutputBase64 string `json:"output_base64"`
	Extension    string `json:"extension"`
}

type hashCheckRequest struct {
	ContentBase64  string `json:"content_base64"`
	HashFileBase64 string `json:"hash_file_base64"`
	HashFileName   string `json:"hash_file_name"`
	Algorithm      string `json:"algorithm,omitempty"`
}

type hashCheckResponse struct {
	Valid        bool   `json:"valid"`
	Algorithm    string `json:"algorithm"`
	Format       string `json:"format"`
	ExpectedHash string `json:"expected_hash"`
	ActualHash   string `json:"actual_hash"`
}

// CreateHashJSON calcula la huella de un fichero con los mismos algoritmos,
// codificaciones y contenido de fichero que el escritorio (.hexhash, .hashb64
// o .hash binario).
func (f *Facade) CreateHashJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("huella")
	}
	var req hashCreateRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "huella", &req); err != nil {
		return "", err
	}
	if err := validateBoundedText("algorithm", req.Algorithm, 16, true); err != nil {
		return "", err
	}
	if err := validateBoundedText("format", req.Format, 16, true); err != nil {
		return "", err
	}
	content, err := decodeBase64Limited(req.ContentBase64, "content_base64", maxDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(content)
	format, err := application.ParseHashOutputFormat(req.Format)
	if err != nil {
		return "", newFacadeError("formato de huella no admitido")
	}
	result, err := application.NuevoCreateHashUseCase().Execute(context.Background(), application.CreateHashCommand{
		Data:      content,
		Algorithm: req.Algorithm,
		Format:    format,
	})
	if err != nil {
		return "", newFacadeError("algoritmo de huella no admitido")
	}
	output := []byte(result.Encoded)
	if result.Format == application.HashFormatBinary {
		output = append([]byte(nil), result.Digest...)
	}
	return marshal(hashCreateResponse{
		Algorithm:    result.Algorithm,
		Format:       string(result.Format),
		Hash:         result.Encoded,
		OutputBase64: base64.StdEncoding.EncodeToString(output),
		Extension:    hashFileExtension(result.Format),
	})
}

// CheckHashJSON compara un fichero con una huella guardada. El formato y el
// algoritmo se deducen del fichero de huella como en escritorio.
func (f *Facade) CheckHashJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("comprobacion de huella")
	}
	var req hashCheckRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "comprobacion de huella", &req); err != nil {
		return "", err
	}
	if err := validateBoundedText("hash_file_name", req.HashFileName, 160, true); err != nil {
		return "", err
	}
	if err := validateBoundedText("algorithm", req.Algorithm, 16, true); err != nil {
		return "", err
	}
	stored, err := decodeBase64Limited(req.HashFileBase64, "hash_file_base64", maxStoredHashBytes)
	if err != nil {
		return "", newFacadeError(mobileHashFileInvalidMessage)
	}
	expected, algorithm, format, err := application.ParseStoredHash(stored, req.HashFileName)
	if err != nil {
		return "", newFacadeError(mobileHashFileInvalidMessage)
	}
	if strings.TrimSpace(req.Algorithm) != "" {
		algorithm = req.Algorithm
	}
	content, err := decodeBase64Limited(req.ContentBase64, "content_base64", maxDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(content)
	result, err := application.NuevoCheckHashUseCase().Execute(context.Background(), application.CheckHashCommand{
		Data:         content,
		ExpectedHash: expected,
		Algorithm:    algorithm,
		Format:       format,
	})
	if err != nil {
		return "", newFacadeError(mobileHashFileInvalidMessage)
	}
	if len(result.ExpectedDigest) != len(result.ActualDigest) {
		return "", newFacadeError(mobileHashFileInvalidMessage)
	}
	return marshal(hashCheckResponse{
		Valid:        result.Valid,
		Algorithm:    result.Algorithm,
		Format:       string(result.Format),
		ExpectedHash: result.ExpectedEncoded,
		ActualHash:   result.ActualEncoded,
	})
}

func hashFileExtension(format application.HashOutputFormat) string {
	switch format {
	case application.HashFormatBinary:
		return "hash"
	case application.HashFormatBase64:
		return "hashb64"
	default:
		return "hexhash"
	}
}

type protectionRecipientRequest struct {
	CertificateBase64 string `json:"certificate_base64"`
}

type protectRequest struct {
	Name                      string                       `json:"name"`
	ContentBase64             string                       `json:"content_base64"`
	MIMEType                  string                       `json:"mime_type"`
	Container                 string                       `json:"container"`
	Recipients                []protectionRecipientRequest `json:"recipients,omitempty"`
	IncludeSessionCertificate bool                         `json:"include_session_certificate,omitempty"`
	Sign                      bool                         `json:"sign,omitempty"`
	CertificateID             string                       `json:"certificate_id,omitempty"`
}

type protectResponse struct {
	Name           string `json:"name"`
	MIMEType       string `json:"mime_type"`
	ContentBase64  string `json:"content_base64"`
	Container      string `json:"container"`
	RecipientCount int    `json:"recipient_count"`
	CertificateID  string `json:"certificate_id,omitempty"`
}

type unprotectRequest struct {
	Name          string `json:"name"`
	ContentBase64 string `json:"content_base64"`
	MIMEType      string `json:"mime_type"`
}

type unprotectResponse struct {
	Name          string `json:"name"`
	MIMEType      string `json:"mime_type"`
	ContentBase64 string `json:"content_base64"`
}

// ProtectJSON cifra un fichero con los contenedores CMS del escritorio:
// EnvelopedData y AuthEnvelopedData para destinatarios X.509 RSA, o
// EncryptedData con una clave AES-256 transitoria. Con sign=true crea
// SignedAndEnvelopedData firmado con la identidad de la sesión (PKCS#12 o
// DNIe, que solo firma el resumen; véase external_cms.go).
// secret contiene la clave Base64 en ASCII; se borra antes de volver.
func (f *Facade) ProtectJSON(payload string, secret []byte) (string, error) {
	defer zeroBytes(secret)
	if f == nil || f.session == nil {
		return "", errNoConfigurado("proteccion")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	var req protectRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "proteccion", &req); err != nil {
		return "", err
	}
	if err := validateDocumentMetadata(req.Name, req.MIMEType); err != nil {
		return "", err
	}
	if err := validateBoundedText("certificate_id", req.CertificateID, 128, true); err != nil {
		return "", err
	}
	container, err := mobileProtectionContainer(req.Container, req.Sign)
	if err != nil {
		return "", err
	}
	if len(req.Recipients) > maxProtectionRecipients {
		return "", newFacadeError("recipients supera el limite permitido")
	}
	key, err := decodeProtectionSecretText(secret)
	if err != nil {
		return "", err
	}
	defer zeroBytes(key)
	if container == "cms-encrypted" {
		if key == nil || len(req.Recipients) > 0 || req.IncludeSessionCertificate {
			return "", newFacadeError(mobileProtectionKeyInvalidMessage)
		}
	} else if key != nil {
		return "", newFacadeError("la clave transitoria solo se usa con EncryptedData")
	}
	content, err := decodeBase64Limited(req.ContentBase64, "content_base64", maxDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(content)

	catalog := mobileRecipientCatalog{}
	if container != "cms-encrypted" {
		catalog, err = f.recipientCatalog(req)
		if err != nil {
			return "", err
		}
		if len(catalog) == 0 {
			return "", newFacadeError("debe indicarse al menos un destinatario")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	options := map[string]string{"container": container}
	var protected domain.ProtectedPayload
	certificateID := ""
	if req.Sign {
		if !f.session.hasSigningIdentity(req.CertificateID) {
			return "", newFacadeError(mobileProtectSignIdentityMessage)
		}
		cmd, err := application.NewProtectAndSignCommand(req.Name, content, req.MIMEType,
			string(domain.ProtectionProfileCompat), catalog.ids(), req.CertificateID, options)
		if err != nil {
			return "", newFacadeError("solicitud de proteccion no valida")
		}
		useCase := application.NuevoProtectAndSignDocumentUseCase(f.session, f.session, catalog,
			protector.NuevoCMSSignedEnvelopedProtector(), nativeExplicitApproval{}, nil, nil)
		result, err := useCase.Execute(ctx, cmd)
		if err != nil {
			return "", safeOperationError("proteccion")
		}
		protected = result.Protected
		certificateID = result.CertificateUsed.ID
	} else {
		cmd, err := application.NewProtectCommand(req.Name, content, req.MIMEType,
			string(domain.ProtectionProfileCompat), catalog.ids(), options)
		if err != nil {
			return "", newFacadeError("solicitud de proteccion no valida")
		}
		cmd.SymmetricKey = key
		useCase := application.NuevoProtectDocumentUseCase(catalog, protector.NuevoAdaptiveProtector(),
			nativeExplicitApproval{}, nil, nil)
		result, err := useCase.Execute(ctx, cmd)
		if err != nil {
			return "", safeOperationError("proteccion")
		}
		protected = result.Protected
	}
	data := protected.Document.Content
	defer zeroBytes(data)
	if len(data) == 0 || len(data) > maxSignedDocumentBytes {
		return "", newFacadeError("el resultado de proteccion tiene un tamano no permitido")
	}
	return marshal(protectResponse{
		Name:           sanitizeOutputText(protected.Document.Name, 200),
		MIMEType:       sanitizeOutputText(protected.Document.MIMEType, 160),
		ContentBase64:  base64.StdEncoding.EncodeToString(data),
		Container:      container,
		RecipientCount: protected.RecipientCount,
		CertificateID:  sanitizeOutputText(certificateID, 128),
	})
}

// UnprotectJSON descifra un contenedor CMS (o el sobre JSON compatible) con la
// identidad PKCS#12 de la sesión o con la clave transitoria de EncryptedData.
func (f *Facade) UnprotectJSON(payload string, secret []byte) (string, error) {
	defer zeroBytes(secret)
	if f == nil || f.session == nil {
		return "", errNoConfigurado("desproteccion")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	var req unprotectRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "desproteccion", &req); err != nil {
		return "", err
	}
	if err := validateDocumentMetadata(req.Name, req.MIMEType); err != nil {
		return "", err
	}
	key, err := decodeProtectionSecretText(secret)
	if err != nil {
		return "", err
	}
	defer zeroBytes(key)
	content, err := decodeBase64Limited(req.ContentBase64, "content_base64", maxSignedDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(content)
	doc, err := domain.NewDocument(req.Name, content, domain.MIMETypeProtectedCMS)
	if err != nil {
		return "", newFacadeError("documento protegido no valido")
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	useCase := application.NuevoUnprotectDocumentUseCase(mobileSessionDecryptionKeys{f.session},
		protector.NuevoAdaptiveProtector(), nil, nil)
	result, err := useCase.Execute(ctx, application.UnprotectCommand{ProtectedDocument: doc, SymmetricKey: key})
	if err != nil {
		return "", newFacadeError(mobileUnprotectFailedMessage)
	}
	data := result.Unprotected.Document.Content
	defer zeroBytes(data)
	if len(data) == 0 || len(data) > maxSignedDocumentBytes {
		return "", newFacadeError("el resultado de desproteccion tiene un tamano no permitido")
	}
	return marshal(unprotectResponse{
		Name:          sanitizeOutputText(result.Unprotected.Document.Name, 200),
		MIMEType:      sanitizeOutputText(result.Unprotected.Document.MIMEType, 160),
		ContentBase64: base64.StdEncoding.EncodeToString(data),
	})
}

func mobileProtectionContainer(raw string, sign bool) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if sign {
		switch normalized {
		case "", "signedandenvelopeddata":
			return "signedandenvelopeddata", nil
		default:
			return "", newFacadeError("proteger y firmar solo admite SignedAndEnvelopedData")
		}
	}
	switch normalized {
	case "", "cms", "envelopeddata":
		return "cms", nil
	case "authenvelopeddata":
		return "authenvelopeddata", nil
	case "cms-encrypted", "encrypteddata":
		return "cms-encrypted", nil
	default:
		return "", newFacadeError("contenedor de proteccion no habilitado en mobile")
	}
}

// decodeProtectionSecretText acepta solo la representación canónica de una
// clave de 32 bytes. Devuelve nil si no se ha indicado clave.
func decodeProtectionSecretText(secret []byte) ([]byte, error) {
	if len(secret) == 0 {
		return nil, nil
	}
	if len(secret) != protectionSecretTextBytes {
		return nil, newFacadeError(mobileProtectionKeyInvalidMessage)
	}
	key := make([]byte, base64.StdEncoding.DecodedLen(len(secret)))
	n, err := base64.StdEncoding.Strict().Decode(key, secret)
	if err != nil || n != protectionSecretBytes {
		zeroBytes(key)
		return nil, newFacadeError(mobileProtectionKeyInvalidMessage)
	}
	key = key[:n]
	canonical := make([]byte, protectionSecretTextBytes)
	base64.StdEncoding.Encode(canonical, key)
	equal := true
	for i := range canonical {
		equal = equal && canonical[i] == secret[i]
	}
	zeroBytes(canonical)
	if !equal {
		zeroBytes(key)
		return nil, newFacadeError(mobileProtectionKeyInvalidMessage)
	}
	return key, nil
}

func (f *Facade) recipientCatalog(req protectRequest) (mobileRecipientCatalog, error) {
	catalog := mobileRecipientCatalog{}
	if req.IncludeSessionCertificate {
		der, ok := f.session.certificateDER(req.CertificateID)
		if !ok {
			return nil, newFacadeError("certificado de sesión no disponible")
		}
		recipient, err := protector.ValidatePublicRecipientCertificate(der)
		if err != nil {
			return nil, newFacadeError(mobileProtectionRecipientMessage)
		}
		catalog[recipient.ID] = recipient
	}
	for _, item := range req.Recipients {
		der, err := decodeBase64Limited(item.CertificateBase64, "certificate_base64", 64<<10)
		if err != nil {
			return nil, newFacadeError(mobileProtectionRecipientMessage)
		}
		recipient, err := protector.ValidatePublicRecipientCertificate(der)
		if err != nil {
			return nil, newFacadeError(mobileProtectionRecipientMessage)
		}
		catalog[recipient.ID] = recipient
	}
	return catalog, nil
}

// mobileRecipientCatalog contiene solo los destinatarios de la petición en
// curso. Android no guarda libreta de destinatarios.
type mobileRecipientCatalog map[string]domain.ProtectionRecipient

func (c mobileRecipientCatalog) ids() []string {
	out := make([]string, 0, len(c))
	for id := range c {
		out = append(out, id)
	}
	return out
}

func (c mobileRecipientCatalog) Resolve(ctx context.Context, ids []string) ([]domain.ProtectionRecipient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]domain.ProtectionRecipient, 0, len(ids))
	for _, id := range ids {
		recipient, ok := c[id]
		if !ok {
			return nil, errors.New("destinatario desconocido")
		}
		out = append(out, recipient)
	}
	return out, nil
}
