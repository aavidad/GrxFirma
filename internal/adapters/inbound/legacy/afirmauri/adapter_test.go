// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"encoding/base64"
	"net/url"
	"testing"

	"grxfirma/internal/domain"
)

func TestParse_URIValidaSignRemota(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://sign?fileId=req-42&retrieveServlet=https%3A%2F%2Fretrieve.example%2FRetrieveService&storageServlet=https%3A%2F%2Fstore.example%2FStorageService&signFormat=CAdES")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if solicitud.Operacion != OperacionFirma {
		t.Fatalf("operacion inesperada: %s", solicitud.Operacion)
	}
	if solicitud.AccionFirma != domain.ActionSign {
		t.Fatalf("accion inesperada: %s", solicitud.AccionFirma)
	}
	if solicitud.Formato != domain.FormatCAdES {
		t.Fatalf("formato inesperado: %s", solicitud.Formato)
	}
	if solicitud.Sesion.RequestID != "req-42" {
		t.Fatalf("requestID inesperado: %s", solicitud.Sesion.RequestID)
	}
	if solicitud.Sesion.RetrieveEndpoint == "" || solicitud.Sesion.UploadEndpoint == "" {
		t.Fatal("la sesion no contiene endpoints completos")
	}
	if solicitud.RetrieveCommand == nil {
		t.Fatal("se esperaba RetrieveRequestCommand para flujo remoto")
	}
}

func TestParse_URIValidaSignRemota_ConParamsConservaOptions(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://sign?fileId=req-42&retrieveServlet=https%3A%2F%2Fretrieve.example%2FRetrieveService&storageServlet=https%3A%2F%2Fstore.example%2FStorageService&signFormat=XAdES&params=profile=T")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.RetrieveCommand == nil {
		t.Fatal("se esperaba RetrieveRequestCommand para flujo remoto")
	}
	if got := solicitud.Options["profile"]; got != "T" {
		t.Fatalf("profile inesperado: %q", got)
	}
}

func TestParse_URIValidaSignRemota_FormatoAutoCompat(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	solicitud, err := adaptador.ParseSocket(context.Background(),
		"afirma://sign?op=sign&idsession=ses-legacy-1&algorithm=SHA256withRSA&format=AUTO&properties=bW9kZT1pbXBsaWNpdA==&sticky=false")
	if err != nil {
		t.Fatalf("no se esperaba error con formato AUTO legacy: %v", err)
	}

	if solicitud.Operacion != OperacionFirma {
		t.Fatalf("operacion inesperada: %s", solicitud.Operacion)
	}
	if solicitud.Formato != domain.FormatCAdES {
		t.Fatalf("formato inesperado para AUTO: %s", solicitud.Formato)
	}
	if solicitud.SignCommand != nil || solicitud.RetrieveCommand != nil {
		t.Fatalf("la firma local legacy no debe requerir comandos remotos: %+v", solicitud)
	}
	if got := solicitud.LegacyParams.Get("idsession"); got != "ses-legacy-1" {
		t.Fatalf("idsession = %q, want %q", got, "ses-legacy-1")
	}
}

func TestParseSocket_SaveConDatosBinariosCrudosCompat(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	rawPayload := string([]byte{0x30, 0x82, 0x01, 0x02, 0x00, 0x0a, 0xff, 0x41})

	solicitud, err := adaptador.ParseSocket(context.Background(),
		"afirma://save?op=save&idsession=ses-legacy-2&title=Guardar%20firma&dat="+rawPayload)
	if err != nil {
		t.Fatalf("no se esperaba error con save crudo legacy: %v", err)
	}
	if solicitud.Operacion != OperacionSave {
		t.Fatalf("operacion inesperada: %s", solicitud.Operacion)
	}
	if got := solicitud.LegacyParams.Get("dat"); got != rawPayload {
		t.Fatalf("dat inesperado: %q", got)
	}
}

