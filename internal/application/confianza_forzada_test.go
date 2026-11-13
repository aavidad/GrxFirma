// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

const huellaPruebaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// almacenConfianzaMock guarda en memoria lo aceptado.
type almacenConfianzaMock struct {
	confianza domain.ConfianzaForzada
	errCargar error
	aceptadas []domain.AceptacionConfianza
}

func (m *almacenConfianzaMock) CargarConfianzaForzada(context.Context) (domain.ConfianzaForzada, error) {
	return m.confianza, m.errCargar
}

func (m *almacenConfianzaMock) AceptarConfianzaForzada(_ context.Context, a domain.AceptacionConfianza) error {
	if m.confianza.Prohibida {
		return domain.ErrConfianzaProhibida
	}
	m.aceptadas = append(m.aceptadas, a)
	m.confianza.Aceptaciones = append(m.confianza.Aceptaciones, a)
	return nil
}

func (m *almacenConfianzaMock) QuitarConfianzaForzada(_ context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error) {
	return false, nil
}

// verificadorConObstaculos imita a los verificadores reales: cada obstáculo
// se decide en el registro del contexto; si todos están aceptados la firma
// es válida y queda marcada. Con siempreInvalida hay además un problema que
// no se puede forzar.
type verificadorConObstaculos struct {
	obstaculos      []domain.ObstaculoConfianza
	ocultos         []domain.ObstaculoConfianza // solo se ven tras aceptar los anteriores
	siempreInvalida bool
	llamadas        int
}

func (v *verificadorConObstaculos) Verify(ctx context.Context, _ domain.Document, _ domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	v.llamadas++
	registro := domain.RegistroConfianzaForzadaDe(ctx)
	res := domain.VerificationResult{Valid: true, Reason: "firma correcta"}
	for _, lista := range [][]domain.ObstaculoConfianza{v.obstaculos, v.ocultos} {
		todas := true
		for _, o := range lista {
			if registro.Decidir(o) {
				res = res.MarcarConfianzaForzada(o)
				continue
			}
			todas = false
		}
		if !todas {
			res.Valid = false
			res.Reason = "obstáculo"
			break
		}
	}
	if v.siempreInvalida {
		res.Valid = false
		res.Reason = "integridad rota"
	}
	return res, nil, nil
}

func ucConConfianza(verificador *verificadorConObstaculos, almacen *almacenConfianzaMock, logger *loggerMock) *application.VerifySignatureUseCase {
	return application.NuevoVerifySignatureUseCase(
		&trustAnchorMock{},
		verificador,
		application.NuevoAuditUseCase(relojMock{}, logger),
	).ConConfianzaForzada(almacen)
}

func contieneTexto(lista []string, buscado string) bool {
	for _, s := range lista {
		if s == buscado {
			return true
		}
	}
	return false
}

func TestVerifyConfianzaForzada_SinAceptarOfreceLoForzable(t *testing.T) {
	ca := domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: huellaPruebaA, Sujeto: "CN=Raíz"}
	componente := domain.ObstaculoConfianza{Tipo: domain.ConfianzaCertificadoComponente, Huella: strings.Repeat("b", 64)}
	verificador := &verificadorConObstaculos{obstaculos: []domain.ObstaculoConfianza{ca}, ocultos: []domain.ObstaculoConfianza{componente}}
	res, err := ucConConfianza(verificador, &almacenConfianzaMock{}, &loggerMock{}).Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verification.Valid {
		t.Fatal("sin aceptar nada la firma no puede ser válida")
	}
	if len(res.ConfianzaForzada.Forzable) != 2 {
		t.Fatalf("se esperaban los dos obstáculos (también el que solo se ve tras aceptar la CA): %+v", res.ConfianzaForzada.Forzable)
	}
	if !contieneTexto(res.Verification.Warnings, domain.ClavePistaConfianzaForzable) {
		t.Fatalf("falta la pista de que se puede aceptar: %v", res.Verification.Warnings)
	}
	for _, e := range res.Verification.Evidence {
		if e.Type == domain.EvidenciaConfianzaForzada {
			t.Fatalf("una firma no válida no puede llevar la marca de confianza forzada: %v", res.Verification.Evidence)
		}
	}
}

func TestVerifyConfianzaForzada_AceptadoEsValidoConAvisoYAuditoria(t *testing.T) {
	ca := domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: huellaPruebaA}
	almacen := &almacenConfianzaMock{confianza: domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{{Tipo: ca.Tipo, Huella: ca.Huella}}}}
	logger := &loggerMock{}
	res, err := ucConConfianza(&verificadorConObstaculos{obstaculos: []domain.ObstaculoConfianza{ca}}, almacen, logger).Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verification.Valid {
		t.Fatalf("aceptada la CA la firma debe ser válida: %s", res.Verification.Reason)
	}
	if !contieneTexto(res.Verification.Warnings, domain.ConfianzaCADesconocida.ClaveAplicada()) {
		t.Fatalf("falta el aviso visible: %v", res.Verification.Warnings)
	}
	if len(res.ConfianzaForzada.Aplicada) != 1 || len(res.ConfianzaForzada.Forzable) != 0 {
		t.Fatalf("resultado de confianza forzada inesperado: %+v", res.ConfianzaForzada)
	}
	if len(logger.registros) != 1 || !strings.Contains(string(logger.registros[0].Payload), "confianza_forzada=ca_desconocida:aaaaaaaaaaaaaaaa") {
		t.Fatalf("la auditoría debe dejar constancia de la confianza forzada: %v", logger.registros)
	}
}

