// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package servicemanager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"grxfirma/internal/ports"
)

const plantillaUnidad = `[Unit]
Description=GrxFirma – motor de firma digital
After=network.target

[Service]
Type=simple
ExecStart=%s --server --server-modo ipc --ipc-socket %s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`

// GestorSystemd implementa ports.GestorServicio usando systemd --user en Linux.
type GestorSystemd struct {
	// RutaBinario es la ruta al binario que implementa el modo servidor IPC.
	RutaBinario string
}

// New construye un GestorSystemd. Si rutaBinario esta vacio se detecta
// automaticamente buscando grxfirma-gui en el PATH.
func New(rutaBinario string) *GestorSystemd {
	return &GestorSystemd{RutaBinario: rutaBinario}
}

// Estado implementa ports.GestorServicio.
func (g *GestorSystemd) Estado(ctx context.Context) (ports.EstadoServicio, error) {
	out, err := g.systemctl(ctx, "is-active", nombreServicio)
	activo := err == nil && strings.TrimSpace(out) == "active"

	_, errInstall := g.rutaUnidad()
	instalado := errInstall == nil

	return ports.EstadoServicio{
		Instalado:  instalado,
		Activo:     activo,
		Plataforma: "linux",
		Metodo:     "systemd-user",
	}, nil
}

// Instalar implementa ports.GestorServicio.
func (g *GestorSystemd) Instalar(ctx context.Context, socketPath string) error {
	binario, err := g.resolverBinario()
	if err != nil {
		return err
	}

	// Validar socketPath para evitar inyeccion de directivas en el fichero de unidad.
	// Un atacante podria inyectar \n para anadir lineas arbitrarias al .service.
	if err := validarSocketPath(socketPath); err != nil {
		return fmt.Errorf("ruta de socket invalida: %w", err)
	}

	dir, err := dirUnidades()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creando directorio de unidades: %w", err)
	}

	contenido := fmt.Sprintf(plantillaUnidad, binario, socketPath)
	ruta := filepath.Join(dir, nombreServicio)
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		return fmt.Errorf("escribiendo unidad systemd: %w", err)
	}

	if _, err := g.systemctl(ctx, "daemon-reload"); err != nil {
		return fmt.Errorf("recargando systemd: %w", err)
	}
	if _, err := g.systemctl(ctx, "enable", nombreServicio); err != nil {
		return fmt.Errorf("habilitando servicio: %w", err)
	}
	return nil
}

// Desinstalar implementa ports.GestorServicio.
func (g *GestorSystemd) Desinstalar(ctx context.Context) error {
	// Intentar detener primero, ignorar error si no esta activo.
	_, _ = g.systemctl(ctx, "stop", nombreServicio)
	_, _ = g.systemctl(ctx, "disable", nombreServicio)

	ruta, err := g.rutaUnidad()
	if err != nil {
		return nil // ya no existe
	}
	if err := os.Remove(ruta); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("eliminando unidad systemd: %w", err)
	}
	_, _ = g.systemctl(ctx, "daemon-reload")
	return nil
}

// Iniciar implementa ports.GestorServicio.
func (g *GestorSystemd) Iniciar(ctx context.Context) error {
	if _, err := g.systemctl(ctx, "start", nombreServicio); err != nil {
		return fmt.Errorf("iniciando servicio: %w", err)
	}
	return nil
}

// Detener implementa ports.GestorServicio.
func (g *GestorSystemd) Detener(ctx context.Context) error {
	if _, err := g.systemctl(ctx, "stop", nombreServicio); err != nil {
		return fmt.Errorf("deteniendo servicio: %w", err)
	}
	return nil
}

// systemctl ejecuta systemctl --user con los argumentos dados.
func (g *GestorSystemd) systemctl(ctx context.Context, args ...string) (string, error) {
	// #nosec G204 -- this unexported helper is called only with the fixed
	// systemctl verbs and unit name declared in this package.
	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("systemctl --user %s: %w — %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

func (g *GestorSystemd) rutaUnidad() (string, error) {
	dir, err := dirUnidades()
	if err != nil {
		return "", err
	}
	ruta := filepath.Join(dir, nombreServicio)
	if _, err := os.Stat(ruta); err != nil {
		return "", err
	}
	return ruta, nil
}

func (g *GestorSystemd) resolverBinario() (string, error) {
	if g.RutaBinario != "" {
		return g.RutaBinario, nil
	}
	for _, nombre := range []string{"grxfirma-gui", "grxfirma"} {
		if ruta, err := exec.LookPath(nombre); err == nil {
			return ruta, nil
		}
	}
	return "", fmt.Errorf("no se encontro un binario con modo servidor IPC en el PATH")
}

func dirUnidades() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("obteniendo directorio home: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

// validarSocketPath rechaza rutas que contengan caracteres que permitirian
// inyectar directivas adicionales en el fichero de unidad systemd.
func validarSocketPath(ruta string) error {
	// Caracteres prohibidos: CR, LF, NUL y cualquier otro control ASCII.
	for i, r := range ruta {
		if r < 32 || r == 127 {
			return fmt.Errorf("caracter de control en posicion %d (0x%02x)", i, r)
		}
	}
	// La ruta debe ser absoluta y no contener secuencias de escape de ini.
	if !filepath.IsAbs(ruta) {
		return fmt.Errorf("la ruta del socket debe ser absoluta")
	}
	// Rechazar caracteres que tienen significado especial en ficheros ini/systemd.
	for _, c := range []string{"[", "]", "=", "\\", "\"", "'"} {
		if strings.Contains(ruta, c) {
			return fmt.Errorf("caracter no permitido en la ruta del socket: %q", c)
		}
	}
	// Sin espacios: la ruta se interpola en ExecStart, donde systemd separa los
	// argumentos por espacios. Una ruta como "/tmp/s.sock --otra-flag valor"
	// pasaria el resto de comprobaciones y anadiria flags arbitrarias a la
	// invocacion del servicio. El socketPath llega por JSON desde los
	// adaptadores IPC y REST, asi que es entrada externa.
	if strings.ContainsFunc(ruta, unicode.IsSpace) {
		return fmt.Errorf("la ruta del socket no puede contener espacios")
	}
	return nil
}
