// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/inbound/common/secretinput"
	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/adapters/outbound/common/components"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/eni"
	"grxfirma/internal/adapters/outbound/common/informeverificacion"
	"grxfirma/internal/adapters/outbound/common/limits"
	pkcs12importer "grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/securefile"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/updatecheck"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/adapters/outbound/desktop/truststore"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// SignatureFormatsHelp mantiene en un único punto la lista canónica que
// publican la ayuda CLI detallada y el binario principal.
const SignatureFormatsHelp = "auto|pades|cades|xades|xmldsig|odf|ooxml|facturae|verifactu|asic-xades"

const (
	envPKCS12PasswordCLI = "GRXFIRMA_PKCS12_PASSWORD"
	envProtectionSecret  = "GRXFIRMA_PROTECTION_SECRET_B64"
	// envPDFPassword es la contraseña de un PDF cifrado; se consume y se
	// borra del entorno al arrancar, como las demás claves.
	envPDFPassword = "GRXFIRMA_PDF_PASSWORD"
)

// SignDocumentUseCase define el contrato mínimo esperado por el adaptador CLI.
type SignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error)
}

// ProcessBatchUseCase define el contrato mínimo esperado para procesado de lotes.
type ProcessBatchUseCase interface {
	Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error)
}

// VerifySignatureUseCase define el contrato mínimo esperado para verificación.
type VerifySignatureUseCase interface {
	Execute(ctx context.Context, cmd application.VerifyCommand) (application.VerifyResult, error)
}

// CreateHashUseCase define el contrato mínimo esperado para crear huellas.
type CreateHashUseCase interface {
	Execute(ctx context.Context, cmd application.CreateHashCommand) (application.CreateHashResult, error)
}

// CheckHashUseCase define el contrato mínimo esperado para comprobar huellas.
type CheckHashUseCase interface {
	Execute(ctx context.Context, cmd application.CheckHashCommand) (application.CheckHashResult, error)
}

// CreateDirectoryHashUseCase define el contrato mínimo esperado para crear manifiestos de directorio.
type CreateDirectoryHashUseCase interface {
	Execute(ctx context.Context, cmd application.CreateDirectoryHashManifestCommand) (application.CreateDirectoryHashManifestResult, error)
}

// CheckDirectoryHashUseCase define el contrato mínimo esperado para comprobar manifiestos de directorio.
type CheckDirectoryHashUseCase interface {
	Execute(ctx context.Context, cmd application.CheckDirectoryHashManifestCommand) (application.CheckDirectoryHashManifestResult, error)
}

// ProtectDocumentUseCase define el contrato mínimo esperado para protección/cifrado.
type ProtectDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.ProtectCommand) (application.ProtectResult, error)
}

// ProtectAndSignDocumentUseCase define el contrato mínimo esperado para
// protección firmada CMS.
type ProtectAndSignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.ProtectAndSignCommand) (application.ProtectAndSignResult, error)
}

// UnprotectDocumentUseCase define el contrato mínimo esperado para desprotección/descifrado.
type UnprotectDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.UnprotectCommand) (application.UnprotectResult, error)
}

// ExportProtectionRecipientUseCase define el contrato mínimo esperado para exportar destinatarios públicos.
type ExportProtectionRecipientUseCase interface {
	Execute(ctx context.Context, cmd application.ExportProtectionRecipientCommand) (application.ExportProtectionRecipientResult, error)
}

// ImportProtectionRecipientUseCase define el contrato mínimo esperado para importar destinatarios públicos.
type ImportProtectionRecipientUseCase interface {
	Execute(ctx context.Context, cmd application.ImportProtectionRecipientCommand) (application.ImportProtectionRecipientResult, error)
}

// ManageTrustedDomainUseCase define el contrato mínimo esperado para la confianza.
type ManageTrustedDomainUseCase interface {
	Execute(ctx context.Context, cmd application.ManageTrustedDomainCommand) (application.ManageTrustedDomainResult, error)
}

// ProtectionRecipients expone destinatarios de protección listables y resolubles.
type ProtectionRecipients interface {
	ports.ProtectionRecipientCatalog
	List(ctx context.Context) ([]domain.ProtectionRecipient, error)
}

// Adaptador implementa la entrada CLI hacia los casos de uso de la aplicación.
type Adaptador struct {
	Firmar             SignDocumentUseCase
	ProcesarLote       ProcessBatchUseCase
	Verificar          VerifySignatureUseCase
	CrearHash          CreateHashUseCase
	ComprobarHash      CheckHashUseCase
	CrearHashDir       CreateDirectoryHashUseCase
	ComprobarHashDir   CheckDirectoryHashUseCase
	InformeHashDir     ports.DirectoryHashReportCodec
	Proteger           ProtectDocumentUseCase
	ProtegerFirmando   ProtectAndSignDocumentUseCase
	Desproteger        UnprotectDocumentUseCase
	ExportarProteccion ExportProtectionRecipientUseCase
	ImportarProteccion ImportProtectionRecipientUseCase
	Destinatarios      ProtectionRecipients
	GestionDominios    ManageTrustedDomainUseCase
	Catalogo           ports.CertificateCatalog
	Claves             ports.SigningKeyProvider
	Localizador        ports.Localizador
	VisorPDF           ports.VisualizadorPDF
	ConfigDir          string
	Version            string
	Stdout             io.Writer
	Stderr             io.Writer
	Stdin              io.Reader
	LeerFichero        func(string) ([]byte, error)
	Escribir           func(string, []byte, os.FileMode) error
	Limits             limits.Limits
	// escrituraConPolitica indica que Escribir es la escritura real por
	// defecto; entonces los resultados se guardan con guardarSalidaCLI. Un
	// Escribir inyectado (pruebas con sistema de ficheros simulado) se usa tal
	// cual.
	escrituraConPolitica bool
	instalarConfianzaTLS func(context.Context, string) error
	passwordP12Compat    string
	protectionCompat     string
	pdfPasswordCompat    string
}

type configCLI struct {
	modoCLI               bool
	mostrarAyuda          bool
	operacion             string
	entrada               string
	salida                string
	informe               string
	original              string
	formato               string
	accion                string
	certificado           string
	certIndex             int
	certContains          string
	listarCerts           bool
	comprobarCerts        bool
	json                  bool
	noGuardar             bool
	imprimirFirma         bool
	permitirPDFInval      bool
	compatEstrica         bool
	sobrescribir          string
	selloVisible          bool
	selloPagina           uint
	selloPaginas          string
	selloX                float64
	selloY                float64
	selloW                float64
	selloH                float64
	qrSello               string
	motivoFirma           string
	ubicacionFirma        string
	contactoFirma         string
	idiomaSello           string
	disposicionSello      string
	margenSelloFooter     float64
	dominio               string
	ficheroDominios       string
	ficheroAutoseleccion  string
	ficheroP12            string
	contrasenaP12         string
	contrasenaP12Stdin    bool
	perfilProteccion      string
	contenedorProteccion  string
	secretProteccionB64   string
	secretProteccionStdin bool
	contrasenaPDFStdin    bool
	ficheroHash           string
	algoritmoHash         string
	formatoHash           string
	hashRecursivo         bool
	destinatarios         []string
	lote                  string
	timeout               time.Duration
	opciones              map[string]string
}

type opcionesCLI map[string]string

type listaValoresCLI []string

type certificadoCLI struct {
	Indice      int
	Ref         domain.CertificateRef
	PuedeFirmar bool
	Problema    string
}

func (o *opcionesCLI) String() string {
	if o == nil || len(*o) == 0 {
		return ""
	}
	partes := make([]string, 0, len(*o))
	for k, v := range *o {
		partes = append(partes, k+"="+v)
	}
	slices.Sort(partes)
	return strings.Join(partes, ",")
}

func (o *opcionesCLI) Set(value string) error {
	if o == nil {
		return fmt.Errorf("contenedor de opciones no inicializado")
	}
	if err := secretinput.ValidateOptionAssignment(value); err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("la opcion no puede estar vacia")
	}
	partes := strings.SplitN(value, "=", 2)
	if len(partes) != 2 {
		return fmt.Errorf("la opcion debe tener formato clave=valor")
	}
	clave := strings.TrimSpace(partes[0])
	valor := strings.TrimSpace(partes[1])
	if clave == "" {
		return fmt.Errorf("la clave de la opcion no puede estar vacia")
	}
	if *o == nil {
		*o = make(map[string]string)
	}
	(*o)[clave] = valor
	return nil
}

func (l *listaValoresCLI) String() string {
	if l == nil || len(*l) == 0 {
		return ""
	}
	return strings.Join(*l, ",")
}

func (l *listaValoresCLI) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("el valor no puede estar vacio")
	}
	*l = append(*l, trimmed)
	return nil
}

type elementoLoteCLI struct {
	Ruta     string            `json:"ruta"`
	Nombre   string            `json:"nombre"`
	TipoMIME string            `json:"tipo_mime"`
	Formato  string            `json:"formato"`
	Accion   string            `json:"accion"`
	Opciones map[string]string `json:"opciones"`
}

// New crea un adaptador CLI con dependencias por defecto.
func New(firmar SignDocumentUseCase, procesarLote ProcessBatchUseCase) *Adaptador {
	return &Adaptador{
		Firmar:               firmar,
		ProcesarLote:         procesarLote,
		Stdout:               os.Stdout,
		Stderr:               os.Stderr,
		Stdin:                os.Stdin,
		LeerFichero:          os.ReadFile,
		Escribir:             os.WriteFile,
		escrituraConPolitica: true,
		Limits:               limits.FromEnv(limits.Default()),
		instalarConfianzaTLS: localtlstrust.EnsureManagedTrusted,
	}
}

func (a *Adaptador) WithCatalogo(catalogo ports.CertificateCatalog) *Adaptador {
	a.Catalogo = catalogo
	return a
}

func (a *Adaptador) WithClaves(claves ports.SigningKeyProvider) *Adaptador {
	a.Claves = claves
	return a
}

// WithLocalizador inyecta el catálogo usado por los mensajes visibles de la
// CLI. Si no se configura, el adaptador conserva los textos en castellano.
func (a *Adaptador) WithLocalizador(localizador ports.Localizador) *Adaptador {
	a.Localizador = localizador
	return a
}

// WithVisorPDF inyecta el rasterizador de páginas PDF que usa la lectura del
// QR tributario desde un PDF. Sin él, solo se admiten URL e imágenes.
func (a *Adaptador) WithVisorPDF(visor ports.VisualizadorPDF) *Adaptador {
	a.VisorPDF = visor
	return a
}

func (a *Adaptador) WithVerificador(verificador VerifySignatureUseCase) *Adaptador {
	a.Verificar = verificador
	return a
}

func (a *Adaptador) WithHashes(crear CreateHashUseCase, comprobar CheckHashUseCase) *Adaptador {
	a.CrearHash = crear
	a.ComprobarHash = comprobar
	return a
}

func (a *Adaptador) WithDirectoryHashes(crear CreateDirectoryHashUseCase, comprobar CheckDirectoryHashUseCase) *Adaptador {
	a.CrearHashDir = crear
	a.ComprobarHashDir = comprobar
	return a
}

func (a *Adaptador) WithDirectoryHashReports(codec ports.DirectoryHashReportCodec) *Adaptador {
	a.InformeHashDir = codec
	return a
}

func (a *Adaptador) WithProteccion(proteger ProtectDocumentUseCase, desproteger UnprotectDocumentUseCase, destinatarios ProtectionRecipients) *Adaptador {
	a.Proteger = proteger
	a.Desproteger = desproteger
	a.Destinatarios = destinatarios
	return a
}

func (a *Adaptador) WithProteccionFirmada(protegerFirmando ProtectAndSignDocumentUseCase) *Adaptador {
	a.ProtegerFirmando = protegerFirmando
	return a
}

func (a *Adaptador) WithIntercambioProteccion(exportar ExportProtectionRecipientUseCase, importar ImportProtectionRecipientUseCase) *Adaptador {
	a.ExportarProteccion = exportar
	a.ImportarProteccion = importar
	return a
}

func (a *Adaptador) WithGestionDominios(gestion ManageTrustedDomainUseCase) *Adaptador {
	a.GestionDominios = gestion
	return a
}

func (a *Adaptador) WithConfigDir(configDir string) *Adaptador {
	a.ConfigDir = configDir
	return a
}

// WithCompatibilitySecrets transfiere al adaptador secretos ya retirados del
// entorno por el proceso principal. Nunca los reinserta en entorno ni argv.
func (a *Adaptador) WithCompatibilitySecrets(passwordP12, protection string) *Adaptador {
	a.passwordP12Compat = passwordP12
	a.protectionCompat = protection
	return a
}

// WithVersion fija la versión en ejecución, usada por la comprobación
// opt-in de actualizaciones del diagnóstico.
func (a *Adaptador) WithVersion(version string) *Adaptador {
	a.Version = version
	return a
}

// Run analiza argumentos, construye comandos internos y ejecuta la operación correspondiente.
func (a *Adaptador) Run(ctx context.Context, args []string) int {
	if a.Stdout == nil {
		a.Stdout = io.Discard
	}
	if a.Stderr == nil {
		a.Stderr = io.Discard
	}
	if a.Stdin == nil {
		a.Stdin = os.Stdin
	}
	if a.LeerFichero == nil {
		a.LeerFichero = os.ReadFile
	}
	if a.Escribir == nil {
		a.Escribir = os.WriteFile
		a.escrituraConPolitica = true
	}
	if a.Limits.MaxPayloadBytes == 0 {
		a.Limits = limits.FromEnv(limits.Default())
	}
	environment, err := secretinput.ConsumeEnvironment(
		[]string{envPKCS12PasswordCLI, envProtectionSecret, envPDFPassword, "GRXFIRMA_REST_TOKEN"},
		os.LookupEnv,
		os.Unsetenv,
	)
	if err != nil {
		fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), secretinput.UserMessage(err, a.translateSecret))
		return 1
	}
	if value, present := environment.Take(envPKCS12PasswordCLI); present {
		a.passwordP12Compat = value
	}
	if value, present := environment.Take(envProtectionSecret); present {
		a.protectionCompat = value
	}
	if value, present := environment.Take(envPDFPassword); present {
		a.pdfPasswordCompat = value
	}
	_, _ = environment.Take("GRXFIRMA_REST_TOKEN")

	cfg, err := parsearArgs(args)
	if err != nil {
		var inputErr *secretinput.Error
		if errors.As(err, &inputErr) {
			fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), secretinput.UserMessage(err, a.translateSecret))
		} else {
			fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), traducirErrorCLI(err))
		}
		fmt.Fprintln(a.Stderr)
		a.escribirAyuda()
		return 1
	}
	if cfg.mostrarAyuda || (cfg.modoCLI && cfg.operacion == "" && !cfg.listarCerts && !cfg.comprobarCerts && cfg.entrada == "" && cfg.lote == "") {
		a.escribirAyuda()
		return 0
	}

	operacion := normalizarOperacion(cfg.operacion, cfg.accion, cfg.lote, cfg.entrada)
	if err := a.resolverSecretosEntrada(&cfg, operacion); err != nil {
		var inputErr *secretinput.Error
		if errors.As(err, &inputErr) {
			fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), secretinput.UserMessage(err, a.translateSecret))
		} else {
			fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), err)
		}
		return 1
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	if cfg.listarCerts || cfg.comprobarCerts {
		if rc := a.ejecutarListadoCertificados(ctx, cfg); rc != 0 {
			return rc
		}
		if strings.TrimSpace(cfg.operacion) == "" {
			return 0
		}
	}

	switch operacion {
	case "":
		fmt.Fprintln(
			a.Stderr,
			a.t("cli.error.prefix", "error:"),
			a.t("cli.error.no_operation", "no se ha indicado ninguna operación CLI"),
		)
		return 1
	case "sign", "cosign", "countersign":
		if cfg.lote != "" {
			cfg.accion = normalizarOperacion(cfg.operacion, cfg.accion, cfg.lote, cfg.entrada)
			return a.ejecutarLote(ctx, cfg)
		}
		cfg.accion = normalizarOperacion(cfg.operacion, cfg.accion, cfg.lote, cfg.entrada)
		return a.ejecutarFirma(ctx, cfg)
	case "verify":
		return a.ejecutarVerificacion(ctx, cfg)
	case "hash-create":
		return a.ejecutarCrearHash(ctx, cfg)
	case "hash-check":
		return a.ejecutarComprobarHash(ctx, cfg)
	case "validar-verifactu", "leer-qr-verifactu", "cotejar-qr-verifactu":
		cfg.operacion = operacion
		return a.ejecutarVeriFactu(ctx, cfg)
	case "facturae-check":
		return a.ejecutarValidarFactura(cfg)
	case "eni-check":
		return a.ejecutarValidarENI(cfg)
	case "eni-create":
		return a.ejecutarGenerarENI(cfg)
	case "eni-file-create":
		return a.ejecutarGenerarExpediente(ctx, cfg)
	case "audit-verify":
		return a.ejecutarVerificarAuditoria(cfg)
	case "protect":
		return a.ejecutarProteccion(ctx, cfg)
	case "protect-sign":
		return a.ejecutarProteccionFirmada(ctx, cfg)
	case "unprotect":
		return a.ejecutarDesproteccion(ctx, cfg)
	case "protect-recipients-list":
		return a.ejecutarDestinatariosProteccionListar(ctx, cfg)
	case "protect-recipient-export":
		return a.ejecutarDestinatarioProteccionExportar(ctx, cfg)
	case "protect-recipient-import":
		return a.ejecutarDestinatarioProteccionImportar(ctx, cfg)
	case "diagnostics-report":
		return a.ejecutarDiagnostico(ctx)
	case "domains-list":
		return a.ejecutarDominiosListar(ctx, cfg)
	case "domains-add":
		return a.ejecutarDominiosGestion(ctx, cfg, application.TrustActionAllow)
	case "domains-remove":
		return a.ejecutarDominiosGestion(ctx, cfg, application.TrustActionRemove)
	case "domains-import":
		return a.ejecutarDominiosImportar(ctx, cfg)
	case "domains-export":
		return a.ejecutarDominiosExportar(ctx, cfg)
	case "domains-clear":
		return a.ejecutarDominiosLimpiar(ctx, cfg)
	case "import-p12":
		return a.ejecutarImportarP12(ctx, cfg)
	case "cert-selection-list":
		return a.ejecutarAutoseleccionListar(ctx, cfg)
	case "cert-selection-remove":
		return a.ejecutarAutoseleccionEliminar(ctx, cfg)
	case "cert-selection-reset":
		return a.ejecutarAutoseleccionResetear(ctx, cfg)
	case "cert-selection-export":
		return a.ejecutarAutoseleccionExportar(ctx, cfg)
	case "cert-selection-import":
		return a.ejecutarAutoseleccionImportar(ctx, cfg)
	case "cert-selection-clear":
		return a.ejecutarAutoseleccionLimpiar(ctx, cfg)
	case "tls-store-status":
		return a.ejecutarTLSAlmacenEstado(ctx, cfg)
	case "tls-store-clear":
		return a.ejecutarTLSAlmacenLimpiar(ctx, cfg)
	case "tls-trust-status":
		return a.ejecutarTLSEstadoConfianza(ctx, cfg)
	case "tls-generate-certs":
		return a.ejecutarTLSGenerarCerts(ctx, cfg)
	case "tls-install-trust":
		return a.ejecutarTLSInstalarConfianza(ctx, cfg)
	default:
		fmt.Fprintln(
			a.Stderr,
			a.t("cli.error.prefix", "error:"),
			a.t("cli.error.unsupported_operation", "operación no soportada en CLI: %s", cfg.operacion),
		)
		return 1
	}
}

func (a *Adaptador) resolverSecretosEntrada(cfg *configCLI, operacion string) error {
	switch operacion {
	case "import-p12":
		password, err := a.leerSecretoEntrada(
			cfg.contrasenaP12Stdin,
			a.t("security.secret.prompt.p12", "Contraseña PKCS#12: "),
			a.passwordP12Compat,
		)
		if err != nil {
			return fmt.Errorf(
				"%s: %w",
				a.t("cli.error.p12_secret_input", "no se pudo leer la contraseña PKCS#12"),
				err,
			)
		}
		a.passwordP12Compat = ""
		cfg.contrasenaP12 = password
	case "protect", "unprotect":
		secret, err := a.leerSecretoEntrada(
			cfg.secretProteccionStdin,
			a.t("security.secret.prompt.protection", "Clave de protección Base64: "),
			a.protectionCompat,
		)
		if err != nil {
			return fmt.Errorf(
				"%s: %w",
				a.t("cli.error.protection_secret_input", "no se pudo leer la clave de protección"),
				err,
			)
		}
		a.protectionCompat = ""
		cfg.secretProteccionB64 = secret
	case "sign", "cosign", "countersign":
		if !cfg.contrasenaPDFStdin && a.pdfPasswordCompat == "" {
			return nil
		}
		password, err := a.leerSecretoEntrada(
			cfg.contrasenaPDFStdin,
			a.t("security.secret.prompt.pdf", "Contraseña del PDF: "),
			a.pdfPasswordCompat,
		)
		if err != nil {
			return fmt.Errorf("%s: %w", a.t("cli.error.pdf_secret_input", "no se pudo leer la contraseña del PDF"), err)
		}
		a.pdfPasswordCompat = ""
		if password != "" {
			if cfg.opciones == nil {
				cfg.opciones = map[string]string{}
			}
			// Nunca llega por argv: se lee de la entrada segura.
			cfg.opciones["userPassword"] = password
		}
	}
	return nil
}

func (a *Adaptador) leerSecretoEntrada(fromStdin bool, prompt, compatible string) (string, error) {
	if fromStdin {
		return secretinput.Read(a.Stdin, a.Stderr, prompt)
	}
	return compatible, nil
}

