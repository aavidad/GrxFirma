// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sync"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
)

// claveConfianzaForzada es la clave de config.json con lo aceptado.
const claveConfianzaForzada = "confianza_forzada"

// maxConfianzaForzada limita las entradas que se guardan y se leen.
const maxConfianzaForzada = 256

// AlmacenConfianzaForzada lee y escribe en el config.json del usuario lo que
// acepta al verificar firmas (lote 14), y aplica la política de máquina
// prohibir_confianza_forzada. Implementa ports.AlmacenConfianzaForzada.
//
// Solo lo componen la CLI (cmd/grxfirma, fuera del servidor REST) y el
// backend del escritorio (cmd/grxfirma-gui). Ningún canal web lo recibe.
type AlmacenConfianzaForzada struct {
	dirUsuario string
	dirPolicy  string
	mu         sync.Mutex
}

// NuevoAlmacenConfianzaForzada usa ~/.config/grxfirma/config.json y la
// política de máquina del sistema.
func NuevoAlmacenConfianzaForzada(dirUsuario string) *AlmacenConfianzaForzada {
	return &AlmacenConfianzaForzada{dirUsuario: dirUsuario, dirPolicy: DirPolicyDefecto}
}

// NuevoAlmacenConfianzaForzadaConPolicy permite inyectar la política (pruebas).
func NuevoAlmacenConfianzaForzadaConPolicy(dirUsuario, dirPolicy string) *AlmacenConfianzaForzada {
	return &AlmacenConfianzaForzada{dirUsuario: dirUsuario, dirPolicy: dirPolicy}
}

// CargarConfianzaForzada devuelve las entradas válidas y si la política las
// prohíbe. Las entradas mal formadas se ignoran con un aviso en el registro.
func (a *AlmacenConfianzaForzada) CargarConfianzaForzada(_ context.Context) (domain.ConfianzaForzada, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cfg, err := LoadWithPolicyDir(a.dirUsuario, a.dirPolicy)
	if err != nil {
		return domain.ConfianzaForzada{Prohibida: true}, err
	}
	return domain.ConfianzaForzada{
		Aceptaciones: aceptacionesValidas(cfg.ConfianzaForzada),
		Prohibida:    cfg.ProhibirConfianzaForzada,
	}, nil
}

func aceptacionesValidas(entradas []domain.AceptacionConfianza) []domain.AceptacionConfianza {
	var out []domain.AceptacionConfianza
	for _, entrada := range entradas {
		if len(out) == maxConfianzaForzada {
			break
		}
		normalizada, err := entrada.Normalizar()
		if err != nil {
			slog.Warn("se ignora una entrada de confianza aceptada mal formada", "tipo", string(entrada.Tipo), "error", err)
			continue
		}
		out = append(out, normalizada)
	}
	return out
}

// prohibida lee solo la política de máquina, sin el fichero del usuario.
func (a *AlmacenConfianzaForzada) prohibida() bool {
	var cfg Config
	raw, err := loadSystemPolicy(a.dirPolicy)
	aplicarPolicyCargada(&cfg, raw, err)
	return cfg.ProhibirConfianzaForzada
}

// AceptarConfianzaForzada añade la entrada (o renueva la del mismo tipo y
// huella) en config.json, con permisos 0600.
func (a *AlmacenConfianzaForzada) AceptarConfianzaForzada(_ context.Context, aceptacion domain.AceptacionConfianza) error {
	aceptacion, err := aceptacion.Normalizar()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prohibida() {
		return domain.ErrConfianzaProhibida
	}
	return a.actualizar(func(actuales []domain.AceptacionConfianza) []domain.AceptacionConfianza {
		out := quitarAceptacion(actuales, aceptacion.Tipo, aceptacion.Huella)
		return append(out, aceptacion)
	})
}

// QuitarConfianzaForzada borra la entrada del tipo y huella indicados.
func (a *AlmacenConfianzaForzada) QuitarConfianzaForzada(_ context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	quitada := false
	err := a.actualizar(func(actuales []domain.AceptacionConfianza) []domain.AceptacionConfianza {
		out := quitarAceptacion(actuales, tipo, huella)
		quitada = len(out) != len(actuales)
		return out
	})
	return quitada, err
}

func quitarAceptacion(lista []domain.AceptacionConfianza, tipo domain.TipoConfianzaForzada, huella string) []domain.AceptacionConfianza {
	out := make([]domain.AceptacionConfianza, 0, len(lista))
	for _, entrada := range lista {
		if entrada.Tipo == tipo && entrada.Huella == huella {
			continue
		}
		out = append(out, entrada)
	}
	return out
}

// actualizar lee config.json, cambia solo la clave confianza_forzada y lo
// escribe de forma atómica con permisos 0600. El resto de claves se conserva
// tal cual.
func (a *AlmacenConfianzaForzada) actualizar(cambiar func([]domain.AceptacionConfianza) []domain.AceptacionConfianza) error {
	ruta := filepath.Join(a.dirUsuario, ficheroConfig)
	campos := map[string]json.RawMessage{}
	datos, err := securefile.ReadFileLimit(ruta, maxConfigFileBytes)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := crearFichero(ruta, Default()); err != nil {
			return err
		}
		datos, err = securefile.ReadFileLimit(ruta, maxConfigFileBytes)
		if err != nil {
			return fmt.Errorf("leer %s: %w", ruta, err)
		}
	case err != nil:
		return fmt.Errorf("leer %s: %w", ruta, err)
	}
	if err := json.Unmarshal(datos, &campos); err != nil {
		return fmt.Errorf("parsear %s: %w", ruta, err)
	}
	var actuales []domain.AceptacionConfianza
	if raw, ok := campos[claveConfianzaForzada]; ok && len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &actuales); err != nil {
			return fmt.Errorf("parsear %s de %s: %w", claveConfianzaForzada, ruta, err)
		}
	}
	nuevas := cambiar(aceptacionesValidas(actuales))
	if len(nuevas) > maxConfianzaForzada {
		nuevas = nuevas[len(nuevas)-maxConfianzaForzada:]
	}
	if len(nuevas) == 0 {
		delete(campos, claveConfianzaForzada)
	} else {
		raw, err := json.Marshal(nuevas)
		if err != nil {
			return err
		}
		campos[claveConfianzaForzada] = raw
	}
	salida, err := json.MarshalIndent(campos, "", "  ")
	if err != nil {
		return err
	}
	if err := securefile.WriteFileAtomic(ruta, salida, 0o600); err != nil {
		return fmt.Errorf("escribir %s: %w", ruta, err)
	}
	return nil
}