func TestParseSocket_SignConPayloadCrudoCompat(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	rawPayload := "<xml><firma>demo</firma></xml>"

	solicitud, err := adaptador.ParseSocket(context.Background(),
		"afirma://sign?op=sign&idsession=ses-legacy-3&algorithm=SHA256withRSA&format=XAdES&dat="+url.QueryEscape(rawPayload))
	if err != nil {
		t.Fatalf("no se esperaba error con sign crudo legacy: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand para payload crudo legacy")
	}
	if got := string(solicitud.SignCommand.Document.Content); got != rawPayload {
		t.Fatalf("payload inesperado: %q", got)
	}
}

func TestParse_URIValidaConPayloadEmbebido(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://firmar?id=req-local&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat=QUJD")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand para payload embebido")
	}
	if string(solicitud.SignCommand.Document.Content) != "ABC" {
		t.Fatalf("contenido inesperado: %q", string(solicitud.SignCommand.Document.Content))
	}
}

func TestParse_URIValidaConPayloadEmbebido_TrasladaPropertiesYParams(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	properties := base64.StdEncoding.EncodeToString([]byte("profile=T\npolicy=demo"))

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://firmar?id=req-local&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat=QUJD&properties="+
			properties+"&params=mode=implicit")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand para payload embebido")
	}
	if got := solicitud.SignCommand.Options["profile"]; got != "T" {
		t.Fatalf("profile inesperado: %q", got)
	}
	if got := solicitud.SignCommand.Options["policy"]; got != "demo" {
		t.Fatalf("policy inesperada: %q", got)
	}
	if got := solicitud.SignCommand.Options["mode"]; got != "implicit" {
		t.Fatalf("mode inesperado: %q", got)
	}
}

func TestParse_TriConServerURLEsFirmaTrifasicaNoDescarga(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	remoteRef := base64.StdEncoding.EncodeToString([]byte("a7e41400-3690-4ae9-b481-0cc1727d6688"))
	properties := base64.StdEncoding.EncodeToString([]byte("serverUrl=https://firma.example/triphaseSignService\nprecalculatedHashAlgorithm=SHA-512"))

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://sign?id=req-local&stservlet=https%3A%2F%2Fstore.example%2FStorageService&key=20547643&format=CAdEStri&properties="+url.QueryEscape(properties)+"&dat="+remoteRef)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	// Con serverUrl es una firma trifásica contra ese servidor (FIRe), como en
	// AutoFirma Java: "dat" es el identificador para el servidor y "id" el de
	// la subida del resultado.
	if solicitud.SignCommand == nil || solicitud.RetrieveCommand != nil {
		t.Fatal("se esperaba una firma con dat, no una descarga desde el servidor trifásico")
	}
	if got := solicitud.Sesion.RequestID; got != "req-local" {
		t.Fatalf("requestID inesperado: %q", got)
	}
	if got := string(solicitud.SignCommand.Document.Content); got != "a7e41400-3690-4ae9-b481-0cc1727d6688" {
		t.Fatalf("dat inesperado: %q", got)
	}
}

func TestParse_BatchJSONEmbebidoGeneraProcessBatchCommand(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	batchJSON := `{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc1","datareference":"` +
		base64.StdEncoding.EncodeToString([]byte("ABC")) +
		`","extraparams":"mode=implicit"}]}`

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://batch?id=req-batch&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat="+base64.StdEncoding.EncodeToString([]byte(batchJSON)))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if solicitud.BatchCommand == nil {
		t.Fatal("se esperaba ProcessBatchCommand para batch embebido")
	}
	if solicitud.RetrieveCommand != nil {
		t.Fatal("no se esperaba RetrieveRequestCommand cuando el lote viaja embebido")
	}
	if got := len(solicitud.BatchCommand.Jobs); got != 1 {
		t.Fatalf("numero de trabajos inesperado: %d", got)
	}
	job := solicitud.BatchCommand.Jobs[0]
	if string(job.Document.Content) != "ABC" {
		t.Fatalf("contenido inesperado: %q", string(job.Document.Content))
	}
	if job.Document.Name != "batch-doc1.bin" {
		t.Fatalf("nombre inesperado: %s", job.Document.Name)
	}
	if job.Options["algorithm"] != "SHA256withRSA" {
		t.Fatalf("algoritmo inesperado en opciones: %q", job.Options["algorithm"])
	}
	if job.Options["mode"] != "implicit" {
		t.Fatalf("extraParams no trasladados: %#v", job.Options)
	}
}