func parsearArgs(args []string) (configCLI, error) {
	if err := secretinput.RejectArgv(args); err != nil {
		return configCLI{}, err
	}

	fs := flag.NewFlagSet("grxfirma", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	cfg := configCLI{
		certIndex:         -1,
		sobrescribir:      "rename",
		perfilProteccion:  "compat",
		disposicionSello:  "manual",
		selloPagina:       1,
		selloX:            0.62,
		selloY:            0.04,
		selloW:            0.34,
		selloH:            0.12,
		margenSelloFooter: 0.02,
	}
	var timeoutRaw string
	var opFirmar, opCofirmar, opContrafirmar, opVerificar bool
	var opCrearHash, opComprobarHash bool
	var opProteger, opProtegerFirmando, opDesproteger, opListarDestinatariosProteccion bool
	var opExportarDestinatarioProteccion, opImportarDestinatarioProteccion bool
	var opDiagnostico, opListarDominios, opAnadirDominio, opEliminarDominio, opImportarDominios, opExportarDominios, opLimpiarDominios bool
	var opListarAutoseleccion bool
	var opEliminarAutoseleccion bool
	var opResetearAutoseleccion bool
	var opExportarAutoseleccion bool
	var opImportarAutoseleccion bool
	var opEstadoAlmacenTLS, opLimpiarAlmacenTLS, opEstadoConfianzaTLS, opGenerarCertificadosTLS, opInstalarConfianzaTLS bool
	var opLimpiarAutoseleccion bool
	opts := opcionesCLI{}
	destinatarios := listaValoresCLI{}

	fs.BoolVar(&cfg.modoCLI, "cli", false, "")
	fs.BoolVar(&cfg.modoCLI, "modo-cli", false, "")
	fs.BoolVar(&cfg.mostrarAyuda, "a", false, "")
	fs.BoolVar(&cfg.mostrarAyuda, "h", false, "")
	fs.BoolVar(&cfg.mostrarAyuda, "help", false, "")
	fs.BoolVar(&cfg.mostrarAyuda, "ayuda", false, "")
	fs.BoolVar(&cfg.mostrarAyuda, "cli-help", false, "")
	fs.BoolVar(&cfg.mostrarAyuda, "ayuda-cli", false, "")
	fs.StringVar(&cfg.operacion, "op", "", "")
	fs.StringVar(&cfg.operacion, "operacion", "", "")
	fs.BoolVar(&opFirmar, "firmar", false, "")
	fs.BoolVar(&opCofirmar, "cofirmar", false, "")
	fs.BoolVar(&opContrafirmar, "contrafirmar", false, "")
	fs.BoolVar(&opVerificar, "verificar", false, "")
	fs.BoolVar(&opCrearHash, "crear-hash", false, "")
	fs.BoolVar(&opCrearHash, "createdigest", false, "")
	fs.BoolVar(&opComprobarHash, "comprobar-hash", false, "")
	fs.BoolVar(&opComprobarHash, "checkdigest", false, "")
	fs.BoolVar(&opProteger, "proteger", false, "")
	fs.BoolVar(&opProtegerFirmando, "proteger-firmando", false, "")
	fs.BoolVar(&opDesproteger, "desproteger", false, "")
	fs.BoolVar(&opListarDestinatariosProteccion, "listar-destinatarios-proteccion", false, "")
	fs.BoolVar(&opExportarDestinatarioProteccion, "exportar-destinatario-proteccion", false, "")
	fs.BoolVar(&opImportarDestinatarioProteccion, "importar-destinatario-proteccion", false, "")
	fs.BoolVar(&opDiagnostico, "informe-diagnostico", false, "")
	fs.BoolVar(&opListarDominios, "listar-dominios", false, "")
	fs.BoolVar(&opAnadirDominio, "anadir-dominio", false, "")
	fs.BoolVar(&opAnadirDominio, "añadir-dominio", false, "")
	fs.BoolVar(&opEliminarDominio, "eliminar-dominio", false, "")
	fs.BoolVar(&opImportarDominios, "importar-dominios", false, "")
	fs.StringVar(&cfg.ficheroP12, "importar-p12", "", "")
	fs.StringVar(&cfg.ficheroP12, "fichero-p12", "", "")
	fs.BoolVar(&cfg.contrasenaP12Stdin, "contrasena-p12-stdin", false, "")
	fs.BoolVar(&cfg.contrasenaP12Stdin, "password-p12-stdin", false, "")
	fs.StringVar(&cfg.perfilProteccion, "protection-profile", "compat", "")
	fs.StringVar(&cfg.perfilProteccion, "perfil-proteccion", "compat", "")
	fs.StringVar(&cfg.contenedorProteccion, "protection-container", "", "")
	fs.StringVar(&cfg.contenedorProteccion, "contenedor-proteccion", "", "")
	fs.BoolVar(&cfg.secretProteccionStdin, "protection-secret-stdin", false, "")
	fs.BoolVar(&cfg.secretProteccionStdin, "clave-proteccion-stdin", false, "")
	fs.BoolVar(&cfg.contrasenaPDFStdin, "contrasena-pdf-stdin", false, "")
	fs.BoolVar(&cfg.contrasenaPDFStdin, "pdf-password-stdin", false, "")
	fs.BoolVar(&opExportarDominios, "exportar-dominios", false, "")
	fs.BoolVar(&opLimpiarDominios, "limpiar-dominios", false, "")
	fs.BoolVar(&opLimpiarDominios, "borrar-dominios", false, "")
	fs.BoolVar(&opListarAutoseleccion, "listar-autoseleccion-cert", false, "")
	fs.BoolVar(&opListarAutoseleccion, "ver-autoseleccion-cert", false, "")
	fs.BoolVar(&opEliminarAutoseleccion, "eliminar-autoseleccion-cert", false, "")
	fs.BoolVar(&opEliminarAutoseleccion, "borrar-autoseleccion-portal", false, "")
	fs.BoolVar(&opResetearAutoseleccion, "resetear-autoseleccion-cert", false, "")
	fs.BoolVar(&opResetearAutoseleccion, "restablecer-autoseleccion-cert", false, "")
	fs.BoolVar(&opExportarAutoseleccion, "exportar-autoseleccion-cert", false, "")
	fs.BoolVar(&opImportarAutoseleccion, "importar-autoseleccion-cert", false, "")
	fs.BoolVar(&opEstadoAlmacenTLS, "estado-almacen-tls", false, "")
	fs.BoolVar(&opLimpiarAlmacenTLS, "limpiar-almacen-tls", false, "")
	fs.BoolVar(&opEstadoConfianzaTLS, "estado-confianza-tls", false, "")
	fs.BoolVar(&opGenerarCertificadosTLS, "generar-certificados-tls", false, "")
	fs.BoolVar(&opInstalarConfianzaTLS, "instalar-confianza-tls", false, "")
	fs.BoolVar(&opLimpiarAutoseleccion, "limpiar-autoseleccion-cert", false, "")
	fs.BoolVar(&opLimpiarAutoseleccion, "borrar-autoseleccion-cert", false, "")
	fs.BoolVar(&opLimpiarAutoseleccion, "limpiar-seleccion-certificado", false, "")
	fs.StringVar(&cfg.ficheroAutoseleccion, "fichero-autoseleccion", "", "")
	fs.StringVar(&cfg.entrada, "in", "", "")
	fs.StringVar(&cfg.entrada, "e", "", "")
	fs.StringVar(&cfg.entrada, "entrada", "", "")
	fs.StringVar(&cfg.salida, "out", "", "")
	fs.StringVar(&cfg.salida, "s", "", "")
	fs.StringVar(&cfg.salida, "salida", "", "")
	fs.StringVar(&cfg.informe, "informe", "", "")
	fs.StringVar(&cfg.informe, "report", "", "")
	fs.StringVar(&cfg.original, "original", "", "")
	fs.StringVar(&cfg.original, "documento-original", "", "")
	fs.StringVar(&cfg.ficheroHash, "hash-file", "", "")
	fs.StringVar(&cfg.ficheroHash, "fichero-hash", "", "")
	fs.StringVar(&cfg.formato, "format", "", "")
	fs.StringVar(&cfg.formato, "f", "", "")
	fs.StringVar(&cfg.formato, "formato", "", "")
	fs.StringVar(&cfg.algoritmoHash, "hash-algorithm", "", "")
	fs.StringVar(&cfg.algoritmoHash, "algoritmo-hash", "", "")
	fs.StringVar(&cfg.formatoHash, "hash-format", "", "")
	fs.StringVar(&cfg.formatoHash, "formato-hash", "", "")
	fs.BoolVar(&cfg.hashRecursivo, "recursive", false, "")
	fs.BoolVar(&cfg.hashRecursivo, "recursivo", false, "")
	fs.BoolVar(&cfg.hashRecursivo, "r", false, "")
	fs.StringVar(&cfg.certificado, "cert-id", "", "")
	fs.StringVar(&cfg.certificado, "id", "", "")
	fs.StringVar(&cfg.certificado, "id-certificado", "", "")
	fs.StringVar(&cfg.certificado, "certificado", "", "")
	fs.IntVar(&cfg.certIndex, "cert-index", -1, "")
	fs.IntVar(&cfg.certIndex, "idx", -1, "")
	fs.IntVar(&cfg.certIndex, "indice-certificado", -1, "")
	fs.StringVar(&cfg.certContains, "cert-contains", "", "")
	fs.StringVar(&cfg.certContains, "certificado-contiene", "", "")
	fs.BoolVar(&cfg.listarCerts, "list-certs", false, "")
	fs.BoolVar(&cfg.listarCerts, "ll", false, "")
	fs.BoolVar(&cfg.listarCerts, "listar-certificados", false, "")
	fs.BoolVar(&cfg.comprobarCerts, "check-certs", false, "")
	fs.BoolVar(&cfg.comprobarCerts, "cc", false, "")
	fs.BoolVar(&cfg.comprobarCerts, "comprobar-certificados", false, "")
	fs.BoolVar(&cfg.json, "json", false, "")
	fs.BoolVar(&cfg.json, "j", false, "")
	fs.BoolVar(&cfg.json, "salida-json", false, "")
	fs.BoolVar(&cfg.noGuardar, "no-save", false, "")
	fs.BoolVar(&cfg.noGuardar, "no-guardar", false, "")
	fs.BoolVar(&cfg.imprimirFirma, "print-signature", false, "")
	fs.BoolVar(&cfg.imprimirFirma, "imprimir-firma", false, "")
	fs.BoolVar(&cfg.permitirPDFInval, "allow-invalid-pdf", false, "")
	fs.BoolVar(&cfg.permitirPDFInval, "permitir-pdf-invalido", false, "")
	fs.BoolVar(&cfg.compatEstrica, "strict-compat", false, "")
	fs.BoolVar(&cfg.compatEstrica, "compatibilidad-estricta", false, "")
	fs.StringVar(&cfg.sobrescribir, "overwrite", "rename", "")
	fs.StringVar(&cfg.sobrescribir, "sobrescribir", "rename", "")
	fs.BoolVar(&cfg.selloVisible, "visible-seal", false, "")
	fs.BoolVar(&cfg.selloVisible, "sello-visible", false, "")
	fs.UintVar(&cfg.selloPagina, "seal-page", 1, "")
	fs.UintVar(&cfg.selloPagina, "sello-pagina", 1, "")
	fs.StringVar(&cfg.selloPaginas, "seal-pages", "", "")
	fs.StringVar(&cfg.selloPaginas, "sello-paginas", "", "")
	fs.Float64Var(&cfg.selloX, "seal-x", 0.62, "")
	fs.Float64Var(&cfg.selloX, "sello-x", 0.62, "")
	fs.Float64Var(&cfg.selloY, "seal-y", 0.04, "")
	fs.Float64Var(&cfg.selloY, "sello-y", 0.04, "")
	fs.Float64Var(&cfg.selloW, "seal-w", 0.34, "")
	fs.Float64Var(&cfg.selloW, "sello-ancho", 0.34, "")
	fs.Float64Var(&cfg.selloH, "seal-h", 0.12, "")
	fs.Float64Var(&cfg.selloH, "sello-alto", 0.12, "")
	fs.StringVar(&cfg.qrSello, "seal-qr", "", "")
	fs.StringVar(&cfg.qrSello, "sello-qr", "", "")
	fs.StringVar(&cfg.motivoFirma, "signature-reason", "", "")
	fs.StringVar(&cfg.motivoFirma, "motivo-firma", "", "")
	fs.StringVar(&cfg.ubicacionFirma, "signature-location", "", "")
	fs.StringVar(&cfg.ubicacionFirma, "ubicacion-firma", "", "")
	fs.StringVar(&cfg.contactoFirma, "signature-contact", "", "")
	fs.StringVar(&cfg.contactoFirma, "contacto-firma", "", "")
	fs.StringVar(&cfg.idiomaSello, "seal-language", "", "")
	fs.StringVar(&cfg.idiomaSello, "idioma-sello", "", "")
	fs.StringVar(&cfg.disposicionSello, "seal-layout", "manual", "")
	fs.StringVar(&cfg.disposicionSello, "disposicion-sello", "manual", "")
	fs.Float64Var(&cfg.margenSelloFooter, "seal-footer-margin", 0.02, "")
	fs.Float64Var(&cfg.margenSelloFooter, "margen-inferior-sello", 0.02, "")
	fs.StringVar(&cfg.dominio, "domain", "", "")
	fs.StringVar(&cfg.dominio, "dominio", "", "")
	fs.StringVar(&cfg.ficheroDominios, "domain-file", "", "")
	fs.StringVar(&cfg.ficheroDominios, "fichero-dominios", "", "")
	fs.StringVar(&cfg.accion, "accion", "", "")
	fs.StringVar(&cfg.lote, "lote", "", "")
	fs.StringVar(&timeoutRaw, "timeout", "30s", "")
	fs.StringVar(&timeoutRaw, "t", "30s", "")
	fs.StringVar(&timeoutRaw, "tiempo-espera", "30s", "")
	fs.Var(&opts, "opcion", "")
	fs.Var(&destinatarios, "recipient", "")
	fs.Var(&destinatarios, "destinatario", "")

	if err := fs.Parse(args); err != nil {
		return configCLI{}, err
	}
	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil {
		return configCLI{}, fmt.Errorf("tiempo de espera invalido: %w", err)
	}
	cfg.timeout = timeout
	cfg.opciones = opts
	cfg.destinatarios = append(cfg.destinatarios, destinatarios...)
	if normalized, err := validarSeleccionPaginasSello(cfg.selloPaginas); err != nil {
		return configCLI{}, err
	} else {
		cfg.selloPaginas = normalized
	}
	if strings.TrimSpace(cfg.operacion) == "" {
		switch {
		case opFirmar:
			cfg.operacion = "firmar"
		case opCofirmar:
			cfg.operacion = "cofirmar"
		case opContrafirmar:
			cfg.operacion = "contrafirmar"
		case opVerificar:
			cfg.operacion = "verificar"
		case opCrearHash:
			cfg.operacion = "crear-hash"
		case opComprobarHash:
			cfg.operacion = "comprobar-hash"
		case opProteger:
			cfg.operacion = "proteger"
		case opProtegerFirmando:
			cfg.operacion = "proteger-firmando"
		case opDesproteger:
			cfg.operacion = "desproteger"
		case opListarDestinatariosProteccion:
			cfg.operacion = "listar-destinatarios-proteccion"
		case opExportarDestinatarioProteccion:
			cfg.operacion = "exportar-destinatario-proteccion"
		case opImportarDestinatarioProteccion:
			cfg.operacion = "importar-destinatario-proteccion"
		case opDiagnostico:
			cfg.operacion = "informe-diagnostico"
		case opListarDominios:
			cfg.operacion = "listar-dominios"
		case opAnadirDominio:
			cfg.operacion = "anadir-dominio"
		case opEliminarDominio:
			cfg.operacion = "eliminar-dominio"
		case opImportarDominios:
			cfg.operacion = "importar-dominios"
		case strings.TrimSpace(cfg.ficheroP12) != "":
			cfg.operacion = "importar-p12"
		case opExportarDominios:
			cfg.operacion = "exportar-dominios"
		case opLimpiarDominios:
			cfg.operacion = "limpiar-dominios"
		case opListarAutoseleccion:
			cfg.operacion = "listar-autoseleccion-cert"
		case opEliminarAutoseleccion:
			cfg.operacion = "eliminar-autoseleccion-cert"
		case opResetearAutoseleccion:
			cfg.operacion = "resetear-autoseleccion-cert"
		case opExportarAutoseleccion:
			cfg.operacion = "exportar-autoseleccion-cert"
		case opImportarAutoseleccion:
			cfg.operacion = "importar-autoseleccion-cert"
		case opEstadoAlmacenTLS:
			cfg.operacion = "estado-almacen-tls"
		case opLimpiarAlmacenTLS:
			cfg.operacion = "limpiar-almacen-tls"
		case opEstadoConfianzaTLS:
			cfg.operacion = "estado-confianza-tls"
		case opGenerarCertificadosTLS:
			cfg.operacion = "generar-certificados-tls"
		case opInstalarConfianzaTLS:
			cfg.operacion = "instalar-confianza-tls"
		case opLimpiarAutoseleccion:
			cfg.operacion = "limpiar-autoseleccion-cert"
		}
	}

	if cfg.operacion == "" && cfg.modoCLI && !cfg.listarCerts && !cfg.comprobarCerts && cfg.entrada == "" && cfg.lote == "" && !cfg.mostrarAyuda {
		cfg.mostrarAyuda = true
	}

	if cfg.entrada == "" && cfg.lote == "" && !cfg.listarCerts && !cfg.comprobarCerts && !cfg.mostrarAyuda {
		switch normalizarOperacion(cfg.operacion, cfg.accion, cfg.lote, cfg.entrada) {
		case "domains-list", "domains-add", "domains-remove", "domains-import", "domains-export", "domains-clear", "diagnostics-report",
			"protect-recipients-list",
			"import-p12", "cert-selection-list", "cert-selection-remove", "cert-selection-reset", "cert-selection-export", "cert-selection-import", "cert-selection-clear", "tls-store-status", "tls-store-clear", "tls-trust-status", "tls-generate-certs", "tls-install-trust",
			"audit-verify":
		default:
			return configCLI{}, fmt.Errorf("debe indicar -entrada o -lote")
		}
	}

	return cfg, nil
}

func (a *Adaptador) ejecutarFirma(ctx context.Context, cfg configCLI) int {
	if a.Firmar == nil {
		a.escribirErrorCLI(a.t("Firma", "Firma"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}

	var entrada []byte
	var err error
	if strings.EqualFold(strings.TrimSpace(cfg.formato), "verifactu") {
		entrada, err = securefile.ReadFileLimit(cfg.entrada, commonsigner.VeriFactuMaxXMLBytes)
	} else {
		entrada, err = a.LeerFichero(cfg.entrada)
	}
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	if err := a.Limits.CheckPayload(entrada); err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}

	certID, certRef, err := a.resolverCertificadoFirma(ctx, cfg)
	if err != nil {
		a.escribirErrorCLI(a.t("Certificado", "Certificado"), err)
		return 1
	}

	formato := inferirFormatoFirma(cfg.formato, cfg.entrada)
	if strings.TrimSpace(cfg.idiomaSello) == "" {
		// Por defecto el sello sigue el idioma de la propia CLI.
		cfg.idiomaSello = a.idiomaInterfaz()
	}
	opciones := construirOpcionesFirmaCompat(cfg, formato)
	cmd, err := application.NewSignCommand(
		filepath.Base(cfg.entrada),
		entrada,
		inferirTipoMIME(cfg.entrada),
		formato,
		cfg.accion,
		certID,
		opciones,
	)
	if err != nil {
		a.escribirErrorCLI(a.t("Operación", "Operación"), err)
		return 1
	}
	resultado, err := a.Firmar.Execute(ctx, cmd)
	if err != nil {
		a.escribirErrorCLI(a.t("Firma", "Firma"), err)
		return 1
	}
	if strings.TrimSpace(certRef.ID) == "" {
		certRef = resultado.CertificateUsed
	}
	if strings.TrimSpace(certRef.Subject) == "" && strings.TrimSpace(resultado.CertificateUsed.Subject) != "" {
		certRef.Subject = resultado.CertificateUsed.Subject
	}

	signatureB64 := base64.StdEncoding.EncodeToString(resultado.Result.Data)
	salidaFinal := ""
	renamed := false
	overwrote := false
	if !cfg.noGuardar {
		objetivo := strings.TrimSpace(cfg.salida)
		if objetivo == "" {
			objetivo = construirRutaSalidaPorDefecto(cfg.entrada, resultado.Result.Format)
		}
		resuelta, fueRename, fueOverwrite, err := a.guardarSalida(objetivo, cfg.sobrescribir, resultado.Result.Data)
		if err != nil {
			a.escribirErrorCLI(a.t("Salida", "Salida"), err)
			return 1
		}
		salidaFinal = resuelta
		renamed = fueRename
		overwrote = fueOverwrite
	}

	if cfg.json {
		payload := map[string]any{
			"exito":          true,
			"operacion":      accionCastellano(cfg.accion),
			"entrada":        cfg.entrada,
			"salida":         salidaFinal,
			"formato":        resultado.Result.Format,
			"certificado_id": resultado.CertificateUsed.ID,
			"certificado":    certRef.Subject,
			"renombrado":     renamed,
			"sobrescrito":    overwrote,
		}
		if cfg.imprimirFirma || cfg.noGuardar {
			payload["firma_base64"] = signatureB64
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s %s=%s %s=%s\n",
		a.t("Firma completada correctamente.", "Firma completada correctamente."),
		a.t("Operación", "Operación"),
		cfg.accion,
		a.t("Formato", "Formato"),
		resultado.Result.Format,
		a.t("Certificado", "Certificado"),
		resultado.CertificateUsed.ID,
	)
	if salidaFinal != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Salida", "Salida"), salidaFinal)
	}
	if cfg.imprimirFirma || cfg.noGuardar {
		fmt.Fprintf(a.Stdout, "%s Base64: %s\n", a.t("Firma", "Firma"), signatureB64)
	}
	return 0
}

func (a *Adaptador) ejecutarLote(ctx context.Context, cfg configCLI) int {
	if a.ProcesarLote == nil {
		a.escribirErrorCLI(a.t("Resultados del lote", "Resultados del lote"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	var manifest []elementoLoteCLI
	if esDirectorioRuta(cfg.lote) {
		var err error
		if manifest, err = a.manifiestoDesdeCarpeta(cfg.lote); err != nil {
			a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
			return 1
		}
	} else {
		var manifestBytes []byte
		var err error
		if strings.EqualFold(strings.TrimSpace(cfg.formato), "verifactu") {
			manifestBytes, err = securefile.ReadFileLimit(cfg.lote, 1024*1024)
		} else {
			manifestBytes, err = a.LeerFichero(cfg.lote)
		}
		if err != nil {
			a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
			return 1
		}
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
			return 1
		}
	}
	entradas := make([]application.BatchItemInput, 0, len(manifest))
	for _, item := range manifest {
		requested := item.Formato
		if strings.TrimSpace(requested) == "" {
			requested = cfg.formato
		}
		var contenido []byte
		var err error
		if strings.EqualFold(strings.TrimSpace(requested), "verifactu") {
			contenido, err = securefile.ReadFileLimit(item.Ruta, commonsigner.VeriFactuMaxXMLBytes)
		} else {
			contenido, err = a.LeerFichero(item.Ruta)
		}
		if err != nil {
			a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
			return 1
		}
		if err := a.Limits.CheckPayload(contenido); err != nil {
			a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
			return 1
		}
		nombre := item.Nombre
		if nombre == "" {
			nombre = filepath.Base(item.Ruta)
		}
		formato := item.Formato
		if formato == "" {
			formato = inferirFormatoFirma(cfg.formato, item.Ruta)
		}
		accion := item.Accion
		if accion == "" {
			accion = cfg.accion
		}
		entradas = append(entradas, application.BatchItemInput{
			Nombre:    nombre,
			Contenido: contenido,
			TipoMIME:  item.TipoMIME,
			Formato:   formato,
			Accion:    accion,
			Opciones:  item.Opciones,
		})
	}
	cmd, err := application.NewProcessBatchCommand(entradas)
	if err != nil {
		a.escribirErrorCLI(a.t("Resultados del lote", "Resultados del lote"), err)
		return 1
	}
	resultado, err := a.ProcesarLote.Execute(ctx, cmd)
	if err != nil {
		a.escribirErrorCLI(a.t("Resultados del lote", "Resultados del lote"), err)
		return 1
	}
	// Los resultados solo incluyen los trabajos correctos, en orden.
	var salidas []string
	if !cfg.noGuardar {
		k := 0
		for i, item := range manifest {
			if _, fallo := resultado.Errores[i]; fallo {
				continue
			}
			if k >= len(resultado.Results) {
				break
			}
			firmado := resultado.Results[k].Result
			k++
			objetivo := construirRutaSalidaPorDefecto(item.Ruta, firmado.Format)
			if destino := strings.TrimSpace(cfg.salida); destino != "" {
				objetivo = filepath.Join(destino, filepath.Base(objetivo))
			}
			resuelta, _, _, err := a.guardarSalida(objetivo, cfg.sobrescribir, firmado.Data)
			if err != nil {
				a.escribirErrorCLI(a.t("Salida", "Salida"), err)
				return 1
			}
			salidas = append(salidas, resuelta)
		}
	}
	if cfg.json {
		payload := map[string]any{
			"exito":         len(resultado.Errores) == 0,
			"trabajos":      len(resultado.Results) + len(resultado.Errores),
			"firmas_ok":     len(resultado.Results),
			"errores":       len(resultado.Errores),
			"indices_error": clavesErrores(resultado.Errores),
			"operacion":     accionCastellano(cfg.accion),
			"salidas":       salidas,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(
		a.Stdout,
		"%s: %s=%d %s=%d\n",
		a.t("Resultados del lote", "Resultados del lote"),
		a.t("Total", "Total"),
		len(resultado.Results)+len(resultado.Errores),
		a.t("Errores", "Errores"),
		len(resultado.Errores),
	)
	for _, salida := range salidas {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Salida", "Salida"), salida)
	}
	for _, i := range clavesErrores(resultado.Errores) {
		if i < len(manifest) {
			fmt.Fprintf(a.Stdout, "%s: %s: %v\n", a.t("Errores", "Errores"), filepath.Base(manifest[i].Ruta), resultado.Errores[i])
		}
	}
	return 0
}

// maxDocumentosCarpetaLote coincide con el límite del lote del escritorio.
const maxDocumentosCarpetaLote = 128

// manifiestoDesdeCarpeta firma todos los ficheros regulares de una carpeta
// (sin subcarpetas, ocultos ni enlaces), con el formato deducido de cada uno.
func (a *Adaptador) manifiestoDesdeCarpeta(dir string) ([]elementoLoteCLI, error) {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var manifest []elementoLoteCLI
	for _, e := range entradas {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if len(manifest) == maxDocumentosCarpetaLote {
			return nil, errors.New(a.t("La carpeta contiene más de %d documentos; divídela en varios lotes.", "La carpeta contiene más de %d documentos; divídela en varios lotes.", maxDocumentosCarpetaLote))
		}
		manifest = append(manifest, elementoLoteCLI{Ruta: filepath.Join(dir, e.Name())})
	}
	if len(manifest) == 0 {
		return nil, errors.New(a.t("La carpeta no contiene documentos que firmar.", "La carpeta no contiene documentos que firmar."))
	}
	return manifest, nil
}

func (a *Adaptador) ejecutarVerificacion(ctx context.Context, cfg configCLI) int {
	if a.Verificar == nil {
		a.escribirErrorCLI(a.t("Validar", "Validar"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	data, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	signedDoc, err := domain.NewDocument(filepath.Base(cfg.entrada), data, inferirTipoMIME(cfg.entrada))
	if err != nil {
		a.escribirErrorCLI(a.t("Documento", "Documento"), err)
		return 1
	}
	var original *domain.Document
	if strings.TrimSpace(cfg.original) != "" {
		origData, err := a.LeerFichero(cfg.original)
		if err != nil {
			a.escribirErrorCLI(a.t("Original", "Original"), err)
			return 1
		}
		doc, err := domain.NewDocument(filepath.Base(cfg.original), origData, inferirTipoMIME(cfg.original))
		if err != nil {
			a.escribirErrorCLI(a.t("Original", "Original"), err)
			return 1
		}
		original = &doc
	}
	resultado, err := a.Verificar.Execute(ctx, application.VerifyCommand{
		SignedDocument:   signedDoc,
		OriginalDocument: original,
	})
	if err != nil {
		a.escribirErrorCLI(a.t("Validar", "Validar"), err)
		return 1
	}
	if destino := strings.TrimSpace(cfg.informe); destino != "" {
		informe, err := informeverificacion.HTML(informeverificacion.Datos{
			NombreDocumento: filepath.Base(cfg.entrada), Contenido: data,
			Resultado: resultado.Verification, Fecha: time.Now(), VersionApp: a.Version,
			Idioma: a.idiomaInterfaz(),
		})
		if err == nil {
			destino, _, _, err = a.guardarSalida(destino, cfg.sobrescribir, informe)
		}
		if err != nil {
			a.escribirErrorCLI(a.t("Informe de validación", "Informe de validación"), err)
			return 1
		}
		fmt.Fprintf(a.Stderr, "%s: %s\n", a.t("Informe de validación", "Informe de validación"), destino)
	}
	if cfg.json {
		firmantes := make([]map[string]string, 0, len(resultado.Firmantes))
		signers := make([]string, 0, len(resultado.Firmantes))
		for _, f := range resultado.Firmantes {
			signers = append(signers, f.ID)
			firmantes = append(firmantes, map[string]string{
				"identificador": f.ID,
				"titular":       f.Subject,
				"emisor":        f.Issuer,
				"huella_sha256": f.Fingerprint,
			})
		}
		payload := map[string]any{
			"exito":     resultado.Verification.Valid,
			"motivo":    resultado.Verification.Reason,
			"detalles":  resultado.Verification.Details,
			"firmantes": firmantes,
			"firma":     cfg.entrada,
			"resultado": construirResultadoRicoVerificacionCLI(resultado.Verification, resultado.Firmantes, signers),
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}
	if resultado.Verification.Valid {
		fmt.Fprintln(a.Stdout, a.t("Firma válida.", "Firma válida."))
	} else {
		fmt.Fprintln(a.Stdout, a.t("Firma no válida.", "Firma no válida."))
	}
	if format := strings.TrimSpace(resultado.Verification.Format); format != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Formato", "Formato"), format)
	}
	if coverage := strings.TrimSpace(resultado.Verification.Coverage); coverage != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Cobertura", "Cobertura"), coverage)
	}
	if resultado.Verification.Reason != "" {
		// Los motivos del motor son literales que el catálogo traduce usando
		// el propio literal como clave; la salida JSON los conserva intactos.
		reason := resultado.Verification.Reason
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Motivo", "Motivo"), a.textoMotor(reason))
	}
	for _, detalle := range resultado.Verification.Details {
		fmt.Fprintf(a.Stdout, "- %s\n", a.textoMotor(detalle))
	}
	if len(resultado.Verification.Warnings) > 0 {
		fmt.Fprintf(a.Stdout, "%s:\n", a.t("Advertencias", "Advertencias"))
		for _, warning := range resultado.Verification.Warnings {
			fmt.Fprintf(a.Stdout, "- %s\n", a.textoMotor(warning))
		}
	}
	return 0
}

func construirResultadoRicoVerificacionCLI(v domain.VerificationResult, signerRefs []domain.CertificateRef, signers []string) map[string]any {
	out := map[string]any{
		"valido":    v.Valid,
		"motivo":    v.Reason,
		"detalles":  append([]string(nil), v.Details...),
		"firmantes": append([]string(nil), signers...),
	}
	if format := strings.TrimSpace(v.Format); format != "" {
		out["formato"] = format
	}
	if coverage := strings.TrimSpace(v.Coverage); coverage != "" {
		out["cobertura"] = coverage
	}
	out["integridad"] = map[string]any{
		"estado":   string(v.Integrity.Status),
		"motivo":   v.Integrity.Reason,
		"detalles": append([]string(nil), v.Integrity.Details...),
	}
	out["certificado"] = map[string]any{
		"estado":   string(v.Certificate.Status),
		"motivo":   v.Certificate.Reason,
		"detalles": append([]string(nil), v.Certificate.Details...),
	}
	out["confianza"] = map[string]any{
		"estado":   string(v.Trust.Status),
		"motivo":   v.Trust.Reason,
		"detalles": append([]string(nil), v.Trust.Details...),
	}
	if len(v.SignerSummaries) > 0 {
		summaries := make([]map[string]string, 0, len(v.SignerSummaries))
		for _, signer := range v.SignerSummaries {
			summaries = append(summaries, map[string]string{
				"id":            signer.ID,
				"titular":       signer.Subject,
				"emisor":        signer.Issuer,
				"huella_sha256": signer.Fingerprint,
			})
		}
		out["resumen_firmantes"] = summaries
	} else if len(signerRefs) > 0 {
		summaries := make([]map[string]string, 0, len(signerRefs))
		for _, signer := range signerRefs {
			summaries = append(summaries, map[string]string{
				"id":            signer.ID,
				"titular":       signer.Subject,
				"emisor":        signer.Issuer,
				"huella_sha256": signer.Fingerprint,
			})
		}
		out["resumen_firmantes"] = summaries
	}
	if len(v.Warnings) > 0 {
		out["advertencias"] = append([]string(nil), v.Warnings...)
	}
	if len(v.Errors) > 0 {
		out["errores"] = append([]string(nil), v.Errors...)
	}
	if len(v.Evidence) > 0 {
		evidence := make([]map[string]string, 0, len(v.Evidence))
		for _, item := range v.Evidence {
			evidence = append(evidence, map[string]string{
				"tipo":    item.Type,
				"resumen": item.Summary,
			})
		}
		out["evidencias"] = evidence
	}
	return out
}

func (a *Adaptador) ejecutarCrearHash(ctx context.Context, cfg configCLI) int {
	if a.CrearHash == nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	if strings.TrimSpace(cfg.entrada) == "" {
		a.escribirErrorCLI(
			a.t("Entrada", "Entrada"),
			errors.New(a.t("Selecciona un fichero o indica una ruta local.", "Selecciona un fichero o indica una ruta local.")),
		)
		return 1
	}
	if esDirectorioRuta(cfg.entrada) {
		return a.ejecutarCrearHashDirectorio(ctx, cfg)
	}

	data, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	if err := a.Limits.CheckPayload(data); err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}

	format, err := application.ParseHashOutputFormat(cfg.formatoHash)
	if err != nil {
		a.escribirErrorCLI(a.t("Formato", "Formato"), err)
		return 1
	}
	resultado, err := a.CrearHash.Execute(ctx, application.CreateHashCommand{
		Data:      data,
		Algorithm: cfg.algoritmoHash,
		Format:    format,
	})
	if err != nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), err)
		return 1
	}

	hashBytes := serializarHashCreado(resultado)
	salidaFinal := ""
	renamed := false
	overwrote := false
	if !cfg.noGuardar {
		objetivo := strings.TrimSpace(cfg.salida)
		if objetivo == "" {
			objetivo = construirRutaHashPorDefecto(cfg.entrada, resultado.Format)
		}
		resuelta, fueRename, fueOverwrite, err := a.guardarSalida(objetivo, cfg.sobrescribir, hashBytes)
		if err != nil {
			a.escribirErrorCLI(a.t("Salida", "Salida"), err)
			return 1
		}
		salidaFinal = resuelta
		renamed = fueRename
		overwrote = fueOverwrite
	}

	if cfg.json {
		payload := map[string]any{
			"exito":       true,
			"operacion":   "crear-hash",
			"entrada":     cfg.entrada,
			"salida":      salidaFinal,
			"algoritmo":   resultado.Algorithm,
			"formato":     resultado.Format,
			"hash":        resultado.Encoded,
			"renombrado":  renamed,
			"sobrescrito": overwrote,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s %s=%s\n",
		a.t("Huella generada correctamente.", "Huella generada correctamente."),
		a.t("Algoritmo", "Algoritmo"),
		resultado.Algorithm,
		a.t("Formato", "Formato"),
		resultado.Format,
	)
	if salidaFinal != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Salida", "Salida"), salidaFinal)
	}
	fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Huella", "Huella"), resultado.Encoded)
	return 0
}

func (a *Adaptador) ejecutarComprobarHash(ctx context.Context, cfg configCLI) int {
	if strings.TrimSpace(cfg.entrada) == "" {
		a.escribirErrorCLI(
			a.t("Entrada", "Entrada"),
			errors.New(a.t("Selecciona un fichero o indica una ruta local.", "Selecciona un fichero o indica una ruta local.")),
		)
		return 1
	}
	if strings.TrimSpace(cfg.ficheroHash) == "" {
		a.escribirErrorCLI(
			a.t("Huella", "Huella"),
			errors.New(a.t("Selecciona un fichero de huella/manifiesto o indica su ruta.", "Selecciona un fichero de huella/manifiesto o indica su ruta.")),
		)
		return 1
	}
	if esDirectorioRuta(cfg.entrada) {
		return a.ejecutarComprobarHashDirectorio(ctx, cfg)
	}
	if a.ComprobarHash == nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}

	data, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	if err := a.Limits.CheckPayload(data); err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	hashData, err := a.LeerFichero(cfg.ficheroHash)
	if err != nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), err)
		return 1
	}

	expectedDigest, inferredAlg, inferredFormat, err := application.ParseStoredHash(hashData, cfg.ficheroHash)
	if err != nil {
		a.escribirErrorCLI(a.t("Formato", "Formato"), err)
		return 1
	}
	algoritmo := inferredAlg
	if trimmed := strings.TrimSpace(cfg.algoritmoHash); trimmed != "" {
		algoritmo = trimmed
	}
	resultado, err := a.ComprobarHash.Execute(ctx, application.CheckHashCommand{
		Data:         data,
		ExpectedHash: expectedDigest,
		Algorithm:    algoritmo,
		Format:       inferredFormat,
	})
	if err != nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), err)
		return 1
	}

	if cfg.json {
		payload := map[string]any{
			"exito":             resultado.Valid,
			"operacion":         "comprobar-hash",
			"entrada":           cfg.entrada,
			"fichero_hash":      cfg.ficheroHash,
			"algoritmo":         resultado.Algorithm,
			"formato":           resultado.Format,
			"hash_esperado":     resultado.ExpectedEncoded,
			"hash_calculado":    resultado.ActualEncoded,
			"algoritmo_fichero": inferredAlg,
			"formato_fichero":   inferredFormat,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		if resultado.Valid {
			return 0
		}
		return 2
	}

	if resultado.Valid {
		fmt.Fprintf(
			a.Stdout,
			"%s %s=%s %s=%s\n",
			a.t("Huella válida.", "Huella válida."),
			a.t("Algoritmo", "Algoritmo"),
			resultado.Algorithm,
			a.t("Formato", "Formato"),
			resultado.Format,
		)
		return 0
	}
	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s %s=%s\n",
		a.t("La huella no coincide con el fichero.", "La huella no coincide con el fichero."),
		a.t("Algoritmo", "Algoritmo"),
		resultado.Algorithm,
		a.t("Formato", "Formato"),
		resultado.Format,
	)
	fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Esperada", "Esperada"), resultado.ExpectedEncoded)
	fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Estado actual", "Estado actual"), resultado.ActualEncoded)
	return 2
}

