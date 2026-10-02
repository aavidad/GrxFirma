// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const maxBytesCRL = 64 << 20

var (
	oidDeltaCRLIndicator        = asn1.ObjectIdentifier{2, 5, 29, 27}
	oidIssuingDistributionPoint = asn1.ObjectIdentifier{2, 5, 29, 28}
)

// CRLLocal es una CRL leída del directorio, con su emisor ya extraído.
type CRLLocal struct {
	DER   []byte
	lista *x509.RevocationList
}

type entradaCRL struct {
	modificado time.Time
	tamano     int64
	crls       []CRLLocal
}

// AlmacenCRL lee CRL de un directorio local fijado por configuración. Se
// relee en cada consulta para admitir actualizaciones sin reiniciar, pero
// cada fichero solo se vuelve a analizar si cambian su fecha o su tamaño.
type AlmacenCRL struct {
	dir   string
	mu    sync.Mutex
	cache map[string]entradaCRL
}

// NuevoAlmacenCRL comprueba que el directorio existe y es legible.
func NuevoAlmacenCRL(dir string) (*AlmacenCRL, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("verificacionlocal: directorio de CRL vacío")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("verificacionlocal: no se puede leer el directorio de CRL: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("verificacionlocal: la ruta de CRL debe ser un directorio")
	}
	return &AlmacenCRL{dir: dir, cache: map[string]entradaCRL{}}, nil
}

// CRLsDe devuelve las CRL del directorio cuyo emisor coincide con el sujeto
// del emisor indicado. La autenticación y la vigencia se comprueban después.
func (a *AlmacenCRL) CRLsDe(ctx context.Context, emisor *x509.Certificate) ([]CRLLocal, error) {
	if a == nil || emisor == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ficheros, err := listarFicheros(a.dir, []string{".crl", ".pem", ".der"})
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	vigentes := make(map[string]struct{}, len(ficheros))
	var out []CRLLocal
	for _, fichero := range ficheros {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vigentes[fichero] = struct{}{}
		crls := a.cargar(fichero)
		for _, crl := range crls {
			if bytes.Equal(crl.lista.RawIssuer, emisor.RawSubject) {
				out = append(out, crl)
			}
		}
	}
	for ruta := range a.cache {
		if _, sigue := vigentes[ruta]; !sigue {
			delete(a.cache, ruta)
		}
	}
	return out, nil
}

// cargar devuelve las CRL de un fichero; un fichero ilegible o mal formado
// se ignora (no aporta evidencia) sin impedir usar los demás.
func (a *AlmacenCRL) cargar(ruta string) []CRLLocal {
	info, err := os.Stat(ruta)
	if err != nil {
		return nil
	}
	if previa, ok := a.cache[ruta]; ok && previa.modificado.Equal(info.ModTime()) && previa.tamano == info.Size() {
		return previa.crls
	}
	contenido, err := leerAcotado(ruta, maxBytesCRL)
	if err != nil {
		a.cache[ruta] = entradaCRL{modificado: info.ModTime(), tamano: info.Size()}
		return nil
	}
	var bloques [][]byte
	if bytes.Contains(contenido, []byte("-----BEGIN")) {
		resto := contenido
		for {
			var bloque *pem.Block
			bloque, resto = pem.Decode(resto)
			if bloque == nil {
				break
			}
			if bloque.Type == "X509 CRL" {
				bloques = append(bloques, bloque.Bytes)
			}
		}
	} else {
		bloques = [][]byte{contenido}
	}
	var crls []CRLLocal
	for _, der := range bloques {
		lista, err := x509.ParseRevocationList(der)
		if err != nil {
			continue
		}
		crls = append(crls, CRLLocal{DER: bytes.Clone(der), lista: lista})
	}
	a.cache[ruta] = entradaCRL{modificado: info.ModTime(), tamano: info.Size(), crls: crls}
	return crls
}

