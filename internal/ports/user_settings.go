// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// ConfiguracionUsuarioGeneral agrupa un primer subconjunto tipado y estable
// de preferencias generales de interfaz.
type ConfiguracionUsuarioGeneral struct {
	Idioma              *string
	ThemeIndex          *int
	ExpertMode          *bool
	AutoClose           *bool
	ConfirmToSign       *bool
	OmitAskOnClose      *bool
	CloseBehavior       *string
	HideDnieStartScreen *bool
	CheckForUpdates     *bool
	SecureConnections   *bool
	SecureDomainsList   []string
}

// ConfiguracionUsuarioFirma agrupa un segundo subconjunto tipado y estable
// de preferencias de firma por defecto.
type ConfiguracionUsuarioFirma struct {
	Action          *string
	Format          *string
	Profile         *string
	Overwrite       *string
	StrictCompat    *bool
	AllowInvalidPDF *bool
}

// ConfiguracionUsuarioFormatosAutomaticos agrupa las preferencias por defecto
// de formato de firma segun el tipo documental detectado.
type ConfiguracionUsuarioFormatosAutomaticos struct {
	PDF      *string
	OOXML    *string
	FacturaE *string
	ODF      *string
	XML      *string
	Binary   *string
}

// ConfiguracionUsuarioFirmaMetadatos agrupa metadatos opcionales de firma
// que la GUI usa para precargar razon, ubicacion y contacto.
type ConfiguracionUsuarioFirmaMetadatos struct {
	Reason      *string
	Location    *string
	ContactInfo *string
}

// ConfiguracionUsuarioFacturaE agrupa el subconjunto tipado y estable de
// preferencias por defecto especificas de FacturaE.
type ConfiguracionUsuarioFacturaE struct {
	PolicyVersion   *string
	PolicyID        *string
	PolicyHash      *string
	PolicyQualifier *string
	SignerRole      *string
	City            *string
	Province        *string
	PostalCode      *string
	Country         *string
}

// ConfiguracionUsuarioPAdES agrupa un primer subconjunto tipado y estable de
// preferencias por defecto especificas de PAdES.
type ConfiguracionUsuarioPAdES struct {
	SubFilter           *string
	PolicyID            *string
	PolicyHash          *string
	PolicyHashAlgorithm *string
	PolicyQualifier     *string
	ObfuscateCertInfo   *bool
	VisibleStamp        *bool
	AllowShadowAttack   *bool
	AllowCertifiedPDF   *bool
	CertificationLevel  *int
}

// ConfiguracionUsuarioCAdES agrupa el subconjunto tipado y estable de
// preferencias por defecto especificas de CAdES.
type ConfiguracionUsuarioCAdES struct {
	PolicyID            *string
	PolicyHash          *string
	PolicyHashAlgorithm *string
	PolicyQualifier     *string
	ImplicitMode        *bool
	Multisign           *string
}

// ConfiguracionUsuarioXAdES agrupa el subconjunto tipado y estable de
// preferencias por defecto especificas de XAdES.
type ConfiguracionUsuarioXAdES struct {
	PolicyID            *string
	PolicyHash          *string
	PolicyHashAlgorithm *string
	PolicyQualifier     *string
	SignFormat          *string
	Multisign           *string
	ClaimedRole         *string
	City                *string
	Province            *string
	PostalCode          *string
	Country             *string
}

// ConfiguracionUsuarioPAdESVisible agrupa el subconjunto basico y estable de
// preferencias del sello visible PAdES.
type ConfiguracionUsuarioPAdESVisible struct {
	Enabled            *bool
	Pages              *string
	AllPages           *bool
	X                  *float64
	Y                  *float64
	W                  *float64
	H                  *float64
	KeepText           *bool
	Rotation           *int
	ImagePath          *string
	LogoOpacityPercent *int
	QRContent          *string
}

// ConfiguracionUsuarioMultiCoSign agrupa preferencias persistidas de la
// cofirma múltiple guiada.
type ConfiguracionUsuarioMultiCoSign struct {
	Enabled        *bool
	PrimaryID      *string
	CertificateIDs []string
}

// ConfiguracionUsuarioHash agrupa un subconjunto tipado y estable de
// preferencias por defecto para huellas e integridad.
type ConfiguracionUsuarioHash struct {
	Algorithm       *string
	CopyToClipboard *bool
	FormatFile      *string
	FormatDirectory *string
	Recursive       *bool
	SaveReport      *bool
}

// ConfiguracionUsuarioProxy agrupa un subconjunto tipado y estable de
// preferencias de red para el proxy manual.
type ConfiguracionUsuarioProxy struct {
	Enabled      *bool
	Type         *string
	Host         *string
	Port         *int
	SecretID     *string
	Realm        *string
	ExcludedURLs []string
}

// ConfiguracionUsuarioCertificados agrupa un subconjunto tipado y estable de
// preferencias de seleccion y persistencia del certificado.
type ConfiguracionUsuarioCertificados struct {
	StickySigner                  *bool
	AutoSelectSingleCertificate   *bool
	PreferDefaultCertificate      *bool
	PreferredCertificateID        *string
	DefaultCertificateID          *string
	DefaultKeystore               *string
	DefaultLocalKeystorePath      *string
	UseDefaultStoreInBrowserCalls *bool
	UseOnlySignatureCertificates  *bool
	UseOnlyAliasCertificates      *bool
	SkipAuthCertDnie              *bool
	ShowDefaultFirst              *bool
	ShowUsableFirst               *bool
	ShowValidFirst                *bool
	RememberFilter                *bool
	FilterText                    *string
	ShowExpired                   *bool
	ShowInvalid                   *bool
	TypeFilter                    []string
	RequireNIF                    *bool
	RequireOrganization           *bool
}

// ConfiguracionUsuarioDesktop agrupa un subconjunto tipado y estable de
// preferencias legacy de desktop/Fyne.
type ConfiguracionUsuarioDesktop struct {
	LegacyWebUISize                 *string
	LegacyWebTrayResident           *bool
	WebCompatibilityDurationMinutes *int
	FacturaeToolsEnabled            *bool
}

// ConfiguracionUsuarioTSA agrupa un subconjunto tipado y estable de
// preferencias de sello de tiempo.
type ConfiguracionUsuarioTSA struct {
	Enabled *bool
	URL     *string
}

// DocumentoConfiguracionUsuario conserva un bloque tipado y las claves
// legacy desconocidas para mantener compatibilidad durante la migracion.
type DocumentoConfiguracionUsuario struct {
	General      ConfiguracionUsuarioGeneral
	Firma        ConfiguracionUsuarioFirma
	FormatosAuto ConfiguracionUsuarioFormatosAutomaticos
	FirmaMeta    ConfiguracionUsuarioFirmaMetadatos
	FacturaE     ConfiguracionUsuarioFacturaE
	PAdES        ConfiguracionUsuarioPAdES
	CAdES        ConfiguracionUsuarioCAdES
	XAdES        ConfiguracionUsuarioXAdES
	PAdESVisible ConfiguracionUsuarioPAdESVisible
	MultiCoSign  ConfiguracionUsuarioMultiCoSign
	Hash         ConfiguracionUsuarioHash
	Proxy        ConfiguracionUsuarioProxy
	Certificados ConfiguracionUsuarioCertificados
	Desktop      ConfiguracionUsuarioDesktop
	TSA          ConfiguracionUsuarioTSA
	Extras       map[string]any
}