func (a *Adaptador) ejecutarCrearHashDirectorio(ctx context.Context, cfg configCLI) int {
	if a.CrearHashDir == nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	format, err := parseDirectoryHashManifestFormat(cfg.formatoHash)
	if err != nil {
		a.escribirErrorCLI(a.t("Formato", "Formato"), err)
		return 1
	}
	resultado, err := a.CrearHashDir.Execute(ctx, application.CreateDirectoryHashManifestCommand{
		RootPath:  cfg.entrada,
		Algorithm: cfg.algoritmoHash,
		Format:    format,
		Recursive: cfg.hashRecursivo,
	})
	if err != nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), err)
		return 1
	}
	salidaFinal := ""
	renamed := false
	overwrote := false
	if !cfg.noGuardar {
		objetivo := strings.TrimSpace(cfg.salida)
		if objetivo == "" {
			objetivo = construirRutaHashDirectorioPorDefecto(cfg.entrada, resultado.Format)
		}
		resuelta, fueRename, fueOverwrite, err := a.guardarSalida(objetivo, cfg.sobrescribir, resultado.Data)
		if err != nil {
			a.escribirErrorCLI(a.t("Salida", "Salida"), err)
			return 1
		}
		salidaFinal = resuelta
		renamed = fueRename
		overwrote = fueOverwrite
	}
	if cfg.json {
		payload := map[string]any{
			"exito":       true,
			"operacion":   "crear-hash",
			"entrada":     cfg.entrada,
			"salida":      salidaFinal,
			"algoritmo":   resultado.Algorithm,
			"formato":     resultado.Format,
			"recursivo":   resultado.Manifest.Recursive,
			"entradas":    len(resultado.Manifest.Entries),
			"renombrado":  renamed,
			"sobrescrito": overwrote,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s %s=%s %s=%d %s=%t\n",
		a.t("Manifiesto de huellas generado.", "Manifiesto de huellas generado."),
		a.t("Algoritmo", "Algoritmo"),
		resultado.Algorithm,
		a.t("Formato", "Formato"),
		resultado.Format,
		a.t("Entradas", "Entradas"),
		len(resultado.Manifest.Entries),
		a.t("Recursivo", "Recursivo"),
		resultado.Manifest.Recursive,
	)
	if salidaFinal != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Salida", "Salida"), salidaFinal)
	}
	return 0
}

func (a *Adaptador) ejecutarComprobarHashDirectorio(ctx context.Context, cfg configCLI) int {
	if a.ComprobarHashDir == nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	manifest, err := a.LeerFichero(cfg.ficheroHash)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	resultado, err := a.ComprobarHashDir.Execute(ctx, application.CheckDirectoryHashManifestCommand{
		RootPath:     cfg.entrada,
		ManifestData: manifest,
		ManifestHint: cfg.ficheroHash,
	})
	if err != nil {
		a.escribirErrorCLI(a.t("Huella", "Huella"), err)
		return 1
	}
	var reportData []byte
	if a.InformeHashDir != nil {
		reportData, err = a.InformeHashDir.EncodeReport(ctx, resultado.Report)
		if err != nil {
			a.escribirErrorCLI(a.t("Informe", "Informe"), err)
			return 1
		}
	}
	reportPath := strings.TrimSpace(cfg.salida)
	if reportPath != "" {
		if len(reportData) == 0 {
			a.escribirErrorCLI(a.t("Informe", "Informe"), errors.New(a.t("No disponible", "No disponible")))
			return 1
		}
		if err := a.Escribir(reportPath, reportData, 0o600); err != nil {
			a.escribirErrorCLI(a.t("Informe", "Informe"), err)
			return 1
		}
	}
	if cfg.json {
		payload := map[string]any{
			"exito":             resultado.Valid,
			"operacion":         "comprobar-hash",
			"entrada":           cfg.entrada,
			"fichero_hash":      cfg.ficheroHash,
			"algoritmo":         resultado.Report.Algorithm,
			"recursivo":         resultado.Report.Recursive,
			"matching_hash":     append([]string(nil), resultado.Report.MatchingHash...),
			"not_matching_hash": append([]string(nil), resultado.Report.NotMatchingHash...),
			"hash_without_file": append([]string(nil), resultado.Report.HashWithoutFile...),
			"file_without_hash": append([]string(nil), resultado.Report.FileWithoutHash...),
		}
		if len(reportData) > 0 {
			payload["report_base64"] = base64.StdEncoding.EncodeToString(reportData)
		}
		if reportPath != "" {
			payload["salida_informe"] = reportPath
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		if resultado.Valid {
			return 0
		}
		return 2
	}
	if resultado.Valid {
		fmt.Fprintf(
			a.Stdout,
			"%s %s=%s %s=%d\n",
			a.t("Directorio íntegro.", "Directorio íntegro."),
			a.t("Algoritmo", "Algoritmo"),
			resultado.Report.Algorithm,
			a.t("Coinciden", "Coinciden"),
			len(resultado.Report.MatchingHash),
		)
		return 0
	}
	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s\n",
		a.t("Directorio con diferencias respecto al manifiesto.", "Directorio con diferencias respecto al manifiesto."),
		a.t("Algoritmo", "Algoritmo"),
		resultado.Report.Algorithm,
	)
	if len(resultado.Report.NotMatchingHash) > 0 {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("No coinciden", "No coinciden"), strings.Join(resultado.Report.NotMatchingHash, ", "))
	}
	if len(resultado.Report.HashWithoutFile) > 0 {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Hash sin fichero", "Hash sin fichero"), strings.Join(resultado.Report.HashWithoutFile, ", "))
	}
	if len(resultado.Report.FileWithoutHash) > 0 {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Fichero sin hash", "Fichero sin hash"), strings.Join(resultado.Report.FileWithoutHash, ", "))
	}
	if reportPath != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Informe", "Informe"), reportPath)
	}
	return 2
}