func TestParseBatchPayload_PlantillaGlobalDeSelloYOverridePorDocumento(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	documento := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7"))
	batchJSON := `{
		"format":"PAdES",
		"extraparams":"visibleSeal=true\nvisibleSealRectX=36\nvisibleSealRectY=36\nvisibleSealRectW=220\nvisibleSealRectH=70\npage=all",
		"singlesigns":[
			{"id":"global","datareference":"` + documento + `"},
			{"id":"pagina","datareference":"` + documento + `","extraparams":"page=2"},
			{"id":"rango","datareference":"` + documento + `","extraparams":"page=1,3-5\nvisibleSealRectX=72"}
		]
	}`

	cmd, err := adaptador.ParseBatchPayload([]byte(batchJSON), domain.ExchangeSession{})
	if err != nil {
		t.Fatalf("ParseBatchPayload() error = %v", err)
	}
	if got := len(cmd.Jobs); got != 3 {
		t.Fatalf("jobs = %d, want 3", got)
	}
	if got := cmd.Jobs[0].Options["page"]; got != "all" {
		t.Fatalf("page global = %q, want all", got)
	}
	if got := cmd.Jobs[1].Options["page"]; got != "2" {
		t.Fatalf("page del documento = %q, want 2", got)
	}
	if got := cmd.Jobs[2].Options["page"]; got != "1,3-5" {
		t.Fatalf("rango del documento = %q, want 1,3-5", got)
	}
	if got := cmd.Jobs[2].Options["visibleSealRectX"]; got != "72" {
		t.Fatalf("rectX del documento = %q, want 72", got)
	}
	for i, job := range cmd.Jobs {
		if got := job.Options["visibleSeal"]; got != "true" {
			t.Fatalf("job %d no heredó visibleSeal: %q", i, got)
		}
		if got := job.Options["visibleSealRectW"]; got != "220" {
			t.Fatalf("job %d no heredó rectW: %q", i, got)
		}
	}
}

func TestParseBatchPayload_ExtraParamsBase64CanonicosV19(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	documento := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7"))
	globalParams := base64.StdEncoding.EncodeToString([]byte("visibleSeal=true\npage=all"))
	itemParams := base64.StdEncoding.EncodeToString([]byte("page=2\ntarget=tree"))
	batchJSON := `{
		"format":"PAdES",
		"algorithm":"SHA256withRSA",
		"extraparams":"` + globalParams + `",
		"singlesigns":[{
			"id":"doc-v19",
			"datareference":"` + documento + `",
			"extraparams":"` + itemParams + `"
		}]
	}`

	cmd, err := adaptador.ParseBatchPayload([]byte(batchJSON), domain.ExchangeSession{})
	if err != nil {
		t.Fatalf("ParseBatchPayload() error = %v", err)
	}
	if got := len(cmd.Jobs); got != 1 {
		t.Fatalf("jobs = %d, want 1", got)
	}
	options := cmd.Jobs[0].Options
	if options["visibleSeal"] != "true" || options["page"] != "2" || options["target"] != "tree" {
		t.Fatalf("extraparams Base64 V1.9 no trasladados: %#v", options)
	}
	if _, leaked := options[globalParams]; leaked {
		t.Fatalf("el Base64 de extraparams se interpretó como clave Properties: %#v", options)
	}
}

func TestParseBatchPayload_XMLExtraParamsBase64CanonicosV19(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	params := base64.StdEncoding.EncodeToString([]byte("target=tree\nmode=implicit"))
	xmlPayload := `<signbatch stoponerror="true" algorithm="SHA256withRSA" suboperation="countersign">` +
		`<singlesign Id="xml-v19"><datasource>` +
		base64.StdEncoding.EncodeToString([]byte("firma-previa")) +
		`</datasource><format>CAdES</format><extraparams>` + params +
		`</extraparams></singlesign></signbatch>`

	cmd, err := adaptador.ParseBatchPayload([]byte(xmlPayload), domain.ExchangeSession{})
	if err != nil {
		t.Fatalf("ParseBatchPayload() error = %v", err)
	}
	if !cmd.StopOnError {
		t.Fatal("stoponerror V1.9 no se trasladó al comando de aplicación")
	}
	if got := cmd.Jobs[0].Options["target"]; got != "tree" {
		t.Fatalf("target = %q, want tree", got)
	}
	if got := cmd.Jobs[0].Options["mode"]; got != "implicit" {
		t.Fatalf("mode = %q, want implicit", got)
	}
}