// ConfiguracionUsuarioTipada expone una variante tipada y compatible del
// almacenamiento de preferencias de usuario.
type ConfiguracionUsuarioTipada interface {
	CargarDocumento(ctx context.Context) (DocumentoConfiguracionUsuario, error)
	GuardarDocumento(ctx context.Context, doc DocumentoConfiguracionUsuario) error
}

var ErrProxySecretEnClaro = errors.New("las credenciales de proxy en claro no pueden persistirse en settings; use proxySecretId y almacen seguro del SO")

const claveCredencialSeguridadUIEnClaro = "securityAccessPassword"

const (
	WebCompatibilityMinDurationMinutes = 5
	WebCompatibilityMaxDurationMinutes = 240
)

var ErrWebCompatibilityDuration = errors.New("webCompatibilityDurationMinutes debe ser un entero entre 5 y 240 minutos")

var ErrOpacidadLogoSello = errors.New("signSealLogoOpacityPercent debe ser un entero entre 0 y 100")

// ValidarOpacidadLogoSello rechaza valores fuera de rango o no enteros antes
// de persistir las preferencias del sello. La ausencia equivale a 100 %.
func ValidarOpacidadLogoSello(datos map[string]any) error {
	if datos == nil {
		return nil
	}
	if _, presente := datos["signSealLogoOpacityPercent"]; !presente {
		return nil
	}
	opacidad, ok := extraerInt(datos, "signSealLogoOpacityPercent")
	if !ok || opacidad < 0 || opacidad > 100 {
		return ErrOpacidadLogoSello
	}
	return nil
}

// ValidarSinCredencialesProxyEnClaro rechaza intentos de persistir
// proxyUsername/proxyPassword en claro y limpia claves vacías residuales.
func ValidarSinCredencialesProxyEnClaro(datos map[string]any) error {
	if datos == nil {
		return nil
	}
	for _, clave := range []string{"proxyUsername", "proxyPassword"} {
		valor, presente := datos[clave]
		if !presente {
			continue
		}
		if strings.TrimSpace(valorTextoSettings(valor)) != "" {
			return ErrProxySecretEnClaro
		}
		delete(datos, clave)
	}
	return nil
}

// EliminarCredencialSeguridadUIEnClaro impide que el bloqueo visual de la
// pestaña de seguridad se convierta en una credencial persistente. Esa clave
// solo tiene semántica de sesión dentro de QML: no autentica ninguna operación
// del backend y no está respaldada por el almacén seguro del sistema operativo.
//
// Devuelve true cuando eliminó una clave heredada, para que los adaptadores que
// mantienen un fichero legacy puedan reescribirlo ya saneado.
func EliminarCredencialSeguridadUIEnClaro(datos map[string]any) bool {
	if datos == nil {
		return false
	}
	if _, presente := datos[claveCredencialSeguridadUIEnClaro]; !presente {
		return false
	}
	delete(datos, claveCredencialSeguridadUIEnClaro)
	return true
}

// ValidarDuracionCompatibilidadWeb impide persistir una caducidad que la GUI
// no pueda aplicar de forma segura. La activacion nunca se persiste: solo se
// conserva la duracion elegida para la siguiente activacion explicita.
func ValidarDuracionCompatibilidadWeb(datos map[string]any) error {
	if datos == nil {
		return nil
	}
	if _, presente := datos["webCompatibilityDurationMinutes"]; !presente {
		return nil
	}
	minutos, ok := extraerInt(datos, "webCompatibilityDurationMinutes")
	if !ok ||
		minutos < WebCompatibilityMinDurationMinutes ||
		minutos > WebCompatibilityMaxDurationMinutes {
		return ErrWebCompatibilityDuration
	}
	return nil
}