func (a *Adaptador) ejecutarProteccion(ctx context.Context, cfg configCLI) int {
	if a.Proteger == nil {
		a.escribirErrorCLI(a.t("Operación", "Operación"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	contenedorProteccion := strings.TrimSpace(cfg.contenedorProteccion)
	if len(cfg.destinatarios) == 0 && !contenedorProteccionSinDestinatarios(contenedorProteccion) {
		a.escribirErrorCLI(
			a.t("Destinatarios", "Destinatarios"),
			errors.New(a.t("Selecciona al menos un destinatario de protección.", "Selecciona al menos un destinatario de protección.")),
		)
		return 1
	}

	data, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	if err := a.Limits.CheckPayload(data); err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}

	optionsProteccion := mapsClone(cfg.opciones)
	if trimmed := contenedorProteccion; trimmed != "" {
		optionsProteccion["container"] = trimmed
	}
	if trimmed := strings.TrimSpace(cfg.secretProteccionB64); trimmed != "" {
		optionsProteccion["secret_b64"] = trimmed
	}

	cmd, err := application.NewProtectCommand(
		filepath.Base(cfg.entrada),
		data,
		inferirTipoMIME(cfg.entrada),
		cfg.perfilProteccion,
		cfg.destinatarios,
		optionsProteccion,
	)
	if err != nil {
		a.escribirErrorCLI(a.t("Operación", "Operación"), err)
		return 1
	}
	resultado, err := a.Proteger.Execute(ctx, cmd)
	if err != nil {
		a.escribirErrorCLI(a.t("Resultado", "Resultado"), err)
		return 1
	}

	protectedB64 := base64.StdEncoding.EncodeToString(resultado.Protected.Document.Content)
	salidaFinal := ""
	renamed := false
	overwrote := false
	if !cfg.noGuardar {
		objetivo := strings.TrimSpace(cfg.salida)
		if objetivo == "" {
			objetivo = construirRutaProtegidaPorDefecto(cfg.entrada, resultado.Protected.Document.Name)
		}
		resuelta, fueRename, fueOverwrite, err := a.guardarSalida(objetivo, cfg.sobrescribir, resultado.Protected.Document.Content)
		if err != nil {
			a.escribirErrorCLI(a.t("Salida", "Salida"), err)
			return 1
		}
		salidaFinal = resuelta
		renamed = fueRename
		overwrote = fueOverwrite
	}

	if cfg.json {
		payload := map[string]any{
			"exito":               true,
			"operacion":           "proteger",
			"entrada":             cfg.entrada,
			"salida":              salidaFinal,
			"perfil_proteccion":   resultado.Protected.Profile,
			"nombre_documento":    resultado.Protected.Document.Name,
			"tipo_mime":           resultado.Protected.Document.MIMEType,
			"contenedor":          optionsProteccion["container"],
			"destinatarios":       append([]string(nil), cfg.destinatarios...),
			"total_destinatarios": resultado.Protected.RecipientCount,
			"renombrado":          renamed,
			"sobrescrito":         overwrote,
		}
		if cfg.noGuardar || cfg.imprimirFirma {
			payload["contenido_protegido_base64"] = protectedB64
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}

	contenedor := optionsProteccion["container"]
	if strings.TrimSpace(contenedor) == "" {
		contenedor = "json"
	}
	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s %s=%s %s=%d\n",
		a.t("Protección completada correctamente.", "Protección completada correctamente."),
		a.t("Perfil", "Perfil"),
		resultado.Protected.Profile,
		a.t("Contenedor", "Contenedor"),
		contenedor,
		a.t("Destinatarios", "Destinatarios"),
		resultado.Protected.RecipientCount,
	)
	if salidaFinal != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Salida", "Salida"), salidaFinal)
	}
	if cfg.noGuardar || cfg.imprimirFirma {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Base64", "Base64"), protectedB64)
	}
	return 0
}

func (a *Adaptador) ejecutarProteccionFirmada(ctx context.Context, cfg configCLI) int {
	if a.ProtegerFirmando == nil {
		a.escribirErrorCLI(a.t("Operación", "Operación"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	if len(cfg.destinatarios) == 0 {
		a.escribirErrorCLI(
			a.t("Destinatarios", "Destinatarios"),
			errors.New(a.t("Selecciona al menos un destinatario de protección.", "Selecciona al menos un destinatario de protección.")),
		)
		return 1
	}

	data, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	if err := a.Limits.CheckPayload(data); err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}

	optionsProteccion := mapsClone(cfg.opciones)
	if trimmed := strings.TrimSpace(cfg.contenedorProteccion); trimmed != "" {
		optionsProteccion["container"] = trimmed
	}
	if err := normalizarContenedorProteccionFirmada(optionsProteccion); err != nil {
		a.escribirErrorCLI(a.t("Contenedor", "Contenedor"), err)
		return 1
	}
	cmd, err := application.NewProtectAndSignCommand(
		filepath.Base(cfg.entrada),
		data,
		inferirTipoMIME(cfg.entrada),
		cfg.perfilProteccion,
		cfg.destinatarios,
		cfg.certificado,
		optionsProteccion,
	)
	if err != nil {
		a.escribirErrorCLI(a.t("Operación", "Operación"), err)
		return 1
	}
	resultado, err := a.ProtegerFirmando.Execute(ctx, cmd)
	if err != nil {
		a.escribirErrorCLI(a.t("Resultado", "Resultado"), err)
		return 1
	}

	protectedB64 := base64.StdEncoding.EncodeToString(resultado.Protected.Document.Content)
	salidaFinal := ""
	renamed := false
	overwrote := false
	if !cfg.noGuardar {
		objetivo := strings.TrimSpace(cfg.salida)
		if objetivo == "" {
			objetivo = construirRutaProtegidaPorDefecto(cfg.entrada, resultado.Protected.Document.Name)
		}
		resuelta, fueRename, fueOverwrite, err := a.guardarSalida(objetivo, cfg.sobrescribir, resultado.Protected.Document.Content)
		if err != nil {
			a.escribirErrorCLI(a.t("Salida", "Salida"), err)
			return 1
		}
		salidaFinal = resuelta
		renamed = fueRename
		overwrote = fueOverwrite
	}

	if cfg.json {
		payload := map[string]any{
			"exito":               true,
			"operacion":           "proteger-firmando",
			"entrada":             cfg.entrada,
			"salida":              salidaFinal,
			"perfil_proteccion":   resultado.Protected.Profile,
			"nombre_documento":    resultado.Protected.Document.Name,
			"tipo_mime":           resultado.Protected.Document.MIMEType,
			"contenedor":          "signedandenvelopeddata",
			"destinatarios":       append([]string(nil), cfg.destinatarios...),
			"total_destinatarios": resultado.Protected.RecipientCount,
			"certificado":         resultado.CertificateUsed.ID,
			"renombrado":          renamed,
			"sobrescrito":         overwrote,
		}
		if cfg.noGuardar || cfg.imprimirFirma {
			payload["contenido_protegido_base64"] = protectedB64
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s %s=SignedAndEnvelopedData %s=%d %s=%s\n",
		a.t("Protección completada correctamente.", "Protección completada correctamente."),
		a.t("Perfil", "Perfil"),
		resultado.Protected.Profile,
		a.t("Contenedor", "Contenedor"),
		a.t("Destinatarios", "Destinatarios"),
		resultado.Protected.RecipientCount,
		a.t("Certificado", "Certificado"),
		resultado.CertificateUsed.ID,
	)
	if salidaFinal != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Salida", "Salida"), salidaFinal)
	}
	if cfg.noGuardar || cfg.imprimirFirma {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Base64", "Base64"), protectedB64)
	}
	return 0
}

func normalizarContenedorProteccionFirmada(options map[string]string) error {
	container := normalizarTokenContenedorProteccion(options["container"])
	switch container {
	case "", "cmssignedandenveloped", "signedandenveloped", "signedandenvelopeddata":
		options["container"] = "signedandenvelopeddata"
		return nil
	case "cmsauthenveloped", "authenveloped", "authenvelopeddata", "authenticatedenvelopeddata":
		return errors.New("AuthEnvelopedData no esta soportado en proteger-firmando; solo se admite SignedAndEnvelopedData")
	case "cmsauthenticated", "cmsauthenticateddata", "authenticated", "authenticateddata":
		return errors.New("AuthenticatedData no esta soportado en proteger-firmando; solo se admite SignedAndEnvelopedData")
	case "cmscompressed", "cmscompresseddata", "compressed", "compresseddata":
		return errors.New("CompressedData no esta soportado en proteger-firmando; solo se admite SignedAndEnvelopedData")
	case "cmsencrypted", "cmsencrypteddata", "encrypted", "encrypteddata":
		return errors.New("EncryptedData no corresponde a proteger-firmando; use proteger con contenedor cms-encrypted")
	case "cms", "cmsenveloped", "cmsenvelopeddata", "enveloped", "envelopeddata":
		return errors.New("EnvelopedData no corresponde a proteger-firmando; use proteger con contenedor cms")
	default:
		return fmt.Errorf("contenedor CMS no soportado en proteger-firmando: %q", container)
	}
}

func normalizarTokenContenedorProteccion(raw string) string {
	normalizado := strings.ToLower(strings.TrimSpace(raw))
	normalizado = strings.ReplaceAll(normalizado, "-", "")
	normalizado = strings.ReplaceAll(normalizado, "_", "")
	normalizado = strings.ReplaceAll(normalizado, " ", "")
	return normalizado
}

func contenedorProteccionSinDestinatarios(raw string) bool {
	switch normalizarTokenContenedorProteccion(raw) {
	case "cmsencrypted", "cmsencrypteddata", "encrypted", "encrypteddata":
		return true
	default:
		return false
	}
}

func (a *Adaptador) ejecutarDesproteccion(ctx context.Context, cfg configCLI) int {
	if a.Desproteger == nil {
		a.escribirErrorCLI(a.t("Operación", "Operación"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}

	data, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	if err := a.Limits.CheckPayload(data); err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	doc, err := domain.NewDocument(filepath.Base(cfg.entrada), data, inferirTipoMIME(cfg.entrada))
	if err != nil {
		a.escribirErrorCLI(a.t("Documento", "Documento"), err)
		return 1
	}

	optionsDesproteccion := map[string]string{}
	if trimmed := strings.TrimSpace(cfg.secretProteccionB64); trimmed != "" {
		optionsDesproteccion["secret_b64"] = trimmed
	}
	resultado, err := a.Desproteger.Execute(ctx, application.UnprotectCommand{
		ProtectedDocument: doc,
		Options:           optionsDesproteccion,
	})
	if err != nil {
		a.escribirErrorCLI(a.t("Resultado", "Resultado"), err)
		return 1
	}

	plainB64 := base64.StdEncoding.EncodeToString(resultado.Unprotected.Document.Content)
	salidaFinal := ""
	renamed := false
	overwrote := false
	if !cfg.noGuardar {
		objetivo := strings.TrimSpace(cfg.salida)
		if objetivo == "" {
			objetivo = construirRutaDesprotegidaPorDefecto(cfg.entrada, resultado.Unprotected.Document.Name)
		}
		resuelta, fueRename, fueOverwrite, err := a.guardarSalida(objetivo, cfg.sobrescribir, resultado.Unprotected.Document.Content)
		if err != nil {
			a.escribirErrorCLI(a.t("Salida", "Salida"), err)
			return 1
		}
		salidaFinal = resuelta
		renamed = fueRename
		overwrote = fueOverwrite
	}

	if cfg.json {
		payload := map[string]any{
			"exito":             true,
			"operacion":         "desproteger",
			"entrada":           cfg.entrada,
			"salida":            salidaFinal,
			"perfil_proteccion": resultado.Unprotected.Profile,
			"destinatario":      resultado.Unprotected.RecipientID,
			"documento":         resultado.Unprotected.Document.Name,
			"renombrado":        renamed,
			"sobrescrito":       overwrote,
		}
		if cfg.noGuardar || cfg.imprimirFirma {
			payload["contenido_desprotegido_base64"] = plainB64
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(
		a.Stdout,
		"%s %s=%s %s=%s\n",
		a.t("Desprotección completada correctamente.", "Desprotección completada correctamente."),
		a.t("Perfil", "Perfil"),
		resultado.Unprotected.Profile,
		a.t("Destinatario", "Destinatario"),
		resultado.Unprotected.RecipientID,
	)
	if salidaFinal != "" {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Salida", "Salida"), salidaFinal)
	}
	if cfg.noGuardar || cfg.imprimirFirma {
		fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Base64", "Base64"), plainB64)
	}
	return 0
}

func (a *Adaptador) ejecutarDestinatariosProteccionListar(ctx context.Context, cfg configCLI) int {
	if a.Destinatarios == nil {
		a.escribirErrorCLI(a.t("Destinatarios", "Destinatarios"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	recipients, err := a.Destinatarios.List(ctx)
	if err != nil {
		a.escribirErrorCLI(a.t("Destinatarios", "Destinatarios"), err)
		return 1
	}
	if cfg.json {
		out := make([]map[string]any, 0, len(recipients))
		for _, r := range recipients {
			perfil, algoritmo := describirDestinatarioProteccionCLI(r)
			out = append(out, map[string]any{
				"id":        r.ID,
				"nombre":    r.Label,
				"perfil":    perfil,
				"algoritmo": algoritmo,
			})
		}
		if err := escribirJSON(a.Stdout, map[string]any{"exito": true, "destinatarios": out}); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}
	if len(recipients) == 0 {
		fmt.Fprintln(a.Stdout, a.t("No hay destinatarios cargados para este perfil.", "No hay destinatarios cargados para este perfil."))
		fmt.Fprintln(a.Stdout, a.t(
			"El perfil compat requiere una identidad RSA con clave privada descifrable, por ejemplo un P12/PFX autorizado. Los certificados opacos del almacén del sistema siguen disponibles para firmar; carga un P12/PFX apto o usa el perfil alto.",
			"El perfil compat requiere una identidad RSA con clave privada descifrable, por ejemplo un P12/PFX autorizado. Los certificados opacos del almacén del sistema siguen disponibles para firmar; carga un P12/PFX apto o usa el perfil alto.",
		))
		return 0
	}
	hasCompat := false
	for _, recipient := range recipients {
		perfil, _ := describirDestinatarioProteccionCLI(recipient)
		hasCompat = hasCompat || perfil == "compat"
		fmt.Fprintf(a.Stdout, "%s\t%s\t%s\n", recipient.ID, perfil, recipient.Label)
	}
	if !hasCompat {
		fmt.Fprintln(a.Stdout, a.t(
			"No hay identidades para el perfil compat. Requiere una clave RSA descifrable, por ejemplo desde un P12/PFX autorizado. Los certificados opacos del almacén del sistema siguen disponibles para firmar; carga un P12/PFX apto o usa el perfil alto.",
			"No hay identidades para el perfil compat. Requiere una clave RSA descifrable, por ejemplo desde un P12/PFX autorizado. Los certificados opacos del almacén del sistema siguen disponibles para firmar; carga un P12/PFX apto o usa el perfil alto.",
		))
	}
	return 0
}

func (a *Adaptador) ejecutarDestinatarioProteccionExportar(ctx context.Context, cfg configCLI) int {
	if a.ExportarProteccion == nil {
		a.escribirErrorCLI(a.t("Destinatarios", "Destinatarios"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	if len(cfg.destinatarios) == 0 {
		a.escribirErrorCLI(
			a.t("Destinatario", "Destinatario"),
			errors.New(a.t("Selecciona al menos un destinatario de protección.", "Selecciona al menos un destinatario de protección.")),
		)
		return 1
	}
	resultado, err := a.ExportarProteccion.Execute(ctx, application.ExportProtectionRecipientCommand{
		RecipientID: strings.TrimSpace(cfg.destinatarios[0]),
	})
	if err != nil {
		a.escribirErrorCLI(a.t("Destinatario", "Destinatario"), err)
		return 1
	}
	destino := strings.TrimSpace(cfg.salida)
	if destino == "" {
		destino = construirRutaDestinatarioProteccionPorDefecto(resultado.Recipient.ID)
	}
	if err := a.Escribir(destino, resultado.Data, 0o600); err != nil {
		a.escribirErrorCLI(a.t("Salida", "Salida"), err)
		return 1
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, map[string]any{
			"exito":     true,
			"destino":   destino,
			"id":        resultado.Recipient.ID,
			"nombre":    resultado.Recipient.Label,
			"perfil":    "alto",
			"algoritmo": "ML-KEM-768 + X25519",
		}); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(
		a.Stdout,
		"%s=exportar-destinatario\n%s=%s\n%s=%s\n",
		a.t("Operación", "Operación"),
		a.t("Destinatario", "Destinatario"),
		resultado.Recipient.ID,
		a.t("Salida", "Salida"),
		destino,
	)
	return 0
}

func (a *Adaptador) ejecutarDestinatarioProteccionImportar(ctx context.Context, cfg configCLI) int {
	if a.ImportarProteccion == nil {
		a.escribirErrorCLI(a.t("Destinatarios", "Destinatarios"), errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	ruta := strings.TrimSpace(cfg.entrada)
	if ruta == "" {
		a.escribirErrorCLI(
			a.t("Entrada", "Entrada"),
			errors.New(a.t("Selecciona un fichero de destinatario fuerte.", "Selecciona un fichero de destinatario fuerte.")),
		)
		return 1
	}
	data, err := a.LeerFichero(ruta)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	resultado, err := a.ImportarProteccion.Execute(ctx, application.ImportProtectionRecipientCommand{Data: data})
	if err != nil {
		a.escribirErrorCLI(a.t("Destinatario", "Destinatario"), err)
		return 1
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, map[string]any{
			"exito":     true,
			"id":        resultado.Recipient.ID,
			"nombre":    resultado.Recipient.Label,
			"perfil":    "alto",
			"algoritmo": "ML-KEM-768 + X25519",
		}); err != nil {
			a.escribirErrorCLI("JSON", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(
		a.Stdout,
		"%s=importar-destinatario\n%s=%s\n%s=%s\n",
		a.t("Operación", "Operación"),
		a.t("Destinatario", "Destinatario"),
		resultado.Recipient.ID,
		a.t("Nombre", "Nombre"),
		resultado.Recipient.Label,
	)
	return 0
}

func describirDestinatarioProteccionCLI(recipient domain.ProtectionRecipient) (perfil, algoritmo string) {
	switch {
	case len(recipient.MLKEM768PublicKey) > 0 && len(recipient.X25519PublicKey) > 0:
		return "alto", "ML-KEM-768 + X25519"
	case len(recipient.RSAOAEP256PublicKeyDER) > 0:
		return "compat", "RSA-OAEP-SHA256"
	default:
		return "compat", "desconocido"
	}
}

func (a *Adaptador) ejecutarListadoCertificados(ctx context.Context, cfg configCLI) int {
	certs, err := a.listarCertificados(ctx)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error listando certificados:", err)
		return 1
	}
	if cfg.json {
		out := make([]map[string]any, 0, len(certs))
		for _, c := range certs {
			out = append(out, map[string]any{
				"indice":         c.Indice,
				"id":             c.Ref.ID,
				"nombre":         mejorNombreCertificado(c.Ref),
				"titular":        c.Ref.Subject,
				"emisor":         c.Ref.Issuer,
				"huella_sha256":  c.Ref.Fingerprint,
				"valido_hasta":   c.Ref.NotAfter,
				"puede_firmar":   c.PuedeFirmar,
				"problema_firma": c.Problema,
			})
		}
		if err := escribirJSON(a.Stdout, map[string]any{"exito": true, "certificados": out}); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	if len(certs) == 0 {
		fmt.Fprintln(a.Stdout, "No se encontraron certificados.")
		return 0
	}
	for _, c := range certs {
		canSign := "sí"
		if !c.PuedeFirmar {
			canSign = "no"
		}
		fmt.Fprintf(a.Stdout, "[%d] %s\n", c.Indice, mejorNombreCertificado(c.Ref))
		fmt.Fprintf(a.Stdout, "    identificador=%s puede_firmar=%s valido_hasta=%s\n", c.Ref.ID, canSign, c.Ref.NotAfter.Format(time.RFC3339))
		if c.Problema != "" {
			fmt.Fprintf(a.Stdout, "    problemaFirma=%s\n", c.Problema)
		}
	}
	return 0
}

func (a *Adaptador) ejecutarDiagnostico(ctx context.Context) int {
	certs, err := a.listarCertificados(ctx)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error obteniendo certificados:", err)
		return 1
	}
	dominios, err := a.leerDominiosConfiados(ctx)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error leyendo dominios confiados:", err)
		return 1
	}
	puedenFirmar := 0
	for _, cert := range certs {
		if cert.PuedeFirmar {
			puedenFirmar++
		}
	}
	comps := serializarComponentes(components.Comprobar())
	payload := map[string]any{
		"exito":                true,
		"total_certificados":   len(certs),
		"total_firmables":      puedenFirmar,
		"dominios_confiados":   serializarDominiosConfiados(dominios),
		"componentes_externos": comps,
	}
	// Comprobación de versión estrictamente opt-in (GRXFIRMA_CHECK_UPDATES=1):
	// sin esa variable no se contacta con ningún servidor externo.
	var avisoActualizacion string
	if updatecheck.Habilitado() {
		if res, err := updatecheck.New().Comprobar(ctx, a.Version); err != nil {
			payload["actualizacion"] = map[string]any{
				"error":   updatecheck.ErrorCode(err),
				"mensaje": a.translateSecret(updatecheck.MessageKey(err)),
			}
		} else {
			if res.Estado == updatecheck.EstadoSinPublicaciones {
				res.Titulo = a.t("Sin versiones publicadas", "Sin versiones publicadas")
				res.Mensaje = a.t("Todavía no hay versiones publicadas en el canal oficial.", "Todavía no hay versiones publicadas en el canal oficial.")
			}
			payload["actualizacion"] = res
			if res.HayNueva {
				avisoActualizacion = fmt.Sprintf("hay una versión más reciente disponible: %s (actual %s) — %s",
					res.UltimaVersion, res.VersionActual, res.URL)
			}
		}
	}
	if err := escribirJSON(a.Stdout, payload); err != nil {
		fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
		return 1
	}
	// Aviso legible por stderr de los componentes externos que faltan, para que
	// el operador sepa qué funcionalidad quedará limitada sin tener que leer el
	// JSON.
	for _, e := range components.Faltantes() {
		fmt.Fprintln(a.Stderr, "aviso:", e.MensajeFalta())
	}
	if avisoActualizacion != "" {
		fmt.Fprintln(a.Stderr, "aviso:", avisoActualizacion)
	}
	return 0
}

// serializarComponentes convierte los estados de componentes externos en una
// estructura serializable a JSON.
func serializarComponentes(estados []components.Estado) []map[string]any {
	out := make([]map[string]any, 0, len(estados))
	for _, e := range estados {
		if !e.Relevante {
			continue
		}
		out = append(out, map[string]any{
			"nombre":            e.Nombre,
			"para_que":          e.ParaQue,
			"disponible":        e.Disponible,
			"ruta":              e.Ruta,
			"pista_instalacion": e.PistaInstalacion,
		})
	}
	return out
}

func (a *Adaptador) ejecutarDominiosListar(ctx context.Context, cfg configCLI) int {
	dominios, err := a.leerDominiosConfiados(ctx)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error leyendo dominios confiados:", err)
		return 1
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, map[string]any{"exito": true, "dominios": serializarDominiosConfiados(dominios)}); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	if len(dominios) == 0 {
		if a.debePreguntarImportacionInicial(cfg) && a.preguntarImportacionInicial() {
			if importados, err := a.importarDominiosIniciales(ctx); err != nil {
				fmt.Fprintln(a.Stderr, "error importando dominios públicos iniciales:", err)
				return 1
			} else {
				fmt.Fprintf(a.Stdout, "Se han importado %d dominios públicos iniciales.\n", importados)
			}
			dominios, err = a.leerDominiosConfiados(ctx)
			if err != nil {
				fmt.Fprintln(a.Stderr, "error leyendo dominios confiados:", err)
				return 1
			}
			if len(dominios) == 0 {
				fmt.Fprintln(a.Stdout, "No hay dominios confiados.")
				return 0
			}
			for _, d := range dominios {
				fmt.Fprintf(a.Stdout, "%s %s\n", estadoConfianzaCastellano(d.Status), d.Origin)
			}
			return 0
		}
		fmt.Fprintln(a.Stdout, "No hay dominios confiados.")
		return 0
	}
	for _, d := range dominios {
		fmt.Fprintf(a.Stdout, "%s %s\n", estadoConfianzaCastellano(d.Status), d.Origin)
	}
	return 0
}

func (a *Adaptador) ejecutarDominiosGestion(ctx context.Context, cfg configCLI, accion application.TrustAction) int {
	if a.GestionDominios == nil {
		fmt.Fprintln(a.Stderr, "error: gestión de dominios no configurada")
		return 1
	}
	origen := strings.TrimSpace(cfg.dominio)
	if origen == "" {
		fmt.Fprintln(a.Stderr, "error: debe indicar -domain o -dominio")
		return 1
	}
	if _, err := a.GestionDominios.Execute(ctx, application.ManageTrustedDomainCommand{
		Origin: origen,
		Action: accion,
	}); err != nil {
		fmt.Fprintln(a.Stderr, "error gestionando dominio:", err)
		return 1
	}
	return a.ejecutarDominiosListar(ctx, cfg)
}

func (a *Adaptador) ejecutarDominiosImportar(ctx context.Context, cfg configCLI) int {
	if a.GestionDominios == nil {
		fmt.Fprintln(a.Stderr, "error: gestión de dominios no configurada")
		return 1
	}
	ruta := strings.TrimSpace(cfg.ficheroDominios)
	if ruta == "" {
		fmt.Fprintln(a.Stderr, "error: debe indicar -domain-file o -fichero-dominios")
		return 1
	}
	data, err := a.LeerFichero(ruta)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error leyendo fichero de dominios:", err)
		return 1
	}
	dominios, err := parsearFicheroDominios(data)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error parseando dominios:", err)
		return 1
	}
	added := 0
	for _, dominio := range dominios {
		if _, err := a.GestionDominios.Execute(ctx, application.ManageTrustedDomainCommand{
			Origin: dominio,
			Action: application.TrustActionAllow,
		}); err == nil {
			added++
		}
	}
	if cfg.json {
		payload := map[string]any{
			"exito":      true,
			"fichero":    ruta,
			"procesados": len(dominios),
			"anadidos":   added,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Importación completada: procesados=%d añadidos=%d fichero=%s\n", len(dominios), added, ruta)
	return a.ejecutarDominiosListar(ctx, cfg)
}

func (a *Adaptador) ejecutarDominiosExportar(ctx context.Context, cfg configCLI) int {
	ruta := strings.TrimSpace(cfg.ficheroDominios)
	if ruta == "" {
		fmt.Fprintln(a.Stderr, "error: debe indicar -fichero-dominios o -domain-file")
		return 1
	}
	dominios, err := a.leerDominiosConfiados(ctx)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error leyendo dominios confiados:", err)
		return 1
	}
	permitidos := extraerDominiosPermitidos(dominios)
	contenido, err := serializarFicheroDominios(ruta, permitidos)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error preparando exportación de dominios:", err)
		return 1
	}
	if err := a.Escribir(ruta, contenido, 0o600); err != nil {
		fmt.Fprintln(a.Stderr, "error escribiendo fichero de dominios:", err)
		return 1
	}
	if cfg.json {
		payload := map[string]any{
			"exito":               true,
			"fichero":             ruta,
			"dominios_exportados": len(permitidos),
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Exportación completada: dominios=%d fichero=%s\n", len(permitidos), ruta)
	return 0
}

func (a *Adaptador) ejecutarDominiosLimpiar(ctx context.Context, cfg configCLI) int {
	if a.GestionDominios == nil {
		fmt.Fprintln(a.Stderr, "error: gestión de dominios no configurada")
		return 1
	}
	dominios, err := a.leerDominiosConfiados(ctx)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error leyendo dominios confiados:", err)
		return 1
	}
	permitidos := extraerDominiosPermitidos(dominios)
	eliminados := 0
	for _, origen := range permitidos {
		if _, err := a.GestionDominios.Execute(ctx, application.ManageTrustedDomainCommand{
			Origin: origen,
			Action: application.TrustActionRemove,
		}); err == nil {
			eliminados++
		}
	}
	if cfg.json {
		payload := map[string]any{
			"exito":      true,
			"eliminados": eliminados,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Limpieza completada: dominios eliminados=%d\n", eliminados)
	return a.ejecutarDominiosListar(ctx, cfg)
}

func (a *Adaptador) ejecutarImportarP12(ctx context.Context, cfg configCLI) int {
	ruta := strings.TrimSpace(cfg.ficheroP12)
	if ruta == "" {
		fmt.Fprintln(a.Stderr, "error: debe indicar -importar-p12 o -fichero-p12")
		return 1
	}
	data, err := a.LeerFichero(ruta)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error leyendo el fichero P12:", err)
		return 1
	}
	identidad, err := pkcs12importer.New().Import(ctx, data, cfg.contrasenaP12)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error validando el contenedor P12:", err)
		return 1
	}
	destinoDir := a.directorioP12()
	if err := os.MkdirAll(destinoDir, 0o700); err != nil {
		fmt.Fprintln(a.Stderr, "error creando el directorio P12:", err)
		return 1
	}
	destino := filepath.Join(destinoDir, nombreDestinoP12(ruta, identidad))
	if err := a.Escribir(destino, data, 0o600); err != nil {
		fmt.Fprintln(a.Stderr, "error guardando el P12 importado:", err)
		return 1
	}
	if cfg.json {
		payload := map[string]any{
			"exito":         true,
			"origen":        ruta,
			"destino":       destino,
			"certificado":   identidad.Subject,
			"huella_sha256": identidad.Fingerprint,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Importación P12 completada.\nOrigen: %s\nDestino: %s\nCertificado: %s\nHuella SHA-256: %s\n", ruta, destino, identidad.Subject, identidad.Fingerprint)
	return 0
}

func (a *Adaptador) ejecutarAutoseleccionLimpiar(_ context.Context, cfg configCLI) int {
	persistente := a.rutaAutoseleccionPersistente()
	sesion := a.rutaAutoseleccionSesion()
	eliminados := 0

	for _, ruta := range []string{persistente, sesion} {
		if err := os.Remove(ruta); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(a.Stderr, "error limpiando autoselección en %s: %v\n", ruta, err)
				return 1
			}
			continue
		}
		eliminados++
	}

	if cfg.json {
		payload := map[string]any{
			"exito":               true,
			"fichero_persistente": persistente,
			"fichero_sesion":      sesion,
			"eliminados":          eliminados,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(a.Stdout, "Autoselección de certificado limpiada.\nPersistente: %s\nSesión: %s\nFicheros eliminados: %d\n", persistente, sesion, eliminados)
	return 0
}

func (a *Adaptador) ejecutarAutoseleccionListar(_ context.Context, cfg configCLI) int {
	persistente := a.rutaAutoseleccionPersistente()
	sesion := a.rutaAutoseleccionSesion()

	preferenciasPersistentes, err := leerMapaAutoseleccion(persistente)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error leyendo autoselección persistente: %v\n", err)
		return 1
	}
	preferenciasSesion, err := leerMapaAutoseleccion(sesion)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error leyendo autoselección de sesión: %v\n", err)
		return 1
	}

	if origen := strings.TrimSpace(cfg.dominio); origen != "" {
		preferenciasPersistentes = filtrarMapaAutoseleccion(preferenciasPersistentes, origen)
		preferenciasSesion = filtrarMapaAutoseleccion(preferenciasSesion, origen)
	}

	if cfg.json {
		payload := map[string]any{
			"exito":               true,
			"dominio_filtrado":    strings.TrimSpace(cfg.dominio),
			"fichero_persistente": persistente,
			"fichero_sesion":      sesion,
			"persistente":         preferenciasPersistentes,
			"sesion":              preferenciasSesion,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(a.Stdout, "Autoselección de certificados.\nPersistente: %s\nSesión: %s\n", persistente, sesion)
	if origen := strings.TrimSpace(cfg.dominio); origen != "" {
		fmt.Fprintf(a.Stdout, "Filtro de origen: %s\n", origen)
	}
	escribirMapaAutoseleccionHumano(a.Stdout, "Persistente", preferenciasPersistentes)
	escribirMapaAutoseleccionHumano(a.Stdout, "Sesión", preferenciasSesion)
	return 0
}

func (a *Adaptador) ejecutarAutoseleccionEliminar(_ context.Context, cfg configCLI) int {
	origen := strings.TrimSpace(cfg.dominio)
	if origen == "" {
		fmt.Fprintln(a.Stderr, "error: debe indicar -dominio o -domain")
		return 1
	}

	persistente := a.rutaAutoseleccionPersistente()
	sesion := a.rutaAutoseleccionSesion()
	eliminadoPersistente, err := eliminarEntradaAutoseleccion(persistente, origen)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error eliminando autoselección persistente: %v\n", err)
		return 1
	}
	eliminadoSesion, err := eliminarEntradaAutoseleccion(sesion, origen)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error eliminando autoselección de sesión: %v\n", err)
		return 1
	}

	if cfg.json {
		payload := map[string]any{
			"exito":                 true,
			"dominio":               origen,
			"fichero_persistente":   persistente,
			"fichero_sesion":        sesion,
			"eliminado_persistente": eliminadoPersistente,
			"eliminado_sesion":      eliminadoSesion,
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(a.Stdout, "Autoselección eliminada para %s.\nPersistente: %t\nSesión: %t\n", origen, eliminadoPersistente, eliminadoSesion)
	return 0
}

func (a *Adaptador) ejecutarAutoseleccionResetear(ctx context.Context, cfg configCLI) int {
	if cfg.json {
		fmt.Fprintln(a.Stderr, "error: resetear-autoseleccion-cert no admite -salida-json; usa limpiar-autoseleccion-cert para automatización")
		return 1
	}

	persistente := a.rutaAutoseleccionPersistente()
	sesion := a.rutaAutoseleccionSesion()

	preferenciasPersistentes, err := leerMapaAutoseleccion(persistente)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error leyendo autoselección persistente: %v\n", err)
		return 1
	}
	preferenciasSesion, err := leerMapaAutoseleccion(sesion)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error leyendo autoselección de sesión: %v\n", err)
		return 1
	}

	totalPersistente := len(preferenciasPersistentes)
	totalSesion := len(preferenciasSesion)
	if totalPersistente == 0 && totalSesion == 0 {
		fmt.Fprintln(a.Stdout, "No hay autoselecciones guardadas.")
		return 0
	}

	fmt.Fprintf(a.Stdout, "Se van a borrar las autoselecciones guardadas.\nPersistente: %d\nSesión: %d\n", totalPersistente, totalSesion)
	fmt.Fprint(a.Stdout, "¿Continuar? [s/N]: ")
	if !a.confirmacionPositiva() {
		fmt.Fprintln(a.Stdout, "Operación cancelada.")
		return 0
	}

	return a.ejecutarAutoseleccionLimpiar(ctx, cfg)
}

type exportacionAutoseleccionCLI struct {
	Persistente map[string]string `json:"persistente"`
	Sesion      map[string]string `json:"sesion"`
}

func (a *Adaptador) ejecutarAutoseleccionExportar(_ context.Context, cfg configCLI) int {
	ruta := strings.TrimSpace(cfg.ficheroAutoseleccion)
	if ruta == "" {
		fmt.Fprintln(a.Stderr, "error: debe indicar -fichero-autoseleccion")
		return 1
	}

	persistente := a.rutaAutoseleccionPersistente()
	sesion := a.rutaAutoseleccionSesion()
	preferenciasPersistentes, err := leerMapaAutoseleccion(persistente)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error leyendo autoselección persistente: %v\n", err)
		return 1
	}
	preferenciasSesion, err := leerMapaAutoseleccion(sesion)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error leyendo autoselección de sesión: %v\n", err)
		return 1
	}

	exportacion := exportacionAutoseleccionCLI{
		Persistente: preferenciasPersistentes,
		Sesion:      preferenciasSesion,
	}
	data, err := json.MarshalIndent(exportacion, "", "  ")
	if err != nil {
		fmt.Fprintf(a.Stderr, "error serializando autoselección: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		fmt.Fprintf(a.Stderr, "error preparando destino: %v\n", err)
		return 1
	}
	if err := securefile.WriteFileAtomic(ruta, data, 0o600); err != nil {
		fmt.Fprintf(a.Stderr, "error escribiendo exportación: %v\n", err)
		return 1
	}

	if cfg.json {
		payload := map[string]any{
			"exito":               true,
			"fichero":             ruta,
			"fichero_persistente": persistente,
			"fichero_sesion":      sesion,
			"persistente":         len(preferenciasPersistentes),
			"sesion":              len(preferenciasSesion),
		}
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(a.Stdout, "Autoselección exportada.\nDestino: %s\nPersistente: %d entradas\nSesión: %d entradas\n", ruta, len(preferenciasPersistentes), len(preferenciasSesion))
	return 0
}

func (a *Adaptador) ejecutarAutoseleccionImportar(_ context.Context, cfg configCLI) int {
	ruta := strings.TrimSpace(cfg.ficheroAutoseleccion)
	if ruta == "" {
		fmt.Fprintln(a.Stderr, "error: debe indicar -fichero-autoseleccion")
		return 1
	}

	data, err := securefile.ReadFileLimit(ruta, 1024*1024)
	if err != nil {
		fmt.Fprintf(a.Stderr, "error leyendo importación: %v\n", err)
		return 1
	}

	var payload exportacionAutoseleccionCLI
	if err := json.Unmarshal(data, &payload); err != nil {
		fmt.Fprintf(a.Stderr, "error interpretando importación: %v\n", err)
		return 1
	}
	if payload.Persistente == nil {
		payload.Persistente = map[string]string{}
	}
	if payload.Sesion == nil {
		payload.Sesion = map[string]string{}
	}

	persistente := a.rutaAutoseleccionPersistente()
	sesion := a.rutaAutoseleccionSesion()
	if err := guardarMapaAutoseleccion(persistente, payload.Persistente); err != nil {
		fmt.Fprintf(a.Stderr, "error guardando autoselección persistente: %v\n", err)
		return 1
	}
	if err := guardarMapaAutoseleccion(sesion, payload.Sesion); err != nil {
		fmt.Fprintf(a.Stderr, "error guardando autoselección de sesión: %v\n", err)
		return 1
	}

	if cfg.json {
		respuesta := map[string]any{
			"exito":               true,
			"fichero":             ruta,
			"fichero_persistente": persistente,
			"fichero_sesion":      sesion,
			"persistente":         len(payload.Persistente),
			"sesion":              len(payload.Sesion),
		}
		if err := escribirJSON(a.Stdout, respuesta); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(a.Stdout, "Autoselección importada.\nOrigen: %s\nPersistente: %d entradas\nSesión: %d entradas\n", ruta, len(payload.Persistente), len(payload.Sesion))
	return 0
}

func (a *Adaptador) ejecutarTLSAlmacenEstado(_ context.Context, cfg configCLI) int {
	dir := a.directorioTLS()
	total, certificados, claves := contarArtefactosTLS(dir)
	payload := map[string]any{
		"exito":              true,
		"directorio_tls":     dir,
		"artefactos_totales": total,
		"certificados":       certificados,
		"claves":             claves,
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Almacén TLS local: %s\n", dir)
	fmt.Fprintf(a.Stdout, "Artefactos: total=%d certificados=%d claves=%d\n", total, certificados, claves)
	return 0
}

func (a *Adaptador) ejecutarTLSAlmacenLimpiar(ctx context.Context, cfg configCLI) int {
	dir := a.directorioTLS()
	eliminados, err := resttls.ClearManagedLocalhostTLS(ctx, dir)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error retirando confianza y artefactos TLS gestionados:", err)
		return 1
	}
	payload := map[string]any{
		"exito":      true,
		"directorio": dir,
		"eliminados": eliminados,
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Almacén TLS local limpiado.\nElementos eliminados=%d\nDirectorio=%s\n", eliminados, dir)
	return 0
}

func (a *Adaptador) ejecutarTLSGenerarCerts(_ context.Context, cfg configCLI) int {
	certFile, keyFile, rootFile, _, err := resttls.EnsureBrowserCompatibleLocalhostCertificate(a.directorioTLS(), resttls.ManagedLocalhostPrefix)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error generando certificados TLS:", err)
		return 1
	}
	payload := map[string]any{
		"exito":           true,
		"certificado_tls": certFile,
		"clave_tls":       keyFile,
		"ca_tls":          rootFile,
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Certificados TLS locales listos.\nCertificado=%s\nClave=%s\nCA=%s\n", certFile, keyFile, rootFile)
	return 0
}

func (a *Adaptador) ejecutarTLSInstalarConfianza(ctx context.Context, cfg configCLI) int {
	_, _, certFile, _, err := resttls.EnsureBrowserCompatibleLocalhostCertificate(a.directorioTLS(), resttls.ManagedLocalhostPrefix)
	if err != nil {
		fmt.Fprintln(a.Stderr, "error generando el certificado TLS local:", err)
		return 1
	}
	err = a.instalarConfianzaTLS(ctx, certFile)
	if err != nil && !errors.Is(err, localtlstrust.ErrSoporteNoDisponible) && !errors.Is(err, localtlstrust.ErrHerramientaNoDisponible) {
		fmt.Fprintln(a.Stderr, "error instalando la confianza TLS:", err)
		return 1
	}
	estado, detalle := "instalada", ""
	if errors.Is(err, localtlstrust.ErrSoporteNoDisponible) {
		estado = "soporte_no_disponible"
		detalle = err.Error()
	}
	if errors.Is(err, localtlstrust.ErrHerramientaNoDisponible) {
		estado = "herramienta_no_disponible"
		detalle = err.Error()
	}
	payload := map[string]any{
		"exito":           err == nil,
		"estado":          estado,
		"detalle":         detalle,
		"certificado_tls": certFile,
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Instalación de confianza TLS: %s\nCertificado=%s\n", estado, certFile)
	if detalle != "" {
		fmt.Fprintf(a.Stdout, "Detalle: %s\n", detalle)
	}
	return 0
}

func (a *Adaptador) ejecutarTLSEstadoConfianza(_ context.Context, cfg configCLI) int {
	certFile := filepath.Join(a.directorioTLS(), resttls.ManagedLocalhostPrefix+"-root.crt.pem")
	_, err := os.Stat(certFile)
	estado := "no_generado"
	if err == nil {
		estado = "certificado_generado"
	}
	payload := map[string]any{
		"exito":           true,
		"estado":          estado,
		"certificado_tls": certFile,
		"detalle":         "la verificación detallada del almacén del sistema depende de la plataforma; use instalar-confianza-tls para instalar o reinstalar la confianza",
	}
	if cfg.json {
		if err := escribirJSON(a.Stdout, payload); err != nil {
			fmt.Fprintln(a.Stderr, "error escribiendo JSON:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.Stdout, "Estado de confianza TLS: %s\nCertificado=%s\n", estado, certFile)
	fmt.Fprintln(a.Stdout, "La verificación detallada del almacén del sistema depende de la plataforma; use instalar-confianza-tls para instalar o reinstalar la confianza.")
	return 0
}

func (a *Adaptador) listarCertificados(ctx context.Context) ([]certificadoCLI, error) {
	if a.Catalogo == nil {
		return nil, errors.New("catálogo de certificados no configurado")
	}
	refs, err := a.Catalogo.List(ctx)
	if err != nil {
		return nil, err
	}
	resultado := make([]certificadoCLI, 0, len(refs))
	now := time.Now()
	for i, ref := range refs {
		info := certificadoCLI{
			Indice:      i,
			Ref:         ref,
			PuedeFirmar: ref.HasSigningKey,
		}
		switch {
		case !ref.HasSigningKey:
			info.Problema = "el catálogo no encontró una clave privada asociada"
		case ref.NotAfter.IsZero():
			info.PuedeFirmar = false
			info.Problema = "el certificado no informa de su vigencia"
		case ref.IsExpired(now):
			info.PuedeFirmar = false
			info.Problema = "el certificado está caducado"
		}
		resultado = append(resultado, info)
	}
	return resultado, nil
}

func (a *Adaptador) resolverCertificadoFirma(ctx context.Context, cfg configCLI) (string, domain.CertificateRef, error) {
	id := strings.TrimSpace(cfg.certificado)
	if id != "" && a.Catalogo == nil {
		return id, domain.CertificateRef{ID: id}, nil
	}
	if a.Catalogo == nil {
		if id == "" && cfg.certIndex < 0 && strings.TrimSpace(cfg.certContains) == "" {
			return "", domain.CertificateRef{}, nil
		}
		return "", domain.CertificateRef{}, errors.New("catálogo de certificados no configurado")
	}

	certs, err := a.listarCertificados(ctx)
	if err != nil {
		return "", domain.CertificateRef{}, err
	}
	if len(certs) == 0 {
		return "", domain.CertificateRef{}, errors.New("no hay certificados disponibles")
	}

	if id != "" {
		for _, c := range certs {
			if c.Ref.ID == id {
				if !c.PuedeFirmar {
					return "", domain.CertificateRef{}, fmt.Errorf("el certificado seleccionado no puede firmar: %s", c.Problema)
				}
				return c.Ref.ID, c.Ref, nil
			}
		}
		return "", domain.CertificateRef{}, fmt.Errorf("no se encontró certificado con id %s", id)
	}

	if cfg.certIndex >= 0 {
		if cfg.certIndex >= len(certs) {
			return "", domain.CertificateRef{}, fmt.Errorf("indice de certificado fuera de rango: %d (máximo %d)", cfg.certIndex, len(certs)-1)
		}
		c := certs[cfg.certIndex]
		if !c.PuedeFirmar {
			return "", domain.CertificateRef{}, fmt.Errorf("el certificado seleccionado no puede firmar: %s", c.Problema)
		}
		return c.Ref.ID, c.Ref, nil
	}

	contains := strings.ToLower(strings.TrimSpace(cfg.certContains))
	if contains != "" {
		for _, c := range certs {
			campos := []string{
				strings.ToLower(mejorNombreCertificado(c.Ref)),
				strings.ToLower(c.Ref.Subject),
				strings.ToLower(c.Ref.Issuer),
				strings.ToLower(c.Ref.Fingerprint),
			}
			if contieneAlguno(campos, contains) {
				if !c.PuedeFirmar {
					return "", domain.CertificateRef{}, fmt.Errorf("el certificado seleccionado no puede firmar: %s", c.Problema)
				}
				return c.Ref.ID, c.Ref, nil
			}
		}
		return "", domain.CertificateRef{}, fmt.Errorf("no se encontró certificado que contenga: %s", contains)
	}

	for _, c := range certs {
		if c.PuedeFirmar {
			return c.Ref.ID, c.Ref, nil
		}
	}
	return "", domain.CertificateRef{}, errors.New("ningún certificado disponible puede firmar")
}

func (a *Adaptador) leerDominiosConfiados(_ context.Context) ([]domain.TrustDecision, error) {
	configDir := strings.TrimSpace(a.ConfigDir)
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		configDir = appdirs.Config(home)
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, err
	}
	policy, err := config.LoadPolicy(config.DirPolicyDefecto)
	if err != nil {
		return nil, err
	}
	gestor, err := truststore.NewWithOptions(configDir, truststore.Options{
		SystemAllowlistFile: truststore.SystemAllowlistPath,
		Headless:            false,
		TOFUEnabled:         cfg.TofuHabilitado,
		ExtraAllowed:        policy.DominiosDeConfianza,
	})
	if err != nil {
		return nil, err
	}
	return gestor.DecisionsSnapshot(), nil
}

func (a *Adaptador) debePreguntarImportacionInicial(cfg configCLI) bool {
	if cfg.json || a.GestionDominios == nil {
		return false
	}
	in, ok := a.Stdin.(*os.File)
	if !ok {
		return false
	}
	out, ok := a.Stdout.(*os.File)
	if !ok {
		return false
	}
	inInfo, err := in.Stat()
	if err != nil {
		return false
	}
	outInfo, err := out.Stat()
	if err != nil {
		return false
	}
	return (inInfo.Mode()&os.ModeCharDevice) != 0 && (outInfo.Mode()&os.ModeCharDevice) != 0
}

func (a *Adaptador) preguntarImportacionInicial() bool {
	fmt.Fprint(a.Stdout, "No hay dominios confiados. ¿Quieres importar los dominios públicos iniciales? [s/N]: ")
	return a.confirmacionPositiva()
}

func (a *Adaptador) confirmacionPositiva() bool {
	lector := bufio.NewReader(a.Stdin)
	linea, err := lector.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	respuesta := strings.ToLower(strings.TrimSpace(linea))
	return respuesta == "s" || respuesta == "si" || respuesta == "sí" || respuesta == "y" || respuesta == "yes"
}

func (a *Adaptador) importarDominiosIniciales(ctx context.Context) (int, error) {
	if a.GestionDominios == nil {
		return 0, fmt.Errorf("gestión de dominios no configurada")
	}
	importados := 0
	for _, origin := range truststore.DefaultTrustedOrigins() {
		if _, err := a.GestionDominios.Execute(ctx, application.ManageTrustedDomainCommand{
			Origin: origin,
			Action: application.TrustActionAllow,
		}); err != nil {
			return importados, err
		}
		importados++
	}
	return importados, nil
}

func (a *Adaptador) escribirAyuda() {
	var b strings.Builder
	b.WriteString(a.t("cli.help.title", "Modo CLI de grxfirma"))
	b.WriteString("\n\n")
	b.WriteString(a.t("cli.help.summary", "Resumen de opciones:"))
	b.WriteString("\n")
	b.WriteString("  -ayuda | -a | -h | -ayuda-cli\n")
	b.WriteString("    Muestra esta ayuda específica del modo CLI.\n")
	b.WriteString("  -listar-certificados | -ll  [-comprobar-certificados | -cc] [-salida-json]\n")
	b.WriteString("    Lista certificados disponibles y comprueba si pueden firmar.\n")
	b.WriteString("  -operacion firmar|cofirmar|contrafirmar|verificar|proteger|proteger-firmando|desproteger|listar-destinatarios-proteccion|exportar-destinatario-proteccion|importar-destinatario-proteccion\n")
	b.WriteString("    Operación principal. Los nombres en inglés siguen funcionando como alias.\n")
	b.WriteString("    También puedes usar directamente: -firmar, -cofirmar, -contrafirmar, -verificar, -proteger, -proteger-firmando, -desproteger, -listar-destinatarios-proteccion,\n")
	b.WriteString("    -exportar-destinatario-proteccion e -importar-destinatario-proteccion.\n")
	b.WriteString("  -entrada <fichero> | -in <fichero> | -e <fichero>\n")
	b.WriteString("    Documento de entrada.\n")
	b.WriteString("  -salida <fichero> | -out <fichero> | -s <fichero>\n")
	b.WriteString("    Ruta de salida para la firma. En comprobar-hash de directorios puede guardar un informe .hashreport.\n")
	b.WriteString("  -documento-original <fichero> | -original <fichero>\n")
	b.WriteString("    Documento original para verificar firmas desacopladas.\n")
	b.WriteString("  -id-certificado <id> | -indice-certificado <n> | -certificado-contiene <texto>\n")
	b.WriteString("    Selección de certificado. Los alias ingleses también funcionan.\n")
	b.WriteString("  -formato " + SignatureFormatsHelp + " | -format ... | -f ...\n")
	b.WriteString("    Formato de firma/verificación.\n")
	b.WriteString("  -salida-json | -json | -j\n")
	b.WriteString("    Salida estructurada JSON.\n")
	b.WriteString("  -no-guardar | -no-save\n")
	b.WriteString("    No guarda el resultado firmado en disco.\n")
	b.WriteString("  -imprimir-firma | -print-signature\n")
	b.WriteString("    Imprime la firma en Base64.\n")
	b.WriteString("  -sobrescribir error|renombrar|forzar | -overwrite ...\n")
	b.WriteString("    Política al existir salida.\n")
	b.WriteString("  -perfil-proteccion compat|alto | -protection-profile ...\n")
	b.WriteString("    " + a.t(
		"Perfil de protección/cifrado. compat exige una identidad RSA cuya clave privada pueda desproteger, por ejemplo un P12/PFX autorizado.",
		"Perfil de protección/cifrado. compat exige una identidad RSA cuya clave privada pueda desproteger, por ejemplo un P12/PFX autorizado.",
	) + "\n")
	b.WriteString("    " + a.t(
		"Los certificados opacos del almacén del sistema siguen disponibles para firmar, pero no se anuncian para cifrado compat; carga un P12/PFX apto o usa alto.",
		"Los certificados opacos del almacén del sistema siguen disponibles para firmar, pero no se anuncian para cifrado compat; carga un P12/PFX apto o usa alto.",
	) + "\n")
	b.WriteString("  -contenedor-proteccion json|cms | -protection-container ...\n")
	b.WriteString("    Contenedor de salida para protección. json mantiene el sobre .afp nativo; cms genera un CMS interoperable EnvelopedData (.enveloped); cms-encrypted genera CMS EncryptedData (.encrypted.p7m).\n")
	b.WriteString("    En proteger-firmando solo se admite SignedAndEnvelopedData y sus aliases legacy; AuthEnvelopedData, EnvelopedData y el resto de modos CMS incompatibles se rechazan con error claro.\n")
	b.WriteString("  -clave-proteccion-stdin | -protection-secret-stdin\n")
	b.WriteString("    Lee por stdin sin eco una clave Base64 canónica de 32 bytes; nunca se persiste.\n")
	b.WriteString("  -operacion crear-hash|comprobar-hash\n")
	b.WriteString("    Utilidades de integridad tipo createdigest/checkdigest de AutoFirma 1.9.\n")
	b.WriteString("    Crear requiere -entrada y opcionalmente -algoritmo-hash/-formato-hash/-salida.\n")
	b.WriteString("    Comprobar requiere -entrada y -fichero-hash. Si -entrada es un directorio, -salida guarda un informe .hashreport.\n")
	b.WriteString("    También puedes usar directamente: -crear-hash, -createdigest, -comprobar-hash y -checkdigest.\n")
	b.WriteString("  " + a.localizadorENI().T("eni.validacion.help") + "\n")
	b.WriteString("  " + a.t("cli.help.eni_usage", "-operacion generar-eni -entrada <firma> [-original <documento>] -opcion eni.organo=<DIR3> -opcion eni.origen=ciudadano|administracion") + "\n")
	b.WriteString("    " + a.t("cli.help.eni", "Genera un documento electrónico ENI (NTI de Documento Electrónico) con el contenido, los metadatos obligatorios y la firma PAdES, CAdES o XAdES. Opciones: eni.tipoDocumental (TD99 por defecto), eni.estado (EE01 por defecto), eni.identificador, eni.documentoOrigen, eni.fechaCaptura y eni.formato.") + "\n")
	b.WriteString("  " + a.t("cli.help.eni_file_usage", "-operacion generar-expediente -lote <carpeta> -opcion exp.organo=<DIR3> -opcion exp.clasificacion=<SIA>") + "\n")
	b.WriteString("    " + a.t("cli.help.eni_file", "Genera un expediente electrónico ENI con los documentos ENI de la carpeta (en orden alfabético) y firma su índice con el certificado elegido. Opciones: exp.estado (E01 abierto por defecto, E02 cerrado, E03 índice para remisión cerrado), exp.interesado, exp.identificador y exp.fechaApertura.") + "\n")
	b.WriteString("  " + a.t("cli.help.audit_usage", "-operacion verificar-auditoria [-entrada <audit.jsonl>]") + "\n")
	b.WriteString("    " + a.t("cli.help.audit", "Comprueba la cadena de huellas del registro de auditoría: detecta registros alterados, insertados o borrados. Para acreditar también que no se ha recortado el final, firma el fichero con tu certificado al entregarlo.") + "\n")
	b.WriteString("  " + a.t("cli.help.report_usage", "-informe <informe.html>") + "\n")
	b.WriteString("    " + a.t("cli.help.report", "Con la verificación, guarda un informe de validación en HTML imprimible con el veredicto, las comprobaciones, los firmantes y la huella SHA-256 del fichero verificado.") + "\n")
	b.WriteString("  " + a.t("cli.help.pdf_password_usage", "-contrasena-pdf-stdin | -pdf-password-stdin") + "\n")
	b.WriteString("    " + a.t("cli.help.pdf_password", "Lee sin eco la contraseña de un PDF cifrado (de usuario o de propietario) para firmarlo conservando su cifrado. También se admite la variable GRXFIRMA_PDF_PASSWORD, que se borra al leerla; nunca se acepta como argumento.") + "\n")
	b.WriteString("  " + a.t("cli.help.batch_usage", "-lote <manifiesto.json|carpeta>") + "\n")
	b.WriteString("    " + a.t("cli.help.batch", "Firma varios documentos con un único certificado. Con una carpeta firma sus ficheros (máximo 128, sin subcarpetas ni ocultos) y guarda cada firma junto al original o en -salida; los fallos se indican por documento.") + "\n")
	b.WriteString("  " + a.localizadorENI().T("verifactu.cli_usage") + "\n")
	b.WriteString("    " + a.localizadorENI().T("verifactu.scope") + "\n")
	b.WriteString("    " + a.localizadorENI().T("verifactu.cli_qr_input") + "\n")
	b.WriteString("  " + a.t("cli.help.facturae_check_usage", "-operacion validar-factura -entrada ...") + "\n")
	b.WriteString("    " + a.t("cli.help.facturae_check", "Revisa una factura FacturaE, UBL o CII: totales (reglas EN 16931 en UBL y CII), impuestos, NIF/NIE/CIF y centros DIR3 que exige FACe.") + "\n")
	b.WriteString("    " + a.t("cli.help.facturae_check_exit", "Devuelve 0 si no hay errores y 1 si FACe o el receptor rechazarían la factura.") + "\n")
	b.WriteString("  -algoritmo-hash sha1|sha256|sha384|sha512 | -hash-algorithm ...\n")
	b.WriteString("    Algoritmo de huella. Por defecto SHA-256.\n")
	b.WriteString("  -formato-hash hex|base64|bin|xml|txt|csv | -hash-format ...\n")
	b.WriteString("    Formato de salida o del fichero de huella: .hexhash, .hashb64, .hash o manifiestos de directorio .hashfiles/.txthashfiles/.csv.\n")
	b.WriteString("  -fichero-hash <fichero> | -hash-file <fichero>\n")
	b.WriteString("    Ruta del fichero de huella para comprobar la integridad.\n")
	b.WriteString("  -recursive | -recursivo | -r\n")
	b.WriteString("    Al crear o comprobar hashes de directorio, incluye subdirectorios.\n")
	b.WriteString("  -destinatario <id> | -recipient <id>\n")
	b.WriteString("    Destinatario de protección. Repite la bandera para varios destinatarios.\n")
	b.WriteString("  -sello-visible ... | -visible-seal ...\n")
	b.WriteString("    Activa el sello visible PAdES.\n")
	b.WriteString("  -sello-pagina <n> | -seal-page <n>\n")
	b.WriteString("    Página única donde colocar el sello.\n")
	b.WriteString("  -sello-paginas <seleccion> | -seal-pages <seleccion>\n")
	b.WriteString("    Selección avanzada de páginas del sello: all, 1 o 1,3-5. Tiene prioridad sobre -sello-pagina.\n")
	b.WriteString("  -sello-qr <texto|url> | -seal-qr <texto|url>\n")
	b.WriteString("    Inserta un QR opcional dentro del sello visible cuando no se usa imagen personalizada.\n")
	b.WriteString("  -motivo-firma <texto> | -signature-reason <texto>\n")
	b.WriteString("    Motivo PAdES opcional embebido en la firma.\n")
	b.WriteString("  -ubicacion-firma <texto> | -signature-location <texto>\n")
	b.WriteString("    Ubicación PAdES opcional embebida en la firma.\n")
	b.WriteString("  -idioma-sello <es|en|ca|va|gl|eu|fr|de|it|pt|zh> | -seal-language <es|en|...>\n")
	b.WriteString("    " + a.t("cli.help.seal_language", "Idioma de los rótulos del sello visible. Por defecto, el idioma de la CLI.") + "\n")
	b.WriteString("  -contacto-firma <texto> | -signature-contact <texto>\n")
	b.WriteString("    Contacto PAdES opcional embebido en la firma.\n")
	b.WriteString("  -tiempo-espera 30s|1m|... | -t 30s\n")
	b.WriteString("    Tiempo máximo de ejecución del comando.\n")
	b.WriteString("  -operacion listar-dominios|añadir-dominio|eliminar-dominio|importar-dominios|exportar-dominios|limpiar-dominios\n")
	b.WriteString("    Gestión del círculo de confianza de dominios. Usa -dominio/-domain y -fichero-dominios/-domain-file.\n")
	b.WriteString("    Añadir un dominio lo autoriza para operaciones remotas y ayuda a no firmar en dominios no confiables o maliciosos.\n")
	b.WriteString("    En el primer arranque se cargan dominios públicos habituales (Gobierno, Junta y otras AAPP) como semilla editable.\n")
	b.WriteString("    Exportar dominios guarda ese círculo de confianza para reutilizarlo en otra instalación.\n")
	b.WriteString("    Limpiar dominios elimina todos los dominios permitidos del círculo de confianza actual.\n")
	b.WriteString("    También puedes usar directamente: -listar-dominios, -añadir-dominio, -eliminar-dominio, -importar-dominios, -exportar-dominios y -limpiar-dominios.\n")
	b.WriteString("    Alias compatibles no visibles: -anadir-dominio y -borrar-dominios.\n")
	b.WriteString("  -operacion importar-p12\n")
	b.WriteString("    Valida un contenedor PKCS#12 y lo copia al directorio local de certificados P12 del perfil.\n")
	b.WriteString("    También puedes usar directamente: -importar-p12 <ruta> [-contrasena-p12-stdin].\n")
	b.WriteString("  -operacion listar-destinatarios-proteccion\n")
	b.WriteString("    Lista destinatarios locales disponibles para proteger/cifrar en perfiles compat y alto.\n")
	b.WriteString("    " + a.t(
		"La lista falla cerrado: omite identidades de firma o hardware que no demuestren capacidad real de desprotección RSA.",
		"La lista falla cerrado: omite identidades de firma o hardware que no demuestren capacidad real de desprotección RSA.",
	) + "\n")
	b.WriteString("    También puedes usar directamente: -listar-destinatarios-proteccion.\n")
	b.WriteString("  -operacion exportar-destinatario-proteccion\n")
	b.WriteString("    Exporta el material público de un destinatario fuerte. Requiere -destinatario <id> y opcionalmente -salida.\n")
	b.WriteString("    También puedes usar directamente: -exportar-destinatario-proteccion -destinatario <id>.\n")
	b.WriteString("  -operacion importar-destinatario-proteccion\n")
	b.WriteString("    Importa un destinatario fuerte previamente exportado. Requiere -entrada <fichero>.\n")
	b.WriteString("    También puedes usar directamente: -importar-destinatario-proteccion -entrada /tmp/destinatario.afpr.json.\n")
	b.WriteString("  -operacion listar-autoseleccion-cert\n")
	b.WriteString("    Lista la autoselección guardada del certificado en persistencia y sesión.\n")
	b.WriteString("    Puedes filtrar por origen con -dominio https://portal.ejemplo.\n")
	b.WriteString("    También puedes usar directamente: -listar-autoseleccion-cert.\n")
	b.WriteString("  -operacion eliminar-autoseleccion-cert\n")
	b.WriteString("    Elimina la autoselección guardada para un origen concreto. Requiere -dominio.\n")
	b.WriteString("    También puedes usar directamente: -eliminar-autoseleccion-cert -dominio https://portal.ejemplo.\n")
	b.WriteString("  -operacion resetear-autoseleccion-cert\n")
	b.WriteString("    Pide confirmación y elimina toda la autoselección guardada.\n")
	b.WriteString("    También puedes usar directamente: -resetear-autoseleccion-cert.\n")
	b.WriteString("  -operacion exportar-autoseleccion-cert\n")
	b.WriteString("    Exporta a JSON la autoselección guardada. Requiere -fichero-autoseleccion.\n")
	b.WriteString("    También puedes usar directamente: -exportar-autoseleccion-cert -fichero-autoseleccion /tmp/autoseleccion.json.\n")
	b.WriteString("  -operacion importar-autoseleccion-cert\n")
	b.WriteString("    Importa desde JSON y reemplaza la autoselección persistente y de sesión. Requiere -fichero-autoseleccion.\n")
	b.WriteString("    También puedes usar directamente: -importar-autoseleccion-cert -fichero-autoseleccion /tmp/autoseleccion.json.\n")
	b.WriteString("  -operacion limpiar-autoseleccion-cert\n")
	b.WriteString("    Borra la selección automática de certificado guardada para sesión y persistencia.\n")
	b.WriteString("    También puedes usar directamente: -limpiar-autoseleccion-cert.\n")
	b.WriteString("  -operacion informe-diagnostico\n")
	b.WriteString("    Resumen técnico del estado local.\n\n")
	b.WriteString("    También puedes usar directamente: -informe-diagnostico.\n\n")
	b.WriteString("  -operacion estado-almacen-tls|limpiar-almacen-tls|estado-confianza-tls|generar-certificados-tls|instalar-confianza-tls\n")
	b.WriteString("    Gestión local de certificados y confianza TLS para endpoints locales.\n\n")
	b.WriteString("    También puedes usar directamente: -estado-almacen-tls, -limpiar-almacen-tls, -estado-confianza-tls, -generar-certificados-tls e -instalar-confianza-tls.\n\n")
	b.WriteString(a.t("cli.help.examples", "Ejemplos:"))
	b.WriteString("\n")
	b.WriteString("  grxfirma -modo-cli -listar-certificados\n")
	b.WriteString("  grxfirma -modo-cli -operacion firmar -entrada /ruta/entrada.pdf -indice-certificado 0\n")
	b.WriteString("  grxfirma -modo-cli -operacion verificar -entrada /ruta/firmado.pdf -documento-original /ruta/original.pdf\n")
	b.WriteString("  grxfirma -modo-cli -operacion crear-hash -entrada /ruta/documento.pdf -algoritmo-hash sha256 -formato-hash hex\n")
	b.WriteString("  grxfirma -modo-cli -operacion comprobar-hash -entrada /ruta/documento.pdf -fichero-hash /ruta/documento.pdf.hexhash\n")
	b.WriteString("  grxfirma -modo-cli -operacion crear-hash -entrada /ruta/directorio -formato-hash xml -recursive\n")
	b.WriteString("  grxfirma -modo-cli -operacion comprobar-hash -entrada /ruta/directorio -fichero-hash /ruta/directorio.hashfiles -salida /ruta/directorio.hashreport\n")
	b.WriteString("  grxfirma -modo-cli -operacion listar-destinatarios-proteccion -salida-json\n")
	b.WriteString("  grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -destinatario cert-1 -perfil-proteccion compat\n")
	b.WriteString("  grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -destinatario cert-1 -perfil-proteccion compat -contenedor-proteccion cms\n")
	b.WriteString("  grxfirma -modo-cli -operacion proteger-firmando -entrada /ruta/secreto.pdf -destinatario cert-1 -id-certificado cert-firma-1 -perfil-proteccion compat\n")
	b.WriteString("  grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -perfil-proteccion compat -contenedor-proteccion cms-encrypted -clave-proteccion-stdin\n")
	b.WriteString("  grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.afp -salida /ruta/secreto.pdf\n")
	b.WriteString("  grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.encrypted.p7m -clave-proteccion-stdin\n")
	b.WriteString("  grxfirma -modo-cli -operacion exportar-destinatario-proteccion -destinatario <id_alto> -salida /tmp/destinatario.afpr.json\n")
	b.WriteString("  grxfirma -modo-cli -operacion importar-destinatario-proteccion -entrada /tmp/destinatario.afpr.json\n")
	b.WriteString("  grxfirma -modo-cli -operacion añadir-dominio -dominio https://firma.ejemplo.gob.es\n")
	b.WriteString("  grxfirma -modo-cli -operacion exportar-dominios -fichero-dominios /tmp/dominios.json\n")
	b.WriteString("  grxfirma -modo-cli -operacion limpiar-dominios\n")
	b.WriteString("  grxfirma -modo-cli -operacion listar-autoseleccion-cert\n")
	b.WriteString("  grxfirma -modo-cli -operacion listar-autoseleccion-cert -dominio https://portal.ejemplo -salida-json\n")
	b.WriteString("  grxfirma -modo-cli -operacion eliminar-autoseleccion-cert -dominio https://portal.ejemplo\n")
	b.WriteString("  grxfirma -modo-cli -operacion resetear-autoseleccion-cert\n")
	b.WriteString("  grxfirma -modo-cli -operacion exportar-autoseleccion-cert -fichero-autoseleccion /tmp/autoseleccion.json\n")
	b.WriteString("  grxfirma -modo-cli -operacion importar-autoseleccion-cert -fichero-autoseleccion /tmp/autoseleccion.json\n")
	b.WriteString("  grxfirma -modo-cli -operacion limpiar-autoseleccion-cert\n")
	b.WriteString("  grxfirma -importar-p12 /ruta/certificado.p12 -contrasena-p12-stdin\n")
	b.WriteString("  grxfirma -modo-cli -operacion generar-certificados-tls -salida-json\n")
	b.WriteString("  grxfirma -modo-cli -operacion instalar-confianza-tls -salida-json\n")
	_, _ = io.WriteString(a.Stdout, b.String())
}

func (a *Adaptador) t(id, fallback string, args ...any) string {
	if a.Localizador != nil {
		if traducido := a.Localizador.T(id, args...); traducido != "" && traducido != id {
			return traducido
		}
	}
	if len(args) == 0 {
		return fallback
	}
	return fmt.Sprintf(fallback, args...)
}

func (a *Adaptador) escribirErrorCLI(contexto string, err error) {
	if err == nil {
		fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), contexto)
		return
	}
	fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), contexto+":", err)
}

func (a *Adaptador) translateSecret(id string, args ...any) string {
	if a.Localizador == nil {
		return id
	}
	return a.Localizador.T(id, args...)
}

func traducirErrorCLI(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	switch {
	case strings.HasPrefix(lower, "flag provided but not defined: "):
		return "bandera no definida: " + strings.TrimSpace(msg[len("flag provided but not defined: "):])
	case strings.HasPrefix(lower, "flag needs an argument: "):
		return "la bandera requiere un valor: " + strings.TrimSpace(msg[len("flag needs an argument: "):])
	case strings.Contains(lower, "invalid value") && strings.Contains(lower, "for flag -"):
		return "valor invalido para una bandera: " + msg
	default:
		return msg
	}
}

func normalizarOperacion(op, accion, lote, entrada string) string {
	raw := strings.ToLower(strings.TrimSpace(op))
	switch raw {
	case "":
		if lote != "" || entrada != "" {
			return normalizarAccion(accion)
		}
		return ""
	case "firmar":
		return "sign"
	case "cofirmar":
		return "cosign"
	case "contrafirmar":
		return "countersign"
	case "verificar":
		return "verify"
	case "crear-hash":
		return "hash-create"
	case "comprobar-hash":
		return "hash-check"
	case "validar-factura", "revisar-factura", "facturae-check":
		return "facturae-check"
	case "validar-eni", "eni-check":
		return "eni-check"
	case "generar-eni", "eni-create", "documento-eni":
		return "eni-create"
	case "generar-expediente", "eni-file-create", "expediente-eni":
		return "eni-file-create"
	case "verificar-auditoria", "audit-verify":
		return "audit-verify"
	case "proteger":
		return "protect"
	case "proteger-firmando":
		return "protect-sign"
	case "desproteger":
		return "unprotect"
	case "listar-destinatarios-proteccion":
		return "protect-recipients-list"
	case "exportar-destinatario-proteccion":
		return "protect-recipient-export"
	case "importar-destinatario-proteccion":
		return "protect-recipient-import"
	case "informe-diagnostico":
		return "diagnostics-report"
	case "listar-dominios":
		return "domains-list"
	case "anadir-dominio", "añadir-dominio":
		return "domains-add"
	case "eliminar-dominio":
		return "domains-remove"
	case "importar-dominios":
		return "domains-import"
	case "exportar-dominios":
		return "domains-export"
	case "limpiar-dominios":
		return "domains-clear"
	case "importar-p12":
		return "import-p12"
	case "listar-autoseleccion-cert", "ver-autoseleccion-cert":
		return "cert-selection-list"
	case "eliminar-autoseleccion-cert", "borrar-autoseleccion-portal":
		return "cert-selection-remove"
	case "resetear-autoseleccion-cert", "restablecer-autoseleccion-cert":
		return "cert-selection-reset"
	case "exportar-autoseleccion-cert":
		return "cert-selection-export"
	case "importar-autoseleccion-cert":
		return "cert-selection-import"
	case "limpiar-autoseleccion-cert", "borrar-autoseleccion-cert", "limpiar-seleccion-certificado":
		return "cert-selection-clear"
	case "estado-almacen-tls":
		return "tls-store-status"
	case "limpiar-almacen-tls":
		return "tls-store-clear"
	case "estado-confianza-tls":
		return "tls-trust-status"
	case "generar-certificados-tls":
		return "tls-generate-certs"
	case "instalar-confianza-tls":
		return "tls-install-trust"
	default:
		return raw
	}
}

func normalizarAccion(accion string) string {
	switch strings.ToLower(strings.TrimSpace(accion)) {
	case "", "sign", "firmar":
		return "sign"
	case "cosign", "cofirmar":
		return "cosign"
	case "countersign", "contrafirmar":
		return "countersign"
	default:
		return strings.ToLower(strings.TrimSpace(accion))
	}
}

func inferirFormatoFirma(formato, entrada string) string {
	raw := strings.ToLower(strings.TrimSpace(formato))
	switch raw {
	case "pades", "cades", "xades", "odf", "ooxml":
		return strings.ToUpper(raw[:1]) + raw[1:]
	case "xmldsig", "xmlsig", "xml-dsig":
		return "XMLdSig"
	case "verifactu":
		return "VeriFactu"
	case "facturae":
		return "FacturaE"
	case "asic-xades", "asicxades", "xades-asic", "xadesasics":
		return "ASiC-XAdES"
	case "auto":
	}
	switch strings.ToLower(filepath.Ext(entrada)) {
	case ".pdf":
		return "PAdES"
	case ".asics":
		return "ASiC-XAdES"
	case ".odt", ".ods", ".odp", ".odg", ".odf":
		return "ODF"
	case ".docx", ".xlsx", ".pptx", ".ppsx":
		return "OOXML"
	case ".dsig", ".xmlsig":
		return "XMLdSig"
	case ".xml", ".xsig":
		return "XAdES"
	default:
		return "CAdES"
	}
}

func construirRutaProtegidaPorDefecto(rutaEntrada, suggestedName string) string {
	if trimmed := strings.TrimSpace(suggestedName); trimmed != "" {
		return filepath.Join(filepath.Dir(rutaEntrada), filepath.Base(trimmed))
	}
	base := strings.TrimSpace(rutaEntrada)
	if base == "" {
		base = "documento"
	}
	return base + ".afp"
}

func construirRutaHashPorDefecto(rutaEntrada string, format application.HashOutputFormat) string {
	switch format {
	case application.HashFormatBinary:
		return rutaEntrada + ".hash"
	case application.HashFormatBase64:
		return rutaEntrada + ".hashb64"
	default:
		return rutaEntrada + ".hexhash"
	}
}

func construirRutaHashDirectorioPorDefecto(rutaEntrada string, format domain.DirectoryHashManifestFormat) string {
	base := strings.TrimSuffix(rutaEntrada, string(filepath.Ext(rutaEntrada)))
	if strings.TrimSpace(base) == "" {
		base = rutaEntrada
	}
	switch format {
	case domain.DirectoryHashFormatTXT:
		return base + ".txthashfiles"
	case domain.DirectoryHashFormatCSV:
		return base + ".csv"
	default:
		return base + ".hashfiles"
	}
}

func construirRutaDesprotegidaPorDefecto(rutaEntrada, suggestedName string) string {
	if trimmed := strings.TrimSpace(suggestedName); trimmed != "" {
		return filepath.Join(filepath.Dir(rutaEntrada), filepath.Base(trimmed))
	}
	if strings.HasSuffix(strings.ToLower(rutaEntrada), ".afp") {
		return strings.TrimSuffix(rutaEntrada, filepath.Ext(rutaEntrada))
	}
	base := strings.TrimSuffix(rutaEntrada, filepath.Ext(rutaEntrada))
	if base == "" {
		base = rutaEntrada
	}
	return base + "_desprotegido"
}

func serializarHashCreado(resultado application.CreateHashResult) []byte {
	if resultado.Format == application.HashFormatBinary {
		return append([]byte(nil), resultado.Digest...)
	}
	return []byte(resultado.Encoded)
}

func parseDirectoryHashManifestFormat(raw string) (domain.DirectoryHashManifestFormat, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "xml", "hashfiles":
		return domain.DirectoryHashFormatXML, nil
	case "txt", "txthashfiles":
		return domain.DirectoryHashFormatTXT, nil
	case "csv":
		return domain.DirectoryHashFormatCSV, nil
	default:
		return "", errors.New("formato de manifiesto no soportado")
	}
}

func esDirectorioRuta(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func construirRutaDestinatarioProteccionPorDefecto(recipientID string) string {
	id := strings.TrimSpace(recipientID)
	if id == "" {
		id = "destinatario"
	}
	return id + ".afpr.json"
}

func mapsClone(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func construirOpcionesFirmaCompat(cfg configCLI, formato string) map[string]string {
	out := make(map[string]string, len(cfg.opciones)+12)
	for k, v := range cfg.opciones {
		out[k] = v
	}
	if cfg.permitirPDFInval {
		out["allowInvalidPDF"] = "true"
	}
	if cfg.compatEstrica {
		out["strictCompat"] = "true"
		if strings.EqualFold(formato, "PAdES") {
			out["subfilter"] = "adbe.pkcs7.detached"
		}
	}
	if cfg.selloVisible && strings.EqualFold(formato, "PAdES") {
		out["visibleSeal"] = "true"
		x := clamp01(cfg.selloX)
		y := clamp01(cfg.selloY)
		w := clamp01(cfg.selloW)
		h := clamp01(cfg.selloH)
		if strings.EqualFold(strings.TrimSpace(cfg.disposicionSello), "footer") {
			m := clamp01(cfg.margenSelloFooter)
			if m > 0.2 {
				m = 0.2
			}
			x = m
			y = m
			w = 1.0 - (2.0 * m)
			h = 0.10
		}
		out["visibleSealRectX"] = strconv.FormatFloat(x*595.28, 'f', -1, 64)
		out["visibleSealRectY"] = strconv.FormatFloat(y*841.89, 'f', -1, 64)
		out["visibleSealRectW"] = strconv.FormatFloat(w*595.28, 'f', -1, 64)
		out["visibleSealRectH"] = strconv.FormatFloat(h*841.89, 'f', -1, 64)
		if _, ok := out["page"]; !ok {
			if page, ok := normalizarSeleccionPaginasSello(cfg.selloPaginas); ok {
				out["page"] = page
			} else {
				page := cfg.selloPagina
				if page == 0 {
					page = 1
				}
				out["page"] = strconv.FormatUint(uint64(page), 10)
			}
		}
	}
	if strings.EqualFold(formato, "PAdES") {
		if qr := strings.TrimSpace(cfg.qrSello); qr != "" {
			out["qrContent"] = qr
		}
		if reason := strings.TrimSpace(cfg.motivoFirma); reason != "" {
			out["reason"] = reason
		}
		if location := strings.TrimSpace(cfg.ubicacionFirma); location != "" {
			out["location"] = location
		}
		if contact := strings.TrimSpace(cfg.contactoFirma); contact != "" {
			out["contactInfo"] = contact
		}
		if _, explicito := out["sealLanguage"]; !explicito {
			if idioma := strings.TrimSpace(cfg.idiomaSello); idioma != "" {
				out["sealLanguage"] = idioma
			}
		}
	}
	return out
}

// textoMotor traduce un literal del motor que el catálogo usa como clave;
// si no está catalogado (valores técnicos), lo devuelve tal cual.
func (a *Adaptador) textoMotor(literal string) string {
	if a.Localizador == nil || literal == "" {
		return literal
	}
	return a.Localizador.T(literal)
}

// idiomaInterfaz devuelve el idioma del catálogo de la CLI, o "" si el
// localizador inyectado no lo expone.
func (a *Adaptador) idiomaInterfaz() string {
	if l, ok := a.Localizador.(interface{ Locale() string }); ok {
		return l.Locale()
	}
	return ""
}

func normalizarSeleccionPaginasSello(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	switch {
	case strings.EqualFold(value, "all"),
		strings.EqualFold(value, "todas"),
		strings.EqualFold(value, "todos"),
		value == "*":
		return "all", true
	default:
		return value, true
	}
}

func validarSeleccionPaginasSello(raw string) (string, error) {
	value, ok := normalizarSeleccionPaginasSello(raw)
	if !ok {
		return "", nil
	}
	if value == "all" {
		return value, nil
	}
	partes := strings.Split(value, ",")
	for _, parte := range partes {
		if !strings.Contains(parte, "-") {
			pagina, err := strconv.Atoi(parte)
			if err != nil || pagina < 1 {
				return "", fmt.Errorf("seleccion de paginas invalida: use 1, 1,3-5 o all")
			}
			continue
		}
		limites := strings.SplitN(parte, "-", 2)
		if len(limites) != 2 {
			return "", fmt.Errorf("seleccion de paginas invalida: use 1, 1,3-5 o all")
		}
		inicio, errInicio := strconv.Atoi(limites[0])
		fin, errFin := strconv.Atoi(limites[1])
		if errInicio != nil || errFin != nil || inicio < 1 || fin < 1 || inicio > fin {
			return "", fmt.Errorf("seleccion de paginas invalida: use 1, 1,3-5 o all")
		}
	}
	return value, nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func mejorNombreCertificado(ref domain.CertificateRef) string {
	if strings.TrimSpace(ref.Subject) != "" {
		return ref.Subject
	}
	if strings.TrimSpace(ref.Fingerprint) != "" {
		return ref.Fingerprint
	}
	return ref.ID
}

func contieneAlguno(campos []string, needle string) bool {
	for _, campo := range campos {
		if strings.Contains(campo, needle) {
			return true
		}
	}
	return false
}

func accionCastellano(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "sign", "firmar":
		return "firmar"
	case "cosign", "cofirmar":
		return "cofirmar"
	case "countersign", "contrafirmar":
		return "contrafirmar"
	case "verify", "verificar":
		return "verificar"
	case "hash-create", "crear-hash", "createdigest":
		return "crear-hash"
	case "hash-check", "comprobar-hash", "checkdigest":
		return "comprobar-hash"
	case "facturae-check", "validar-factura", "revisar-factura":
		return "validar-factura"
	case "eni-check", "validar-eni":
		return "validar-eni"
	case "eni-create", "generar-eni", "documento-eni":
		return "generar-eni"
	case "eni-file-create", "generar-expediente", "expediente-eni":
		return "generar-expediente"
	case "audit-verify", "verificar-auditoria":
		return "verificar-auditoria"
	case "diagnostics-report", "informe-diagnostico":
		return "informe-diagnostico"
	case "domains-list", "listar-dominios":
		return "listar-dominios"
	case "domains-add", "anadir-dominio", "añadir-dominio":
		return "anadir-dominio"
	case "domains-remove", "eliminar-dominio":
		return "eliminar-dominio"
	case "domains-import", "importar-dominios":
		return "importar-dominios"
	case "domains-export", "exportar-dominios":
		return "exportar-dominios"
	case "domains-clear", "limpiar-dominios", "borrar-dominios":
		return "limpiar-dominios"
	case "cert-selection-list", "listar-autoseleccion-cert", "ver-autoseleccion-cert":
		return "listar-autoseleccion-cert"
	case "cert-selection-remove", "eliminar-autoseleccion-cert", "borrar-autoseleccion-portal":
		return "eliminar-autoseleccion-cert"
	case "cert-selection-reset", "resetear-autoseleccion-cert", "restablecer-autoseleccion-cert":
		return "resetear-autoseleccion-cert"
	case "cert-selection-export", "exportar-autoseleccion-cert":
		return "exportar-autoseleccion-cert"
	case "cert-selection-import", "importar-autoseleccion-cert":
		return "importar-autoseleccion-cert"
	case "cert-selection-clear", "limpiar-autoseleccion-cert", "borrar-autoseleccion-cert", "limpiar-seleccion-certificado":
		return "limpiar-autoseleccion-cert"
	case "tls-store-status", "estado-almacen-tls":
		return "estado-almacen-tls"
	case "tls-store-clear", "limpiar-almacen-tls":
		return "limpiar-almacen-tls"
	case "tls-trust-status", "estado-confianza-tls":
		return "estado-confianza-tls"
	case "tls-generate-certs", "generar-certificados-tls":
		return "generar-certificados-tls"
	case "tls-install-trust", "instalar-confianza-tls":
		return "instalar-confianza-tls"
	default:
		return strings.TrimSpace(raw)
	}
}

func estadoConfianzaCastellano(status domain.TrustStatus) string {
	switch status {
	case domain.TrustAllowed:
		return "permitido"
	case domain.TrustDenied:
		return "denegado"
	case domain.TrustPending:
		return "pendiente"
	default:
		return string(status)
	}
}

func serializarDominiosConfiados(dominios []domain.TrustDecision) []map[string]string {
	out := make([]map[string]string, 0, len(dominios))
	for _, d := range dominios {
		out = append(out, map[string]string{
			"origen": d.Origin,
			"estado": estadoConfianzaCastellano(d.Status),
			"motivo": d.Reason,
		})
	}
	return out
}

func (a *Adaptador) directorioTLS() string {
	configDir := strings.TrimSpace(a.ConfigDir)
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "grxfirma", "tls")
		}
		configDir = appdirs.Config(home)
	}
	return filepath.Join(configDir, "tls")
}

func (a *Adaptador) directorioP12() string {
	configDir := strings.TrimSpace(a.ConfigDir)
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", "pkcs12")
		}
		configDir = appdirs.Config(home)
	}
	if cfg, err := config.Load(configDir); err == nil && strings.TrimSpace(cfg.DirectorioP12) != "" {
		return cfg.DirectorioP12
	}
	return filepath.Join(configDir, "pkcs12")
}

func (a *Adaptador) rutaAutoseleccionPersistente() string {
	configDir := strings.TrimSpace(a.ConfigDir)
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", "preferred-certificates.json")
		}
		configDir = appdirs.Config(home)
	}
	return filepath.Join(configDir, "preferred-certificates.json")
}

func (a *Adaptador) rutaAutoseleccionSesion() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); dir != "" {
		return filepath.Join(dir, "grxfirma", "preferred-certificates-session.json")
	}
	usuario := strings.TrimSpace(os.Getenv("USER"))
	if usuario == "" {
		usuario = "default"
	}
	return filepath.Join(os.TempDir(), "grxfirma-"+usuario, "preferred-certificates-session.json")
}

func leerMapaAutoseleccion(ruta string) (map[string]string, error) {
	data, err := securefile.ReadFileLimit(ruta, 1024*1024)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	raw := map[string]string{}
	if len(data) == 0 {
		return raw, nil
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func escribirMapaAutoseleccionHumano(w io.Writer, titulo string, datos map[string]string) {
	_, _ = fmt.Fprintf(w, "\n%s:\n", titulo)
	if len(datos) == 0 {
		_, _ = fmt.Fprintln(w, "  (sin entradas)")
		return
	}
	claves := make([]string, 0, len(datos))
	for origin := range datos {
		claves = append(claves, origin)
	}
	slices.Sort(claves)
	for _, origin := range claves {
		_, _ = fmt.Fprintf(w, "  %s -> %s\n", origin, strings.TrimSpace(datos[origin]))
	}
}

func filtrarMapaAutoseleccion(datos map[string]string, origen string) map[string]string {
	origen = strings.TrimSpace(origen)
	if origen == "" {
		return datos
	}
	valor, ok := datos[origen]
	if !ok || strings.TrimSpace(valor) == "" {
		return map[string]string{}
	}
	return map[string]string{
		origen: strings.TrimSpace(valor),
	}
}

func eliminarEntradaAutoseleccion(ruta, origen string) (bool, error) {
	datos, err := leerMapaAutoseleccion(ruta)
	if err != nil {
		return false, err
	}
	origen = strings.TrimSpace(origen)
	if origen == "" {
		return false, nil
	}
	if _, ok := datos[origen]; !ok {
		return false, nil
	}
	delete(datos, origen)
	if len(datos) == 0 {
		if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return true, nil
	}
	data, err := json.MarshalIndent(datos, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		return false, err
	}
	if err := securefile.WriteFileAtomic(ruta, data, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func guardarMapaAutoseleccion(ruta string, datos map[string]string) error {
	if len(datos) == 0 {
		if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(datos, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		return err
	}
	return securefile.WriteFileAtomic(ruta, data, 0o600)
}

func nombreDestinoP12(origen string, ref domain.CertificateRef) string {
	base := filepath.Base(strings.TrimSpace(origen))
	ext := strings.ToLower(filepath.Ext(base))
	if ext == ".p12" || ext == ".pfx" {
		return base
	}
	fp := strings.ToLower(strings.TrimSpace(ref.Fingerprint))
	if fp == "" {
		return "certificado-importado.p12"
	}
	return "certificado-" + fp + ".p12"
}

func contarArtefactosTLS(dir string) (total, certificados, claves int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), resttls.ManagedLocalhostPrefix) {
			continue
		}
		total++
		nombre := strings.ToLower(entry.Name())
		switch {
		case strings.HasSuffix(nombre, ".crt.pem") || strings.HasSuffix(nombre, ".cer") || strings.HasSuffix(nombre, ".crt"):
			certificados++
		case strings.HasSuffix(nombre, ".key.pem") || strings.HasSuffix(nombre, ".key"):
			claves++
		}
	}
	return total, certificados, claves
}

func escribirJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func construirRutaSalidaPorDefecto(entrada string, formato domain.SignatureFormat) string {
	base := strings.TrimSuffix(entrada, filepath.Ext(entrada))
	switch formato {
	case domain.FormatPAdES:
		return base + ".signed.pdf"
	case domain.FormatXAdES:
		return base + ".xsig"
	case domain.SignatureFormat("XMLdSig"):
		return base + ".dsig"
	case domain.SignatureFormat("VeriFactu"):
		return base + "-signed.xml"
	case domain.SignatureFormat("FacturaE"):
		return base + "_firmada.xml"
	case domain.SignatureFormat("ASiC-XAdES"):
		return base + ".asics"
	default:
		return base + ".csig"
	}
}

// guardarSalida guarda un resultado con la política de --overwrite, salvo que
// Escribir se haya sustituido por un doble de pruebas.
func (a *Adaptador) guardarSalida(destino, politica string, datos []byte) (string, bool, bool, error) {
	if !a.escrituraConPolitica {
		return destino, false, false, a.Escribir(destino, datos, 0o600)
	}
	return guardarSalidaCLI(destino, politica, datos)
}

// guardarSalidaCLI escribe datos aplicando la política de --overwrite en el
// mismo paso en que crea el fichero: con «rename» y «fail» la publicación es
// exclusiva, así que un fichero que aparezca mientras se firma tampoco se
// reemplaza. Devuelve la ruta real y si se renombró o se reemplazó.
func guardarSalidaCLI(destino, politica string, datos []byte) (ruta string, renamed bool, overwrote bool, err error) {
	var modo filesystem.PoliticaSobreescritura
	switch strings.ToLower(strings.TrimSpace(politica)) {
	case "force", "overwrite", "forzar":
		modo = filesystem.PoliticaForzar
	case "fail", "error":
		modo = filesystem.PoliticaFallar
	default:
		modo = filesystem.PoliticaRenombrar
	}
	_, statErr := os.Lstat(destino)
	existia := statErr == nil
	ruta, err = filesystem.NuevoEscritorResultado(modo).EscribirEnDirectorioExistente(destino, datos)
	if err != nil {
		return "", false, false, err
	}
	return ruta, ruta != destino, modo == filesystem.PoliticaForzar && existia, nil
}

func inferirTipoMIME(path string) string {
	switch filepath.Ext(path) {
	case ".pdf":
		return "application/pdf"
	case ".odt":
		return "application/vnd.oasis.opendocument.text"
	case ".ods":
		return "application/vnd.oasis.opendocument.spreadsheet"
	case ".odp":
		return "application/vnd.oasis.opendocument.presentation"
	case ".odg":
		return "application/vnd.oasis.opendocument.graphics"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".ppsx":
		return "application/vnd.openxmlformats-officedocument.presentationml.slideshow"
	case ".xml", ".xsig":
		return "application/xml"
	case ".json":
		return "application/json"
	case ".p7s", ".csig":
		return "application/pkcs7-signature"
	case ".enveloped", ".p7m":
		return domain.MIMETypeProtectedCMS
	case ".afp":
		return domain.MIMETypeProtectedEnvelope
	default:
		return "application/octet-stream"
	}
}

func clavesErrores(errores map[int]error) []int {
	indices := make([]int, 0, len(errores))
	for i := range errores {
		indices = append(indices, i)
	}
	slices.Sort(indices)
	return indices
}

func parsearFicheroDominios(data []byte) ([]string, error) {
	trim := strings.TrimSpace(string(data))
	if trim == "" {
		return nil, nil
	}
	if strings.HasPrefix(trim, "[") {
		var domains []string
		if err := json.Unmarshal([]byte(trim), &domains); err != nil {
			return nil, err
		}
		return limpiarDominios(domains), nil
	}
	lines := strings.Split(trim, "\n")
	domains := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		domains = append(domains, line)
	}
	return limpiarDominios(domains), nil
}

func limpiarDominios(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	slices.Sort(out)
	return out
}

func extraerDominiosPermitidos(in []domain.TrustDecision) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item.Status != domain.TrustAllowed {
			continue
		}
		out = append(out, item.Origin)
	}
	return limpiarDominios(out)
}

func serializarFicheroDominios(ruta string, dominios []string) ([]byte, error) {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(ruta)), ".json") {
		return json.MarshalIndent(dominios, "", "  ")
	}
	if len(dominios) == 0 {
		return []byte{}, nil
	}
	return []byte(strings.Join(dominios, "\n") + "\n"), nil
}

// ejecutarValidarFactura revisa el contenido de una FacturaE sin firmarla.
func (a *Adaptador) ejecutarValidarFactura(cfg configCLI) int {
	if strings.TrimSpace(cfg.entrada) == "" {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), errors.New(a.t("Indica la factura con -entrada.", "Indica la factura con -entrada.")))
		return 1
	}
	data, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	if err := a.Limits.CheckPayload(data); err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	formato, incidencias, err := commonsigner.RevisarFactura(data)
	if err != nil {
		a.escribirErrorCLI(a.t("Factura", "Factura"), err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "%s: %s\n", a.t("Formato", "Formato"), formato)
	if len(incidencias) == 0 {
		fmt.Fprintln(a.Stdout, a.t("La factura no presenta incidencias.", "La factura no presenta incidencias."))
		return 0
	}
	errores := 0
	for _, i := range incidencias {
		if i.Nivel == commonsigner.IncidenciaError {
			errores++
		}
		fmt.Fprintln(a.Stdout, i.String())
	}
	fmt.Fprintln(a.Stdout, a.t("Incidencias: %d (errores: %d).", "Incidencias: %d (errores: %d).", len(incidencias), errores))
	if errores > 0 {
		return 1
	}
	return 0
}

// ejecutarGenerarENI envuelve una firma hecha con la aplicación en un
// documento electrónico ENI con sus metadatos obligatorios.
func (a *Adaptador) ejecutarGenerarENI(cfg configCLI) int {
	titulo := a.t("Documento ENI", "Documento ENI")
	if strings.TrimSpace(cfg.entrada) == "" {
		a.escribirErrorCLI(titulo, errors.New(a.t("Indica la firma con -entrada.", "Indica la firma con -entrada.")))
		return 1
	}
	firma, err := a.LeerFichero(cfg.entrada)
	if err == nil {
		err = a.Limits.CheckPayload(firma)
	}
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	var original []byte
	if strings.TrimSpace(cfg.original) != "" {
		if original, err = a.LeerFichero(cfg.original); err == nil {
			err = a.Limits.CheckPayload(original)
		}
		if err != nil {
			a.escribirErrorCLI(a.t("Original", "Original"), err)
			return 1
		}
	}
	doc, err := a.documentoENIDesdeFirma(firma, original, cfg.opciones)
	if err != nil {
		a.escribirErrorCLI(titulo, err)
		return 1
	}
	xmlENI, err := eni.Generar(doc, time.Now())
	if err != nil {
		a.escribirErrorCLI(titulo, err)
		return 1
	}
	destino := strings.TrimSpace(cfg.salida)
	if destino == "" {
		destino = strings.TrimSuffix(cfg.entrada, filepath.Ext(cfg.entrada)) + "_eni.xml"
	}
	destino, _, _, err = a.guardarSalida(destino, cfg.sobrescribir, xmlENI)
	if err != nil {
		a.escribirErrorCLI(a.t("Salida", "Salida"), err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "%s: %s\n", titulo, destino)
	return 0
}

// documentoENIDesdeFirma reconoce el tipo de firma (PAdES, CAdES implícita o
// explícita, XAdES) y toma los metadatos de las opciones eni.*.
func (a *Adaptador) documentoENIDesdeFirma(firma, original []byte, opciones map[string]string) (eni.Documento, error) {
	op := func(k string) string { return strings.TrimSpace(opciones["eni."+k]) }
	var doc eni.Documento
	recortado := bytes.TrimLeft(firma, " \t\r\n\ufeff")
	switch {
	case bytes.HasPrefix(recortado, []byte("%PDF-")):
		if !bytes.Contains(firma, []byte("/ByteRange")) {
			return doc, errors.New(a.t("El PDF no está firmado.", "El PDF no está firmado."))
		}
		if commonsigner.ComprobarIntegridadPAdES(context.Background(), firma) != nil {
			return doc, a.errorFirmaNoCorresponde()
		}
		doc.Contenido, doc.NombreFormato = firma, "PDF"
		doc.Firmas = []eni.Firma{{Tipo: eni.FirmaPAdES}}
	case len(recortado) > 0 && recortado[0] == 0x30:
		contenido, implicita, err := commonsigner.ContenidoCAdESImplicito(firma)
		if err != nil {
			return doc, err
		}
		if implicita {
			doc.Contenido = contenido
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaCAdESImplicit, Datos: firma}}
		} else {
			if len(original) == 0 {
				return doc, errors.New(a.t("La firma CAdES es explícita: indica el documento firmado con -original.", "La firma CAdES es explícita: indica el documento firmado con -original."))
			}
			if commonsigner.CotejarCAdESExplicita(firma, original) != nil {
				return doc, a.errorFirmaNoCorresponde()
			}
			doc.Contenido = original
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaCAdESExplicit, Datos: firma}}
		}
	case bytes.HasPrefix(recortado, []byte("<")) && bytes.Contains(firma, []byte("http://www.w3.org/2000/09/xmldsig#")):
		if len(original) > 0 {
			if commonsigner.CotejarXAdESSeparada(firma, original) != nil {
				return doc, a.errorFirmaNoCorresponde()
			}
			doc.Contenido = original
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaXAdESDetached, Datos: firma}}
		} else {
			doc.Contenido, doc.NombreFormato = firma, "XML"
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaXAdESEnvelope, Datos: firma}}
		}
	default:
		return doc, errors.New(a.t("El fichero no es una firma PAdES, CAdES o XAdES reconocida.", "El fichero no es una firma PAdES, CAdES o XAdES reconocida."))
	}
	if f := op("formato"); f != "" {
		doc.NombreFormato = strings.ToUpper(f)
	} else if doc.NombreFormato == "" {
		doc.NombreFormato = formatoContenidoENI(doc.Contenido)
	}
	if doc.NombreFormato == "" {
		return doc, errors.New(a.t("No se reconoce el formato del contenido: indícalo con -opcion eni.formato=PDF.", "No se reconoce el formato del contenido: indícalo con -opcion eni.formato=PDF."))
	}
	m := &doc.Metadatos
	for _, o := range strings.Split(op("organo"), ",") {
		if o = strings.ToUpper(strings.TrimSpace(o)); o != "" {
			m.Organos = append(m.Organos, o)
		}
	}
	switch strings.ToLower(op("origen")) {
	case "administracion", "administración":
		m.OrigenAdministracion = true
	case "ciudadano":
	default:
		return doc, errors.New(a.t("Indica el origen con -opcion eni.origen=ciudadano o administracion.", "Indica el origen con -opcion eni.origen=ciudadano o administracion."))
	}
	m.Identificador = op("identificador")
	m.EstadoElaboracion = strings.ToUpper(op("estado"))
	if m.EstadoElaboracion == "" {
		m.EstadoElaboracion = "EE01"
	}
	m.IdentificadorDocumentoOrigen = op("documentoOrigen")
	m.TipoDocumental = strings.ToUpper(op("tipoDocumental"))
	if m.TipoDocumental == "" {
		m.TipoDocumental = "TD99"
	}
	if f := op("fechaCaptura"); f != "" {
		fecha, err := time.Parse(time.RFC3339, f)
		if err != nil {
			return doc, errors.New(a.t("La fecha de captura (eni.fechaCaptura) debe tener el formato 2026-09-26T10:00:00+02:00.", "La fecha de captura (eni.fechaCaptura) debe tener el formato 2026-09-26T10:00:00+02:00."))
		}
		m.FechaCaptura = fecha
	}
	return doc, nil
}