func TestParse_BatchJSONRemotoLegacyGeneraRemoteBatchCommand(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	batchJSON := `{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc1","datareference":"token-remoto"}]}`

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://batch?id=req-batch&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat="+
			base64.StdEncoding.EncodeToString([]byte(batchJSON))+
			"&jsonbatch=true&batchpresignerurl=https%3A%2F%2Fpre.example%2Fbatch&batchpostsignerurl=https%3A%2F%2Fpost.example%2Fbatch&needcert=true")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if solicitud.RemoteBatch == nil {
		t.Fatal("se esperaba RemoteBatchCommand para batch remoto legacy")
	}
	if solicitud.BatchCommand != nil {
		t.Fatal("no se esperaba ProcessBatchCommand cuando el lote es remoto")
	}
	if solicitud.RemoteBatch.PreSignEndpoint != "https://pre.example/batch" {
		t.Fatalf("presign inesperado: %s", solicitud.RemoteBatch.PreSignEndpoint)
	}
	if solicitud.RemoteBatch.PostSignEndpoint != "https://post.example/batch" {
		t.Fatalf("postsign inesperado: %s", solicitud.RemoteBatch.PostSignEndpoint)
	}
	if !solicitud.RemoteBatch.NeedCert {
		t.Fatal("needcert debería conservarse")
	}
	if got := len(solicitud.Origenes); got != 3 {
		t.Fatalf("se esperaban 3 orígenes (store/pre/post), obtenidos %d: %#v", got, solicitud.Origenes)
	}
}

func TestParseSocket_BatchJSONRemotoLegacySinJsonBatchSigueSiendoRemoto(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	batchJSON := `{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc1","datareference":"token-remoto"}]}`

	solicitud, err := adaptador.ParseSocket(context.Background(),
		"afirma://batch?op=batch&id=req-batch-socket&key=clave123&stservlet=https%3A%2F%2Fstore.example%2FStorageService&dat="+
			base64.StdEncoding.EncodeToString([]byte(batchJSON))+
			"&batchpresignerurl=https%3A%2F%2Fpre.example%2Fbatch&batchpostsignerurl=https%3A%2F%2Fpost.example%2Fbatch&needcert=true")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if solicitud.RemoteBatch == nil {
		t.Fatal("se esperaba RemoteBatchCommand en ParseSocket aunque jsonbatch no venga explícito")
	}
	if solicitud.BatchCommand != nil {
		t.Fatal("no se esperaba ProcessBatchCommand cuando el lote remoto trae pre/post signer")
	}
	if got := solicitud.RemoteBatch.Session.RequestID; got != "req-batch-socket" {
		t.Fatalf("requestID inesperado en sesión remota: %q", got)
	}
	if got := solicitud.RemoteBatch.Session.SessionKey; got != "clave123" {
		t.Fatalf("sessionKey inesperada en sesión remota: %q", got)
	}
	if got := solicitud.RemoteBatch.Session.UploadEndpoint; got != "https://store.example/StorageService" {
		t.Fatalf("uploadEndpoint inesperado en sesión remota: %q", got)
	}
	if got := solicitud.RemoteBatch.Session.RetrieveEndpoint; got != "https://store.example/StorageService" {
		t.Fatalf("retrieveEndpoint inesperado en sesión remota: %q", got)
	}
	if !solicitud.RemoteBatch.NeedCert {
		t.Fatal("needcert debería conservarse")
	}
}

