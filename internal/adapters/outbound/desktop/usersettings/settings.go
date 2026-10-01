// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package usersettings implementa ports.ConfiguracionUsuario persistiendo
// las preferencias del usuario como JSON en ~/.config/grxfirma/settings.json.
package usersettings

import (
	"context"
	"encoding/json"
	"fmt"
	"grxfirma/internal/appdirs"
	"os"
	"path/filepath"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/ports"
)

const (
	nombreFichero            = "settings.json"
	maxUserSettingsFileBytes = 4 * 1024 * 1024
)

// Almacen implementa ports.ConfiguracionUsuario.
type Almacen struct {
	// ConfigDir es el directorio donde se guarda settings.json.
	// Si esta vacio se usa ~/.config/grxfirma.
	ConfigDir string
}

// New construye un Almacen con el directorio de configuracion dado.
func New(configDir string) *Almacen {
	return &Almacen{ConfigDir: configDir}
}

// CargarDocumentoCompat devuelve el documento tipado usando la via tipada
// cuando el store la implementa, con fallback al mapa legacy.
func CargarDocumentoCompat(ctx context.Context, configDir string) (ports.DocumentoConfiguracionUsuario, error) {
	almacen := New(configDir)
	if tipado, ok := any(almacen).(ports.ConfiguracionUsuarioTipada); ok {
		return tipado.CargarDocumento(ctx)
	}
	datos, err := almacen.Cargar(ctx)
	if err != nil {
		return ports.DocumentoConfiguracionUsuario{}, err
	}
	return ports.DocumentoConfiguracionUsuarioDesdeMapa(datos), nil
}

// CargarDesktopCompat devuelve solo el subconjunto tipado de desktop/Fyne.
func CargarDesktopCompat(ctx context.Context, configDir string) (ports.ConfiguracionUsuarioDesktop, error) {
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return ports.ConfiguracionUsuarioDesktop{}, err
	}
	return doc.Desktop, nil
}

// CargarFirmaCompat devuelve el subconjunto tipado de preferencias de firma,
// metadatos de firma y TSA.
func CargarFirmaCompat(ctx context.Context, configDir string) (ports.ConfiguracionUsuarioFirma, ports.ConfiguracionUsuarioFirmaMetadatos, ports.ConfiguracionUsuarioTSA, error) {
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return ports.ConfiguracionUsuarioFirma{}, ports.ConfiguracionUsuarioFirmaMetadatos{}, ports.ConfiguracionUsuarioTSA{}, err
	}
	return doc.Firma, doc.FirmaMeta, doc.TSA, nil
}

// CargarFormatosAutoCompat devuelve el subconjunto tipado de formatos por
// defecto según el tipo documental detectado.
func CargarFormatosAutoCompat(ctx context.Context, configDir string) (ports.ConfiguracionUsuarioFormatosAutomaticos, error) {
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return ports.ConfiguracionUsuarioFormatosAutomaticos{}, err
	}
	return doc.FormatosAuto, nil
}

// CargarMultiCoSignCompat devuelve el subconjunto tipado de cofirma multiple.
func CargarMultiCoSignCompat(ctx context.Context, configDir string) (ports.ConfiguracionUsuarioMultiCoSign, error) {
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return ports.ConfiguracionUsuarioMultiCoSign{}, err
	}
	return doc.MultiCoSign, nil
}

// CargarPAdESVisibleCompat devuelve el subconjunto tipado del sello visible
// PAdES persistido por la GUI Qt.
func CargarPAdESVisibleCompat(ctx context.Context, configDir string) (ports.ConfiguracionUsuarioPAdESVisible, error) {
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return ports.ConfiguracionUsuarioPAdESVisible{}, err
	}
	return doc.PAdESVisible, nil
}

// CargarPAdESCompat devuelve el subconjunto tipado de preferencias PAdES
// generales, separado del sello visible.
func CargarPAdESCompat(ctx context.Context, configDir string) (ports.ConfiguracionUsuarioPAdES, error) {
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return ports.ConfiguracionUsuarioPAdES{}, err
	}
	return doc.PAdES, nil
}

