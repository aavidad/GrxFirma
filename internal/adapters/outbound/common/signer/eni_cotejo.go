// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
)

// ErrFirmaNoCorrespondeOriginal indica que la firma no cubre el documento
// aportado como original, o que no se ha podido comprobar. Antes de envolver
// una firma en un documento ENI se coteja sin red: un ENI que une un
// documento con una firma de otro aparentaría estar firmado sin estarlo.
var ErrFirmaNoCorrespondeOriginal = errors.New("la firma no corresponde al documento original")

// ErrFirmaCorrespondeConSHA1 indica que el cotejo ha ido bien (el contenido
// corresponde y la firma es correcta) pero alguna firma usa SHA-1. Sigue el
// criterio general de SHA-1 del proyecto (algoritmo_antiguo.go): se admite
// para no dejar fuera firmas antiguas, con aviso visible y registrado. Nunca
// se devuelve si el contenido no corresponde. Los llamantes lo resuelven con
// AdmitirCotejoSHA1.
var ErrFirmaCorrespondeConSHA1 = errors.New("la firma corresponde al documento, pero usa SHA-1")

// ErrFirmaCorrespondeConC14NAntigua indica que se pudo cotejar el contenido
// integrado byte a byte, aunque la firma necesita el lector XML histórico.
var ErrFirmaCorrespondeConC14NAntigua = errors.New(excepciones.Warning("xml_c14n_antigua"))

// ErrCotejoSHA1Prohibido indica que la firma corresponde, usa SHA-1 y la
// política de máquina lo prohíbe (permitir_sha1_legacy deshabilitada).
var ErrCotejoSHA1Prohibido = errors.New("excepciones.sha1_conservar.bloqueo")

// AvisoFirmaSHA1Corresponde es la clave de catálogo del aviso que acompaña a
// un cotejo admitido con SHA-1.
var AvisoFirmaSHA1Corresponde = excepciones.Warning("sha1_conservar")

// AdmitirCotejoSHA1 resuelve el resultado de un cotejo. Con
// ErrFirmaCorrespondeConSHA1 devuelve aviso=true y err=nil, salvo que la
// política lo prohíba (ErrCotejoSHA1Prohibido); deja constancia en el
// registro. Cualquier otro error se devuelve tal cual.
func AdmitirCotejoSHA1(err error, operacion string, contexts ...context.Context) (aviso bool, errFinal error) {
	if !errors.Is(err, ErrFirmaCorrespondeConSHA1) {
		return false, err
	}
	if err := excepciones.Check("sha1_conservar", contexts...); err != nil {
		slog.Warn("cotejo_firma_sha1_prohibido", "operacion", operacion)
		return false, cotejoSHA1Bloqueado{err}
	}
	slog.Warn("cotejo_firma_sha1_admitido", "operacion", operacion)
	return true, nil
}

type cotejoSHA1Bloqueado struct{ causa error }

func (e cotejoSHA1Bloqueado) Error() string        { return e.causa.Error() }
func (e cotejoSHA1Bloqueado) Unwrap() error        { return e.causa }
func (e cotejoSHA1Bloqueado) Is(target error) bool { return target == ErrCotejoSHA1Prohibido }

// CotejarCAdESExplicita comprueba que el atributo messageDigest de cada
// firmante coincide con el resumen del original y que la firma de los
// atributos es válida. No consulta revocación ni confianza. Si todo
// corresponde pero algún firmante usa SHA-1, devuelve
// ErrFirmaCorrespondeConSHA1.
func CotejarCAdESExplicita(cmsDER, original []byte) error {
	if len(original) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	ci, err := parseContentInfo(cmsDER)
	if err != nil || !ci.ContentType.Equal(oidSignedData) {
		return ErrFirmaNoCorrespondeOriginal
	}
	sd, err := parseSignedData(ci.Content.Bytes)
	if err != nil {
		return ErrFirmaNoCorrespondeOriginal
	}
	certs, _, err := parseCertificateSet(sd.Certificates)
	if err != nil || len(certs) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	signerInfos, err := parseSignerInfos(sd.SignerInfos)
	if err != nil || len(signerInfos) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	conSHA1 := false
	for _, si := range signerInfos {
		cert, err := findSignerCertificate(certs, si.SID)
		if err != nil {
			return ErrFirmaNoCorrespondeOriginal
		}
		// SHA-1 se comprueba igual que los demás resúmenes; solo cambia el
		// resultado final, que lleva el aviso (lote 16c).
		spec, antiguo, err := validarAlgoritmosSignerInfo(cert, si)
		if err != nil {
			return ErrFirmaNoCorrespondeOriginal
		}
		if verifyMessageDigest(si.SignedAttributes, original, spec.hash) != nil ||
			verifyCMSignature(cert, si, spec.hash) != nil {
			return ErrFirmaNoCorrespondeOriginal
		}
		conSHA1 = conSHA1 || antiguo
	}
	if conSHA1 {
		return ErrFirmaCorrespondeConSHA1
	}
	return nil
}

