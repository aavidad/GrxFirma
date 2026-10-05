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
	"sync"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// grupoBarrera imita la autorización conjunta: ningún trabajo obtiene su
// firma hasta que todos han entregado su resumen o han terminado.
type grupoBarrera struct {
	id         int
	mu         sync.Mutex
	pendientes int
	listo      chan struct{}
	hechos     []bool
	cerrado    bool
}

func (g *grupoBarrera) retirar() {
	g.pendientes--
	if g.pendientes == 0 {
		close(g.listo)
	}
}

func (g *grupoBarrera) firmar(i int) error {
	g.mu.Lock()
	if g.hechos[i] {
		g.mu.Unlock()
		return errors.New("firma repetida")
	}
	g.hechos[i] = true
	g.retirar()
	g.mu.Unlock()
	select {
	case <-g.listo:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("el grupo no se firmó a la vez")
	}
}

func (g *grupoBarrera) Key(i int) ports.SigningKey {
	return claveGrupo{grupo: g, indice: i}
}

func (g *grupoBarrera) Done(i int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.hechos[i] {
		g.hechos[i] = true
		g.retirar()
	}
}

func (g *grupoBarrera) Close() {
	g.mu.Lock()
	g.cerrado = true
	g.mu.Unlock()
}

type claveGrupo struct {
	grupo  *grupoBarrera
	indice int
}

func (c claveGrupo) KeyID() string                 { return fmt.Sprintf("g%d-%d", c.grupo.id, c.indice) }
func (c claveGrupo) CertificateChainDER() [][]byte { return nil }

type claveLoteMock struct {
	claveMock
	capacidad int
	errCap    error
	errInicio error
	mu        sync.Mutex
	grupos    []*grupoBarrera
}

func (c *claveLoteMock) BatchCapacity(int) (int, error) { return c.capacidad, c.errCap }

func (c *claveLoteMock) BeginBatch(_ context.Context, n int) (ports.SigningBatch, error) {
	if c.errInicio != nil {
		return nil, c.errInicio
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	g := &grupoBarrera{id: len(c.grupos), pendientes: n, listo: make(chan struct{}), hechos: make([]bool, n)}
	c.grupos = append(c.grupos, g)
	return g, nil
}

// motorGrupoMock firma con la clave recibida; los documentos «falla*»
// fallan antes de pedir la firma.
type motorGrupoMock struct {
	mu      sync.Mutex
	sueltas int
}

func (m *motorGrupoMock) Sign(_ context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if strings.HasPrefix(job.Document.Name, "falla") {
		return domain.SignatureResult{}, errors.New("documento no válido")
	}
	if k, ok := key.(claveGrupo); ok {
		if err := k.grupo.firmar(k.indice); err != nil {
			return domain.SignatureResult{}, err
		}
	} else {
		m.mu.Lock()
		m.sueltas++
		m.mu.Unlock()
	}
	return domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte(job.Document.Name + "@" + key.KeyID())}, nil
}

func loteDeNombres(nombres ...string) application.ProcessBatchCommand {
	cmd := lotePrueba()
	cmd.Jobs = nil
	for _, n := range nombres {
		doc, _ := domain.NewDocument(n, []byte(n), "application/pdf")
		cmd.Jobs = append(cmd.Jobs, domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign})
	}
	return cmd
}

func nuevoLoteConClave(clave ports.SigningKey, motor ports.SignerEngine) *application.ProcessBatchUseCase {
	return application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: clave},
		motor,
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, &loggerMock{}),
		&publicadorMock{},
	)
}

func TestProcessBatch_ClaveAgrupableFirmaPorGruposALaVez(t *testing.T) {
	t.Parallel()
	clave := &claveLoteMock{claveMock: claveMock{"remota"}, capacidad: 3}
	motor := &motorGrupoMock{}
	uc := nuevoLoteConClave(clave, motor)
	res, err := uc.Ejecutar(context.Background(), loteDeNombres("a", "b", "falla-c", "d", "e", "f", "g"))
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if len(clave.grupos) != 2 || motor.sueltas != 1 {
		t.Fatalf("grupos = %d, firmas sueltas = %d; se esperaban 2 grupos y el último suelto", len(clave.grupos), motor.sueltas)
	}
	for _, g := range clave.grupos {
		if !g.cerrado {
			t.Fatal("un grupo no se cerró")
		}
	}
	if len(res.Errores) != 1 || res.Errores[2] == nil {
		t.Fatalf("errores = %v", res.Errores)
	}
	var datos []string
	for _, r := range res.Results {
		datos = append(datos, string(r.Result.Data))
	}
	esperado := "a@g0-0,b@g0-1,d@g1-0,e@g1-1,f@g1-2,g@remota"
	if strings.Join(datos, ",") != esperado {
		t.Fatalf("resultados = %v; se esperaba %s", datos, esperado)
	}
}

func TestProcessBatch_ClaveAgrupableConStopOnError(t *testing.T) {
	t.Parallel()
	clave := &claveLoteMock{claveMock: claveMock{"remota"}, capacidad: 4}
	uc := nuevoLoteConClave(clave, &motorGrupoMock{})
	cmd := loteDeNombres("a", "falla-b", "c", "d", "e")
	cmd.StopOnError = true
	res, err := uc.Ejecutar(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 || len(res.Errores) != 1 || res.Errores[1] == nil || len(clave.grupos) != 1 {
		t.Fatalf("resultados=%d errores=%v grupos=%d", len(res.Results), res.Errores, len(clave.grupos))
	}
}

func TestProcessBatch_CapacidadDeLaClaveFallaAntesDeFirmar(t *testing.T) {
	t.Parallel()
	clave := &claveLoteMock{claveMock: claveMock{"remota"}, errCap: errors.New("otp único")}
	motor := &motorGrupoMock{}
	uc := nuevoLoteConClave(clave, motor)
	if _, err := uc.Ejecutar(context.Background(), loteDeNombres("a", "b")); err == nil {
		t.Fatal("se esperaba error")
	}
	if motor.sueltas != 0 || len(clave.grupos) != 0 {
		t.Fatal("no debió firmarse nada")
	}
}

func TestProcessBatch_FalloAlIniciarGrupoMarcaSusTrabajos(t *testing.T) {
	t.Parallel()
	clave := &claveLoteMock{claveMock: claveMock{"remota"}, capacidad: 2, errInicio: errors.New("sin sesión")}
	uc := nuevoLoteConClave(clave, &motorGrupoMock{})
	res, err := uc.Ejecutar(context.Background(), loteDeNombres("a", "b", "c"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errores) != 2 || len(res.Results) != 1 {
		t.Fatalf("errores=%v resultados=%d", res.Errores, len(res.Results))
	}
}

func TestProcessBatch_CapacidadUnoFirmaComoSiempre(t *testing.T) {
	t.Parallel()
	clave := &claveLoteMock{claveMock: claveMock{"remota"}, capacidad: 1}
	motor := &motorGrupoMock{}
	uc := nuevoLoteConClave(clave, motor)
	res, err := uc.Ejecutar(context.Background(), loteDeNombres("a", "b", "c"))
	if err != nil || len(res.Results) != 3 || motor.sueltas != 3 || len(clave.grupos) != 0 {
		t.Fatalf("err=%v resultados=%d sueltas=%d grupos=%d", err, len(res.Results), motor.sueltas, len(clave.grupos))
	}
}