func TestParseBatchPayload_XMLConDataURI(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	xmlPayload := `<signbatch stoponerror="true" algorithm="SHA256withRSA" suboperation="sign"><singlesign Id="xml1"><datasource>data:text/xml;base64,` +
		base64.StdEncoding.EncodeToString([]byte("<root/>")) +
		`</datasource><format>XAdES</format><extraparams>policy=demo</extraparams></singlesign></signbatch>`

	cmd, err := adaptador.ParseBatchPayload([]byte(xmlPayload), domain.ExchangeSession{
		RequestID:        "req-xml",
		UploadEndpoint:   "https://store.example/StorageService",
		RetrieveEndpoint: "https://store.example/RetrieveService",
		State:            domain.SessionActive,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if got := len(cmd.Jobs); got != 1 {
		t.Fatalf("numero de trabajos inesperado: %d", got)
	}
	job := cmd.Jobs[0]
	if job.Format != domain.FormatXAdES {
		t.Fatalf("formato inesperado: %s", job.Format)
	}
	if job.Document.MIMEType != "application/xml" {
		t.Fatalf("mime inesperado: %s", job.Document.MIMEType)
	}
	if string(job.Document.Content) != "<root/>" {
		t.Fatalf("contenido inesperado: %q", string(job.Document.Content))
	}
	if job.Options["policy"] != "demo" {
		t.Fatalf("opciones inesperadas: %#v", job.Options)
	}
}

func TestParse_BatchSinDatRequiereRecuperacionRemota(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://batch?fileId=req-43&retrieveServlet=https%3A%2F%2Fretrieve.example%2FRetrieveService&storageServlet=https%3A%2F%2Fstore.example%2FStorageService")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.BatchCommand != nil {
		t.Fatal("no se esperaba ProcessBatchCommand sin payload embebido")
	}
	if solicitud.RetrieveCommand == nil {
		t.Fatal("se esperaba RetrieveRequestCommand para lote remoto")
	}
}

func TestParseBatchPayload_ReferenciaNoEmbebidaNoSoportada(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	batchJSON := `{"format":"CAdES","singlesigns":[{"id":"doc1","datareference":"https://example.invalid/doc.bin"}]}`

	_, err := adaptador.ParseBatchPayload([]byte(batchJSON), domain.ExchangeSession{
		RequestID:        "req-batch",
		UploadEndpoint:   "https://store.example/StorageService",
		RetrieveEndpoint: "https://store.example/RetrieveService",
		State:            domain.SessionActive,
	})
	if err == nil {
		t.Fatal("se esperaba error para referencia de datos remota no soportada")
	}
}

func TestParse_URIInvalida(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	if _, err := adaptador.Parse(context.Background(), "https://example.com/sign?id=1"); err == nil {
		t.Fatal("se esperaba error para esquema no afirma://")
	}
}

func TestParse_OrigenNoAutorizado(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustDenied})

	_, err := adaptador.Parse(context.Background(),
		"afirma://sign?id=req-7&stservlet=https%3A%2F%2Fstore.example%2FStorageService")
	if err == nil {
		t.Fatal("se esperaba error por origen no autorizado")
	}
}

func TestParse_ParametrosFaltantes(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	if _, err := adaptador.Parse(context.Background(), "afirma://sign?foo=bar"); err == nil {
		t.Fatal("se esperaba error por parametros insuficientes")
	}
}

func TestParse_SaveLoadYSignAndSaveLocalesNoRequierenSesion(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	saveReq, err := adaptador.Parse(context.Background(), "afirma://save?dat=QUJD&filename=demo.txt")
	if err != nil {
		t.Fatalf("save parse error = %v", err)
	}
	if saveReq.Operacion != OperacionSave {
		t.Fatalf("save operacion = %q", saveReq.Operacion)
	}
	if saveReq.RetrieveCommand != nil || saveReq.SignCommand != nil {
		t.Fatalf("save no debe requerir sesion ni SignCommand: %+v", saveReq)
	}

	loadReq, err := adaptador.Parse(context.Background(), "afirma://load?filePath=documento.txt")
	if err != nil {
		t.Fatalf("load parse error = %v", err)
	}
	if loadReq.Operacion != OperacionLoad {
		t.Fatalf("load operacion = %q", loadReq.Operacion)
	}
	if loadReq.RetrieveCommand != nil || loadReq.SignCommand != nil {
		t.Fatalf("load no debe requerir sesion ni SignCommand: %+v", loadReq)
	}

	signSaveReq, err := adaptador.Parse(context.Background(), "afirma://signandsave?dat=QUJD&format=PAdES&filename=salida.pdf")
	if err != nil {
		t.Fatalf("signandsave parse error = %v", err)
	}
	if signSaveReq.Operacion != OperacionSignSave {
		t.Fatalf("signandsave operacion = %q", signSaveReq.Operacion)
	}
	if signSaveReq.SignCommand == nil {
		t.Fatal("signandsave debe construir SignCommand")
	}
	if signSaveReq.Formato != domain.FormatPAdES {
		t.Fatalf("signandsave formato = %q", signSaveReq.Formato)
	}
}