// DocumentoConfiguracionUsuarioDesdeMapa proyecta un mapa legacy a un
// documento tipado, preservando el resto de claves en Extras.
func DocumentoConfiguracionUsuarioDesdeMapa(datos map[string]any) DocumentoConfiguracionUsuario {
	doc := DocumentoConfiguracionUsuario{
		Extras: clonarMapaAny(datos),
	}
	if doc.Extras == nil {
		doc.Extras = map[string]any{}
	}
	EliminarCredencialSeguridadUIEnClaro(doc.Extras)

	if v, ok := extraerString(doc.Extras, "idioma"); ok {
		doc.General.Idioma = &v
		delete(doc.Extras, "idioma")
	}
	if v, ok := extraerInt(doc.Extras, "themeIndex"); ok {
		doc.General.ThemeIndex = &v
		delete(doc.Extras, "themeIndex")
	}
	if v, ok := extraerBool(doc.Extras, "expertMode"); ok {
		doc.General.ExpertMode = &v
		delete(doc.Extras, "expertMode")
	}
	if v, ok := extraerBool(doc.Extras, "autoClose"); ok {
		doc.General.AutoClose = &v
		delete(doc.Extras, "autoClose")
	}
	if v, ok := extraerBool(doc.Extras, "confirmToSign"); ok {
		doc.General.ConfirmToSign = &v
		delete(doc.Extras, "confirmToSign")
	}
	if v, ok := extraerBool(doc.Extras, "omitAskOnClose"); ok {
		doc.General.OmitAskOnClose = &v
		delete(doc.Extras, "omitAskOnClose")
	}
	if v, ok := extraerCloseBehavior(doc.Extras, "closeBehavior"); ok {
		doc.General.CloseBehavior = &v
		delete(doc.Extras, "closeBehavior")
	}
	if v, ok := extraerBool(doc.Extras, "hideDnieStartScreen"); ok {
		doc.General.HideDnieStartScreen = &v
		delete(doc.Extras, "hideDnieStartScreen")
	}
	if v, ok := extraerBool(doc.Extras, "checkForUpdates"); ok {
		doc.General.CheckForUpdates = &v
		delete(doc.Extras, "checkForUpdates")
	}
	if v, ok := extraerBool(doc.Extras, "secureConnections"); ok {
		doc.General.SecureConnections = &v
		delete(doc.Extras, "secureConnections")
	}
	if v, ok := extraerListaStringsNormalizada(doc.Extras, "secureDomainsList"); ok {
		doc.General.SecureDomainsList = v
		delete(doc.Extras, "secureDomainsList")
	}
	if v, ok := extraerAccionFirma(doc.Extras, "signAction"); ok {
		doc.Firma.Action = &v
		delete(doc.Extras, "signAction")
	}
	if v, ok := extraerFormatoFirma(doc.Extras, "signFormat", true, "pades", "cades", "xades", "xmldsig", "odf", "ooxml", "facturae", "asic-xades"); ok {
		doc.Firma.Format = &v
		delete(doc.Extras, "signFormat")
	}
	if v, ok := extraerPerfilFirma(doc.Extras, "signProfile"); ok {
		doc.Firma.Profile = &v
		delete(doc.Extras, "signProfile")
	}
	if v, ok := extraerModoSobrescritura(doc.Extras, "signOverwrite"); ok {
		doc.Firma.Overwrite = &v
		delete(doc.Extras, "signOverwrite")
	}
	if v, ok := extraerBool(doc.Extras, "signStrictCompat"); ok {
		doc.Firma.StrictCompat = &v
		delete(doc.Extras, "signStrictCompat")
	}
	if v, ok := extraerBool(doc.Extras, "signAllowInvalidPDF"); ok {
		doc.Firma.AllowInvalidPDF = &v
		delete(doc.Extras, "signAllowInvalidPDF")
	}
	if v, ok := extraerFormatoFirma(doc.Extras, "autoFormatPdf", false, "pades", "cades"); ok {
		doc.FormatosAuto.PDF = &v
		delete(doc.Extras, "autoFormatPdf")
	}
	if v, ok := extraerFormatoFirma(doc.Extras, "autoFormatOoxml", false, "ooxml", "cades"); ok {
		doc.FormatosAuto.OOXML = &v
		delete(doc.Extras, "autoFormatOoxml")
	}
	if v, ok := extraerFormatoFirma(doc.Extras, "autoFormatFacturae", false, "facturae", "xades"); ok {
		doc.FormatosAuto.FacturaE = &v
		delete(doc.Extras, "autoFormatFacturae")
	}
	if v, ok := extraerFormatoFirma(doc.Extras, "autoFormatOdf", false, "odf", "cades"); ok {
		doc.FormatosAuto.ODF = &v
		delete(doc.Extras, "autoFormatOdf")
	}
	if v, ok := extraerFormatoFirma(doc.Extras, "autoFormatXml", false, "xades", "xmldsig"); ok {
		doc.FormatosAuto.XML = &v
		delete(doc.Extras, "autoFormatXml")
	}
	if v, ok := extraerFormatoFirma(doc.Extras, "autoFormatBinary", false, "cades", "asic-xades"); ok {
		doc.FormatosAuto.Binary = &v
		delete(doc.Extras, "autoFormatBinary")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signReason"); ok {
		doc.FirmaMeta.Reason = &v
		delete(doc.Extras, "signReason")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signLocation"); ok {
		doc.FirmaMeta.Location = &v
		delete(doc.Extras, "signLocation")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signContactInfo"); ok {
		doc.FirmaMeta.ContactInfo = &v
		delete(doc.Extras, "signContactInfo")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "facturaePolicyVersion"); ok {
		doc.FacturaE.PolicyVersion = &v
		delete(doc.Extras, "facturaePolicyVersion")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "policyIdentifier"); ok {
		doc.FacturaE.PolicyID = &v
		delete(doc.Extras, "policyIdentifier")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "policyIdentifierHash"); ok {
		doc.FacturaE.PolicyHash = &v
		delete(doc.Extras, "policyIdentifierHash")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "policyQualifier"); ok {
		doc.FacturaE.PolicyQualifier = &v
		delete(doc.Extras, "policyQualifier")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signerClaimedRole"); ok {
		doc.FacturaE.SignerRole = &v
		delete(doc.Extras, "signerClaimedRole")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signatureProductionCity"); ok {
		doc.FacturaE.City = &v
		delete(doc.Extras, "signatureProductionCity")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signatureProductionProvince"); ok {
		doc.FacturaE.Province = &v
		delete(doc.Extras, "signatureProductionProvince")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signatureProductionPostalCode"); ok {
		doc.FacturaE.PostalCode = &v
		delete(doc.Extras, "signatureProductionPostalCode")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signatureProductionCountry"); ok {
		doc.FacturaE.Country = &v
		delete(doc.Extras, "signatureProductionCountry")
	}
	if v, ok := extraerSubfiltroPAdES(doc.Extras, "padesSubFilter"); ok {
		doc.PAdES.SubFilter = &v
		delete(doc.Extras, "padesSubFilter")
	} else if v, ok := extraerSubfiltroPAdES(doc.Extras, "padesBasicFormat"); ok {
		doc.PAdES.SubFilter = &v
		delete(doc.Extras, "padesBasicFormat")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "padesPolicyIdentifier"); ok {
		doc.PAdES.PolicyID = &v
		delete(doc.Extras, "padesPolicyIdentifier")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "padesPolicyIdentifierHash"); ok {
		doc.PAdES.PolicyHash = &v
		delete(doc.Extras, "padesPolicyIdentifierHash")
	}
	if v, ok := extraerAlgoritmoHash(doc.Extras, "padesPolicyIdentifierHashAlgorithm"); ok {
		doc.PAdES.PolicyHashAlgorithm = &v
		delete(doc.Extras, "padesPolicyIdentifierHashAlgorithm")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "padesPolicyQualifier"); ok {
		doc.PAdES.PolicyQualifier = &v
		delete(doc.Extras, "padesPolicyQualifier")
	}
	if v, ok := extraerBool(doc.Extras, "padesObfuscateCertInfo"); ok {
		doc.PAdES.ObfuscateCertInfo = &v
		delete(doc.Extras, "padesObfuscateCertInfo")
	}
	if v, ok := extraerBool(doc.Extras, "padesVisibleStamp"); ok {
		doc.PAdES.VisibleStamp = &v
		delete(doc.Extras, "padesVisibleStamp")
	}
	if v, ok := extraerBool(doc.Extras, "allowShadowAttack"); ok {
		doc.PAdES.AllowShadowAttack = &v
		delete(doc.Extras, "allowShadowAttack")
	}
	if v, ok := extraerBool(doc.Extras, "allowCertifiedPDF"); ok {
		doc.PAdES.AllowCertifiedPDF = &v
		delete(doc.Extras, "allowCertifiedPDF")
	}
	if v, ok := extraerNivelCertificacionPDF(doc.Extras, "padesCertificationLevel"); ok {
		doc.PAdES.CertificationLevel = &v
		delete(doc.Extras, "padesCertificationLevel")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "cadesPolicyIdentifier"); ok {
		doc.CAdES.PolicyID = &v
		delete(doc.Extras, "cadesPolicyIdentifier")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "cadesPolicyIdentifierHash"); ok {
		doc.CAdES.PolicyHash = &v
		delete(doc.Extras, "cadesPolicyIdentifierHash")
	}
	if v, ok := extraerAlgoritmoHash(doc.Extras, "cadesPolicyIdentifierHashAlgorithm"); ok {
		doc.CAdES.PolicyHashAlgorithm = &v
		delete(doc.Extras, "cadesPolicyIdentifierHashAlgorithm")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "cadesPolicyQualifier"); ok {
		doc.CAdES.PolicyQualifier = &v
		delete(doc.Extras, "cadesPolicyQualifier")
	}
	if v, ok := extraerBool(doc.Extras, "cadesImplicitMode"); ok {
		doc.CAdES.ImplicitMode = &v
		delete(doc.Extras, "cadesImplicitMode")
	}
	if v, ok := extraerModoMultifirma(doc.Extras, "cadesMultisign"); ok {
		doc.CAdES.Multisign = &v
		delete(doc.Extras, "cadesMultisign")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "xadesPolicyIdentifier"); ok {
		doc.XAdES.PolicyID = &v
		delete(doc.Extras, "xadesPolicyIdentifier")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "xadesPolicyIdentifierHash"); ok {
		doc.XAdES.PolicyHash = &v
		delete(doc.Extras, "xadesPolicyIdentifierHash")
	}
	if v, ok := extraerAlgoritmoHash(doc.Extras, "xadesPolicyIdentifierHashAlgorithm"); ok {
		doc.XAdES.PolicyHashAlgorithm = &v
		delete(doc.Extras, "xadesPolicyIdentifierHashAlgorithm")
	}
	if v, ok := extraerURIAbsolutaNoVacia(doc.Extras, "xadesPolicyQualifier"); ok {
		doc.XAdES.PolicyQualifier = &v
		delete(doc.Extras, "xadesPolicyQualifier")
	}
	if v, ok := extraerFormatoXAdES(doc.Extras, "xadesSignFormat"); ok {
		doc.XAdES.SignFormat = &v
		delete(doc.Extras, "xadesSignFormat")
	}
	if v, ok := extraerModoMultifirma(doc.Extras, "xadesMultisign"); ok {
		doc.XAdES.Multisign = &v
		delete(doc.Extras, "xadesMultisign")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "xadesSignerClaimedRole"); ok {
		doc.XAdES.ClaimedRole = &v
		delete(doc.Extras, "xadesSignerClaimedRole")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "xadesSignatureProductionCity"); ok {
		doc.XAdES.City = &v
		delete(doc.Extras, "xadesSignatureProductionCity")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "xadesSignatureProductionProvince"); ok {
		doc.XAdES.Province = &v
		delete(doc.Extras, "xadesSignatureProductionProvince")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "xadesSignatureProductionPostalCode"); ok {
		doc.XAdES.PostalCode = &v
		delete(doc.Extras, "xadesSignatureProductionPostalCode")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "xadesSignatureProductionCountry"); ok {
		doc.XAdES.Country = &v
		delete(doc.Extras, "xadesSignatureProductionCountry")
	}
	if v, ok := extraerBool(doc.Extras, "signVisibleSeal"); ok {
		doc.PAdESVisible.Enabled = &v
		delete(doc.Extras, "signVisibleSeal")
	}
	if v, ok := extraerPaginasSello(doc.Extras, "signSealPages"); ok {
		doc.PAdESVisible.Pages = &v
		delete(doc.Extras, "signSealPages")
	}
	if v, ok := extraerBool(doc.Extras, "signSealAllPages"); ok {
		doc.PAdESVisible.AllPages = &v
		delete(doc.Extras, "signSealAllPages")
	}
	if v, ok := extraerFloat01(doc.Extras, "signSealX"); ok {
		doc.PAdESVisible.X = &v
		delete(doc.Extras, "signSealX")
	}
	if v, ok := extraerFloat01(doc.Extras, "signSealY"); ok {
		doc.PAdESVisible.Y = &v
		delete(doc.Extras, "signSealY")
	}
	if v, ok := extraerFloat01(doc.Extras, "signSealW"); ok {
		doc.PAdESVisible.W = &v
		delete(doc.Extras, "signSealW")
	}
	if v, ok := extraerFloat01(doc.Extras, "signSealH"); ok {
		doc.PAdESVisible.H = &v
		delete(doc.Extras, "signSealH")
	}
	if v, ok := extraerBool(doc.Extras, "signSealKeepText"); ok {
		doc.PAdESVisible.KeepText = &v
		delete(doc.Extras, "signSealKeepText")
	}
	if v, ok := extraerRotacionSello(doc.Extras, "signSealRotation"); ok {
		doc.PAdESVisible.Rotation = &v
		delete(doc.Extras, "signSealRotation")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signSealImagePath"); ok {
		doc.PAdESVisible.ImagePath = &v
		delete(doc.Extras, "signSealImagePath")
	}
	if v, ok := extraerInt(doc.Extras, "signSealLogoOpacityPercent"); ok && v >= 0 && v <= 100 {
		doc.PAdESVisible.LogoOpacityPercent = &v
		delete(doc.Extras, "signSealLogoOpacityPercent")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "signQRContent"); ok {
		doc.PAdESVisible.QRContent = &v
		delete(doc.Extras, "signQRContent")
	}
	if v, ok := extraerBool(doc.Extras, "multiCosignEnabled"); ok {
		doc.MultiCoSign.Enabled = &v
		delete(doc.Extras, "multiCosignEnabled")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "multiCosignPrimaryCertificateId"); ok {
		doc.MultiCoSign.PrimaryID = &v
		delete(doc.Extras, "multiCosignPrimaryCertificateId")
	}
	if v, ok := extraerListaStringsNormalizada(doc.Extras, "multiCosignCertificateIds"); ok {
		doc.MultiCoSign.CertificateIDs = v
		delete(doc.Extras, "multiCosignCertificateIds")
	}
	if v, ok := extraerAlgoritmoHash(doc.Extras, "defaultHashAlgorithm"); ok {
		doc.Hash.Algorithm = &v
		delete(doc.Extras, "defaultHashAlgorithm")
	}
	if v, ok := extraerBool(doc.Extras, "defaultHashCopyToClipboard"); ok {
		doc.Hash.CopyToClipboard = &v
		delete(doc.Extras, "defaultHashCopyToClipboard")
	}
	if v, ok := extraerFormatoHashFichero(doc.Extras, "defaultHashFormatFile"); ok {
		doc.Hash.FormatFile = &v
		delete(doc.Extras, "defaultHashFormatFile")
	}
	if v, ok := extraerFormatoHashDirectorio(doc.Extras, "defaultHashFormatDirectory"); ok {
		doc.Hash.FormatDirectory = &v
		delete(doc.Extras, "defaultHashFormatDirectory")
	}
	if v, ok := extraerBool(doc.Extras, "defaultHashRecursive"); ok {
		doc.Hash.Recursive = &v
		delete(doc.Extras, "defaultHashRecursive")
	}
	if v, ok := extraerBool(doc.Extras, "defaultHashSaveReport"); ok {
		doc.Hash.SaveReport = &v
		delete(doc.Extras, "defaultHashSaveReport")
	}
	if v, ok := extraerBool(doc.Extras, "proxyEnabled"); ok {
		doc.Proxy.Enabled = &v
		delete(doc.Extras, "proxyEnabled")
	}
	if v, ok := extraerProxyType(doc.Extras, "proxyType"); ok {
		doc.Proxy.Type = &v
		delete(doc.Extras, "proxyType")
	}
	if v, ok := extraerString(doc.Extras, "proxyHost"); ok {
		doc.Proxy.Host = &v
		delete(doc.Extras, "proxyHost")
	}
	if v, ok := extraerInt(doc.Extras, "proxyPort"); ok {
		doc.Proxy.Port = &v
		delete(doc.Extras, "proxyPort")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "proxySecretId"); ok {
		doc.Proxy.SecretID = &v
		delete(doc.Extras, "proxySecretId")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "proxyRealm"); ok {
		doc.Proxy.Realm = &v
		delete(doc.Extras, "proxyRealm")
	}
	if v, ok := extraerListaStringsNormalizada(doc.Extras, "proxyExcludedUrls"); ok {
		doc.Proxy.ExcludedURLs = v
		delete(doc.Extras, "proxyExcludedUrls")
	}
	if v, ok := extraerBool(doc.Extras, "stickySigner"); ok {
		doc.Certificados.StickySigner = &v
		delete(doc.Extras, "stickySigner")
	}
	if v, ok := extraerBool(doc.Extras, "autoSelectSingleCertificate"); ok {
		doc.Certificados.AutoSelectSingleCertificate = &v
		delete(doc.Extras, "autoSelectSingleCertificate")
	}
	if v, ok := extraerBool(doc.Extras, "preferDefaultCertificate"); ok {
		doc.Certificados.PreferDefaultCertificate = &v
		delete(doc.Extras, "preferDefaultCertificate")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "preferredCertificateId"); ok {
		doc.Certificados.PreferredCertificateID = &v
		delete(doc.Extras, "preferredCertificateId")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "defaultCertificateId"); ok {
		doc.Certificados.DefaultCertificateID = &v
		delete(doc.Extras, "defaultCertificateId")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "defaultKeystore"); ok {
		doc.Certificados.DefaultKeystore = &v
		delete(doc.Extras, "defaultKeystore")
	}
	if v, ok := extraerStringNoVacia(doc.Extras, "defaultLocalKeystorePath"); ok {
		doc.Certificados.DefaultLocalKeystorePath = &v
		delete(doc.Extras, "defaultLocalKeystorePath")
	}
	if v, ok := extraerBool(doc.Extras, "useDefaultStoreInBrowserCalls"); ok {
		doc.Certificados.UseDefaultStoreInBrowserCalls = &v
		delete(doc.Extras, "useDefaultStoreInBrowserCalls")
	}
	if v, ok := extraerBool(doc.Extras, "useOnlySignatureCertificates"); ok {
		doc.Certificados.UseOnlySignatureCertificates = &v
		delete(doc.Extras, "useOnlySignatureCertificates")
	}
	if v, ok := extraerBool(doc.Extras, "useOnlyAliasCertificates"); ok {
		doc.Certificados.UseOnlyAliasCertificates = &v
		delete(doc.Extras, "useOnlyAliasCertificates")
	}
	if v, ok := extraerBool(doc.Extras, "skipAuthCertDnie"); ok {
		doc.Certificados.SkipAuthCertDnie = &v
		delete(doc.Extras, "skipAuthCertDnie")
	}
	if v, ok := extraerBool(doc.Extras, "showDefaultCertificateFirst"); ok {
		doc.Certificados.ShowDefaultFirst = &v
		delete(doc.Extras, "showDefaultCertificateFirst")
	}
	if v, ok := extraerBool(doc.Extras, "showUsableCertificatesFirst"); ok {
		doc.Certificados.ShowUsableFirst = &v
		delete(doc.Extras, "showUsableCertificatesFirst")
	}
	if v, ok := extraerBool(doc.Extras, "showValidCertificatesFirst"); ok {
		doc.Certificados.ShowValidFirst = &v
		delete(doc.Extras, "showValidCertificatesFirst")
	}
	if v, ok := extraerBool(doc.Extras, "rememberCertificateFilter"); ok {
		doc.Certificados.RememberFilter = &v
		delete(doc.Extras, "rememberCertificateFilter")
	}
	if v, ok := extraerStringRecortada(doc.Extras, "certificateFilterText"); ok {
		doc.Certificados.FilterText = &v
		delete(doc.Extras, "certificateFilterText")
	}
	if v, ok := extraerBool(doc.Extras, "certsExpiredShow"); ok {
		doc.Certificados.ShowExpired = &v
		delete(doc.Extras, "certsExpiredShow")
	}
	if v, ok := extraerBool(doc.Extras, "certsInvalidShow"); ok {
		doc.Certificados.ShowInvalid = &v
		delete(doc.Extras, "certsInvalidShow")
	}
	if v, ok := extraerListaTiposCertificado(doc.Extras, "certificateTypeFilter"); ok {
		doc.Certificados.TypeFilter = v
		delete(doc.Extras, "certificateTypeFilter")
	}
	if v, ok := extraerBool(doc.Extras, "certificateRequireNIF"); ok {
		doc.Certificados.RequireNIF = &v
		delete(doc.Extras, "certificateRequireNIF")
	}
	if v, ok := extraerBool(doc.Extras, "certificateRequireOrganization"); ok {
		doc.Certificados.RequireOrganization = &v
		delete(doc.Extras, "certificateRequireOrganization")
	}
	if v, ok := extraerString(doc.Extras, "legacyWebUiSize"); ok {
		doc.Desktop.LegacyWebUISize = &v
		delete(doc.Extras, "legacyWebUiSize")
	}
	if v, ok := extraerBool(doc.Extras, "legacyWebTrayResident"); ok {
		doc.Desktop.LegacyWebTrayResident = &v
		delete(doc.Extras, "legacyWebTrayResident")
	}
	if v, ok := extraerInt(doc.Extras, "webCompatibilityDurationMinutes"); ok &&
		v >= WebCompatibilityMinDurationMinutes &&
		v <= WebCompatibilityMaxDurationMinutes {
		doc.Desktop.WebCompatibilityDurationMinutes = &v
	}
	// Una duracion heredada invalida no debe sobrevivir como Extra y reaparecer
	// en la GUI: el comportamiento seguro es volver al valor por defecto. El
	// estado de ejecucion y su vencimiento nunca son preferencias persistentes.
	delete(doc.Extras, "webCompatibilityDurationMinutes")
	delete(doc.Extras, "webCompatibilityActive")
	delete(doc.Extras, "webCompatibilityExpiresAt")
	if v, ok := extraerBool(doc.Extras, "facturaeToolsEnabled"); ok {
		doc.Desktop.FacturaeToolsEnabled = &v
		delete(doc.Extras, "facturaeToolsEnabled")
	}
	if v, ok := extraerURLHTTPNoVacia(doc.Extras, "tsaUrl"); ok {
		doc.TSA.URL = &v
		delete(doc.Extras, "tsaUrl")
	}
	if v, ok := extraerBool(doc.Extras, "tsaEnabled"); ok {
		doc.TSA.Enabled = &v
		delete(doc.Extras, "tsaEnabled")
	}

	return doc
}

