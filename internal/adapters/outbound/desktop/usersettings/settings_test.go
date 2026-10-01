// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package usersettings_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/ports"
)

func TestCargar_FicheroNoExiste(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(filepath.Join(dir, "noexiste"))
	datos, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar con dir inexistente: %v", err)
	}
	if len(datos) != 0 {
		t.Errorf("esperaba mapa vacio, got %v", datos)
	}
}

func TestGuardarYCargar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"tema":   "oscuro",
		"idioma": "es",
		"dpi":    float64(150),
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	for k, v := range entrada {
		if salida[k] != v {
			t.Errorf("clave %q: got %v, queria %v", k, salida[k], v)
		}
	}
}

func TestGuardar_NoPersisteCredencialSeguridadUIEnClaro(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"idioma":                 "es",
		"securityAccessPassword": "secreto-local",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("leer settings.json: %v", err)
	}
	if string(data) == "" {
		t.Fatal("settings.json no debe quedar vacío")
	}
	var persistido map[string]any
	if err := json.Unmarshal(data, &persistido); err != nil {
		t.Fatalf("parsear settings.json: %v", err)
	}
	if _, presente := persistido["securityAccessPassword"]; presente {
		t.Fatalf("securityAccessPassword no debe persistirse: %#v", persistido)
	}
	if persistido["idioma"] != "es" {
		t.Fatalf("se perdió una preferencia válida al sanear: %#v", persistido)
	}
}

func TestCargar_MigraCredencialSeguridadUIEnClaroHeredada(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ruta := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(ruta, []byte(`{
  "idioma": "es",
  "securityAccessPassword": "secreto-heredado"
}`), 0o600); err != nil {
		t.Fatalf("preparar settings.json heredado: %v", err)
	}

	almacen := usersettings.New(dir)
	datos, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}
	if _, presente := datos["securityAccessPassword"]; presente {
		t.Fatalf("Cargar no debe devolver la credencial heredada: %#v", datos)
	}

	data, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer settings.json migrado: %v", err)
	}
	var persistido map[string]any
	if err := json.Unmarshal(data, &persistido); err != nil {
		t.Fatalf("parsear settings.json migrado: %v", err)
	}
	if _, presente := persistido["securityAccessPassword"]; presente {
		t.Fatalf("la migración no retiró la credencial del disco: %#v", persistido)
	}
	if persistido["idioma"] != "es" {
		t.Fatalf("la migración perdió preferencias válidas: %#v", persistido)
	}
}

func TestCargarDocumentoCompat_UsaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"idioma":          "es",
		"legacyWebUiSize": "extra",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := usersettings.CargarDocumentoCompat(context.Background(), dir)
	if err != nil {
		t.Fatalf("CargarDocumentoCompat: %v", err)
	}

	if doc.General.Idioma == nil || *doc.General.Idioma != "es" {
		t.Fatalf("idioma tipado inesperado: %#v", doc.General.Idioma)
	}
	if doc.Desktop.LegacyWebUISize == nil || *doc.Desktop.LegacyWebUISize != "extra" {
		t.Fatalf("legacyWebUiSize tipado inesperado: %#v", doc.Desktop.LegacyWebUISize)
	}
}

func TestCargarDesktopCompat_UsaBloqueDesktopTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"legacyWebUiSize":                 "normal",
		"legacyWebTrayResident":           true,
		"webCompatibilityDurationMinutes": 30,
		"idioma":                          "es",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	desktop, err := usersettings.CargarDesktopCompat(context.Background(), dir)
	if err != nil {
		t.Fatalf("CargarDesktopCompat: %v", err)
	}

	if desktop.LegacyWebUISize == nil || *desktop.LegacyWebUISize != "normal" {
		t.Fatalf("legacyWebUiSize tipado inesperado: %#v", desktop.LegacyWebUISize)
	}
	if desktop.LegacyWebTrayResident == nil || !*desktop.LegacyWebTrayResident {
		t.Fatalf("legacyWebTrayResident tipado inesperado: %#v", desktop.LegacyWebTrayResident)
	}
	if desktop.WebCompatibilityDurationMinutes == nil || *desktop.WebCompatibilityDurationMinutes != 30 {
		t.Fatalf("webCompatibilityDurationMinutes tipado inesperado: %#v", desktop.WebCompatibilityDurationMinutes)
	}
}

func TestCargarFirmaCompat_UsaBloquesFirmaMetaYTSA(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"signAction":      "sign",
		"signFormat":      "pades",
		"signReason":      "Aprobación interna",
		"signLocation":    "Granada",
		"signContactInfo": "contacto@example.invalid",
		"tsaEnabled":      true,
		"tsaUrl":          "https://tsa.local",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	firma, meta, tsa, err := usersettings.CargarFirmaCompat(context.Background(), dir)
	if err != nil {
		t.Fatalf("CargarFirmaCompat: %v", err)
	}

	if firma.Action == nil || *firma.Action != "sign" {
		t.Fatalf("action tipada inesperada: %#v", firma.Action)
	}
	if firma.Format == nil || *firma.Format != "pades" {
		t.Fatalf("format tipado inesperado: %#v", firma.Format)
	}
	if meta.Reason == nil || *meta.Reason != "Aprobación interna" {
		t.Fatalf("reason tipada inesperada: %#v", meta.Reason)
	}
	if meta.Location == nil || *meta.Location != "Granada" {
		t.Fatalf("location tipada inesperada: %#v", meta.Location)
	}
	if meta.ContactInfo == nil || *meta.ContactInfo != "contacto@example.invalid" {
		t.Fatalf("contactInfo tipada inesperada: %#v", meta.ContactInfo)
	}
	if tsa.Enabled == nil || !*tsa.Enabled {
		t.Fatalf("tsa enabled tipada inesperada: %#v", tsa.Enabled)
	}
	if tsa.URL == nil || *tsa.URL != "https://tsa.local" {
		t.Fatalf("tsa url tipada inesperada: %#v", tsa.URL)
	}
}

func TestCargarFormatosAutoCompat_UsaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"autoFormatPdf":      "pades",
		"autoFormatOoxml":    "ooxml",
		"autoFormatFacturae": "facturae",
		"autoFormatOdf":      "odf",
		"autoFormatXml":      "xades",
		"autoFormatBinary":   "cades",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	formatos, err := usersettings.CargarFormatosAutoCompat(context.Background(), dir)
	if err != nil {
		t.Fatalf("CargarFormatosAutoCompat: %v", err)
	}

	if formatos.PDF == nil || *formatos.PDF != "pades" {
		t.Fatalf("autoFormatPdf tipado inesperado: %#v", formatos.PDF)
	}
	if formatos.OOXML == nil || *formatos.OOXML != "ooxml" {
		t.Fatalf("autoFormatOoxml tipado inesperado: %#v", formatos.OOXML)
	}
	if formatos.FacturaE == nil || *formatos.FacturaE != "facturae" {
		t.Fatalf("autoFormatFacturae tipado inesperado: %#v", formatos.FacturaE)
	}
	if formatos.ODF == nil || *formatos.ODF != "odf" {
		t.Fatalf("autoFormatOdf tipado inesperado: %#v", formatos.ODF)
	}
	if formatos.XML == nil || *formatos.XML != "xades" {
		t.Fatalf("autoFormatXml tipado inesperado: %#v", formatos.XML)
	}
	if formatos.Binary == nil || *formatos.Binary != "cades" {
		t.Fatalf("autoFormatBinary tipado inesperado: %#v", formatos.Binary)
	}
}

func TestCargarMultiCoSignCompat_UsaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"multiCosignEnabled":              true,
		"multiCosignPrimaryCertificateId": "cert-main",
		"multiCosignCertificateIds":       []any{"cert-a", "cert-b"},
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	multi, err := usersettings.CargarMultiCoSignCompat(context.Background(), dir)
	if err != nil {
		t.Fatalf("CargarMultiCoSignCompat: %v", err)
	}

	if multi.Enabled == nil || !*multi.Enabled {
		t.Fatalf("enabled tipado inesperado: %#v", multi.Enabled)
	}
	if multi.PrimaryID == nil || *multi.PrimaryID != "cert-main" {
		t.Fatalf("primary id tipado inesperado: %#v", multi.PrimaryID)
	}
	if len(multi.CertificateIDs) != 2 || multi.CertificateIDs[0] != "cert-a" || multi.CertificateIDs[1] != "cert-b" {
		t.Fatalf("ids tipados inesperados: %#v", multi.CertificateIDs)
	}
}

func TestCargarPAdESVisibleCompat_UsaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"signVisibleSeal":   true,
		"signSealPages":     "all",
		"signSealAllPages":  true,
		"signSealX":         0.62,
		"signSealY":         0.04,
		"signSealW":         0.34,
		"signSealH":         0.12,
		"signSealKeepText":  true,
		"signSealRotation":  270,
		"signSealImagePath": "/tmp/sello.png",
		"signQRContent":     "https://verifica.local/expediente/123",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	padesVisible, err := usersettings.CargarPAdESVisibleCompat(context.Background(), dir)
	if err != nil {
		t.Fatalf("CargarPAdESVisibleCompat: %v", err)
	}

	if padesVisible.Enabled == nil || !*padesVisible.Enabled {
		t.Fatalf("enabled tipado inesperado: %#v", padesVisible.Enabled)
	}
	if padesVisible.Pages == nil || *padesVisible.Pages != "all" {
		t.Fatalf("pages tipadas inesperadas: %#v", padesVisible.Pages)
	}
	if padesVisible.AllPages == nil || !*padesVisible.AllPages {
		t.Fatalf("allPages tipado inesperado: %#v", padesVisible.AllPages)
	}
	if padesVisible.X == nil || *padesVisible.X != 0.62 {
		t.Fatalf("x tipada inesperada: %#v", padesVisible.X)
	}
	if padesVisible.Y == nil || *padesVisible.Y != 0.04 {
		t.Fatalf("y tipada inesperada: %#v", padesVisible.Y)
	}
	if padesVisible.W == nil || *padesVisible.W != 0.34 {
		t.Fatalf("w tipada inesperada: %#v", padesVisible.W)
	}
	if padesVisible.H == nil || *padesVisible.H != 0.12 {
		t.Fatalf("h tipada inesperada: %#v", padesVisible.H)
	}
	if padesVisible.KeepText == nil || !*padesVisible.KeepText {
		t.Fatalf("keepText tipado inesperado: %#v", padesVisible.KeepText)
	}
	if padesVisible.Rotation == nil || *padesVisible.Rotation != 270 {
		t.Fatalf("rotation tipada inesperada: %#v", padesVisible.Rotation)
	}
	if padesVisible.ImagePath == nil || *padesVisible.ImagePath != "/tmp/sello.png" {
		t.Fatalf("imagePath tipado inesperado: %#v", padesVisible.ImagePath)
	}
	if padesVisible.QRContent == nil || *padesVisible.QRContent != "https://verifica.local/expediente/123" {
		t.Fatalf("qrContent tipado inesperado: %#v", padesVisible.QRContent)
	}
}