func TestParseSocket_SignYSelectCertSinServlets(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	signReq, err := adaptador.ParseSocket(context.Background(), "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA")
	if err != nil {
		t.Fatalf("ParseSocket(sign) error = %v", err)
	}
	if signReq.Operacion != OperacionFirma {
		t.Fatalf("sign operacion = %q", signReq.Operacion)
	}
	if signReq.SignCommand == nil {
		t.Fatal("sign por socket debe construir SignCommand")
	}
	if signReq.RetrieveCommand != nil {
		t.Fatalf("sign por socket no debe requerir retrieve: %+v", signReq)
	}
	if signReq.Sesion.RequestID != "" {
		t.Fatalf("sign por socket no debe construir sesion remota: %+v", signReq.Sesion)
	}

	selectReq, err := adaptador.ParseSocket(context.Background(), "afirma://selectcert?sticky=true")
	if err != nil {
		t.Fatalf("ParseSocket(selectcert) error = %v", err)
	}
	if selectReq.Operacion != OperacionSelectCert {
		t.Fatalf("selectcert operacion = %q", selectReq.Operacion)
	}
	if selectReq.Sesion.RequestID != "" || selectReq.RetrieveCommand != nil || selectReq.SignCommand != nil {
		t.Fatalf("selectcert por socket no debe requerir sesion: %+v", selectReq)
	}
}

func TestParseSocket_SignConKSB64SinServlets(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})

	req, err := adaptador.ParseSocket(context.Background(), "afirma://sign?ksb64=QUJDREVGRw==&format=CAdES")
	if err != nil {
		t.Fatalf("ParseSocket(sign ksb64) error = %v", err)
	}
	if req.SignCommand == nil {
		t.Fatal("sign por socket con ksb64 debe construir SignCommand")
	}
	if got := string(req.SignCommand.Document.Content); got != "ABCDEFG" {
		t.Fatalf("contenido = %q, want %q", got, "ABCDEFG")
	}
}

func TestParseSocket_BatchEmbebidoSinServlets(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	batchJSON := `{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc1","datareference":"` +
		base64.StdEncoding.EncodeToString([]byte("ABC")) +
		`"}]}`

	req, err := adaptador.ParseSocket(context.Background(),
		"afirma://batch?dat="+base64.StdEncoding.EncodeToString([]byte(batchJSON)))
	if err != nil {
		t.Fatalf("ParseSocket(batch) error = %v", err)
	}
	if req.Operacion != OperacionLote {
		t.Fatalf("batch operacion = %q", req.Operacion)
	}
	if req.BatchCommand == nil {
		t.Fatal("batch por socket debe construir ProcessBatchCommand")
	}
	if req.RemoteBatch != nil || req.RetrieveCommand != nil {
		t.Fatalf("batch por socket no debe requerir remoto: %+v", req)
	}
}

func TestParseBatchMetadata_JSONYXML(t *testing.T) {
	jsonMeta, err := ParseBatchMetadata([]byte(`{"stoponerror":true,"singlesigns":[{"id":"a"},{"id":"b"}]}`))
	if err != nil {
		t.Fatalf("ParseBatchMetadata(json) error = %v", err)
	}
	if !jsonMeta.IsJSON || !jsonMeta.StopOnError {
		t.Fatalf("json meta inesperada: %+v", jsonMeta)
	}
	if len(jsonMeta.IDs) != 2 || jsonMeta.IDs[0] != "a" || jsonMeta.IDs[1] != "b" {
		t.Fatalf("json ids inesperados: %#v", jsonMeta.IDs)
	}

	xmlMeta, err := ParseBatchMetadata([]byte(`<signbatch stoponerror="false"><singlesign Id="x"></singlesign></signbatch>`))
	if err != nil {
		t.Fatalf("ParseBatchMetadata(xml) error = %v", err)
	}
	if xmlMeta.IsJSON || xmlMeta.StopOnError {
		t.Fatalf("xml meta inesperada: %+v", xmlMeta)
	}
	if len(xmlMeta.IDs) != 1 || xmlMeta.IDs[0] != "x" {
		t.Fatalf("xml ids inesperados: %#v", xmlMeta.IDs)
	}
}