// Mapa devuelve la representacion legacy plana preservando Extras.
func (d DocumentoConfiguracionUsuario) Mapa() map[string]any {
	out := clonarMapaAny(d.Extras)
	if out == nil {
		out = map[string]any{}
	}
	EliminarCredencialSeguridadUIEnClaro(out)
	delete(out, "webCompatibilityDurationMinutes")
	delete(out, "webCompatibilityActive")
	delete(out, "webCompatibilityExpiresAt")

	if d.General.Idioma != nil {
		out["idioma"] = *d.General.Idioma
	}
	if d.General.ThemeIndex != nil {
		out["themeIndex"] = *d.General.ThemeIndex
	}
	if d.General.ExpertMode != nil {
		out["expertMode"] = *d.General.ExpertMode
	}
	if d.General.AutoClose != nil {
		out["autoClose"] = *d.General.AutoClose
	}
	if d.General.ConfirmToSign != nil {
		out["confirmToSign"] = *d.General.ConfirmToSign
	}
	if d.General.OmitAskOnClose != nil {
		out["omitAskOnClose"] = *d.General.OmitAskOnClose
	}
	if d.General.CloseBehavior != nil {
		out["closeBehavior"] = *d.General.CloseBehavior
	}
	if d.General.HideDnieStartScreen != nil {
		out["hideDnieStartScreen"] = *d.General.HideDnieStartScreen
	}
	if d.General.CheckForUpdates != nil {
		out["checkForUpdates"] = *d.General.CheckForUpdates
	}
	if d.General.SecureConnections != nil {
		out["secureConnections"] = *d.General.SecureConnections
	}
	if len(d.General.SecureDomainsList) > 0 {
		out["secureDomainsList"] = append([]string(nil), d.General.SecureDomainsList...)
	}
	if d.Firma.Action != nil {
		out["signAction"] = *d.Firma.Action
	}
	if d.Firma.Format != nil {
		out["signFormat"] = *d.Firma.Format
	}
	if d.Firma.Profile != nil {
		out["signProfile"] = *d.Firma.Profile
	}
	if d.Firma.Overwrite != nil {
		out["signOverwrite"] = *d.Firma.Overwrite
	}
	if d.Firma.StrictCompat != nil {
		out["signStrictCompat"] = *d.Firma.StrictCompat
	}
	if d.Firma.AllowInvalidPDF != nil {
		out["signAllowInvalidPDF"] = *d.Firma.AllowInvalidPDF
	}
	if d.FormatosAuto.PDF != nil {
		out["autoFormatPdf"] = *d.FormatosAuto.PDF
	}
	if d.FormatosAuto.OOXML != nil {
		out["autoFormatOoxml"] = *d.FormatosAuto.OOXML
	}
	if d.FormatosAuto.FacturaE != nil {
		out["autoFormatFacturae"] = *d.FormatosAuto.FacturaE
	}
	if d.FormatosAuto.ODF != nil {
		out["autoFormatOdf"] = *d.FormatosAuto.ODF
	}
	if d.FormatosAuto.XML != nil {
		out["autoFormatXml"] = *d.FormatosAuto.XML
	}
	if d.FormatosAuto.Binary != nil {
		out["autoFormatBinary"] = *d.FormatosAuto.Binary
	}
	if d.FirmaMeta.Reason != nil {
		out["signReason"] = *d.FirmaMeta.Reason
	}
	if d.FirmaMeta.Location != nil {
		out["signLocation"] = *d.FirmaMeta.Location
	}
	if d.FirmaMeta.ContactInfo != nil {
		out["signContactInfo"] = *d.FirmaMeta.ContactInfo
	}
	if d.FacturaE.PolicyVersion != nil {
		out["facturaePolicyVersion"] = *d.FacturaE.PolicyVersion
	}
	if d.FacturaE.PolicyID != nil {
		out["policyIdentifier"] = *d.FacturaE.PolicyID
	}
	if d.FacturaE.PolicyHash != nil {
		out["policyIdentifierHash"] = *d.FacturaE.PolicyHash
	}
	if d.FacturaE.PolicyQualifier != nil {
		out["policyQualifier"] = *d.FacturaE.PolicyQualifier
	}
	if d.FacturaE.SignerRole != nil {
		out["signerClaimedRole"] = *d.FacturaE.SignerRole
	}
	if d.FacturaE.City != nil {
		out["signatureProductionCity"] = *d.FacturaE.City
	}
	if d.FacturaE.Province != nil {
		out["signatureProductionProvince"] = *d.FacturaE.Province
	}
	if d.FacturaE.PostalCode != nil {
		out["signatureProductionPostalCode"] = *d.FacturaE.PostalCode
	}
	if d.FacturaE.Country != nil {
		out["signatureProductionCountry"] = *d.FacturaE.Country
	}
	if d.PAdES.SubFilter != nil {
		out["padesSubFilter"] = *d.PAdES.SubFilter
	}
	if d.PAdES.PolicyID != nil {
		out["padesPolicyIdentifier"] = *d.PAdES.PolicyID
	}
	if d.PAdES.PolicyHash != nil {
		out["padesPolicyIdentifierHash"] = *d.PAdES.PolicyHash
	}
	if d.PAdES.PolicyHashAlgorithm != nil {
		out["padesPolicyIdentifierHashAlgorithm"] = *d.PAdES.PolicyHashAlgorithm
	}
	if d.PAdES.PolicyQualifier != nil {
		out["padesPolicyQualifier"] = *d.PAdES.PolicyQualifier
	}
	if d.PAdES.ObfuscateCertInfo != nil {
		out["padesObfuscateCertInfo"] = *d.PAdES.ObfuscateCertInfo
	}
	if d.PAdES.VisibleStamp != nil {
		out["padesVisibleStamp"] = *d.PAdES.VisibleStamp
	}
	if d.PAdES.AllowShadowAttack != nil {
		out["allowShadowAttack"] = *d.PAdES.AllowShadowAttack
	}
	if d.PAdES.AllowCertifiedPDF != nil {
		out["allowCertifiedPDF"] = *d.PAdES.AllowCertifiedPDF
	}
	if d.PAdES.CertificationLevel != nil {
		out["padesCertificationLevel"] = *d.PAdES.CertificationLevel
	}
	if d.CAdES.PolicyID != nil {
		out["cadesPolicyIdentifier"] = *d.CAdES.PolicyID
	}
	if d.CAdES.PolicyHash != nil {
		out["cadesPolicyIdentifierHash"] = *d.CAdES.PolicyHash
	}
	if d.CAdES.PolicyHashAlgorithm != nil {
		out["cadesPolicyIdentifierHashAlgorithm"] = *d.CAdES.PolicyHashAlgorithm
	}
	if d.CAdES.PolicyQualifier != nil {
		out["cadesPolicyQualifier"] = *d.CAdES.PolicyQualifier
	}
	if d.CAdES.ImplicitMode != nil {
		out["cadesImplicitMode"] = *d.CAdES.ImplicitMode
	}
	if d.CAdES.Multisign != nil {
		out["cadesMultisign"] = *d.CAdES.Multisign
	}
	if d.XAdES.PolicyID != nil {
		out["xadesPolicyIdentifier"] = *d.XAdES.PolicyID
	}
	if d.XAdES.PolicyHash != nil {
		out["xadesPolicyIdentifierHash"] = *d.XAdES.PolicyHash
	}
	if d.XAdES.PolicyHashAlgorithm != nil {
		out["xadesPolicyIdentifierHashAlgorithm"] = *d.XAdES.PolicyHashAlgorithm
	}
	if d.XAdES.PolicyQualifier != nil {
		out["xadesPolicyQualifier"] = *d.XAdES.PolicyQualifier
	}
	if d.XAdES.SignFormat != nil {
		out["xadesSignFormat"] = *d.XAdES.SignFormat
	}
	if d.XAdES.Multisign != nil {
		out["xadesMultisign"] = *d.XAdES.Multisign
	}
	if d.XAdES.ClaimedRole != nil {
		out["xadesSignerClaimedRole"] = *d.XAdES.ClaimedRole
	}
	if d.XAdES.City != nil {
		out["xadesSignatureProductionCity"] = *d.XAdES.City
	}
	if d.XAdES.Province != nil {
		out["xadesSignatureProductionProvince"] = *d.XAdES.Province
	}
	if d.XAdES.PostalCode != nil {
		out["xadesSignatureProductionPostalCode"] = *d.XAdES.PostalCode
	}
	if d.XAdES.Country != nil {
		out["xadesSignatureProductionCountry"] = *d.XAdES.Country
	}
	if d.PAdESVisible.Enabled != nil {
		out["signVisibleSeal"] = *d.PAdESVisible.Enabled
	}
	if d.PAdESVisible.Pages != nil {
		out["signSealPages"] = *d.PAdESVisible.Pages
	}
	if d.PAdESVisible.AllPages != nil {
		out["signSealAllPages"] = *d.PAdESVisible.AllPages
	}
	if d.PAdESVisible.X != nil {
		out["signSealX"] = *d.PAdESVisible.X
	}
	if d.PAdESVisible.Y != nil {
		out["signSealY"] = *d.PAdESVisible.Y
	}
	if d.PAdESVisible.W != nil {
		out["signSealW"] = *d.PAdESVisible.W
	}
	if d.PAdESVisible.H != nil {
		out["signSealH"] = *d.PAdESVisible.H
	}
	if d.PAdESVisible.KeepText != nil {
		out["signSealKeepText"] = *d.PAdESVisible.KeepText
	}
	if d.PAdESVisible.Rotation != nil {
		out["signSealRotation"] = *d.PAdESVisible.Rotation
	}
	if d.PAdESVisible.ImagePath != nil {
		out["signSealImagePath"] = *d.PAdESVisible.ImagePath
	}
	if d.PAdESVisible.LogoOpacityPercent != nil {
		out["signSealLogoOpacityPercent"] = *d.PAdESVisible.LogoOpacityPercent
	}
	if d.PAdESVisible.QRContent != nil {
		out["signQRContent"] = *d.PAdESVisible.QRContent
	}
	if d.MultiCoSign.Enabled != nil {
		out["multiCosignEnabled"] = *d.MultiCoSign.Enabled
	}
	if d.MultiCoSign.PrimaryID != nil {
		out["multiCosignPrimaryCertificateId"] = *d.MultiCoSign.PrimaryID
	}
	if len(d.MultiCoSign.CertificateIDs) > 0 {
		out["multiCosignCertificateIds"] = append([]string(nil), d.MultiCoSign.CertificateIDs...)
	}
	if d.Hash.Algorithm != nil {
		out["defaultHashAlgorithm"] = *d.Hash.Algorithm
	}
	if d.Hash.CopyToClipboard != nil {
		out["defaultHashCopyToClipboard"] = *d.Hash.CopyToClipboard
	}
	if d.Hash.FormatFile != nil {
		out["defaultHashFormatFile"] = *d.Hash.FormatFile
	}
	if d.Hash.FormatDirectory != nil {
		out["defaultHashFormatDirectory"] = *d.Hash.FormatDirectory
	}
	if d.Hash.Recursive != nil {
		out["defaultHashRecursive"] = *d.Hash.Recursive
	}
	if d.Hash.SaveReport != nil {
		out["defaultHashSaveReport"] = *d.Hash.SaveReport
	}
	if d.Proxy.Enabled != nil {
		out["proxyEnabled"] = *d.Proxy.Enabled
	}
	if d.Proxy.Type != nil {
		out["proxyType"] = *d.Proxy.Type
	}
	if d.Proxy.Host != nil {
		out["proxyHost"] = *d.Proxy.Host
	}
	if d.Proxy.Port != nil {
		out["proxyPort"] = *d.Proxy.Port
	}
	if d.Proxy.SecretID != nil {
		out["proxySecretId"] = *d.Proxy.SecretID
	}
	if d.Proxy.Realm != nil {
		out["proxyRealm"] = *d.Proxy.Realm
	}
	if len(d.Proxy.ExcludedURLs) > 0 {
		out["proxyExcludedUrls"] = append([]string(nil), d.Proxy.ExcludedURLs...)
	}
	if d.Certificados.StickySigner != nil {
		out["stickySigner"] = *d.Certificados.StickySigner
	}
	if d.Certificados.AutoSelectSingleCertificate != nil {
		out["autoSelectSingleCertificate"] = *d.Certificados.AutoSelectSingleCertificate
	}
	if d.Certificados.PreferDefaultCertificate != nil {
		out["preferDefaultCertificate"] = *d.Certificados.PreferDefaultCertificate
	}
	if d.Certificados.PreferredCertificateID != nil {
		out["preferredCertificateId"] = *d.Certificados.PreferredCertificateID
	}
	if d.Certificados.DefaultCertificateID != nil {
		out["defaultCertificateId"] = *d.Certificados.DefaultCertificateID
	}
	if d.Certificados.DefaultKeystore != nil {
		out["defaultKeystore"] = *d.Certificados.DefaultKeystore
	}
	if d.Certificados.DefaultLocalKeystorePath != nil {
		out["defaultLocalKeystorePath"] = *d.Certificados.DefaultLocalKeystorePath
	}
	if d.Certificados.UseDefaultStoreInBrowserCalls != nil {
		out["useDefaultStoreInBrowserCalls"] = *d.Certificados.UseDefaultStoreInBrowserCalls
	}
	if d.Certificados.UseOnlySignatureCertificates != nil {
		out["useOnlySignatureCertificates"] = *d.Certificados.UseOnlySignatureCertificates
	}
	if d.Certificados.UseOnlyAliasCertificates != nil {
		out["useOnlyAliasCertificates"] = *d.Certificados.UseOnlyAliasCertificates
	}
	if d.Certificados.SkipAuthCertDnie != nil {
		out["skipAuthCertDnie"] = *d.Certificados.SkipAuthCertDnie
	}
	if d.Certificados.ShowDefaultFirst != nil {
		out["showDefaultCertificateFirst"] = *d.Certificados.ShowDefaultFirst
	}
	if d.Certificados.ShowUsableFirst != nil {
		out["showUsableCertificatesFirst"] = *d.Certificados.ShowUsableFirst
	}
	if d.Certificados.ShowValidFirst != nil {
		out["showValidCertificatesFirst"] = *d.Certificados.ShowValidFirst
	}
	if d.Certificados.RememberFilter != nil {
		out["rememberCertificateFilter"] = *d.Certificados.RememberFilter
	}
	if d.Certificados.FilterText != nil {
		out["certificateFilterText"] = *d.Certificados.FilterText
	}
	if d.Certificados.ShowExpired != nil {
		out["certsExpiredShow"] = *d.Certificados.ShowExpired
	}
	if d.Certificados.ShowInvalid != nil {
		out["certsInvalidShow"] = *d.Certificados.ShowInvalid
	}
	if len(d.Certificados.TypeFilter) > 0 {
		out["certificateTypeFilter"] = append([]string(nil), d.Certificados.TypeFilter...)
	}
	if d.Certificados.RequireNIF != nil {
		out["certificateRequireNIF"] = *d.Certificados.RequireNIF
	}
	if d.Certificados.RequireOrganization != nil {
		out["certificateRequireOrganization"] = *d.Certificados.RequireOrganization
	}
	if d.Desktop.LegacyWebUISize != nil {
		out["legacyWebUiSize"] = *d.Desktop.LegacyWebUISize
	}
	if d.Desktop.LegacyWebTrayResident != nil {
		out["legacyWebTrayResident"] = *d.Desktop.LegacyWebTrayResident
	}
	if d.Desktop.WebCompatibilityDurationMinutes != nil {
		out["webCompatibilityDurationMinutes"] = *d.Desktop.WebCompatibilityDurationMinutes
	}
	if d.Desktop.FacturaeToolsEnabled != nil {
		out["facturaeToolsEnabled"] = *d.Desktop.FacturaeToolsEnabled
	}
	if d.TSA.URL != nil {
		out["tsaUrl"] = *d.TSA.URL
	}
	if d.TSA.Enabled != nil {
		out["tsaEnabled"] = *d.TSA.Enabled
	}

	return out
}