// errorFirmaNoCorresponde se devuelve cuando el cotejo local de la firma con
// el original (o la integridad del PDF firmado) falla antes de crear el ENI.
func (a *Adaptador) errorFirmaNoCorresponde() error {
	return errors.New(a.t("eni.error.signature_mismatch", "La firma no corresponde al documento original o el documento se ha modificado después de firmarlo. Compruebe que ha elegido el original correcto."))
}

// DocumentoENIDesdeFirma comparte con el IPC el reconocimiento de firmas y
// los metadatos usados por la orden generar-eni.
func DocumentoENIDesdeFirma(firma, original []byte, opciones map[string]string) (eni.Documento, error) {
	return (&Adaptador{}).documentoENIDesdeFirma(firma, original, opciones)
}

func formatoContenidoENI(data []byte) string {
	d := bytes.TrimLeft(data, " \t\r\n\ufeff")
	switch {
	case bytes.HasPrefix(d, []byte("%PDF-")):
		return "PDF"
	case bytes.HasPrefix(d, []byte("<")):
		return "XML"
	case bytes.HasPrefix(d, []byte("PK\x03\x04")):
		return "ZIP"
	case bytes.HasPrefix(d, []byte("\x89PNG")):
		return "PNG"
	case bytes.HasPrefix(d, []byte("\xff\xd8\xff")):
		return "JPEG"
	}
	return ""
}