// CargarCertificadosCompat devuelve el subconjunto tipado de preferencias de
// seleccion, filtros y ordenacion del catalogo de certificados.
func CargarCertificadosCompat(ctx context.Context, configDir string) (ports.ConfiguracionUsuarioCertificados, error) {
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return ports.ConfiguracionUsuarioCertificados{}, err
	}
	return doc.Certificados, nil
}

// ActualizarDocumentoCompat carga el documento tipado, aplica una mutacion y
// persiste por la via tipada cuando esta disponible, con fallback legacy.
func ActualizarDocumentoCompat(ctx context.Context, configDir string, update func(*ports.DocumentoConfiguracionUsuario)) error {
	almacen := New(configDir)
	doc, err := CargarDocumentoCompat(ctx, configDir)
	if err != nil {
		return err
	}
	update(&doc)
	if tipado, ok := any(almacen).(ports.ConfiguracionUsuarioTipada); ok {
		return tipado.GuardarDocumento(ctx, doc)
	}
	return almacen.Guardar(ctx, doc.Mapa())
}

// ActualizarDesktopCompat carga el bloque desktop/Fyne, aplica una mutacion y
// persiste el documento completo manteniendo el resto de preferencias.
func ActualizarDesktopCompat(ctx context.Context, configDir string, update func(*ports.ConfiguracionUsuarioDesktop)) error {
	return ActualizarDocumentoCompat(ctx, configDir, func(doc *ports.DocumentoConfiguracionUsuario) {
		update(&doc.Desktop)
	})
}

// ActualizarFirmaCompat carga el subconjunto tipado de firma, aplica una
// mutacion y persiste el documento completo manteniendo el resto de claves.
func ActualizarFirmaCompat(ctx context.Context, configDir string, update func(*ports.ConfiguracionUsuarioFirma, *ports.ConfiguracionUsuarioFirmaMetadatos, *ports.ConfiguracionUsuarioTSA)) error {
	return ActualizarDocumentoCompat(ctx, configDir, func(doc *ports.DocumentoConfiguracionUsuario) {
		update(&doc.Firma, &doc.FirmaMeta, &doc.TSA)
	})
}

// ActualizarFormatosAutoCompat carga el bloque tipado de formatos automáticos,
// aplica una mutación y persiste el documento completo.
func ActualizarFormatosAutoCompat(ctx context.Context, configDir string, update func(*ports.ConfiguracionUsuarioFormatosAutomaticos)) error {
	return ActualizarDocumentoCompat(ctx, configDir, func(doc *ports.DocumentoConfiguracionUsuario) {
		update(&doc.FormatosAuto)
	})
}

// ActualizarMultiCoSignCompat carga el bloque tipado de cofirma multiple,
// aplica una mutacion y persiste el documento completo.
func ActualizarMultiCoSignCompat(ctx context.Context, configDir string, update func(*ports.ConfiguracionUsuarioMultiCoSign)) error {
	return ActualizarDocumentoCompat(ctx, configDir, func(doc *ports.DocumentoConfiguracionUsuario) {
		update(&doc.MultiCoSign)
	})
}

// ActualizarPAdESVisibleCompat carga el bloque tipado del sello visible
// PAdES, aplica una mutacion y persiste el documento completo.
func ActualizarPAdESVisibleCompat(ctx context.Context, configDir string, update func(*ports.ConfiguracionUsuarioPAdESVisible)) error {
	return ActualizarDocumentoCompat(ctx, configDir, func(doc *ports.DocumentoConfiguracionUsuario) {
		update(&doc.PAdESVisible)
	})
}

// ActualizarPAdESCompat carga el bloque tipado de preferencias PAdES,
// aplica una mutacion y persiste el documento completo.
func ActualizarPAdESCompat(ctx context.Context, configDir string, update func(*ports.ConfiguracionUsuarioPAdES)) error {
	return ActualizarDocumentoCompat(ctx, configDir, func(doc *ports.DocumentoConfiguracionUsuario) {
		update(&doc.PAdES)
	})
}