func TestVerifyConfianzaForzada_NoForzableSiAceptarNoBasta(t *testing.T) {
	ca := domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: huellaPruebaA}
	almacen := &almacenConfianzaMock{confianza: domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{{Tipo: ca.Tipo, Huella: ca.Huella}}}}
	res, err := ucConConfianza(&verificadorConObstaculos{obstaculos: []domain.ObstaculoConfianza{ca}, siempreInvalida: true}, almacen, &loggerMock{}).Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verification.Valid || len(res.ConfianzaForzada.Forzable) != 0 {
		t.Fatalf("con un problema que no se puede forzar no se ofrece aceptar: %+v", res.ConfianzaForzada)
	}
	if contieneTexto(res.Verification.Warnings, domain.ClavePistaConfianzaForzable) || contieneTexto(res.Verification.Warnings, domain.ConfianzaCADesconocida.ClaveAplicada()) {
		t.Fatalf("no puede quedar el aviso de «se da por válida» ni la pista: %v", res.Verification.Warnings)
	}
}

func TestVerifyConfianzaForzada_PoliticaProhibeOIlegible(t *testing.T) {
	ca := domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: huellaPruebaA}
	aceptada := []domain.AceptacionConfianza{{Tipo: ca.Tipo, Huella: ca.Huella}}
	for nombre, almacen := range map[string]*almacenConfianzaMock{
		"prohibida": {confianza: domain.ConfianzaForzada{Aceptaciones: aceptada, Prohibida: true}},
		"ilegible":  {confianza: domain.ConfianzaForzada{Aceptaciones: aceptada}, errCargar: errors.New("config.json roto")},
	} {
		t.Run(nombre, func(t *testing.T) {
			res, err := ucConConfianza(&verificadorConObstaculos{obstaculos: []domain.ObstaculoConfianza{ca}}, almacen, &loggerMock{}).Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
			if err != nil {
				t.Fatal(err)
			}
			if res.Verification.Valid {
				t.Fatal("con la política prohibiéndolo, lo aceptado no puede aplicarse")
			}
			if !res.ConfianzaForzada.Prohibida || len(res.ConfianzaForzada.Forzable) != 0 {
				t.Fatalf("resultado inesperado: %+v", res.ConfianzaForzada)
			}
			if !contieneTexto(res.Verification.Warnings, domain.ClaveConfianzaForzadaProhibida) {
				t.Fatalf("el resultado debe decir que la política lo prohíbe: %v", res.Verification.Warnings)
			}
		})
	}
}

// Sin almacén (REST, afirma://, host nativo) no se aplica ni se ofrece nada.
func TestVerifyConfianzaForzada_SinAlmacenNoAplicaNada(t *testing.T) {
	ca := domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: huellaPruebaA}
	verificador := &verificadorConObstaculos{obstaculos: []domain.ObstaculoConfianza{ca}}
	uc := application.NuevoVerifySignatureUseCase(&trustAnchorMock{}, verificador, application.NuevoAuditUseCase(relojMock{}, &loggerMock{}))
	res, err := uc.Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verification.Valid || len(res.ConfianzaForzada.Forzable) != 0 || verificador.llamadas != 1 {
		t.Fatalf("sin almacén: valid=%v forzable=%v llamadas=%d", res.Verification.Valid, res.ConfianzaForzada.Forzable, verificador.llamadas)
	}
}

func TestGestionConfianzaForzada_AceptarValidaYAudita(t *testing.T) {
	almacen := &almacenConfianzaMock{}
	logger := &loggerMock{}
	gestion := application.NuevoGestionConfianzaForzadaUseCase(almacen, application.NuevoAuditUseCase(relojMock{}, logger))

	if _, err := gestion.Aceptar(context.Background(), domain.ObstaculoConfianza{Tipo: "lo_que_sea"}); !errors.Is(err, domain.ErrConfianzaTipoDesconocido) {
		t.Fatalf("tipo desconocido: %v", err)
	}
	if _, err := gestion.Aceptar(context.Background(), domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: "corta"}); !errors.Is(err, domain.ErrConfianzaHuellaInvalida) {
		t.Fatalf("huella inválida: %v", err)
	}
	if _, err := gestion.Aceptar(context.Background(), domain.ObstaculoConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1, Huella: huellaPruebaA}); !errors.Is(err, domain.ErrConfianzaHuellaSobrante) {
		t.Fatalf("huella en un caso global: %v", err)
	}
	aceptada, err := gestion.Aceptar(context.Background(), domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: strings.ToUpper(huellaPruebaA), Sujeto: "CN=Raíz"})
	if err != nil {
		t.Fatal(err)
	}
	if aceptada.Huella != huellaPruebaA || aceptada.Aviso != domain.ConfianzaCADesconocida.ClaveAviso() || aceptada.AceptadaEn.IsZero() {
		t.Fatalf("aceptación mal normalizada: %+v", aceptada)
	}
	if len(logger.registros) != 1 || !strings.Contains(fmt.Sprint(string(logger.registros[0].Payload)), "confianza_forzada_aceptar") {
		t.Fatalf("falta la auditoría de la aceptación: %v", logger.registros)
	}

	almacen.confianza.Prohibida = true
	if _, err := gestion.Aceptar(context.Background(), domain.ObstaculoConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1}); !errors.Is(err, domain.ErrConfianzaProhibida) {
		t.Fatalf("con la política prohibiéndolo no se puede aceptar: %v", err)
	}
}
