// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package identityevidence

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

func TestRegistroCifraEncadenaYReabre(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "privado", "evidencias.log")
	configuracion := Configuracion{Ruta: ruta, Clave: bytes.Repeat([]byte{0x5a}, 32), VersionClave: "clave-2026-01"}
	registro, err := Nuevo(configuracion)
	if err != nil {
		t.Fatalf("construir registro: %v", err)
	}
	evidencia := evidenciaPrueba()
	referencia, err := registro.RegistrarIdentidad(context.Background(), evidencia)
	if err != nil || referencia == "" {
		t.Fatalf("registrar evidencia: referencia=%q error=%v", referencia, err)
	}
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer registro: %v", err)
	}
	if bytes.Contains(contenido, evidencia.ContenidoCanonico) || bytes.Contains(contenido, []byte(evidencia.RetoID)) {
		t.Fatal("el registro expone evidencia en claro")
	}
	if _, err := Nuevo(configuracion); err != nil {
		t.Fatalf("reabrir cadena válida: %v", err)
	}
	assertPermisosRegistro(t, ruta)
}

func TestRegistroDetectaManipulacionAntesDeEscribir(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "privado", "evidencias.log")
	configuracion := Configuracion{Ruta: ruta, Clave: bytes.Repeat([]byte{0x33}, 32), VersionClave: "clave-1"}
	registro, err := Nuevo(configuracion)
	if err != nil {
		t.Fatalf("construir registro: %v", err)
	}
	if _, err := registro.RegistrarIdentidad(context.Background(), evidenciaPrueba()); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	contenido, err := os.ReadFile(ruta)
	if err != nil || len(contenido) == 0 {
		t.Fatalf("leer fixture: longitud=%d error=%v", len(contenido), err)
	}
	contenido[len(contenido)/2] ^= 1
	if err := os.WriteFile(ruta, contenido, 0o600); err != nil {
		t.Fatalf("manipular fixture: %v", err)
	}
	if _, err := Nuevo(configuracion); !errors.Is(err, ErrRegistroCorrupto) {
		t.Fatalf("no detectó manipulación: %v", err)
	}
}

func TestRegistroSerializaEscriturasConcurrentes(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "privado", "evidencias.log")
	configuracion := Configuracion{Ruta: ruta, Clave: bytes.Repeat([]byte{0x77}, 32), VersionClave: "clave-1"}
	registro, err := Nuevo(configuracion)
	if err != nil {
		t.Fatalf("construir registro: %v", err)
	}
	var grupo sync.WaitGroup
	errores := make(chan error, 16)
	for i := 0; i < 16; i++ {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			_, err := registro.RegistrarIdentidad(context.Background(), evidenciaPrueba())
			errores <- err
		}()
	}
	grupo.Wait()
	close(errores)
	for err := range errores {
		if err != nil {
			t.Fatalf("escritura concurrente: %v", err)
		}
	}
	if _, err := Nuevo(configuracion); err != nil {
		t.Fatalf("cadena concurrente inválida: %v", err)
	}
}

func TestRegistroRechazaConfiguracionDebil(t *testing.T) {
	_, err := Nuevo(Configuracion{Ruta: "relativa.log", Clave: make([]byte, 16), VersionClave: " clave "})
	if !errors.Is(err, ErrConfiguracionInvalida) {
		t.Fatalf("configuración débil admitida: %v", err)
	}
}

type lectorCancelaContexto struct{ cancel context.CancelFunc }

func (l lectorCancelaContexto) Read(p []byte) (int, error) {
	l.cancel()
	for i := range p {
		p[i] = 1
	}
	return len(p), nil
}

func TestRegistroNoEscribeSiContextoSeCancelaAntesDeAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "privado", "registro.log")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registro, err := Nuevo(Configuracion{Ruta: path, Clave: make([]byte, 32), VersionClave: "v1", Aleatorio: lectorCancelaContexto{cancel}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registro.RegistrarIdentidad(ctx, evidenciaPrueba()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelación no propagada: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || len(data) != 0 {
		t.Fatalf("escribió tras cancelar: longitud=%d error=%v", len(data), err)
	}
}

func TestRegistroRechazaSustitucionEntreConstruccionYAppend(t *testing.T) {
	for _, directory := range []bool{false, true} {
		for _, link := range []bool{false, true} {
			name := map[bool]string{false: "fichero", true: "directorio"}[directory] + map[bool]string{false: "-sustituido", true: "-enlace"}[link]
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "privado", "registro.log")
				config := Configuracion{Ruta: path, Clave: make([]byte, 32), VersionClave: "v1"}
				registro, err := Nuevo(config)
				if err != nil {
					t.Fatal(err)
				}
				target := path
				if directory {
					target = filepath.Dir(path)
				}
				backup := target + ".original"
				if err := os.Rename(target, backup); err != nil {
					t.Fatal(err)
				}
				if link {
					if err := os.Symlink(backup, target); err != nil {
						t.Skipf("sin privilegio para crear enlace sintético: %v", err)
					}
				} else {
					if directory {
						if err := os.Mkdir(target, 0o700); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := registro.RegistrarIdentidad(context.Background(), evidenciaPrueba()); err == nil {
					t.Fatal("escribió tras sustituir la ruta original")
				}
				original := backup
				if directory {
					original = filepath.Join(backup, "registro.log")
				}
				if data, err := os.ReadFile(original); err != nil || len(data) != 0 {
					t.Fatalf("registro original modificado: longitud=%d error=%v", len(data), err)
				}
				if link {
					if _, err := Nuevo(config); err == nil {
						t.Fatal("reabrió un registro mediante enlace")
					}
				}
			})
		}
	}
}

func evidenciaPrueba() ports.EvidenciaIdentidad {
	ahora := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	dictamen := domain.DictamenComprobacion{Estado: "conforme", Fuente: "prueba", ComprobadoEn: ahora}
	return ports.EvidenciaIdentidad{Contrato: domain.VersionContratoIdentidadReforzada, RetoID: "reto-sensible-1",
		PoliticaID: "politica-1", VersionPolitica: "1", ContenidoCanonico: []byte(`{"dato":"sensible"}`),
		HuellaContenido: "sha256:contenido", HuellaFirma: "sha256:firma", HuellaCertificado: "sha256:certificado",
		Resultado: domain.ResultadoIdentidadAceptada, Integridad: dictamen, Cadena: dictamen, Vigencia: dictamen,
		EKU: dictamen, Politica: dictamen, Revocacion: dictamen, ComprobadoEn: ahora}
}