func clonarMapaAny(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func valorTextoSettings(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}

func extraerString(datos map[string]any, clave string) (string, bool) {
	v, ok := datos[clave]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func extraerStringNoVacia(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerString(datos, clave)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func extraerStringRecortada(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerString(datos, clave)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func extraerPaginasSello(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerString(datos, clave)
	if !ok {
		return "", false
	}
	normalizado := strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if normalizado == "" {
		return "", false
	}
	switch strings.ToLower(normalizado) {
	case "all", "todas", "todos", "*":
		return "all", true
	}
	partes := strings.Split(normalizado, ",")
	if len(partes) == 0 {
		return "", false
	}
	for _, parte := range partes {
		if parte == "" {
			return "", false
		}
		if strings.Contains(parte, "-") {
			limites := strings.SplitN(parte, "-", 2)
			if len(limites) != 2 {
				return "", false
			}
			inicio, okInicio := extraerEnteroPositivo(limites[0])
			fin, okFin := extraerEnteroPositivo(limites[1])
			if !okInicio || !okFin || inicio > fin {
				return "", false
			}
			continue
		}
		if _, ok := extraerEnteroPositivo(parte); !ok {
			return "", false
		}
	}
	return normalizado, true
}

func extraerURIAbsolutaNoVacia(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil || u == nil || u.Scheme == "" {
		return "", false
	}
	if !u.IsAbs() {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "urn", "oid":
	default:
		return "", false
	}
	if u.Host == "" && u.Opaque == "" {
		return "", false
	}
	return s, true
}

func extraerURLHTTPNoVacia(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil || u == nil || !u.IsAbs() {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", false
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", false
	}
	return s, true
}

func extraerEnteroPositivo(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	// La acumulacion manual daba la vuelta al entero con cadenas de digitos
	// largas y colaba valores pequenos como validos: "18446744073709551617"
	// devolvia (1, true). strconv.Atoi acota el rango y falla al desbordar.
	// El bucle anterior se conserva para seguir rechazando signos y espacios
	// que Atoi si aceptaria ("+5", " 5").
	valor, err := strconv.Atoi(s)
	if err != nil || valor < 1 {
		return 0, false
	}
	return valor, true
}

func extraerCloseBehavior(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "exit":
		return "exit", true
	case "resident":
		return "resident", true
	default:
		return "", false
	}
}

func extraerAccionFirma(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sign":
		return "sign", true
	case "cosign":
		return "cosign", true
	case "countersign":
		return "countersign", true
	default:
		return "", false
	}
}

func extraerFormatoFirma(datos map[string]any, clave string, permiteVacio bool, permitidos ...string) (string, bool) {
	s, ok := extraerString(datos, clave)
	if !ok {
		return "", false
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" && permiteVacio {
		return "", true
	}
	for _, permitido := range permitidos {
		if s == permitido {
			return s, true
		}
	}
	return "", false
}

func extraerModoSobrescritura(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "rename":
		return "rename", true
	case "fail":
		return "fail", true
	case "force":
		return "force", true
	default:
		return "", false
	}
}

func extraerModoMultifirma(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cosign":
		return "cosign", true
	case "countersign":
		return "countersign", true
	default:
		return "", false
	}
}