// CotejarXAdESSeparada verifica las referencias y el valor de cada firma XML
// tomando el original para las referencias externas. Si la firma lleva el
// documento dentro de CONTENT (modo compatible con AutoFirma Java), exige que
// alguna referencia lo cubra y que su contenido sea el original.
func CotejarXAdESSeparada(firma, original []byte, contexts ...context.Context) error {
	if len(original) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	ctx := context.Background()
	if len(contexts) > 0 && contexts[0] != nil {
		ctx = contexts[0]
	}
	externa := false
	resolver := func(string) ([]byte, error) {
		externa = true
		return original, nil
	}
	verificacion, err := verifyXMLSignatureDocument(ctx, firma, false, resolver, domain.CertificateChain{}, string(domain.FormatXAdES), "", nil, coberturaDelContenedor)
	if err != nil {
		var block *excepciones.Block
		if errors.As(err, &block) {
			return err
		}
		return ErrFirmaNoCorrespondeOriginal
	}
	if block := excepciones.FromText(verificacion.result.Reason, ctx); block != nil {
		return block
	}
	if !externa && !contenidoInternoCoincide(firma, original) {
		return ErrFirmaNoCorrespondeOriginal
	}
	if compatibilidadC14N(xmlCompatibilityDetails(verificacion.result)) {
		if externa {
			// La canonicalización puede igualar dos archivos distintos. Sin una
			// copia literal dentro de la firma no se acreditan los bytes del ENI.
			return ErrFirmaNoCorrespondeOriginal
		}
		domain.AnotarAviso(ctx, excepciones.Warning("xml_c14n_antigua"))
		if len(algoritmosAntiguos(verificacion.result)) > 0 {
			return errors.Join(ErrFirmaCorrespondeConC14NAntigua, ErrFirmaCorrespondeConSHA1)
		}
		return ErrFirmaCorrespondeConC14NAntigua
	}
	if !verificacion.result.Valid && verificacion.result.Integrity.Status != domain.VerificationStatusValid {
		return ErrFirmaNoCorrespondeOriginal
	}
	if len(algoritmosAntiguos(verificacion.result)) > 0 {
		return ErrFirmaCorrespondeConSHA1
	}
	return nil
}