func TestCargarCertificadosCompat_UsaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"stickySigner":                   true,
		"autoSelectSingleCertificate":    false,
		"preferDefaultCertificate":       true,
		"preferredCertificateId":         " cert-last ",
		"defaultCertificateId":           " cert-default ",
		"defaultKeystore":                " windows-my ",
		"defaultLocalKeystorePath":       " /tmp/almacen.p12 ",
		"useDefaultStoreInBrowserCalls":  true,
		"useOnlyAliasCertificates":       true,
		"skipAuthCertDnie":               true,
		"showDefaultCertificateFirst":    true,
		"showUsableCertificatesFirst":    true,
		"showValidCertificatesFirst":     true,
		"rememberCertificateFilter":      true,
		"certificateFilterText":          " dni ",
		"certsExpiredShow":               true,
		"certsInvalidShow":               true,
		"certificateTypeFilter":          []any{"fisica", "empleado_publico"},
		"certificateRequireNIF":          true,
		"certificateRequireOrganization": false,
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	certs, err := usersettings.CargarCertificadosCompat(context.Background(), dir)
	if err != nil {
		t.Fatalf("CargarCertificadosCompat: %v", err)
	}

	if certs.StickySigner == nil || !*certs.StickySigner {
		t.Fatalf("stickySigner tipado inesperado: %#v", certs.StickySigner)
	}
	if certs.AutoSelectSingleCertificate == nil || *certs.AutoSelectSingleCertificate {
		t.Fatalf("autoSelectSingleCertificate tipado inesperado: %#v", certs.AutoSelectSingleCertificate)
	}
	if certs.PreferDefaultCertificate == nil || !*certs.PreferDefaultCertificate {
		t.Fatalf("preferDefaultCertificate tipado inesperado: %#v", certs.PreferDefaultCertificate)
	}
	if certs.PreferredCertificateID == nil || *certs.PreferredCertificateID != "cert-last" {
		t.Fatalf("preferredCertificateId tipado inesperado: %#v", certs.PreferredCertificateID)
	}
	if certs.DefaultCertificateID == nil || *certs.DefaultCertificateID != "cert-default" {
		t.Fatalf("defaultCertificateId tipado inesperado: %#v", certs.DefaultCertificateID)
	}
	if certs.DefaultKeystore == nil || *certs.DefaultKeystore != "windows-my" {
		t.Fatalf("defaultKeystore tipado inesperado: %#v", certs.DefaultKeystore)
	}
	if certs.DefaultLocalKeystorePath == nil || *certs.DefaultLocalKeystorePath != "/tmp/almacen.p12" {
		t.Fatalf("defaultLocalKeystorePath tipado inesperado: %#v", certs.DefaultLocalKeystorePath)
	}
	if certs.UseDefaultStoreInBrowserCalls == nil || !*certs.UseDefaultStoreInBrowserCalls {
		t.Fatalf("useDefaultStoreInBrowserCalls tipado inesperado: %#v", certs.UseDefaultStoreInBrowserCalls)
	}
	if certs.UseOnlyAliasCertificates == nil || !*certs.UseOnlyAliasCertificates {
		t.Fatalf("useOnlyAliasCertificates tipado inesperado: %#v", certs.UseOnlyAliasCertificates)
	}
	if certs.SkipAuthCertDnie == nil || !*certs.SkipAuthCertDnie {
		t.Fatalf("skipAuthCertDnie tipado inesperado: %#v", certs.SkipAuthCertDnie)
	}
	if certs.ShowDefaultFirst == nil || !*certs.ShowDefaultFirst {
		t.Fatalf("showDefaultCertificateFirst tipado inesperado: %#v", certs.ShowDefaultFirst)
	}
	if certs.ShowUsableFirst == nil || !*certs.ShowUsableFirst {
		t.Fatalf("showUsableCertificatesFirst tipado inesperado: %#v", certs.ShowUsableFirst)
	}
	if certs.ShowValidFirst == nil || !*certs.ShowValidFirst {
		t.Fatalf("showValidCertificatesFirst tipado inesperado: %#v", certs.ShowValidFirst)
	}
	if certs.RememberFilter == nil || !*certs.RememberFilter {
		t.Fatalf("rememberCertificateFilter tipado inesperado: %#v", certs.RememberFilter)
	}
	if certs.FilterText == nil || *certs.FilterText != "dni" {
		t.Fatalf("certificateFilterText tipado inesperado: %#v", certs.FilterText)
	}
	if certs.ShowExpired == nil || !*certs.ShowExpired {
		t.Fatalf("certsExpiredShow tipado inesperado: %#v", certs.ShowExpired)
	}
	if certs.ShowInvalid == nil || !*certs.ShowInvalid {
		t.Fatalf("certsInvalidShow tipado inesperado: %#v", certs.ShowInvalid)
	}
	if len(certs.TypeFilter) != 2 || certs.TypeFilter[0] != "fisica" || certs.TypeFilter[1] != "empleado_publico" {
		t.Fatalf("certificateTypeFilter tipado inesperado: %#v", certs.TypeFilter)
	}
	if certs.RequireNIF == nil || !*certs.RequireNIF {
		t.Fatalf("certificateRequireNIF tipado inesperado: %#v", certs.RequireNIF)
	}
	if certs.RequireOrganization == nil || *certs.RequireOrganization {
		t.Fatalf("certificateRequireOrganization tipado inesperado: %#v", certs.RequireOrganization)
	}
}