func extraerProxyType(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return "none", true
	case "manual":
		return "manual", true
	default:
		return "", false
	}
}

func extraerAlgoritmoHash(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SHA-1":
		return "SHA-1", true
	case "SHA-256":
		return "SHA-256", true
	case "SHA-384":
		return "SHA-384", true
	case "SHA-512":
		return "SHA-512", true
	default:
		return "", false
	}
}

func extraerFormatoHashFichero(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hex":
		return "hex", true
	case "base64":
		return "base64", true
	case "bin":
		return "bin", true
	default:
		return "", false
	}
}

func extraerFormatoHashDirectorio(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "xml":
		return "xml", true
	case "txt":
		return "txt", true
	case "csv":
		return "csv", true
	default:
		return "", false
	}
}

func extraerPerfilFirma(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "baseline", "b":
		return "baseline", true
	case "t":
		return "t", true
	case "lt":
		return "lt", true
	case "lta":
		return "lta", true
	default:
		return "", false
	}
}

func extraerFormatoXAdES(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "detached", "xades detached", "xades-detached":
		return "detached", true
	case "enveloped", "xades enveloped", "xades-enveloped":
		return "enveloped", true
	case "enveloping", "xades enveloping", "xades-enveloping":
		return "enveloping", true
	default:
		return "", false
	}
}