// contenidoInternoCoincide busca un CONTENT cuyo contenido es el original y
// exige que TODAS las firmas principales (las ds:Signature que no están
// dentro de otra, es decir, sin contar contrafirmas) lo referencien desde su
// propio ds:SignedInfo. Solo cuentan los ds:Reference (espacio de nombres
// xmldsig) hijos directos de ese ds:SignedInfo: un Reference suelto o dentro
// de un SignedInfo que no es de la firma no está firmado (KD-02). El Id del
// CONTENT tiene que ser único en todo el documento.
func contenidoInternoCoincide(firma, original []byte) bool {
	type contenido struct {
		id, encoding string
		inicio       int64
		fin          int64
	}
	type nodo struct {
		nombre    xml.Name
		contenido *contenido
		firma     int // índice de la firma principal a la que pertenece, o -1
	}
	var (
		contenidos []contenido
		// referenciasPorFirma[i] son los Id que referencia el SignedInfo
		// de la firma principal i.
		referenciasPorFirma []map[string]bool
		ids                 = map[string]int{}
		pila                []nodo
	)
	d := xml.NewDecoder(bytes.NewReader(firma))
	for {
		inicio := d.InputOffset()
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			for _, a := range t.Attr {
				if a.Name.Local == "Id" || a.Name.Local == "ID" || a.Name.Local == "id" {
					ids[a.Value]++
				}
			}
			actual := nodo{nombre: t.Name, firma: -1}
			if len(pila) > 0 {
				actual.firma = pila[len(pila)-1].firma
			}
			esDSig := t.Name.Space == nsXMLDSig
			switch {
			case esDSig && t.Name.Local == "Signature" && actual.firma < 0:
				referenciasPorFirma = append(referenciasPorFirma, map[string]bool{})
				actual.firma = len(referenciasPorFirma) - 1
			case esDSig && t.Name.Local == "Reference" && len(pila) >= 2:
				padre, abuelo := pila[len(pila)-1], pila[len(pila)-2]
				if actual.firma >= 0 &&
					padre.nombre == (xml.Name{Space: nsXMLDSig, Local: "SignedInfo"}) &&
					abuelo.nombre == (xml.Name{Space: nsXMLDSig, Local: "Signature"}) &&
					abuelo.firma == actual.firma && (len(pila) < 3 || pila[len(pila)-3].firma < 0) {
					for _, a := range t.Attr {
						if a.Name.Local == "URI" && a.Name.Space == "" && strings.HasPrefix(a.Value, "#") {
							referenciasPorFirma[actual.firma][strings.TrimPrefix(a.Value, "#")] = true
						}
					}
				}
			case t.Name.Local == "CONTENT" && t.Name.Space == "" && actual.firma < 0:
				c := &contenido{inicio: d.InputOffset()}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "Id":
						c.id = a.Value
					case "Encoding":
						c.encoding = a.Value
					}
				}
				actual.contenido = c
			}
			pila = append(pila, actual)
		case xml.EndElement:
			if len(pila) == 0 {
				return false
			}
			if c := pila[len(pila)-1].contenido; c != nil {
				c.fin = inicio
				contenidos = append(contenidos, *c)
			}
			pila = pila[:len(pila)-1]
		}
	}
	if len(referenciasPorFirma) == 0 {
		return false
	}
	referenciadoPorTodas := func(id string) bool {
		for _, refs := range referenciasPorFirma {
			if !refs[id] {
				return false
			}
		}
		return true
	}
	for _, c := range contenidos {
		if c.id == "" || ids[c.id] != 1 || !referenciadoPorTodas(c.id) || c.fin < c.inicio {
			continue
		}
		dentro := firma[c.inicio:c.fin]
		if strings.EqualFold(c.encoding, "Base64") {
			datos, err := base64.StdEncoding.DecodeString(compactBase64(string(dentro)))
			if err == nil && bytes.Equal(datos, original) {
				return true
			}
			continue
		}
		if strings.TrimSpace(string(dentro)) == stripXMLDeclaration(original) {
			return true
		}
		a, errA := canonicalizeXML(bytes.TrimSpace(dentro), algC14N)
		b, errB := canonicalizeXML([]byte(stripXMLDeclaration(original)), algC14N)
		if errA == nil && errB == nil && bytes.Equal(a, b) {
			return true
		}
	}
	return false
}

// ComprobarIntegridadPAdES verifica sin red que cada firma del PDF cubre sus
// bytes. No evalúa confianza ni revocación.
func ComprobarIntegridadPAdES(ctx context.Context, pdf []byte) error {
	ctx = context.WithValue(ctx, offlineVerificationContextKey{}, true)
	verifier := NewPAdESVerifierWithCAdES(NewCAdESVerifierWithChecker(NewRevocationCheckerOffline()))
	doc, err := domain.NewDocument("documento.pdf", pdf, "application/pdf")
	if err != nil {
		return ErrFirmaNoCorrespondeOriginal
	}
	result, _, err := verifier.Verify(ctx, doc, domain.CertificateChain{})
	if err == nil {
		for _, mensaje := range append(append([]string{result.Reason}, result.Errors...), result.Warnings...) {
			if block := excepciones.FromText(mensaje, ctx); block != nil && block.ID == "sha1_leer" {
				return block
			}
		}
	}
	if err != nil || result.Integrity.Status == domain.VerificationStatusInvalid {
		return ErrFirmaNoCorrespondeOriginal
	}
	// Una firma de subfiltro antiguo que GrxFirma sabe comprobar y no cuadra
	// (el PDF cambió tras firmarlo) solo deja el PDF «no verificado» al
	// verificar, por compatibilidad. Para envolverlo en un ENI no basta: el
	// documento no corresponde a lo firmado (revisión del lote 17a).
	for _, detalle := range result.Integrity.Details {
		if detalle == "firma_no_verificada subfilter="+subFiltroPKCS7SHA1 ||
			detalle == "firma_no_verificada subfilter="+subFiltroX509RSASHA1 {
			return ErrFirmaNoCorrespondeOriginal
		}
	}
	if len(algoritmosAntiguos(result)) > 0 {
		return ErrFirmaCorrespondeConSHA1
	}
	return nil
}