// crlAplicable decide si una CRL cubre de forma completa al certificado.
// Una CRL delta, indirecta, restringida por motivos o particionada en un
// punto de distribución que el certificado no declara no puede acreditar
// que el certificado esté vigente, así que se descarta (fallo cerrado).
func crlAplicable(lista *x509.RevocationList, cert *x509.Certificate) (bool, string) {
	for _, ext := range lista.Extensions {
		if ext.Id.Equal(oidDeltaCRLIndicator) {
			return false, "crl_delta_no_admitida"
		}
	}
	for _, ext := range lista.Extensions {
		if !ext.Id.Equal(oidIssuingDistributionPoint) {
			continue
		}
		idp, err := parsearIDP(ext.Value)
		if err != nil {
			return false, "crl_idp_no_interpretable"
		}
		switch {
		case idp.soloAlgunosMotivos || idp.indirecta || idp.soloAtributos || idp.nombreRelativo:
			return false, "crl_parcial_no_admitida"
		case idp.soloUsuarios && cert.IsCA:
			return false, "crl_no_cubre_ca"
		case idp.soloCA && !cert.IsCA:
			return false, "crl_no_cubre_usuario"
		}
		if len(idp.uris) > 0 && !intersecta(idp.uris, cert.CRLDistributionPoints) {
			return false, "crl_de_otra_particion"
		}
	}
	return true, ""
}

type puntoDistribucionEmisor struct {
	uris               []string
	nombreRelativo     bool
	soloUsuarios       bool
	soloCA             bool
	soloAlgunosMotivos bool
	indirecta          bool
	soloAtributos      bool
}

// parsearIDP interpreta IssuingDistributionPoint (RFC 5280, 5.2.5).
func parsearIDP(der []byte) (puntoDistribucionEmisor, error) {
	var out puntoDistribucionEmisor
	var secuencia asn1.RawValue
	if resto, err := asn1.Unmarshal(der, &secuencia); err != nil || len(resto) != 0 ||
		secuencia.Class != asn1.ClassUniversal || secuencia.Tag != asn1.TagSequence {
		return out, errors.New("IDP no es una SEQUENCE")
	}
	campos := secuencia.Bytes
	for len(campos) > 0 {
		var campo asn1.RawValue
		var err error
		campos, err = asn1.Unmarshal(campos, &campo)
		if err != nil || campo.Class != asn1.ClassContextSpecific {
			return out, errors.New("campo IDP no válido")
		}
		switch campo.Tag {
		case 0:
			if err := parsearNombrePuntoDistribucion(campo.Bytes, &out); err != nil {
				return out, err
			}
		case 1:
			out.soloUsuarios = booleanoImplicito(campo)
		case 2:
			out.soloCA = booleanoImplicito(campo)
		case 3:
			out.soloAlgunosMotivos = true
		case 4:
			out.indirecta = booleanoImplicito(campo)
		case 5:
			out.soloAtributos = booleanoImplicito(campo)
		default:
			return out, errors.New("campo IDP desconocido")
		}
	}
	return out, nil
}

func parsearNombrePuntoDistribucion(der []byte, out *puntoDistribucionEmisor) error {
	var nombre asn1.RawValue
	if _, err := asn1.Unmarshal(der, &nombre); err != nil || nombre.Class != asn1.ClassContextSpecific {
		return errors.New("DistributionPointName no válido")
	}
	if nombre.Tag == 1 {
		out.nombreRelativo = true
		return nil
	}
	if nombre.Tag != 0 {
		return errors.New("DistributionPointName desconocido")
	}
	nombres := nombre.Bytes
	for len(nombres) > 0 {
		var general asn1.RawValue
		var err error
		nombres, err = asn1.Unmarshal(nombres, &general)
		if err != nil {
			return errors.New("GeneralName no válido")
		}
		// uniformResourceIdentifier [6] IA5String
		if general.Class == asn1.ClassContextSpecific && general.Tag == 6 {
			out.uris = append(out.uris, string(general.Bytes))
		}
	}
	return nil
}

func booleanoImplicito(campo asn1.RawValue) bool {
	return len(campo.Bytes) == 1 && campo.Bytes[0] != 0
}

func intersecta(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(y)) {
				return true
			}
		}
	}
	return false
}