// ejecutarGenerarExpediente crea un expediente electrónico ENI con los
// documentos ENI de una carpeta y firma su índice con el certificado elegido.
func (a *Adaptador) ejecutarGenerarExpediente(ctx context.Context, cfg configCLI) int {
	titulo := a.t("Expediente ENI", "Expediente ENI")
	dir := strings.TrimSpace(cfg.lote)
	if dir == "" || !esDirectorioRuta(dir) {
		a.escribirErrorCLI(titulo, errors.New(a.t("Indica con -lote la carpeta con los documentos ENI del expediente.", "Indica con -lote la carpeta con los documentos ENI del expediente.")))
		return 1
	}
	manifest, err := a.manifiestoDesdeCarpeta(dir)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return 1
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Ruta < manifest[j].Ruta })
	docs := make([]eni.DocumentoExpediente, 0, len(manifest))
	for _, item := range manifest {
		data, err := a.LeerFichero(item.Ruta)
		if err == nil {
			err = a.Limits.CheckPayload(data)
		}
		if err != nil {
			a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
			return 1
		}
		docs = append(docs, eni.DocumentoExpediente{XML: data})
	}
	op := func(k string) string { return strings.TrimSpace(cfg.opciones["exp."+k]) }
	meta := eni.MetadatosExpediente{
		Identificador: op("identificador"),
		Clasificacion: op("clasificacion"),
		Estado:        strings.ToUpper(op("estado")),
	}
	if meta.Estado == "" {
		meta.Estado = "E01"
	}
	for _, o := range strings.Split(op("organo"), ",") {
		if o = strings.ToUpper(strings.TrimSpace(o)); o != "" {
			meta.Organos = append(meta.Organos, o)
		}
	}
	meta.Interesados = strings.Split(op("interesado"), ",")
	if f := op("fechaApertura"); f != "" {
		if meta.FechaApertura, err = time.Parse(time.RFC3339, f); err != nil {
			a.escribirErrorCLI(titulo, errors.New(a.t("La fecha de apertura (exp.fechaApertura) debe tener el formato 2026-09-26T10:00:00+02:00.", "La fecha de apertura (exp.fechaApertura) debe tener el formato 2026-09-26T10:00:00+02:00.")))
			return 1
		}
	}
	if err := meta.Validar(); err != nil {
		a.escribirErrorCLI(titulo, err)
		return 1
	}
	if a.Claves == nil {
		a.escribirErrorCLI(titulo, errors.New(a.t("No disponible", "No disponible")))
		return 1
	}
	_, certRef, err := a.resolverCertificadoFirma(ctx, cfg)
	if err != nil {
		a.escribirErrorCLI(a.t("Certificado", "Certificado"), err)
		return 1
	}
	clave, err := a.Claves.KeyFor(ctx, certRef)
	if err != nil {
		ports.CloseSigningKey(clave)
		a.escribirErrorCLI(a.t("Certificado", "Certificado"), err)
		return 1
	}
	defer ports.CloseSigningKey(clave)
	firmar := func(nodo []byte, id string) ([]byte, error) {
		return commonsigner.FirmarNodoXAdES(nodo, id, clave, cfg.opciones)
	}
	xmlExp, err := eni.GenerarExpediente(meta, docs, firmar, commonsigner.CanonicalizarExclusivo, time.Now())
	if err != nil {
		a.escribirErrorCLI(titulo, err)
		return 1
	}
	destino := strings.TrimSpace(cfg.salida)
	if destino == "" {
		destino = filepath.Join(dir, "expediente_eni.xml")
	}
	destino, _, _, err = a.guardarSalida(destino, cfg.sobrescribir, xmlExp)
	if err != nil {
		a.escribirErrorCLI(a.t("Salida", "Salida"), err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "%s: %s (%d)\n", titulo, destino, len(docs))
	return 0
}