// ActualizarCertificadosCompat carga el bloque tipado de certificados, aplica
// una mutacion y persiste el documento completo.
func ActualizarCertificadosCompat(ctx context.Context, configDir string, update func(*ports.ConfiguracionUsuarioCertificados)) error {
	return ActualizarDocumentoCompat(ctx, configDir, func(doc *ports.DocumentoConfiguracionUsuario) {
		update(&doc.Certificados)
	})
}

// Cargar implementa ports.ConfiguracionUsuario.
// Devuelve un mapa vacio si el fichero no existe todavia.
func (a *Almacen) Cargar(ctx context.Context) (map[string]any, error) {
	return a.cargarMapaLegacy(ctx)
}

// CargarDocumento implementa ports.ConfiguracionUsuarioTipada.
func (a *Almacen) CargarDocumento(_ context.Context) (ports.DocumentoConfiguracionUsuario, error) {
	resultado, err := a.cargarMapaLegacy(context.Background())
	if err != nil {
		return ports.DocumentoConfiguracionUsuario{}, err
	}
	return ports.DocumentoConfiguracionUsuarioDesdeMapa(resultado), nil
}

func (a *Almacen) cargarMapaLegacy(_ context.Context) (map[string]any, error) {
	ruta, err := a.rutaFichero()
	if err != nil {
		return nil, err
	}

	data, err := securefile.ReadFileLimit(ruta, maxUserSettingsFileBytes)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leyendo configuracion de usuario: %w", err)
	}

	var resultado map[string]any
	if err := json.Unmarshal(data, &resultado); err != nil {
		return nil, fmt.Errorf("parseando configuracion de usuario: %w", err)
	}
	if ports.EliminarCredencialSeguridadUIEnClaro(resultado) {
		if err := a.guardarMapaEnRuta(ruta, resultado); err != nil {
			return nil, fmt.Errorf("saneando credencial UI heredada de configuracion de usuario: %w", err)
		}
	}
	return resultado, nil
}

// Guardar implementa ports.ConfiguracionUsuario.
func (a *Almacen) Guardar(ctx context.Context, datos map[string]any) error {
	if err := ports.ValidarOpacidadLogoSello(datos); err != nil {
		return err
	}
	return a.GuardarDocumento(ctx, ports.DocumentoConfiguracionUsuarioDesdeMapa(datos))
}

// GuardarDocumento implementa ports.ConfiguracionUsuarioTipada.
func (a *Almacen) GuardarDocumento(_ context.Context, doc ports.DocumentoConfiguracionUsuario) error {
	datos := doc.Mapa()
	if err := ports.ValidarOpacidadLogoSello(datos); err != nil {
		return err
	}
	ruta, err := a.rutaFichero()
	if err != nil {
		return err
	}
	return a.guardarMapaEnRuta(ruta, datos)
}

func (a *Almacen) guardarMapaEnRuta(ruta string, datos map[string]any) error {
	directorio := filepath.Dir(ruta)
	if err := os.MkdirAll(directorio, 0o700); err != nil {
		return fmt.Errorf("creando directorio de configuracion: %w", err)
	}
	if err := protegerDirectorioConfiguracion(directorio); err != nil {
		return fmt.Errorf("restringiendo directorio de configuracion: %w", err)
	}

	ports.EliminarCredencialSeguridadUIEnClaro(datos)
	data, err := json.MarshalIndent(datos, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando configuracion de usuario: %w", err)
	}

	if err := securefile.WriteFileAtomic(ruta, data, 0o600); err != nil {
		return fmt.Errorf("escribiendo configuracion de usuario: %w", err)
	}
	if err := protegerFicheroConfiguracion(ruta); err != nil {
		return fmt.Errorf("restringiendo configuracion de usuario: %w", err)
	}
	return nil
}

func (a *Almacen) rutaFichero() (string, error) {
	dir := a.ConfigDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("obteniendo directorio home: %w", err)
		}
		dir = appdirs.Config(home)
	}
	return filepath.Join(dir, nombreFichero), nil
}