type trustStub struct {
	estado domain.TrustStatus
	err    error
}

func (t trustStub) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	if t.err != nil {
		return domain.TrustDecision{}, t.err
	}
	return domain.TrustDecision{
		Origin: origin,
		Status: t.estado,
	}, nil
}

func (t trustStub) Allow(context.Context, string) error {
	return nil
}

func (t trustStub) Deny(context.Context, string) error {
	return nil
}

func (t trustStub) Remove(context.Context, string) error {
	return nil
}

func TestOpcionesFirmaLegacy_PropagaAlgoritmoYElevaSHA1(t *testing.T) {
	casos := map[string]string{
		"SHA512withRSA":   "SHA512withRSA",
		"SHA384withECDSA": "SHA384withECDSA",
		"SHA1withRSA":     "SHA256withRSA",
		"SHA-1withECDSA":  "SHA256withECDSA",
	}
	for pedido, want := range casos {
		opts := opcionesFirmaLegacy(url.Values{"algorithm": {pedido}})
		if got := opts["algorithm"]; got != want {
			t.Fatalf("algorithm=%q -> %q, want %q", pedido, got, want)
		}
	}
	props := base64.StdEncoding.EncodeToString([]byte("algorithm=SHA384withRSA\n"))
	opts := opcionesFirmaLegacy(url.Values{"algorithm": {"SHA512withRSA"}, "properties": {props}})
	if got := opts["algorithm"]; got != "SHA384withRSA" {
		t.Fatalf("las properties explícitas deben prevalecer: %q", got)
	}
}

func TestParse_SignAndSaveAdmiteCofirmaConCop(t *testing.T) {
	dat := base64.StdEncoding.EncodeToString([]byte("firma previa"))
	sol, err := New(nil).Parse(context.Background(), "afirma://signandsave?op=signandsave&cop=cosign&format=CAdES&dat="+dat)
	if err != nil {
		t.Fatal(err)
	}
	if sol.AccionFirma != domain.ActionCoSign || sol.SignCommand == nil || sol.SignCommand.Action != domain.ActionCoSign {
		t.Fatalf("cop=cosign no aplicado: %+v", sol.SignCommand)
	}
}

// URL real de FIRe (Aragón) con certificado local: CAdEStri, serverUrl y
// "dat" con el identificador de la transacción.
func TestParse_FIReTrifasicoConServidor(t *testing.T) {
	props := base64.URLEncoding.EncodeToString([]byte("serverUrl=https://firesda.example.es/public/afirma/triphaseSignService\nprecalculatedHashAlgorithm=SHA-512\n"))
	raw := "afirma://sign?jvc=3&ver=3&op=sign&id=yLGnJH7bHuFzWgg86p1O&key=93671378" +
		"&stservlet=https%3A%2F%2Ffiresda.example.es%2Fpublic%2Fafirma%2Fstorage&format=CAdEStri&algorithm=SHA512withRSA" +
		"&properties=" + props + "&dat=" + base64.URLEncoding.EncodeToString([]byte("fc00c581-b871-458f-8972-b7d0ff396648"))
	sol, err := New(nil).Parse(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if sol.RetrieveCommand != nil || sol.SignCommand == nil {
		t.Fatalf("debe ser una firma con dat, no una descarga: retrieve=%v sign=%v", sol.RetrieveCommand != nil, sol.SignCommand != nil)
	}
	if sol.Sesion.RequestID != "yLGnJH7bHuFzWgg86p1O" {
		t.Fatalf("el identificador de subida debe ser el parámetro id: %q", sol.Sesion.RequestID)
	}
	if got := string(sol.SignCommand.Document.Content); got != "fc00c581-b871-458f-8972-b7d0ff396648" {
		t.Fatalf("dat = %q", got)
	}
}