// ejecutarVerificarAuditoria comprueba la cadena de huellas del registro de
// auditoría (la rotación y el fichero activo).
func (a *Adaptador) ejecutarVerificarAuditoria(cfg configCLI) int {
	titulo := a.t("Registro de auditoría", "Registro de auditoría")
	ruta := strings.TrimSpace(cfg.entrada)
	if ruta == "" {
		var err error
		if ruta, err = auditlog.RutaPorDefecto(); err != nil {
			a.escribirErrorCLI(titulo, err)
			return 1
		}
	}
	inf, err := auditlog.VerificarCadena(ruta+".1", ruta)
	if err != nil {
		a.escribirErrorCLI(titulo, err)
		return 1
	}
	fmt.Fprintln(a.Stdout, a.t("Registro de auditoría íntegro: %d registros.", "Registro de auditoría íntegro: %d registros.", inf.Registros))
	if inf.InicioRetirado {
		fmt.Fprintln(a.Stdout, a.t("Los registros más antiguos se retiraron por la política de retención; la cadena se comprueba desde el primero conservado.", "Los registros más antiguos se retiraron por la política de retención; la cadena se comprueba desde el primero conservado."))
	}
	if inf.SinCadena > 0 {
		fmt.Fprintln(a.Stdout, a.t("%d registros son anteriores a la cadena de huellas y no pueden comprobarse.", "%d registros son anteriores a la cadena de huellas y no pueden comprobarse.", inf.SinCadena))
	}
	return 0
}