func TestActualizarDocumentoCompat_PreservaExtrasYActualizaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"tsaUrl": "https://tsa.local",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarDocumentoCompat(context.Background(), dir, func(doc *ports.DocumentoConfiguracionUsuario) {
		valor := true
		doc.Desktop.LegacyWebTrayResident = &valor
	}); err != nil {
		t.Fatalf("ActualizarDocumentoCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["legacyWebTrayResident"] != true {
		t.Fatalf("legacyWebTrayResident no persistido: %#v", salida)
	}
	if salida["tsaUrl"] != "https://tsa.local" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func TestActualizarDocumentoCompat_PersistenciaPreferenciasSeguridadYCertificadosV1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"tsaUrl": "https://tsa.local",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarDocumentoCompat(context.Background(), dir, func(doc *ports.DocumentoConfiguracionUsuario) {
		hideDnieStartScreen := true
		secureConnections := true
		defaultKeystore := "shared-nss"
		defaultLocalKeystorePath := "/tmp/certs/local.p12"
		useDefaultStoreInBrowserCalls := true
		useOnlyAliasCertificates := true
		skipAuthCertDnie := true
		doc.General.HideDnieStartScreen = &hideDnieStartScreen
		doc.General.SecureConnections = &secureConnections
		doc.General.SecureDomainsList = []string{"https://sede.dipgra.es", "*.juntadeandalucia.es"}
		doc.Certificados.DefaultKeystore = &defaultKeystore
		doc.Certificados.DefaultLocalKeystorePath = &defaultLocalKeystorePath
		doc.Certificados.UseDefaultStoreInBrowserCalls = &useDefaultStoreInBrowserCalls
		doc.Certificados.UseOnlyAliasCertificates = &useOnlyAliasCertificates
		doc.Certificados.SkipAuthCertDnie = &skipAuthCertDnie
	}); err != nil {
		t.Fatalf("ActualizarDocumentoCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["hideDnieStartScreen"] != true || salida["secureConnections"] != true {
		t.Fatalf("flags generales de seguridad no persistidos: %#v", salida)
	}
	secureDomains, ok := salida["secureDomainsList"].([]any)
	if !ok || len(secureDomains) != 2 || secureDomains[0] != "https://sede.dipgra.es" || secureDomains[1] != "*.juntadeandalucia.es" {
		t.Fatalf("secureDomainsList no persistido: %#v", salida)
	}
	if salida["defaultKeystore"] != "shared-nss" || salida["defaultLocalKeystorePath"] != "/tmp/certs/local.p12" {
		t.Fatalf("preferencias de keystore no persistidas: %#v", salida)
	}
	if salida["useDefaultStoreInBrowserCalls"] != true || salida["useOnlyAliasCertificates"] != true || salida["skipAuthCertDnie"] != true {
		t.Fatalf("flags de certificados V1 no persistidos: %#v", salida)
	}
	if salida["tsaUrl"] != "https://tsa.local" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func TestActualizarDesktopCompat_PreservaExtrasYActualizaBloqueDesktop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"tsaUrl":          "https://tsa.local",
		"legacyWebUiSize": "grande",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarDesktopCompat(context.Background(), dir, func(desktop *ports.ConfiguracionUsuarioDesktop) {
		valor := false
		desktop.LegacyWebTrayResident = &valor
	}); err != nil {
		t.Fatalf("ActualizarDesktopCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["legacyWebTrayResident"] != false {
		t.Fatalf("legacyWebTrayResident no persistido: %#v", salida)
	}
	if salida["legacyWebUiSize"] != "grande" || salida["tsaUrl"] != "https://tsa.local" {
		t.Fatalf("bloque desktop/extras no preservado: %#v", salida)
	}
}

func TestActualizarFirmaCompat_PreservaExtrasYActualizaBloquesFirmaMetaYTSA(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"profile": "B",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarFirmaCompat(context.Background(), dir, func(firma *ports.ConfiguracionUsuarioFirma, meta *ports.ConfiguracionUsuarioFirmaMetadatos, tsa *ports.ConfiguracionUsuarioTSA) {
		action := "cosign"
		reason := "Aprobación interna"
		location := "Granada"
		contact := "contacto@example.invalid"
		enabled := true
		url := "https://tsa.local"
		firma.Action = &action
		meta.Reason = &reason
		meta.Location = &location
		meta.ContactInfo = &contact
		tsa.Enabled = &enabled
		tsa.URL = &url
	}); err != nil {
		t.Fatalf("ActualizarFirmaCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["signAction"] != "cosign" || salida["signReason"] != "Aprobación interna" || salida["signLocation"] != "Granada" || salida["signContactInfo"] != "contacto@example.invalid" || salida["tsaUrl"] != "https://tsa.local" || salida["tsaEnabled"] != true {
		t.Fatalf("bloque firma/meta/tsa no persistido: %#v", salida)
	}
	if salida["profile"] != "B" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func TestActualizarFormatosAutoCompat_PreservaExtrasYActualizaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"profile": "B",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarFormatosAutoCompat(context.Background(), dir, func(formatos *ports.ConfiguracionUsuarioFormatosAutomaticos) {
		pdf := "pades"
		ooxml := "ooxml"
		facturae := "facturae"
		odf := "odf"
		xml := "xades"
		binary := "cades"
		formatos.PDF = &pdf
		formatos.OOXML = &ooxml
		formatos.FacturaE = &facturae
		formatos.ODF = &odf
		formatos.XML = &xml
		formatos.Binary = &binary
	}); err != nil {
		t.Fatalf("ActualizarFormatosAutoCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["autoFormatPdf"] != "pades" ||
		salida["autoFormatOoxml"] != "ooxml" ||
		salida["autoFormatFacturae"] != "facturae" ||
		salida["autoFormatOdf"] != "odf" ||
		salida["autoFormatXml"] != "xades" ||
		salida["autoFormatBinary"] != "cades" {
		t.Fatalf("bloque formatos auto no persistido: %#v", salida)
	}
	if salida["profile"] != "B" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func TestActualizarMultiCoSignCompat_PreservaExtrasYActualizaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"profile": "B",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarMultiCoSignCompat(context.Background(), dir, func(multi *ports.ConfiguracionUsuarioMultiCoSign) {
		enabled := true
		multi.Enabled = &enabled
		primaryID := "cert-main"
		multi.PrimaryID = &primaryID
		multi.CertificateIDs = []string{"cert-a", "cert-b"}
	}); err != nil {
		t.Fatalf("ActualizarMultiCoSignCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["multiCosignEnabled"] != true {
		t.Fatalf("multiCosignEnabled no persistido: %#v", salida)
	}
	if salida["multiCosignPrimaryCertificateId"] != "cert-main" {
		t.Fatalf("primary id multicosign no persistido: %#v", salida)
	}
	ids, ok := salida["multiCosignCertificateIds"].([]any)
	if !ok || len(ids) != 2 || ids[0] != "cert-a" || ids[1] != "cert-b" {
		t.Fatalf("ids multicosign no persistidos: %#v", salida)
	}
	if salida["profile"] != "B" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func TestActualizarPAdESVisibleCompat_PreservaExtrasYActualizaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"profile": "B",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarPAdESVisibleCompat(context.Background(), dir, func(padesVisible *ports.ConfiguracionUsuarioPAdESVisible) {
		enabled := true
		pages := "1,3-5"
		allPages := false
		x := 0.62
		y := 0.04
		w := 0.34
		h := 0.12
		keepText := true
		rotation := 270
		imagePath := "/tmp/sello.png"
		qrContent := "https://verifica.local/expediente/123"
		padesVisible.Enabled = &enabled
		padesVisible.Pages = &pages
		padesVisible.AllPages = &allPages
		padesVisible.X = &x
		padesVisible.Y = &y
		padesVisible.W = &w
		padesVisible.H = &h
		padesVisible.KeepText = &keepText
		padesVisible.Rotation = &rotation
		padesVisible.ImagePath = &imagePath
		padesVisible.QRContent = &qrContent
	}); err != nil {
		t.Fatalf("ActualizarPAdESVisibleCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["signVisibleSeal"] != true || salida["signSealPages"] != "1,3-5" || salida["signSealAllPages"] != false || salida["signSealX"] != 0.62 || salida["signSealY"] != 0.04 || salida["signSealW"] != 0.34 || salida["signSealH"] != 0.12 || salida["signSealKeepText"] != true || salida["signSealRotation"] != float64(270) || salida["signSealImagePath"] != "/tmp/sello.png" || salida["signQRContent"] != "https://verifica.local/expediente/123" {
		t.Fatalf("bloque PAdES visible no persistido: %#v", salida)
	}
	if salida["profile"] != "B" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func TestActualizarCertificadosCompat_PreservaExtrasYActualizaBloqueTipado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{
		"profile": "B",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if err := usersettings.ActualizarCertificadosCompat(context.Background(), dir, func(certs *ports.ConfiguracionUsuarioCertificados) {
		sticky := true
		autoSelectSingle := false
		preferDefault := true
		preferredID := "cert-last"
		defaultID := "cert-default"
		showDefaultFirst := true
		showUsableFirst := true
		showValidFirst := true
		rememberFilter := true
		filterText := "dni"
		showExpired := true
		showInvalid := true
		requireNIF := true
		requireOrganization := false
		certs.StickySigner = &sticky
		certs.AutoSelectSingleCertificate = &autoSelectSingle
		certs.PreferDefaultCertificate = &preferDefault
		certs.PreferredCertificateID = &preferredID
		certs.DefaultCertificateID = &defaultID
		certs.ShowDefaultFirst = &showDefaultFirst
		certs.ShowUsableFirst = &showUsableFirst
		certs.ShowValidFirst = &showValidFirst
		certs.RememberFilter = &rememberFilter
		certs.FilterText = &filterText
		certs.ShowExpired = &showExpired
		certs.ShowInvalid = &showInvalid
		certs.TypeFilter = []string{"fisica", "empleado_publico"}
		certs.RequireNIF = &requireNIF
		certs.RequireOrganization = &requireOrganization
	}); err != nil {
		t.Fatalf("ActualizarCertificadosCompat: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["stickySigner"] != true || salida["autoSelectSingleCertificate"] != false || salida["preferDefaultCertificate"] != true || salida["preferredCertificateId"] != "cert-last" || salida["defaultCertificateId"] != "cert-default" || salida["showDefaultCertificateFirst"] != true || salida["showUsableCertificatesFirst"] != true || salida["showValidCertificatesFirst"] != true || salida["rememberCertificateFilter"] != true || salida["certificateFilterText"] != "dni" || salida["certsExpiredShow"] != true || salida["certsInvalidShow"] != true || salida["certificateRequireNIF"] != true || salida["certificateRequireOrganization"] != false {
		t.Fatalf("bloque certificados no persistido: %#v", salida)
	}
	typeFilter, ok := salida["certificateTypeFilter"].([]any)
	if !ok || len(typeFilter) != 2 || typeFilter[0] != "fisica" || typeFilter[1] != "empleado_publico" {
		t.Fatalf("certificateTypeFilter no persistido: %#v", salida)
	}
	if salida["profile"] != "B" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func TestGuardar_CreaDirectorio(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "sub", "config")
	almacen := usersettings.New(dir)

	if err := almacen.Guardar(context.Background(), map[string]any{"x": 1}); err != nil {
		t.Fatalf("Guardar con directorio nuevo: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("directorio no creado: %v", err)
	}
}

func TestCargar_RechazaSettingsEnlaceSimbolico(t *testing.T) {
	dir := t.TempDir()
	destino := filepath.Join(dir, "destino.json")
	if err := os.WriteFile(destino, []byte(`{"tema":"atacante"}`), 0o600); err != nil {
		t.Fatalf("no se pudo preparar el destino: %v", err)
	}
	configDir := filepath.Join(dir, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatalf("no se pudo crear configDir: %v", err)
	}
	if err := os.Symlink(destino, filepath.Join(configDir, "settings.json")); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}

	almacen := usersettings.New(configDir)
	if _, err := almacen.Cargar(context.Background()); err == nil {
		t.Fatal("Cargar acepto settings.json como enlace simbolico")
	}
}

func TestGuardar_SobreescribeExistente(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	_ = almacen.Guardar(context.Background(), map[string]any{"a": "1"})
	_ = almacen.Guardar(context.Background(), map[string]any{"b": "2"})

	datos, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}
	if _, ok := datos["a"]; ok {
		t.Error("segunda escritura deberia sobreescribir la primera")
	}
	if datos["b"] != "2" {
		t.Errorf("clave b: got %v", datos["b"])
	}
}

func TestCargarDocumento_ProyectaBloqueGeneralYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"idioma":              "es",
		"themeIndex":          2,
		"expertMode":          true,
		"autoClose":           false,
		"confirmToSign":       true,
		"omitAskOnClose":      true,
		"closeBehavior":       "resident",
		"hideDnieStartScreen": true,
		"secureConnections":   true,
		"secureDomainsList":   []any{" https://sede.dipgra.es ", "*.dipgra.es", "https://sede.dipgra.es"},
		"proxyHost":           "proxy.local",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.General.Idioma == nil || *doc.General.Idioma != "es" {
		t.Fatalf("idioma tipado inesperado: %#v", doc.General.Idioma)
	}
	if doc.General.ThemeIndex == nil || *doc.General.ThemeIndex != 2 {
		t.Fatalf("themeIndex tipado inesperado: %#v", doc.General.ThemeIndex)
	}
	if doc.General.ExpertMode == nil || !*doc.General.ExpertMode {
		t.Fatalf("expertMode tipado inesperado: %#v", doc.General.ExpertMode)
	}
	if doc.General.AutoClose == nil || *doc.General.AutoClose {
		t.Fatalf("autoClose tipado inesperado: %#v", doc.General.AutoClose)
	}
	if doc.General.ConfirmToSign == nil || !*doc.General.ConfirmToSign {
		t.Fatalf("confirmToSign tipado inesperado: %#v", doc.General.ConfirmToSign)
	}
	if doc.General.OmitAskOnClose == nil || !*doc.General.OmitAskOnClose {
		t.Fatalf("omitAskOnClose tipado inesperado: %#v", doc.General.OmitAskOnClose)
	}
	if doc.General.CloseBehavior == nil || *doc.General.CloseBehavior != "resident" {
		t.Fatalf("closeBehavior tipado inesperado: %#v", doc.General.CloseBehavior)
	}
	if doc.General.HideDnieStartScreen == nil || !*doc.General.HideDnieStartScreen {
		t.Fatalf("hideDnieStartScreen tipado inesperado: %#v", doc.General.HideDnieStartScreen)
	}
	if doc.General.SecureConnections == nil || !*doc.General.SecureConnections {
		t.Fatalf("secureConnections tipado inesperado: %#v", doc.General.SecureConnections)
	}
	if len(doc.General.SecureDomainsList) != 2 || doc.General.SecureDomainsList[0] != "https://sede.dipgra.es" || doc.General.SecureDomainsList[1] != "*.dipgra.es" {
		t.Fatalf("secureDomainsList tipado inesperado: %#v", doc.General.SecureDomainsList)
	}
	if doc.Proxy.Host == nil || *doc.Proxy.Host != "proxy.local" {
		t.Fatalf("proxy host tipado inesperado: %#v", doc.Proxy.Host)
	}
	if _, ok := doc.Extras["proxyHost"]; ok {
		t.Fatal("proxyHost no deberia permanecer en extras")
	}
}

func TestCargarDocumento_ProyectaBloquesCAdESYXAdESYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"cadesPolicyIdentifier":              "urn:oid:1.2.3.4.5",
		"cadesPolicyIdentifierHash":          "YWJjMTIz",
		"cadesPolicyIdentifierHashAlgorithm": "SHA-256",
		"cadesPolicyQualifier":               "https://policy.example/cades",
		"cadesImplicitMode":                  true,
		"cadesMultisign":                     "cosign",
		"xadesPolicyIdentifier":              "https://policy.example/xades",
		"xadesPolicyIdentifierHash":          "ZGVmNDU2",
		"xadesPolicyIdentifierHashAlgorithm": "SHA-512",
		"xadesPolicyQualifier":               "urn:oid:2.3.4.5.6",
		"xadesSignFormat":                    "XAdES Detached",
		"xadesMultisign":                     "countersign",
		"xadesSignerClaimedRole":             "apoderado",
		"xadesSignatureProductionCity":       "Granada",
		"xadesSignatureProductionProvince":   "Granada",
		"xadesSignatureProductionPostalCode": "18001",
		"xadesSignatureProductionCountry":    "ES",
		"legacyExtra":                        "keep-me",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.CAdES.PolicyID == nil || *doc.CAdES.PolicyID != "urn:oid:1.2.3.4.5" {
		t.Fatalf("cades policy id inesperado: %#v", doc.CAdES.PolicyID)
	}
	if doc.CAdES.PolicyHash == nil || *doc.CAdES.PolicyHash != "YWJjMTIz" {
		t.Fatalf("cades policy hash inesperado: %#v", doc.CAdES.PolicyHash)
	}
	if doc.CAdES.PolicyHashAlgorithm == nil || *doc.CAdES.PolicyHashAlgorithm != "SHA-256" {
		t.Fatalf("cades policy hash algorithm inesperado: %#v", doc.CAdES.PolicyHashAlgorithm)
	}
	if doc.CAdES.PolicyQualifier == nil || *doc.CAdES.PolicyQualifier != "https://policy.example/cades" {
		t.Fatalf("cades policy qualifier inesperado: %#v", doc.CAdES.PolicyQualifier)
	}
	if doc.CAdES.ImplicitMode == nil || !*doc.CAdES.ImplicitMode {
		t.Fatalf("cades implicit inesperado: %#v", doc.CAdES.ImplicitMode)
	}
	if doc.CAdES.Multisign == nil || *doc.CAdES.Multisign != "cosign" {
		t.Fatalf("cades multisign inesperado: %#v", doc.CAdES.Multisign)
	}

	if doc.XAdES.PolicyID == nil || *doc.XAdES.PolicyID != "https://policy.example/xades" {
		t.Fatalf("xades policy id inesperado: %#v", doc.XAdES.PolicyID)
	}
	if doc.XAdES.PolicyHash == nil || *doc.XAdES.PolicyHash != "ZGVmNDU2" {
		t.Fatalf("xades policy hash inesperado: %#v", doc.XAdES.PolicyHash)
	}
	if doc.XAdES.PolicyHashAlgorithm == nil || *doc.XAdES.PolicyHashAlgorithm != "SHA-512" {
		t.Fatalf("xades policy hash algorithm inesperado: %#v", doc.XAdES.PolicyHashAlgorithm)
	}
	if doc.XAdES.PolicyQualifier == nil || *doc.XAdES.PolicyQualifier != "urn:oid:2.3.4.5.6" {
		t.Fatalf("xades policy qualifier inesperado: %#v", doc.XAdES.PolicyQualifier)
	}
	if doc.XAdES.SignFormat == nil || *doc.XAdES.SignFormat != "detached" {
		t.Fatalf("xades sign format inesperado: %#v", doc.XAdES.SignFormat)
	}
	if doc.XAdES.Multisign == nil || *doc.XAdES.Multisign != "countersign" {
		t.Fatalf("xades multisign inesperado: %#v", doc.XAdES.Multisign)
	}
	if doc.XAdES.ClaimedRole == nil || *doc.XAdES.ClaimedRole != "apoderado" {
		t.Fatalf("xades claimed role inesperado: %#v", doc.XAdES.ClaimedRole)
	}
	if doc.XAdES.City == nil || *doc.XAdES.City != "Granada" {
		t.Fatalf("xades city inesperada: %#v", doc.XAdES.City)
	}
	if doc.XAdES.Province == nil || *doc.XAdES.Province != "Granada" {
		t.Fatalf("xades province inesperada: %#v", doc.XAdES.Province)
	}
	if doc.XAdES.PostalCode == nil || *doc.XAdES.PostalCode != "18001" {
		t.Fatalf("xades postal code inesperado: %#v", doc.XAdES.PostalCode)
	}
	if doc.XAdES.Country == nil || *doc.XAdES.Country != "ES" {
		t.Fatalf("xades country inesperado: %#v", doc.XAdES.Country)
	}
	if doc.Extras["legacyExtra"] != "keep-me" {
		t.Fatalf("extras no preservados: %#v", doc.Extras)
	}
}

func TestGuardarDocumento_RoundtripBloquesCAdESYXAdES(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	doc := ports.DocumentoConfiguracionUsuario{
		CAdES: ports.ConfiguracionUsuarioCAdES{
			PolicyID:            ptrString("https://policy.example/cades"),
			PolicyHash:          ptrString("YWJjMTIz"),
			PolicyHashAlgorithm: ptrString("SHA-384"),
			PolicyQualifier:     ptrString("urn:oid:1.2.3.4"),
			ImplicitMode:        ptrBool(false),
			Multisign:           ptrString("countersign"),
		},
		XAdES: ports.ConfiguracionUsuarioXAdES{
			PolicyID:            ptrString("urn:oid:9.9.9"),
			PolicyHash:          ptrString("eHl6"),
			PolicyHashAlgorithm: ptrString("SHA-256"),
			PolicyQualifier:     ptrString("https://policy.example/xades"),
			SignFormat:          ptrString("enveloped"),
			Multisign:           ptrString("cosign"),
			ClaimedRole:         ptrString("representante"),
			City:                ptrString("Granada"),
			Province:            ptrString("Granada"),
			PostalCode:          ptrString("18001"),
			Country:             ptrString("ES"),
		},
		Extras: map[string]any{
			"legacyExtra": "keep-me",
		},
	}
	if err := almacen.GuardarDocumento(context.Background(), doc); err != nil {
		t.Fatalf("GuardarDocumento: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["cadesPolicyIdentifier"] != "https://policy.example/cades" ||
		salida["cadesPolicyIdentifierHash"] != "YWJjMTIz" ||
		salida["cadesPolicyIdentifierHashAlgorithm"] != "SHA-384" ||
		salida["cadesPolicyQualifier"] != "urn:oid:1.2.3.4" ||
		salida["cadesImplicitMode"] != false ||
		salida["cadesMultisign"] != "countersign" {
		t.Fatalf("bloque cades no persistido: %#v", salida)
	}
	if salida["xadesPolicyIdentifier"] != "urn:oid:9.9.9" ||
		salida["xadesPolicyIdentifierHash"] != "eHl6" ||
		salida["xadesPolicyIdentifierHashAlgorithm"] != "SHA-256" ||
		salida["xadesPolicyQualifier"] != "https://policy.example/xades" ||
		salida["xadesSignFormat"] != "enveloped" ||
		salida["xadesMultisign"] != "cosign" ||
		salida["xadesSignerClaimedRole"] != "representante" ||
		salida["xadesSignatureProductionCity"] != "Granada" ||
		salida["xadesSignatureProductionProvince"] != "Granada" ||
		salida["xadesSignatureProductionPostalCode"] != "18001" ||
		salida["xadesSignatureProductionCountry"] != "ES" {
		t.Fatalf("bloque xades no persistido: %#v", salida)
	}
	if salida["legacyExtra"] != "keep-me" {
		t.Fatalf("extras no preservados: %#v", salida)
	}
}

func ptrString(v string) *string { return &v }
func ptrBool(v bool) *bool       { return &v }

func TestCargarDocumento_ProyectaBloqueFirmaYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signAction":                    "sign",
		"signFormat":                    "pades",
		"signProfile":                   "baseline",
		"signOverwrite":                 "rename",
		"signStrictCompat":              true,
		"signAllowInvalidPDF":           false,
		"signReason":                    "Aprobación interna",
		"signLocation":                  "Granada",
		"signContactInfo":               "contacto@example.invalid",
		"facturaePolicyVersion":         "3.1",
		"policyIdentifier":              "https://www.facturae.gob.es/politica.pdf",
		"policyIdentifierHash":          "ZmFrZS1oYXNo",
		"policyQualifier":               "https://www.facturae.gob.es/politica.html",
		"signerClaimedRole":             "emisor",
		"signatureProductionCity":       "Granada",
		"signatureProductionProvince":   "Granada",
		"signatureProductionPostalCode": "18014",
		"signatureProductionCountry":    "ES",
		"padesSubFilter":                "adobe",
		"tsaEnabled":                    true,
		"tsaUrl":                        "https://tsa.local",
		"profile":                       "B",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Firma.Action == nil || *doc.Firma.Action != "sign" {
		t.Fatalf("action tipada inesperada: %#v", doc.Firma.Action)
	}
	if doc.Firma.Format == nil || *doc.Firma.Format != "pades" {
		t.Fatalf("format tipado inesperado: %#v", doc.Firma.Format)
	}
	if doc.Firma.Profile == nil || *doc.Firma.Profile != "baseline" {
		t.Fatalf("profile tipado inesperado: %#v", doc.Firma.Profile)
	}
	if doc.Firma.Overwrite == nil || *doc.Firma.Overwrite != "rename" {
		t.Fatalf("overwrite tipado inesperado: %#v", doc.Firma.Overwrite)
	}
	if doc.Firma.StrictCompat == nil || !*doc.Firma.StrictCompat {
		t.Fatalf("strictCompat tipado inesperado: %#v", doc.Firma.StrictCompat)
	}
	if doc.Firma.AllowInvalidPDF == nil || *doc.Firma.AllowInvalidPDF {
		t.Fatalf("allowInvalidPDF tipado inesperado: %#v", doc.Firma.AllowInvalidPDF)
	}
	if doc.FirmaMeta.Reason == nil || *doc.FirmaMeta.Reason != "Aprobación interna" {
		t.Fatalf("reason tipada inesperada: %#v", doc.FirmaMeta.Reason)
	}
	if doc.FirmaMeta.Location == nil || *doc.FirmaMeta.Location != "Granada" {
		t.Fatalf("location tipada inesperada: %#v", doc.FirmaMeta.Location)
	}
	if doc.FirmaMeta.ContactInfo == nil || *doc.FirmaMeta.ContactInfo != "contacto@example.invalid" {
		t.Fatalf("contactInfo tipada inesperada: %#v", doc.FirmaMeta.ContactInfo)
	}
	if doc.FacturaE.PolicyVersion == nil || *doc.FacturaE.PolicyVersion != "3.1" {
		t.Fatalf("facturaePolicyVersion tipada inesperada: %#v", doc.FacturaE.PolicyVersion)
	}
	if doc.FacturaE.PolicyID == nil || *doc.FacturaE.PolicyID != "https://www.facturae.gob.es/politica.pdf" {
		t.Fatalf("policyIdentifier tipado inesperado: %#v", doc.FacturaE.PolicyID)
	}
	if doc.FacturaE.PolicyHash == nil || *doc.FacturaE.PolicyHash != "ZmFrZS1oYXNo" {
		t.Fatalf("policyIdentifierHash tipado inesperado: %#v", doc.FacturaE.PolicyHash)
	}
	if doc.FacturaE.PolicyQualifier == nil || *doc.FacturaE.PolicyQualifier != "https://www.facturae.gob.es/politica.html" {
		t.Fatalf("policyQualifier tipado inesperado: %#v", doc.FacturaE.PolicyQualifier)
	}
	if doc.FacturaE.SignerRole == nil || *doc.FacturaE.SignerRole != "emisor" {
		t.Fatalf("signerClaimedRole tipado inesperado: %#v", doc.FacturaE.SignerRole)
	}
	if doc.FacturaE.City == nil || *doc.FacturaE.City != "Granada" {
		t.Fatalf("signatureProductionCity tipada inesperada: %#v", doc.FacturaE.City)
	}
	if doc.FacturaE.Province == nil || *doc.FacturaE.Province != "Granada" {
		t.Fatalf("signatureProductionProvince tipada inesperada: %#v", doc.FacturaE.Province)
	}
	if doc.FacturaE.PostalCode == nil || *doc.FacturaE.PostalCode != "18014" {
		t.Fatalf("signatureProductionPostalCode tipada inesperada: %#v", doc.FacturaE.PostalCode)
	}
	if doc.FacturaE.Country == nil || *doc.FacturaE.Country != "ES" {
		t.Fatalf("signatureProductionCountry tipada inesperada: %#v", doc.FacturaE.Country)
	}
	if doc.PAdES.SubFilter == nil || *doc.PAdES.SubFilter != "adobe" {
		t.Fatalf("padesSubFilter tipado inesperado: %#v", doc.PAdES.SubFilter)
	}
	if doc.TSA.URL == nil || *doc.TSA.URL != "https://tsa.local" {
		t.Fatalf("tsa url tipada inesperada: %#v", doc.TSA.URL)
	}
	if doc.TSA.Enabled == nil || !*doc.TSA.Enabled {
		t.Fatalf("tsa enabled tipada inesperada: %#v", doc.TSA.Enabled)
	}
	if got := doc.Extras["profile"]; got != "B" {
		t.Fatalf("extras no preservados: got %v", got)
	}
	if _, ok := doc.Extras["signFormat"]; ok {
		t.Fatal("signFormat no deberia permanecer en extras")
	}
}

func TestCargarDocumento_ProyectaBloqueFormatosAutomaticosYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"autoFormatPdf":      "pades",
		"autoFormatOoxml":    "ooxml",
		"autoFormatFacturae": "facturae",
		"autoFormatOdf":      "odf",
		"autoFormatXml":      "xmldsig",
		"autoFormatBinary":   "cades",
		"profile":            "B",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.FormatosAuto.PDF == nil || *doc.FormatosAuto.PDF != "pades" {
		t.Fatalf("autoFormatPdf tipado inesperado: %#v", doc.FormatosAuto.PDF)
	}
	if doc.FormatosAuto.OOXML == nil || *doc.FormatosAuto.OOXML != "ooxml" {
		t.Fatalf("autoFormatOoxml tipado inesperado: %#v", doc.FormatosAuto.OOXML)
	}
	if doc.FormatosAuto.FacturaE == nil || *doc.FormatosAuto.FacturaE != "facturae" {
		t.Fatalf("autoFormatFacturae tipado inesperado: %#v", doc.FormatosAuto.FacturaE)
	}
	if doc.FormatosAuto.ODF == nil || *doc.FormatosAuto.ODF != "odf" {
		t.Fatalf("autoFormatOdf tipado inesperado: %#v", doc.FormatosAuto.ODF)
	}
	if doc.FormatosAuto.XML == nil || *doc.FormatosAuto.XML != "xmldsig" {
		t.Fatalf("autoFormatXml tipado inesperado: %#v", doc.FormatosAuto.XML)
	}
	if doc.FormatosAuto.Binary == nil || *doc.FormatosAuto.Binary != "cades" {
		t.Fatalf("autoFormatBinary tipado inesperado: %#v", doc.FormatosAuto.Binary)
	}
	if got := doc.Extras["profile"]; got != "B" {
		t.Fatalf("extras no preservados: got %v", got)
	}
	if _, ok := doc.Extras["autoFormatPdf"]; ok {
		t.Fatal("autoFormatPdf no deberia permanecer en extras")
	}
}

func TestCargarDocumento_CanonizaPerfilYSubfiltroPAdES(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signProfile":    "B",
		"padesSubFilter": "adbe.pkcs7.detached",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Firma.Profile == nil || *doc.Firma.Profile != "baseline" {
		t.Fatalf("signProfile tipado inesperado: %#v", doc.Firma.Profile)
	}
	if doc.PAdES.SubFilter == nil || *doc.PAdES.SubFilter != "adobe" {
		t.Fatalf("padesSubFilter tipado inesperado: %#v", doc.PAdES.SubFilter)
	}
	if _, ok := doc.Extras["signProfile"]; ok {
		t.Fatalf("signProfile no deberia permanecer en extras: %#v", doc.Extras)
	}
	if _, ok := doc.Extras["padesSubFilter"]; ok {
		t.Fatalf("padesSubFilter no deberia permanecer en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_ProyectaPoliticaPAdESYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"padesBasicFormat":                   "ETSI.CAdES.detached",
		"padesPolicyIdentifier":              "https://politica.pades.local/id",
		"padesPolicyIdentifierHash":          "c2hhMjU2",
		"padesPolicyIdentifierHashAlgorithm": "sha-256",
		"padesPolicyQualifier":               "https://politica.pades.local/info",
		"padesObfuscateCertInfo":             true,
		"padesVisibleStamp":                  false,
		"allowShadowAttack":                  false,
		"allowCertifiedPDF":                  true,
		"padesCertificationLevel":            "2",
		"profile":                            "B",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.PAdES.SubFilter == nil || *doc.PAdES.SubFilter != "etsi" {
		t.Fatalf("subfilter PAdES tipado inesperado: %#v", doc.PAdES.SubFilter)
	}
	if doc.PAdES.PolicyID == nil || *doc.PAdES.PolicyID != "https://politica.pades.local/id" {
		t.Fatalf("policyId PAdES tipado inesperado: %#v", doc.PAdES.PolicyID)
	}
	if doc.PAdES.PolicyHash == nil || *doc.PAdES.PolicyHash != "c2hhMjU2" {
		t.Fatalf("policyHash PAdES tipado inesperado: %#v", doc.PAdES.PolicyHash)
	}
	if doc.PAdES.PolicyHashAlgorithm == nil || *doc.PAdES.PolicyHashAlgorithm != "SHA-256" {
		t.Fatalf("policyHashAlgorithm PAdES tipado inesperado: %#v", doc.PAdES.PolicyHashAlgorithm)
	}
	if doc.PAdES.PolicyQualifier == nil || *doc.PAdES.PolicyQualifier != "https://politica.pades.local/info" {
		t.Fatalf("policyQualifier PAdES tipado inesperado: %#v", doc.PAdES.PolicyQualifier)
	}
	if doc.PAdES.ObfuscateCertInfo == nil || !*doc.PAdES.ObfuscateCertInfo {
		t.Fatalf("obfuscateCertInfo PAdES tipado inesperado: %#v", doc.PAdES.ObfuscateCertInfo)
	}
	if doc.PAdES.VisibleStamp == nil || *doc.PAdES.VisibleStamp {
		t.Fatalf("visibleStamp PAdES tipado inesperado: %#v", doc.PAdES.VisibleStamp)
	}
	if doc.PAdES.AllowShadowAttack == nil || *doc.PAdES.AllowShadowAttack {
		t.Fatalf("allowShadowAttack PAdES tipado inesperado: %#v", doc.PAdES.AllowShadowAttack)
	}
	if doc.PAdES.AllowCertifiedPDF == nil || !*doc.PAdES.AllowCertifiedPDF {
		t.Fatalf("allowCertifiedPDF PAdES tipado inesperado: %#v", doc.PAdES.AllowCertifiedPDF)
	}
	if doc.PAdES.CertificationLevel == nil || *doc.PAdES.CertificationLevel != 2 {
		t.Fatalf("certificationLevel PAdES tipado inesperado: %#v", doc.PAdES.CertificationLevel)
	}
	if got := doc.Extras["profile"]; got != "B" {
		t.Fatalf("extras no preservados: got %v", got)
	}
	for _, clave := range []string{
		"padesBasicFormat",
		"padesPolicyIdentifier",
		"padesPolicyIdentifierHash",
		"padesPolicyIdentifierHashAlgorithm",
		"padesPolicyQualifier",
		"padesObfuscateCertInfo",
		"padesVisibleStamp",
		"allowShadowAttack",
		"allowCertifiedPDF",
		"padesCertificationLevel",
	} {
		if _, ok := doc.Extras[clave]; ok {
			t.Fatalf("%s no deberia permanecer en extras: %#v", clave, doc.Extras)
		}
	}
}

func TestCargarDocumento_CanonizaEnumsDeFirmaHashYFormato(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"closeBehavior":              "RESIDENT",
		"signAction":                 "COSIGN",
		"signFormat":                 " PAdES ",
		"signOverwrite":              "FORCE",
		"autoFormatPdf":              " CAdES ",
		"autoFormatOoxml":            " OOXML ",
		"autoFormatFacturae":         " XADES ",
		"autoFormatOdf":              " CAdES ",
		"autoFormatXml":              " XMLDSIG ",
		"autoFormatBinary":           " ASIC-XADES ",
		"defaultHashAlgorithm":       "sha-384",
		"defaultHashFormatFile":      "BASE64",
		"defaultHashFormatDirectory": " TXT ",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.General.CloseBehavior == nil || *doc.General.CloseBehavior != "resident" {
		t.Fatalf("closeBehavior tipado inesperado: %#v", doc.General.CloseBehavior)
	}
	if doc.Firma.Action == nil || *doc.Firma.Action != "cosign" {
		t.Fatalf("signAction tipada inesperada: %#v", doc.Firma.Action)
	}
	if doc.Firma.Format == nil || *doc.Firma.Format != "pades" {
		t.Fatalf("signFormat tipado inesperado: %#v", doc.Firma.Format)
	}
	if doc.Firma.Overwrite == nil || *doc.Firma.Overwrite != "force" {
		t.Fatalf("signOverwrite tipado inesperado: %#v", doc.Firma.Overwrite)
	}
	if doc.FormatosAuto.PDF == nil || *doc.FormatosAuto.PDF != "cades" {
		t.Fatalf("autoFormatPdf tipado inesperado: %#v", doc.FormatosAuto.PDF)
	}
	if doc.FormatosAuto.OOXML == nil || *doc.FormatosAuto.OOXML != "ooxml" {
		t.Fatalf("autoFormatOoxml tipado inesperado: %#v", doc.FormatosAuto.OOXML)
	}
	if doc.FormatosAuto.FacturaE == nil || *doc.FormatosAuto.FacturaE != "xades" {
		t.Fatalf("autoFormatFacturae tipado inesperado: %#v", doc.FormatosAuto.FacturaE)
	}
	if doc.FormatosAuto.ODF == nil || *doc.FormatosAuto.ODF != "cades" {
		t.Fatalf("autoFormatOdf tipado inesperado: %#v", doc.FormatosAuto.ODF)
	}
	if doc.FormatosAuto.XML == nil || *doc.FormatosAuto.XML != "xmldsig" {
		t.Fatalf("autoFormatXml tipado inesperado: %#v", doc.FormatosAuto.XML)
	}
	if doc.FormatosAuto.Binary == nil || *doc.FormatosAuto.Binary != "asic-xades" {
		t.Fatalf("autoFormatBinary tipado inesperado: %#v", doc.FormatosAuto.Binary)
	}
	if doc.Hash.Algorithm == nil || *doc.Hash.Algorithm != "SHA-384" {
		t.Fatalf("defaultHashAlgorithm tipado inesperado: %#v", doc.Hash.Algorithm)
	}
	if doc.Hash.FormatFile == nil || *doc.Hash.FormatFile != "base64" {
		t.Fatalf("defaultHashFormatFile tipado inesperado: %#v", doc.Hash.FormatFile)
	}
	if doc.Hash.FormatDirectory == nil || *doc.Hash.FormatDirectory != "txt" {
		t.Fatalf("defaultHashFormatDirectory tipado inesperado: %#v", doc.Hash.FormatDirectory)
	}
	for _, clave := range []string{
		"closeBehavior",
		"signAction",
		"signFormat",
		"signOverwrite",
		"autoFormatPdf",
		"autoFormatOoxml",
		"autoFormatFacturae",
		"autoFormatOdf",
		"autoFormatXml",
		"autoFormatBinary",
		"defaultHashAlgorithm",
		"defaultHashFormatFile",
		"defaultHashFormatDirectory",
	} {
		if _, ok := doc.Extras[clave]; ok {
			t.Fatalf("%s no deberia permanecer en extras: %#v", clave, doc.Extras)
		}
	}
}

func TestCargarDocumento_DejaEnExtrasPerfilYSubfiltroInvalidos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signProfile":    "modo-raro",
		"padesSubFilter": "subfiltro-raro",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Firma.Profile != nil {
		t.Fatalf("signProfile tipado inesperado: %#v", doc.Firma.Profile)
	}
	if doc.PAdES.SubFilter != nil {
		t.Fatalf("padesSubFilter tipado inesperado: %#v", doc.PAdES.SubFilter)
	}
	if doc.Extras["signProfile"] != "modo-raro" {
		t.Fatalf("signProfile invalido deberia quedar en extras: %#v", doc.Extras)
	}
	if doc.Extras["padesSubFilter"] != "subfiltro-raro" {
		t.Fatalf("padesSubFilter invalido deberia quedar en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_DejaEnExtrasEnumsInvalidos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"closeBehavior":                      "tray",
		"signAction":                         "firmar-raro",
		"signFormat":                         "pdf",
		"signOverwrite":                      "skip",
		"autoFormatPdf":                      "xmldsig",
		"autoFormatOoxml":                    "xades",
		"autoFormatFacturae":                 "pades",
		"autoFormatOdf":                      "xades",
		"autoFormatXml":                      "pades",
		"autoFormatBinary":                   "xmldsig",
		"defaultHashAlgorithm":               "MD5",
		"defaultHashFormatFile":              "raw",
		"defaultHashFormatDirectory":         "json",
		"padesPolicyIdentifierHashAlgorithm": "sha3-256",
		"padesCertificationLevel":            "8",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.General.CloseBehavior != nil || doc.Firma.Action != nil || doc.Firma.Format != nil || doc.Firma.Overwrite != nil {
		t.Fatalf("enums de firma no deberian tiparse: %#v", doc)
	}
	if doc.FormatosAuto.PDF != nil || doc.FormatosAuto.OOXML != nil || doc.FormatosAuto.FacturaE != nil || doc.FormatosAuto.ODF != nil || doc.FormatosAuto.XML != nil || doc.FormatosAuto.Binary != nil {
		t.Fatalf("autoformatos invalidos no deberian tiparse: %#v", doc.FormatosAuto)
	}
	if doc.Hash.Algorithm != nil || doc.Hash.FormatFile != nil || doc.Hash.FormatDirectory != nil {
		t.Fatalf("formatos hash invalidos no deberian tiparse: %#v", doc.Hash)
	}
	if doc.PAdES.PolicyHashAlgorithm != nil || doc.PAdES.CertificationLevel != nil {
		t.Fatalf("politica PAdES invalida no deberia tiparse: %#v", doc.PAdES)
	}
	for clave, esperado := range entrada {
		if got := doc.Extras[clave]; got != esperado {
			t.Fatalf("%s invalido deberia quedar en extras: got=%#v want=%#v", clave, got, esperado)
		}
	}
}

func TestCargarDocumento_CanonizaURIsDePoliticaYTSA(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"tsaUrl":                " https://tsa.local/endpoint ",
		"policyIdentifier":      " urn:oid:1.2.3.4.5 ",
		"policyQualifier":       " https://www.facturae.gob.es/politica.html ",
		"padesPolicyIdentifier": " https://politica.pades.local/id ",
		"padesPolicyQualifier":  " urn:oid:1.2.840.113549.1.9.16.6.1 ",
		"profile":               "B",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.TSA.URL == nil || *doc.TSA.URL != "https://tsa.local/endpoint" {
		t.Fatalf("tsaUrl tipada inesperada: %#v", doc.TSA.URL)
	}
	if doc.FacturaE.PolicyID == nil || *doc.FacturaE.PolicyID != "urn:oid:1.2.3.4.5" {
		t.Fatalf("policyIdentifier tipado inesperado: %#v", doc.FacturaE.PolicyID)
	}
	if doc.FacturaE.PolicyQualifier == nil || *doc.FacturaE.PolicyQualifier != "https://www.facturae.gob.es/politica.html" {
		t.Fatalf("policyQualifier tipado inesperado: %#v", doc.FacturaE.PolicyQualifier)
	}
	if doc.PAdES.PolicyID == nil || *doc.PAdES.PolicyID != "https://politica.pades.local/id" {
		t.Fatalf("padesPolicyIdentifier tipado inesperado: %#v", doc.PAdES.PolicyID)
	}
	if doc.PAdES.PolicyQualifier == nil || *doc.PAdES.PolicyQualifier != "urn:oid:1.2.840.113549.1.9.16.6.1" {
		t.Fatalf("padesPolicyQualifier tipado inesperado: %#v", doc.PAdES.PolicyQualifier)
	}
	for _, clave := range []string{"tsaUrl", "policyIdentifier", "policyQualifier", "padesPolicyIdentifier", "padesPolicyQualifier"} {
		if _, ok := doc.Extras[clave]; ok {
			t.Fatalf("%s no deberia permanecer en extras: %#v", clave, doc.Extras)
		}
	}
	if got := doc.Extras["profile"]; got != "B" {
		t.Fatalf("extras no preservados: got %v", got)
	}
}

func TestCargarDocumento_DejaEnExtrasURIsInvalidasDePoliticaYTSA(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"tsaUrl":                "ftp://tsa.local",
		"policyIdentifier":      "politica-sin-uri",
		"policyQualifier":       "/ruta/relativa",
		"padesPolicyIdentifier": "nota-interna",
		"padesPolicyQualifier":  "javascript:alert(1)",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.TSA.URL != nil || doc.FacturaE.PolicyID != nil || doc.FacturaE.PolicyQualifier != nil || doc.PAdES.PolicyID != nil || doc.PAdES.PolicyQualifier != nil {
		t.Fatalf("URIs invalidas no deberian tiparse: %#v", doc)
	}
	for clave, esperado := range entrada {
		if got := doc.Extras[clave]; got != esperado {
			t.Fatalf("%s invalida deberia quedar en extras: got=%#v want=%#v", clave, got, esperado)
		}
	}
}

func TestCargarDocumento_ValidaBloqueTSAYMetadatosFirma(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"tsaUrl":          "   ",
		"tsaEnabled":      true,
		"signReason":      "  ",
		"signLocation":    "\t",
		"signContactInfo": "\n",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.TSA.URL != nil {
		t.Fatalf("tsa url vacia no deberia tiparse: %#v", doc.TSA.URL)
	}
	if doc.TSA.Enabled == nil || !*doc.TSA.Enabled {
		t.Fatalf("tsa enabled deberia tiparse: %#v", doc.TSA.Enabled)
	}
	if doc.FirmaMeta.Reason == nil || *doc.FirmaMeta.Reason != "" {
		t.Fatalf("signReason vacio deberia normalizarse a cadena vacia: %#v", doc.FirmaMeta.Reason)
	}
	if doc.FirmaMeta.Location == nil || *doc.FirmaMeta.Location != "" {
		t.Fatalf("signLocation vacio deberia normalizarse a cadena vacia: %#v", doc.FirmaMeta.Location)
	}
	if doc.FirmaMeta.ContactInfo == nil || *doc.FirmaMeta.ContactInfo != "" {
		t.Fatalf("signContactInfo vacio deberia normalizarse a cadena vacia: %#v", doc.FirmaMeta.ContactInfo)
	}
	if doc.Extras["tsaUrl"] != "   " {
		t.Fatalf("tsaUrl vacia no preservada en extras: %#v", doc.Extras)
	}
	for _, clave := range []string{"signReason", "signLocation", "signContactInfo"} {
		if _, ok := doc.Extras[clave]; ok {
			t.Fatalf("%s no deberia quedar en extras tras normalizarse: %#v", clave, doc.Extras)
		}
	}
}

func TestCargarDocumento_NormalizaTextoLibreFacturaEAVacio(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"facturaePolicyVersion":         "   ",
		"signerClaimedRole":             "\t",
		"signatureProductionCity":       "\n",
		"signatureProductionProvince":   "  ",
		"signatureProductionPostalCode": "",
		"signatureProductionCountry":    "   ",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.FacturaE.PolicyVersion == nil || *doc.FacturaE.PolicyVersion != "" {
		t.Fatalf("facturaePolicyVersion vacia deberia normalizarse: %#v", doc.FacturaE.PolicyVersion)
	}
	if doc.FacturaE.SignerRole == nil || *doc.FacturaE.SignerRole != "" {
		t.Fatalf("signerClaimedRole vacio deberia normalizarse: %#v", doc.FacturaE.SignerRole)
	}
	if doc.FacturaE.City == nil || *doc.FacturaE.City != "" {
		t.Fatalf("signatureProductionCity vacia deberia normalizarse: %#v", doc.FacturaE.City)
	}
	if doc.FacturaE.Province == nil || *doc.FacturaE.Province != "" {
		t.Fatalf("signatureProductionProvince vacia deberia normalizarse: %#v", doc.FacturaE.Province)
	}
	if doc.FacturaE.PostalCode == nil || *doc.FacturaE.PostalCode != "" {
		t.Fatalf("signatureProductionPostalCode vacia deberia normalizarse: %#v", doc.FacturaE.PostalCode)
	}
	if doc.FacturaE.Country == nil || *doc.FacturaE.Country != "" {
		t.Fatalf("signatureProductionCountry vacia deberia normalizarse: %#v", doc.FacturaE.Country)
	}
	for _, clave := range []string{"facturaePolicyVersion", "signerClaimedRole", "signatureProductionCity", "signatureProductionProvince", "signatureProductionPostalCode", "signatureProductionCountry"} {
		if _, ok := doc.Extras[clave]; ok {
			t.Fatalf("%s no deberia quedar en extras tras normalizarse: %#v", clave, doc.Extras)
		}
	}
}

func TestCargarDocumento_ProyectaBloquePAdESVisibleYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signVisibleSeal":   true,
		"signSealPages":     "1,3-5",
		"signSealAllPages":  false,
		"signSealX":         0.62,
		"signSealY":         0.04,
		"signSealW":         0.34,
		"signSealH":         0.12,
		"signSealKeepText":  true,
		"signSealRotation":  180,
		"signSealImagePath": " /tmp/sello.png ",
		"signQRContent":     " https://verifica.local/expediente/123 ",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.PAdESVisible.Enabled == nil || !*doc.PAdESVisible.Enabled {
		t.Fatalf("enabled tipado inesperado: %#v", doc.PAdESVisible.Enabled)
	}
	if doc.PAdESVisible.Pages == nil || *doc.PAdESVisible.Pages != "1,3-5" {
		t.Fatalf("pages tipadas inesperadas: %#v", doc.PAdESVisible.Pages)
	}
	if doc.PAdESVisible.AllPages == nil || *doc.PAdESVisible.AllPages {
		t.Fatalf("allPages tipado inesperado: %#v", doc.PAdESVisible.AllPages)
	}
	if doc.PAdESVisible.X == nil || *doc.PAdESVisible.X != 0.62 {
		t.Fatalf("x tipada inesperada: %#v", doc.PAdESVisible.X)
	}
	if doc.PAdESVisible.Y == nil || *doc.PAdESVisible.Y != 0.04 {
		t.Fatalf("y tipada inesperada: %#v", doc.PAdESVisible.Y)
	}
	if doc.PAdESVisible.W == nil || *doc.PAdESVisible.W != 0.34 {
		t.Fatalf("w tipada inesperada: %#v", doc.PAdESVisible.W)
	}
	if doc.PAdESVisible.H == nil || *doc.PAdESVisible.H != 0.12 {
		t.Fatalf("h tipada inesperada: %#v", doc.PAdESVisible.H)
	}
	if doc.PAdESVisible.KeepText == nil || !*doc.PAdESVisible.KeepText {
		t.Fatalf("keepText tipado inesperado: %#v", doc.PAdESVisible.KeepText)
	}
	if doc.PAdESVisible.Rotation == nil || *doc.PAdESVisible.Rotation != 180 {
		t.Fatalf("rotation tipada inesperada: %#v", doc.PAdESVisible.Rotation)
	}
	if doc.PAdESVisible.ImagePath == nil || *doc.PAdESVisible.ImagePath != "/tmp/sello.png" {
		t.Fatalf("imagePath tipado inesperado: %#v", doc.PAdESVisible.ImagePath)
	}
	if doc.PAdESVisible.QRContent == nil || *doc.PAdESVisible.QRContent != "https://verifica.local/expediente/123" {
		t.Fatalf("qrContent tipado inesperado: %#v", doc.PAdESVisible.QRContent)
	}
	if _, ok := doc.Extras["signSealImagePath"]; ok {
		t.Fatalf("signSealImagePath no deberia permanecer en extras: %#v", doc.Extras)
	}
	if _, ok := doc.Extras["signQRContent"]; ok {
		t.Fatalf("signQRContent no deberia permanecer en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_NormalizaTextoLibreBloquePAdESVisibleAVacio(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signSealImagePath": "   ",
		"signQRContent":     "\t",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.PAdESVisible.ImagePath == nil || *doc.PAdESVisible.ImagePath != "" {
		t.Fatalf("signSealImagePath vacio deberia normalizarse: %#v", doc.PAdESVisible.ImagePath)
	}
	if doc.PAdESVisible.QRContent == nil || *doc.PAdESVisible.QRContent != "" {
		t.Fatalf("signQRContent vacio deberia normalizarse: %#v", doc.PAdESVisible.QRContent)
	}
	for _, clave := range []string{"signSealImagePath", "signQRContent"} {
		if _, ok := doc.Extras[clave]; ok {
			t.Fatalf("%s no deberia quedar en extras tras normalizarse: %#v", clave, doc.Extras)
		}
	}
}

func TestCargarDocumento_CanonizaPaginasBloquePAdESVisible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signSealPages": " todas ",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.PAdESVisible.Pages == nil || *doc.PAdESVisible.Pages != "all" {
		t.Fatalf("pages tipadas inesperadas: %#v", doc.PAdESVisible.Pages)
	}
	if _, ok := doc.Extras["signSealPages"]; ok {
		t.Fatalf("signSealPages no deberia permanecer en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_ValidaPaginasBloquePAdESVisible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signSealPages": "1,,3-",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.PAdESVisible.Pages != nil {
		t.Fatalf("pages no deberian tiparse: %#v", doc.PAdESVisible.Pages)
	}
	if doc.Extras["signSealPages"] != "1,,3-" {
		t.Fatalf("signSealPages invalido no preservado en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_ValidaRotacionBloquePAdESVisible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signSealRotation": 360,
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.PAdESVisible.Rotation != nil {
		t.Fatalf("rotation no deberia tiparse: %#v", doc.PAdESVisible.Rotation)
	}
	if got := doc.Extras["signSealRotation"]; got != float64(360) {
		t.Fatalf("rotacion invalida no preservada en extras: %#v", doc.Extras)
	}
}

func TestGuardarRotacionLibreSello(t *testing.T) {
	t.Parallel()
	almacen := usersettings.New(t.TempDir())
	if err := almacen.Guardar(context.Background(), map[string]any{"signSealRotation": 45}); err != nil {
		t.Fatal(err)
	}
	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doc.PAdESVisible.Rotation == nil || *doc.PAdESVisible.Rotation != 45 {
		t.Fatalf("giro libre no persistido: %#v", doc.PAdESVisible.Rotation)
	}
}

func TestCargarDocumento_ValidaGeometriaBloquePAdESVisible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"signSealX": 2.0,
		"signSealY": -0.1,
		"signSealW": "0.5",
		"signSealH": 1.5,
		"profile":   "B",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.PAdESVisible.X != nil || doc.PAdESVisible.Y != nil || doc.PAdESVisible.W != nil || doc.PAdESVisible.H != nil {
		t.Fatalf("geometria no deberia tiparse: %#v", doc.PAdESVisible)
	}
	if doc.Extras["signSealX"] != float64(2) || doc.Extras["signSealY"] != -0.1 || doc.Extras["signSealW"] != "0.5" || doc.Extras["signSealH"] != 1.5 {
		t.Fatalf("geometria invalida no preservada en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_ProyectaBloqueMultiCoSignYNormalizaIDs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"multiCosignEnabled":              true,
		"multiCosignPrimaryCertificateId": " cert-main ",
		"multiCosignCertificateIds":       []any{" cert-a ", "cert-b", "cert-a", "", "  cert-c  "},
		"profile":                         "B",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.MultiCoSign.Enabled == nil || !*doc.MultiCoSign.Enabled {
		t.Fatalf("enabled tipado inesperado: %#v", doc.MultiCoSign.Enabled)
	}
	if doc.MultiCoSign.PrimaryID == nil || *doc.MultiCoSign.PrimaryID != "cert-main" {
		t.Fatalf("primary id tipado inesperado: %#v", doc.MultiCoSign.PrimaryID)
	}
	want := []string{"cert-a", "cert-b", "cert-c"}
	if len(doc.MultiCoSign.CertificateIDs) != len(want) {
		t.Fatalf("certificate ids tipados inesperados: %#v", doc.MultiCoSign.CertificateIDs)
	}
	for i := range want {
		if doc.MultiCoSign.CertificateIDs[i] != want[i] {
			t.Fatalf("certificate ids tipados inesperados: %#v", doc.MultiCoSign.CertificateIDs)
		}
	}
	if got := doc.Extras["profile"]; got != "B" {
		t.Fatalf("extras no preservados: got %v", got)
	}
}

func TestCargarDocumento_ValidaBloqueMultiCoSignConTiposInvalidos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"multiCosignPrimaryCertificateId": 7,
		"multiCosignCertificateIds":       []any{"cert-a", 2},
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.MultiCoSign.CertificateIDs != nil {
		t.Fatalf("multiCosign ids no deberian tiparse: %#v", doc.MultiCoSign.CertificateIDs)
	}
	if doc.MultiCoSign.PrimaryID != nil {
		t.Fatalf("multiCosign primary no deberia tiparse: %#v", doc.MultiCoSign.PrimaryID)
	}
	got, ok := doc.Extras["multiCosignCertificateIds"].([]any)
	if !ok || len(got) != 2 {
		t.Fatalf("ids invalidos no preservados en extras: %#v", doc.Extras)
	}
	if doc.Extras["multiCosignPrimaryCertificateId"] != float64(7) {
		t.Fatalf("primary invalido no preservado en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_ProyectaBloqueHashYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"defaultHashAlgorithm":       "SHA-384",
		"defaultHashCopyToClipboard": true,
		"defaultHashFormatFile":      "base64",
		"defaultHashFormatDirectory": "csv",
		"defaultHashRecursive":       true,
		"defaultHashSaveReport":      false,
		"proxyHost":                  "proxy.local",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Hash.Algorithm == nil || *doc.Hash.Algorithm != "SHA-384" {
		t.Fatalf("algorithm tipado inesperado: %#v", doc.Hash.Algorithm)
	}
	if doc.Hash.CopyToClipboard == nil || !*doc.Hash.CopyToClipboard {
		t.Fatalf("copyToClipboard tipado inesperado: %#v", doc.Hash.CopyToClipboard)
	}
	if doc.Hash.FormatFile == nil || *doc.Hash.FormatFile != "base64" {
		t.Fatalf("formatFile tipado inesperado: %#v", doc.Hash.FormatFile)
	}
	if doc.Hash.FormatDirectory == nil || *doc.Hash.FormatDirectory != "csv" {
		t.Fatalf("formatDirectory tipado inesperado: %#v", doc.Hash.FormatDirectory)
	}
	if doc.Hash.Recursive == nil || !*doc.Hash.Recursive {
		t.Fatalf("recursive tipado inesperado: %#v", doc.Hash.Recursive)
	}
	if doc.Hash.SaveReport == nil || *doc.Hash.SaveReport {
		t.Fatalf("saveReport tipado inesperado: %#v", doc.Hash.SaveReport)
	}
	if doc.Proxy.Host == nil || *doc.Proxy.Host != "proxy.local" {
		t.Fatalf("proxy host tipado inesperado: %#v", doc.Proxy.Host)
	}
	if _, ok := doc.Extras["proxyHost"]; ok {
		t.Fatal("proxyHost no deberia permanecer en extras")
	}
}

func TestCargarDocumento_ProyectaBloqueProxyYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"proxyEnabled":  true,
		"proxyType":     "manual",
		"proxyHost":     "proxy.local",
		"proxyPort":     3128,
		"proxySecretId": "proxy-secret-1",
		"proxyRealm":    "corp-proxy",
		"proxyExcludedUrls": []any{
			"https://intra.local",
			" *.dipgra.es ",
		},
		"idioma": "es",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Proxy.Enabled == nil || !*doc.Proxy.Enabled {
		t.Fatalf("enabled tipado inesperado: %#v", doc.Proxy.Enabled)
	}
	if doc.Proxy.Type == nil || *doc.Proxy.Type != "manual" {
		t.Fatalf("type tipado inesperado: %#v", doc.Proxy.Type)
	}
	if doc.Proxy.Host == nil || *doc.Proxy.Host != "proxy.local" {
		t.Fatalf("host tipado inesperado: %#v", doc.Proxy.Host)
	}
	if doc.Proxy.Port == nil || *doc.Proxy.Port != 3128 {
		t.Fatalf("port tipado inesperado: %#v", doc.Proxy.Port)
	}
	if doc.Proxy.SecretID == nil || *doc.Proxy.SecretID != "proxy-secret-1" {
		t.Fatalf("proxy secret id tipado inesperado: %#v", doc.Proxy.SecretID)
	}
	if doc.Proxy.Realm == nil || *doc.Proxy.Realm != "corp-proxy" {
		t.Fatalf("proxy realm tipado inesperado: %#v", doc.Proxy.Realm)
	}
	if len(doc.Proxy.ExcludedURLs) != 2 || doc.Proxy.ExcludedURLs[0] != "https://intra.local" || doc.Proxy.ExcludedURLs[1] != "*.dipgra.es" {
		t.Fatalf("excluded urls tipadas inesperadas: %#v", doc.Proxy.ExcludedURLs)
	}
	if doc.General.Idioma == nil || *doc.General.Idioma != "es" {
		t.Fatalf("idioma tipado inesperado: %#v", doc.General.Idioma)
	}
	if _, ok := doc.Extras["idioma"]; ok {
		t.Fatal("idioma no deberia permanecer en extras")
	}
}

func TestCargarDocumento_ProyectaBloqueCertificadosYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"stickySigner":                   true,
		"autoSelectSingleCertificate":    false,
		"preferDefaultCertificate":       true,
		"preferredCertificateId":         " cert-last ",
		"defaultCertificateId":           " cert-default ",
		"defaultKeystore":                " WINDOWS-MY ",
		"defaultLocalKeystorePath":       " /var/tmp/usuario/local.p12 ",
		"useDefaultStoreInBrowserCalls":  true,
		"useOnlySignatureCertificates":   true,
		"useOnlyAliasCertificates":       true,
		"skipAuthCertDnie":               true,
		"showDefaultCertificateFirst":    true,
		"showUsableCertificatesFirst":    true,
		"showValidCertificatesFirst":     true,
		"rememberCertificateFilter":      true,
		"certificateFilterText":          " dni ",
		"certsExpiredShow":               true,
		"certsInvalidShow":               true,
		"certificateTypeFilter":          []any{"fisica", "empleado_publico", " fisica "},
		"certificateRequireNIF":          true,
		"certificateRequireOrganization": false,
		"proxyHost":                      "proxy.local",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Certificados.StickySigner == nil || !*doc.Certificados.StickySigner {
		t.Fatalf("stickySigner tipado inesperado: %#v", doc.Certificados.StickySigner)
	}
	if doc.Certificados.AutoSelectSingleCertificate == nil || *doc.Certificados.AutoSelectSingleCertificate {
		t.Fatalf("autoSelectSingleCertificate tipado inesperado: %#v", doc.Certificados.AutoSelectSingleCertificate)
	}
	if doc.Certificados.PreferDefaultCertificate == nil || !*doc.Certificados.PreferDefaultCertificate {
		t.Fatalf("preferDefaultCertificate tipado inesperado: %#v", doc.Certificados.PreferDefaultCertificate)
	}
	if doc.Certificados.PreferredCertificateID == nil || *doc.Certificados.PreferredCertificateID != "cert-last" {
		t.Fatalf("preferredCertificateId tipado inesperado: %#v", doc.Certificados.PreferredCertificateID)
	}
	if doc.Certificados.DefaultCertificateID == nil || *doc.Certificados.DefaultCertificateID != "cert-default" {
		t.Fatalf("defaultCertificateId tipado inesperado: %#v", doc.Certificados.DefaultCertificateID)
	}
	if doc.Certificados.DefaultKeystore == nil || *doc.Certificados.DefaultKeystore != "WINDOWS-MY" {
		t.Fatalf("defaultKeystore tipado inesperado: %#v", doc.Certificados.DefaultKeystore)
	}
	if doc.Certificados.DefaultLocalKeystorePath == nil || *doc.Certificados.DefaultLocalKeystorePath != "/var/tmp/usuario/local.p12" {
		t.Fatalf("defaultLocalKeystorePath tipado inesperado: %#v", doc.Certificados.DefaultLocalKeystorePath)
	}
	if doc.Certificados.UseDefaultStoreInBrowserCalls == nil || !*doc.Certificados.UseDefaultStoreInBrowserCalls {
		t.Fatalf("useDefaultStoreInBrowserCalls tipado inesperado: %#v", doc.Certificados.UseDefaultStoreInBrowserCalls)
	}
	if doc.Certificados.UseOnlySignatureCertificates == nil || !*doc.Certificados.UseOnlySignatureCertificates {
		t.Fatalf("useOnlySignatureCertificates tipado inesperado: %#v", doc.Certificados.UseOnlySignatureCertificates)
	}
	if doc.Certificados.UseOnlyAliasCertificates == nil || !*doc.Certificados.UseOnlyAliasCertificates {
		t.Fatalf("useOnlyAliasCertificates tipado inesperado: %#v", doc.Certificados.UseOnlyAliasCertificates)
	}
	if doc.Certificados.SkipAuthCertDnie == nil || !*doc.Certificados.SkipAuthCertDnie {
		t.Fatalf("skipAuthCertDnie tipado inesperado: %#v", doc.Certificados.SkipAuthCertDnie)
	}
	if doc.Certificados.ShowDefaultFirst == nil || !*doc.Certificados.ShowDefaultFirst {
		t.Fatalf("showDefaultCertificateFirst tipado inesperado: %#v", doc.Certificados.ShowDefaultFirst)
	}
	if doc.Certificados.ShowUsableFirst == nil || !*doc.Certificados.ShowUsableFirst {
		t.Fatalf("showUsableCertificatesFirst tipado inesperado: %#v", doc.Certificados.ShowUsableFirst)
	}
	if doc.Certificados.ShowValidFirst == nil || !*doc.Certificados.ShowValidFirst {
		t.Fatalf("showValidCertificatesFirst tipado inesperado: %#v", doc.Certificados.ShowValidFirst)
	}
	if doc.Certificados.RememberFilter == nil || !*doc.Certificados.RememberFilter {
		t.Fatalf("rememberCertificateFilter tipado inesperado: %#v", doc.Certificados.RememberFilter)
	}
	if doc.Certificados.FilterText == nil || *doc.Certificados.FilterText != "dni" {
		t.Fatalf("certificateFilterText tipado inesperado: %#v", doc.Certificados.FilterText)
	}
	if doc.Certificados.ShowExpired == nil || !*doc.Certificados.ShowExpired {
		t.Fatalf("certsExpiredShow tipado inesperado: %#v", doc.Certificados.ShowExpired)
	}
	if doc.Certificados.ShowInvalid == nil || !*doc.Certificados.ShowInvalid {
		t.Fatalf("certsInvalidShow tipado inesperado: %#v", doc.Certificados.ShowInvalid)
	}
	if len(doc.Certificados.TypeFilter) != 2 || doc.Certificados.TypeFilter[0] != "fisica" || doc.Certificados.TypeFilter[1] != "empleado_publico" {
		t.Fatalf("certificateTypeFilter tipado inesperado: %#v", doc.Certificados.TypeFilter)
	}
	if doc.Certificados.RequireNIF == nil || !*doc.Certificados.RequireNIF {
		t.Fatalf("certificateRequireNIF tipado inesperado: %#v", doc.Certificados.RequireNIF)
	}
	if doc.Certificados.RequireOrganization == nil || *doc.Certificados.RequireOrganization {
		t.Fatalf("certificateRequireOrganization tipado inesperado: %#v", doc.Certificados.RequireOrganization)
	}
	if doc.Proxy.Host == nil || *doc.Proxy.Host != "proxy.local" {
		t.Fatalf("proxy host tipado inesperado: %#v", doc.Proxy.Host)
	}
	if _, ok := doc.Extras["preferredCertificateId"]; ok {
		t.Fatal("preferredCertificateId no deberia permanecer en extras")
	}
}

func TestCargarDocumento_ValidaIDsCertificadoYNormalizaFiltroVacio(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"preferredCertificateId": "   ",
		"defaultCertificateId":   "",
		"certificateFilterText":  "   ",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Certificados.PreferredCertificateID != nil {
		t.Fatalf("preferredCertificateId vacio no deberia tiparse: %#v", doc.Certificados.PreferredCertificateID)
	}
	if doc.Certificados.DefaultCertificateID != nil {
		t.Fatalf("defaultCertificateId vacio no deberia tiparse: %#v", doc.Certificados.DefaultCertificateID)
	}
	if doc.Certificados.FilterText == nil || *doc.Certificados.FilterText != "" {
		t.Fatalf("certificateFilterText vacio deberia normalizarse a cadena vacia: %#v", doc.Certificados.FilterText)
	}
	if doc.Extras["preferredCertificateId"] != "   " {
		t.Fatalf("preferredCertificateId invalido no preservado en extras: %#v", doc.Extras)
	}
	if doc.Extras["defaultCertificateId"] != "" {
		t.Fatalf("defaultCertificateId invalido no preservado en extras: %#v", doc.Extras)
	}
	if _, ok := doc.Extras["certificateFilterText"]; ok {
		t.Fatalf("certificateFilterText no deberia quedar en extras tras normalizarse: %#v", doc.Extras)
	}
}

func TestCargarDocumento_CanonizaProxyTypeYFiltroTiposCertificado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"proxyType":             "MANUAL",
		"certificateTypeFilter": []any{" fisica ", "EMPLEADO_PUBLICO", "fisica"},
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Proxy.Type == nil || *doc.Proxy.Type != "manual" {
		t.Fatalf("proxyType tipado inesperado: %#v", doc.Proxy.Type)
	}
	if len(doc.Certificados.TypeFilter) != 2 || doc.Certificados.TypeFilter[0] != "fisica" || doc.Certificados.TypeFilter[1] != "empleado_publico" {
		t.Fatalf("certificateTypeFilter tipado inesperado: %#v", doc.Certificados.TypeFilter)
	}
	if _, ok := doc.Extras["proxyType"]; ok {
		t.Fatalf("proxyType no deberia permanecer en extras: %#v", doc.Extras)
	}
	if _, ok := doc.Extras["certificateTypeFilter"]; ok {
		t.Fatalf("certificateTypeFilter no deberia permanecer en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_DejaEnExtrasProxyTypeYFiltroTiposInvalidos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"proxyType":             "auto",
		"certificateTypeFilter": []any{"fisica", "inventado"},
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Proxy.Type != nil {
		t.Fatalf("proxyType tipado inesperado: %#v", doc.Proxy.Type)
	}
	if len(doc.Certificados.TypeFilter) != 0 {
		t.Fatalf("certificateTypeFilter tipado inesperado: %#v", doc.Certificados.TypeFilter)
	}
	if doc.Extras["proxyType"] != "auto" {
		t.Fatalf("proxyType invalido deberia quedar en extras: %#v", doc.Extras)
	}
	valor, ok := doc.Extras["certificateTypeFilter"].([]any)
	if !ok || len(valor) != 2 || valor[0] != "fisica" || valor[1] != "inventado" {
		t.Fatalf("certificateTypeFilter invalido deberia quedar en extras: %#v", doc.Extras)
	}
}

func TestCargarDocumento_ProyectaBloqueDesktopYConservaExtras(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	entrada := map[string]any{
		"legacyWebUiSize":       "extra",
		"legacyWebTrayResident": true,
		"idioma":                "es",
	}
	if err := almacen.Guardar(context.Background(), entrada); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	doc, err := almacen.CargarDocumento(context.Background())
	if err != nil {
		t.Fatalf("CargarDocumento: %v", err)
	}

	if doc.Desktop.LegacyWebUISize == nil || *doc.Desktop.LegacyWebUISize != "extra" {
		t.Fatalf("legacyWebUiSize tipado inesperado: %#v", doc.Desktop.LegacyWebUISize)
	}
	if doc.Desktop.LegacyWebTrayResident == nil || !*doc.Desktop.LegacyWebTrayResident {
		t.Fatalf("legacyWebTrayResident tipado inesperado: %#v", doc.Desktop.LegacyWebTrayResident)
	}
	if doc.General.Idioma == nil || *doc.General.Idioma != "es" {
		t.Fatalf("idioma tipado inesperado: %#v", doc.General.Idioma)
	}
	if _, ok := doc.Extras["legacyWebUiSize"]; ok {
		t.Fatal("legacyWebUiSize no deberia permanecer en extras")
	}
}

func TestGuardarDocumento_GeneraMapaLegacyCompatible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)

	idioma := "en"
	themeIndex := 3
	expertMode := true
	autoClose := true
	confirmToSign := false
	omitAskOnClose := true
	closeBehavior := "exit"
	hideDnieStartScreen := true
	secureConnections := true
	secureDomainsList := []string{"https://sede.dipgra.es", "*.dipgra.es"}
	signAction := "cosign"
	signFormat := "xades"
	signProfile := "t"
	signOverwrite := "force"
	signStrictCompat := true
	signAllowInvalidPDF := false
	autoFormatPDF := "pades"
	autoFormatOOXML := "ooxml"
	autoFormatFacturaE := "facturae"
	autoFormatODF := "odf"
	autoFormatXML := "xmldsig"
	autoFormatBinary := "cades"
	signReason := "Aprobación interna"
	signLocation := "Granada"
	signContactInfo := "contacto@example.invalid"
	facturaePolicyVersion := "3.1"
	facturaePolicyID := "https://www.facturae.gob.es/politica.pdf"
	facturaePolicyHash := "ZmFrZS1oYXNo"
	facturaePolicyQualifier := "https://www.facturae.gob.es/politica.html"
	facturaeSignerRole := "emisor"
	facturaeCity := "Granada"
	facturaeProvince := "Granada"
	facturaePostalCode := "18014"
	facturaeCountry := "ES"
	padesSubFilter := "adobe"
	padesPolicyID := "https://politica.pades.local/id"
	padesPolicyHash := "c2hhMjU2"
	padesPolicyHashAlgorithm := "SHA-256"
	padesPolicyQualifier := "https://politica.pades.local/info"
	padesObfuscateCertInfo := true
	padesVisibleStamp := false
	allowShadowAttack := false
	allowCertifiedPDF := true
	padesCertificationLevel := 2
	signVisibleSeal := true
	signSealPages := "all"
	signSealAllPages := false
	signSealX := 0.62
	signSealY := 0.04
	signSealW := 0.34
	signSealH := 0.12
	signSealKeepText := true
	signSealRotation := 270
	signSealImagePath := "/tmp/sello.png"
	signQRContent := "https://verifica.local/expediente/123"
	multiCosignEnabled := true
	multiCosignCertificateIDs := []string{"cert-a", "cert-b"}
	hashAlgorithm := "SHA-512"
	hashCopyToClipboard := true
	hashFormatFile := "hex"
	hashFormatDirectory := "xml"
	hashRecursive := false
	hashSaveReport := true
	proxyEnabled := true
	proxyType := "manual"
	proxyHost := "proxy.local"
	proxyPort := 8080
	proxySecretID := "proxy-secret-1"
	proxyRealm := "corp-proxy"
	proxyExcludedURLs := []string{"https://intra.local", "*.dipgra.es"}
	stickySigner := true
	autoSelectSingleCertificate := false
	preferDefaultCertificate := true
	preferredCertificateID := "cert-last"
	defaultCertificateID := "cert-default"
	defaultKeystore := "shared-nss"
	defaultLocalKeystorePath := "/tmp/almacen/local.p12"
	useDefaultStoreInBrowserCalls := true
	showDefaultCertificateFirst := true
	showUsableCertificatesFirst := true
	showValidCertificatesFirst := true
	useOnlySignatureCertificates := true
	useOnlyAliasCertificates := true
	rememberCertificateFilter := true
	certificateFilterText := "dni"
	certsExpiredShow := true
	certsInvalidShow := true
	certificateTypeFilter := []string{"fisica", "empleado_publico"}
	certificateRequireNIF := true
	certificateRequireOrganization := false
	skipAuthCertDnie := true
	legacyWebUISize := "grande"
	legacyWebTrayResident := true
	tsaEnabled := true
	tsaURL := "https://tsa.local"
	doc := ports.DocumentoConfiguracionUsuario{
		General: ports.ConfiguracionUsuarioGeneral{
			Idioma:              &idioma,
			ThemeIndex:          &themeIndex,
			ExpertMode:          &expertMode,
			AutoClose:           &autoClose,
			ConfirmToSign:       &confirmToSign,
			OmitAskOnClose:      &omitAskOnClose,
			CloseBehavior:       &closeBehavior,
			HideDnieStartScreen: &hideDnieStartScreen,
			SecureConnections:   &secureConnections,
			SecureDomainsList:   secureDomainsList,
		},
		Firma: ports.ConfiguracionUsuarioFirma{
			Action:          &signAction,
			Format:          &signFormat,
			Profile:         &signProfile,
			Overwrite:       &signOverwrite,
			StrictCompat:    &signStrictCompat,
			AllowInvalidPDF: &signAllowInvalidPDF,
		},
		FormatosAuto: ports.ConfiguracionUsuarioFormatosAutomaticos{
			PDF:      &autoFormatPDF,
			OOXML:    &autoFormatOOXML,
			FacturaE: &autoFormatFacturaE,
			ODF:      &autoFormatODF,
			XML:      &autoFormatXML,
			Binary:   &autoFormatBinary,
		},
		FirmaMeta: ports.ConfiguracionUsuarioFirmaMetadatos{
			Reason:      &signReason,
			Location:    &signLocation,
			ContactInfo: &signContactInfo,
		},
		FacturaE: ports.ConfiguracionUsuarioFacturaE{
			PolicyVersion:   &facturaePolicyVersion,
			PolicyID:        &facturaePolicyID,
			PolicyHash:      &facturaePolicyHash,
			PolicyQualifier: &facturaePolicyQualifier,
			SignerRole:      &facturaeSignerRole,
			City:            &facturaeCity,
			Province:        &facturaeProvince,
			PostalCode:      &facturaePostalCode,
			Country:         &facturaeCountry,
		},
		PAdES: ports.ConfiguracionUsuarioPAdES{
			SubFilter:           &padesSubFilter,
			PolicyID:            &padesPolicyID,
			PolicyHash:          &padesPolicyHash,
			PolicyHashAlgorithm: &padesPolicyHashAlgorithm,
			PolicyQualifier:     &padesPolicyQualifier,
			ObfuscateCertInfo:   &padesObfuscateCertInfo,
			VisibleStamp:        &padesVisibleStamp,
			AllowShadowAttack:   &allowShadowAttack,
			AllowCertifiedPDF:   &allowCertifiedPDF,
			CertificationLevel:  &padesCertificationLevel,
		},
		PAdESVisible: ports.ConfiguracionUsuarioPAdESVisible{
			Enabled:   &signVisibleSeal,
			Pages:     &signSealPages,
			AllPages:  &signSealAllPages,
			X:         &signSealX,
			Y:         &signSealY,
			W:         &signSealW,
			H:         &signSealH,
			KeepText:  &signSealKeepText,
			Rotation:  &signSealRotation,
			ImagePath: &signSealImagePath,
			QRContent: &signQRContent,
		},
		MultiCoSign: ports.ConfiguracionUsuarioMultiCoSign{
			Enabled:        &multiCosignEnabled,
			PrimaryID:      &preferredCertificateID,
			CertificateIDs: multiCosignCertificateIDs,
		},
		Hash: ports.ConfiguracionUsuarioHash{
			Algorithm:       &hashAlgorithm,
			CopyToClipboard: &hashCopyToClipboard,
			FormatFile:      &hashFormatFile,
			FormatDirectory: &hashFormatDirectory,
			Recursive:       &hashRecursive,
			SaveReport:      &hashSaveReport,
		},
		Proxy: ports.ConfiguracionUsuarioProxy{
			Enabled:      &proxyEnabled,
			Type:         &proxyType,
			Host:         &proxyHost,
			Port:         &proxyPort,
			SecretID:     &proxySecretID,
			Realm:        &proxyRealm,
			ExcludedURLs: proxyExcludedURLs,
		},
		Certificados: ports.ConfiguracionUsuarioCertificados{
			StickySigner:                  &stickySigner,
			AutoSelectSingleCertificate:   &autoSelectSingleCertificate,
			PreferDefaultCertificate:      &preferDefaultCertificate,
			PreferredCertificateID:        &preferredCertificateID,
			DefaultCertificateID:          &defaultCertificateID,
			DefaultKeystore:               &defaultKeystore,
			DefaultLocalKeystorePath:      &defaultLocalKeystorePath,
			UseDefaultStoreInBrowserCalls: &useDefaultStoreInBrowserCalls,
			UseOnlySignatureCertificates:  &useOnlySignatureCertificates,
			UseOnlyAliasCertificates:      &useOnlyAliasCertificates,
			SkipAuthCertDnie:              &skipAuthCertDnie,
			ShowDefaultFirst:              &showDefaultCertificateFirst,
			ShowUsableFirst:               &showUsableCertificatesFirst,
			ShowValidFirst:                &showValidCertificatesFirst,
			RememberFilter:                &rememberCertificateFilter,
			FilterText:                    &certificateFilterText,
			ShowExpired:                   &certsExpiredShow,
			ShowInvalid:                   &certsInvalidShow,
			TypeFilter:                    certificateTypeFilter,
			RequireNIF:                    &certificateRequireNIF,
			RequireOrganization:           &certificateRequireOrganization,
		},
		Desktop: ports.ConfiguracionUsuarioDesktop{
			LegacyWebUISize:       &legacyWebUISize,
			LegacyWebTrayResident: &legacyWebTrayResident,
		},
		TSA: ports.ConfiguracionUsuarioTSA{
			Enabled: &tsaEnabled,
			URL:     &tsaURL,
		},
		Extras: map[string]any{
			"profile": "B",
		},
	}

	if err := almacen.GuardarDocumento(context.Background(), doc); err != nil {
		t.Fatalf("GuardarDocumento: %v", err)
	}

	salida, err := almacen.Cargar(context.Background())
	if err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	if salida["idioma"] != "en" || salida["themeIndex"] != float64(3) {
		t.Fatalf("mapa legacy inesperado: %#v", salida)
	}
	if salida["expertMode"] != true || salida["autoClose"] != true || salida["confirmToSign"] != false {
		t.Fatalf("bools legacy inesperados: %#v", salida)
	}
	if salida["omitAskOnClose"] != true {
		t.Fatalf("flag omitAskOnClose legacy inesperado: %#v", salida)
	}
	if salida["hideDnieStartScreen"] != true || salida["secureConnections"] != true {
		t.Fatalf("flags generales de seguridad legacy inesperados: %#v", salida)
	}
	secureDomains, ok := salida["secureDomainsList"].([]any)
	if !ok || len(secureDomains) != 2 || secureDomains[0] != "https://sede.dipgra.es" || secureDomains[1] != "*.dipgra.es" {
		t.Fatalf("secureDomainsList legacy inesperado: %#v", salida)
	}
	if salida["signAction"] != "cosign" || salida["signFormat"] != "xades" || salida["signProfile"] != "t" || salida["signOverwrite"] != "force" {
		t.Fatalf("bloque firma legacy inesperado: %#v", salida)
	}
	if salida["signStrictCompat"] != true || salida["signAllowInvalidPDF"] != false {
		t.Fatalf("flags de firma legacy inesperados: %#v", salida)
	}
	if salida["autoFormatPdf"] != "pades" || salida["autoFormatOoxml"] != "ooxml" || salida["autoFormatFacturae"] != "facturae" {
		t.Fatalf("bloque auto formato legacy inesperado: %#v", salida)
	}
	if salida["autoFormatOdf"] != "odf" || salida["autoFormatXml"] != "xmldsig" || salida["autoFormatBinary"] != "cades" {
		t.Fatalf("bloque auto formato legacy inesperado: %#v", salida)
	}
	if salida["signReason"] != "Aprobación interna" || salida["signLocation"] != "Granada" || salida["signContactInfo"] != "contacto@example.invalid" {
		t.Fatalf("metadatos de firma legacy inesperados: %#v", salida)
	}
	if salida["facturaePolicyVersion"] != "3.1" || salida["policyIdentifier"] != "https://www.facturae.gob.es/politica.pdf" || salida["policyIdentifierHash"] != "ZmFrZS1oYXNo" || salida["policyQualifier"] != "https://www.facturae.gob.es/politica.html" || salida["signerClaimedRole"] != "emisor" || salida["signatureProductionCity"] != "Granada" || salida["signatureProductionProvince"] != "Granada" || salida["signatureProductionPostalCode"] != "18014" || salida["signatureProductionCountry"] != "ES" {
		t.Fatalf("bloque FacturaE legacy inesperado: %#v", salida)
	}
	if salida["padesSubFilter"] != "adobe" || salida["padesPolicyIdentifier"] != "https://politica.pades.local/id" || salida["padesPolicyIdentifierHash"] != "c2hhMjU2" || salida["padesPolicyIdentifierHashAlgorithm"] != "SHA-256" || salida["padesPolicyQualifier"] != "https://politica.pades.local/info" || salida["padesObfuscateCertInfo"] != true || salida["padesVisibleStamp"] != false || salida["allowShadowAttack"] != false || salida["allowCertifiedPDF"] != true || salida["padesCertificationLevel"] != float64(2) {
		t.Fatalf("bloque PAdES legacy inesperado: %#v", salida)
	}
	if salida["signVisibleSeal"] != true || salida["signSealPages"] != "all" || salida["signSealAllPages"] != false || salida["signSealX"] != 0.62 || salida["signSealY"] != 0.04 || salida["signSealW"] != 0.34 || salida["signSealH"] != 0.12 || salida["signSealKeepText"] != true || salida["signSealRotation"] != float64(270) {
		t.Fatalf("bloque pades visible legacy inesperado: %#v", salida)
	}
	if salida["signSealImagePath"] != "/tmp/sello.png" || salida["signQRContent"] != "https://verifica.local/expediente/123" {
		t.Fatalf("recursos pades visibles legacy inesperados: %#v", salida)
	}
	if salida["multiCosignEnabled"] != true {
		t.Fatalf("flag multicosign legacy inesperado: %#v", salida)
	}
	if salida["multiCosignPrimaryCertificateId"] != "cert-last" {
		t.Fatalf("primary multicosign legacy inesperado: %#v", salida)
	}
	ids, ok := salida["multiCosignCertificateIds"].([]any)
	if !ok || len(ids) != 2 || ids[0] != "cert-a" || ids[1] != "cert-b" {
		t.Fatalf("ids multicosign legacy inesperados: %#v", salida)
	}
	if salida["defaultHashAlgorithm"] != "SHA-512" || salida["defaultHashFormatFile"] != "hex" || salida["defaultHashFormatDirectory"] != "xml" {
		t.Fatalf("bloque hash legacy inesperado: %#v", salida)
	}
	if salida["defaultHashCopyToClipboard"] != true || salida["defaultHashRecursive"] != false || salida["defaultHashSaveReport"] != true {
		t.Fatalf("flags hash legacy inesperados: %#v", salida)
	}
	if salida["proxyEnabled"] != true || salida["proxyType"] != "manual" || salida["proxyHost"] != "proxy.local" || salida["proxyPort"] != float64(8080) || salida["proxySecretId"] != "proxy-secret-1" || salida["proxyRealm"] != "corp-proxy" {
		t.Fatalf("bloque proxy legacy inesperado: %#v", salida)
	}
	excluded, ok := salida["proxyExcludedUrls"].([]any)
	if !ok || len(excluded) != 2 || excluded[0] != "https://intra.local" || excluded[1] != "*.dipgra.es" {
		t.Fatalf("exclusiones proxy legacy inesperadas: %#v", salida)
	}
	if salida["stickySigner"] != true || salida["autoSelectSingleCertificate"] != false || salida["preferDefaultCertificate"] != true {
		t.Fatalf("flags de certificados legacy inesperados: %#v", salida)
	}
	if salida["preferredCertificateId"] != "cert-last" || salida["defaultCertificateId"] != "cert-default" {
		t.Fatalf("ids de certificados legacy inesperados: %#v", salida)
	}
	if salida["defaultKeystore"] != "shared-nss" || salida["defaultLocalKeystorePath"] != "/tmp/almacen/local.p12" {
		t.Fatalf("preferencias de keystore legacy inesperadas: %#v", salida)
	}
	if salida["useDefaultStoreInBrowserCalls"] != true {
		t.Fatalf("flag useDefaultStoreInBrowserCalls legacy inesperado: %#v", salida)
	}
	if salida["useOnlySignatureCertificates"] != true {
		t.Fatalf("flag useOnlySignatureCertificates legacy inesperado: %#v", salida)
	}
	if salida["useOnlyAliasCertificates"] != true || salida["skipAuthCertDnie"] != true {
		t.Fatalf("flags alias/DNIe legacy inesperados: %#v", salida)
	}
	if salida["showDefaultCertificateFirst"] != true || salida["showUsableCertificatesFirst"] != true || salida["showValidCertificatesFirst"] != true {
		t.Fatalf("orden visual de certificados legacy inesperado: %#v", salida)
	}
	if salida["rememberCertificateFilter"] != true || salida["certificateFilterText"] != "dni" {
		t.Fatalf("persistencia del filtro de certificados legacy inesperada: %#v", salida)
	}
	if salida["certsExpiredShow"] != true {
		t.Fatalf("flag certsExpiredShow legacy inesperado: %#v", salida)
	}
	if salida["certsInvalidShow"] != true {
		t.Fatalf("flag certsInvalidShow legacy inesperado: %#v", salida)
	}
	typeFilter, ok := salida["certificateTypeFilter"].([]any)
	if !ok || len(typeFilter) != 2 || typeFilter[0] != "fisica" || typeFilter[1] != "empleado_publico" {
		t.Fatalf("filtro de tipos legacy inesperado: %#v", salida)
	}
	if salida["certificateRequireNIF"] != true || salida["certificateRequireOrganization"] != false {
		t.Fatalf("flags de filtro certificado legacy inesperados: %#v", salida)
	}
	if salida["legacyWebUiSize"] != "grande" || salida["legacyWebTrayResident"] != true {
		t.Fatalf("bloque desktop legacy inesperado: %#v", salida)
	}
	if salida["closeBehavior"] != "exit" || salida["tsaUrl"] != "https://tsa.local" || salida["tsaEnabled"] != true || salida["profile"] != "B" {
		t.Fatalf("extras legacy inesperados: %#v", salida)
	}
}