func extraerSubfiltroPAdES(datos map[string]any, clave string) (string, bool) {
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "etsi", "etsi.cades.detached":
		return "etsi", true
	case "adobe", "adbe.pkcs7.detached":
		return "adobe", true
	default:
		return "", false
	}
}

func extraerNivelCertificacionPDF(datos map[string]any, clave string) (int, bool) {
	if v, ok := extraerInt(datos, clave); ok {
		switch v {
		case 0, 1, 2, 3:
			return v, true
		default:
			return 0, false
		}
	}
	s, ok := extraerStringNoVacia(datos, clave)
	if !ok {
		return 0, false
	}
	switch strings.TrimSpace(s) {
	case "0":
		return 0, true
	case "1":
		return 1, true
	case "2":
		return 2, true
	case "3":
		return 3, true
	default:
		return 0, false
	}
}

func extraerBool(datos map[string]any, clave string) (bool, bool) {
	v, ok := datos[clave]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func extraerInt(datos map[string]any, clave string) (int, bool) {
	v, ok := datos[clave]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		i := int(n)
		if float64(i) != n {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func extraerFloat01(datos map[string]any, clave string) (float64, bool) {
	v, ok := datos[clave]
	if !ok {
		return 0, false
	}
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		n = float64(x)
	case int32:
		n = float64(x)
	case int64:
		n = float64(x)
	default:
		return 0, false
	}
	if n < 0 || n > 1 {
		return 0, false
	}
	return n, true
}

func extraerRotacionSello(datos map[string]any, clave string) (int, bool) {
	v, ok := extraerInt(datos, clave)
	if !ok || v < 0 || v > 359 {
		return 0, false
	}
	return v, true
}

func extraerListaStringsNormalizada(datos map[string]any, clave string) ([]string, bool) {
	v, ok := datos[clave]
	if !ok {
		return nil, false
	}

	var raw []string
	switch vv := v.(type) {
	case []string:
		raw = append([]string(nil), vv...)
	case []any:
		raw = make([]string, 0, len(vv))
		for _, item := range vv {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			raw = append(raw, s)
		}
	default:
		return nil, false
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func extraerListaTiposCertificado(datos map[string]any, clave string) ([]string, bool) {
	valores, ok := extraerListaStringsNormalizada(datos, clave)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(valores))
	for _, item := range valores {
		s := strings.ToLower(strings.TrimSpace(item))
		switch s {
		case "fisica", "representacion", "sello", "empleado_publico", "desconocido":
			out = append(out, s)
		default:
			return nil, false
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
