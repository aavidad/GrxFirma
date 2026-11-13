// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	legacyws "grxfirma/internal/adapters/inbound/legacy/websocket"
	"grxfirma/internal/adapters/outbound/common/config"
)

const legacyLaunchControlSocketEnv = "GRXFIRMA_LEGACY_LAUNCH_SOCKET"

type launchControlRequest struct {
	URI string `json:"uri"`
}

type launchControlResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// tryForwardLaunchToResident define el contrato de delegación hacia un proceso
// residente ya vivo.
//
// Ojo: su mera existencia no implica que deba activarse desde run() por
// defecto. El lanzamiento afirma:// inicial todavía no valida TOFU/origin trust
// ni implica consentimiento de firma; solo prepara el canal local. La
// validación de confianza sigue ocurriendo más abajo, cuando la petición real
// entra en el adaptador/orquestador legacy.
//
// Por eso, cualquier activación automática futura debe mantener este patrón:
//  1. intentar delegar solo si el residente está sano;
//  2. conservar la validación posterior de trust/origin en el flujo real;
//  3. caer sin ruido al modo puntual actual si el socket no responde o rechaza.
func tryForwardLaunchToResident(rawURI string) (bool, error) {
	socketPath, err := legacyLaunchControlSocketPath()
	if err != nil {
		return false, err
	}
	conn, err := net.DialTimeout("unix", socketPath, 350*time.Millisecond)
	if err != nil {
		return false, nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	if err := json.NewEncoder(conn).Encode(launchControlRequest{URI: rawURI}); err != nil {
		return false, err
	}
	var resp launchControlResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return false, err
	}
	if !resp.OK {
		if strings.TrimSpace(resp.Error) == "" {
			resp.Error = "el servicio residente rechazó la solicitud"
		}
		return true, errors.New(resp.Error)
	}
	return true, nil
}

func startLegacyLaunchControlServer(
	ctx context.Context,
	logger *slog.Logger,
	configDir string,
	cfg config.Config,
	adapter *legacyws.Adaptador,
) (func(), error) {
	if adapter == nil {
		return func() {}, errors.New("adaptador legacy no configurado")
	}
	socketPath, err := legacyLaunchControlSocketPath()
	if err != nil {
		return func() {}, err
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return func() {}, fmt.Errorf("creando directorio del socket de control: %w", err)
	}
	_ = os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return func() {}, fmt.Errorf("abriendo socket de control: %w", err)
	}
	_ = os.Chmod(socketPath, 0o600)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
		_ = os.Remove(socketPath)
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleLegacyLaunchControlConn(ctx, logger, configDir, cfg, adapter, conn)
		}
	}()
	return func() {
		_ = ln.Close()
		_ = os.Remove(socketPath)
	}, nil
}

func handleLegacyLaunchControlConn(
	ctx context.Context,
	logger *slog.Logger,
	configDir string,
	cfg config.Config,
	adapter *legacyws.Adaptador,
	conn net.Conn,
) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	var req launchControlRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(launchControlResponse{OK: false, Error: "solicitud de control invalida"})
		return
	}
	if err := dispatchResidentLaunch(ctx, logger, configDir, cfg, adapter, req.URI); err != nil {
		_ = json.NewEncoder(conn).Encode(launchControlResponse{OK: false, Error: err.Error()})
		return
	}
	_ = json.NewEncoder(conn).Encode(launchControlResponse{OK: true})
}

func dispatchResidentLaunch(
	ctx context.Context,
	logger *slog.Logger,
	configDir string,
	cfg config.Config,
	adapter *legacyws.Adaptador,
	rawURI string,
) error {
	// Seguridad: el socket de control solo delega el "arranque de canal" al
	// proceso residente. No sustituye la política TOFU ni la validación del
	// origen; ambas siguen produciéndose cuando el portal conecta realmente y el
	// flujo legacy entra en el adaptador/orquestador.
	if req, err := parseWebSocketLaunchURI(rawURI); err == nil {
		return startResidentWebSocketLaunch(ctx, logger, configDir, cfg, adapter, req)
	}
	if req, err := parseServiceLaunchURI(rawURI); err == nil {
		return startResidentServiceLaunch(ctx, logger, configDir, adapter, req)
	}
	return fmt.Errorf("la URI no es un lanzamiento legacy soportado")
}

func startResidentWebSocketLaunch(
	ctx context.Context,
	logger *slog.Logger,
	configDir string,
	cfg config.Config,
	adapter *legacyws.Adaptador,
	req *websocketLaunchRequest,
) error {
	if req == nil {
		return errors.New("solicitud websocket vacia")
	}
	if !cfg.PermiteWebSocket() {
		return errors.New("WebSocket deshabilitado por configuración o política")
	}
	// Modelo conservador: cada URI delegada abre una sesión puntual nueva sobre
	// el residente. Reutilizamos proceso y recursos, pero no compartimos estado
	// terminal, consentimiento ni una UI previa entre solicitudes distintas.
	sessionCtx, cancel := context.WithCancel(ctx)
	cfg.WebsocketHabilitado = true
	srv, err := legacyws.StartTLSServerOnPortsWithHooks(
		sessionCtx,
		cfg,
		adapter,
		filepath.Join(configDir, "tls"),
		req.Ports,
		&legacyws.SessionHooks{ExpectedSessionID: req.SessionID, OnSocketClosed: cancel},
	)
	if err != nil {
		cancel()
		return err
	}
	if srv == nil {
		cancel()
		return errors.New("no se pudo abrir el servidor websocket residente")
	}
	if logger != nil {
		logger.InfoContext(
			ctx,
			"lanzamiento websocket delegado al servicio residente",
			"requested_ports",
			req.Ports,
			"session_id",
			maskSessionForLog(req.SessionID),
			"listen_addr",
			srv.Addr,
		)
	}
	return nil
}

func startResidentServiceLaunch(
	ctx context.Context,
	logger *slog.Logger,
	configDir string,
	adapter *legacyws.Adaptador,
	req *serviceLaunchRequest,
) error {
	if req == nil {
		return errors.New("solicitud service vacia")
	}
	// Igual que en websocket: el residente reutiliza proceso y cachés locales,
	// pero cada lanzamiento service mantiene su propio ciclo de vida y su propia
	// validación posterior cuando la web empiece a interactuar de verdad.
	sessionCtx, cancel := context.WithCancel(ctx)
	srv, err := legacyws.StartLegacySocketServer(
		sessionCtx,
		adapter,
		filepath.Join(configDir, "tls"),
		req.Ports,
		req.SessionID,
		req.Version,
	)
	if err != nil {
		cancel()
		return err
	}
	if srv == nil {
		cancel()
		return errors.New("no se pudo abrir el socket legacy residente")
	}
	go func() {
		<-sessionCtx.Done()
		cancel()
	}()
	if logger != nil {
		logger.InfoContext(
			ctx,
			"lanzamiento service delegado al servicio residente",
			"requested_ports",
			req.Ports,
			"session_id",
			maskSessionForLog(req.SessionID),
			"listen_addr",
			srv.Addr(),
		)
	}
	return nil
}

func legacyLaunchControlSocketPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv(legacyLaunchControlSocketEnv)); override != "" {
		return override, nil
	}
	if runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtimeDir != "" {
		return filepath.Join(runtimeDir, "grxfirma-afirmauri-control.sock"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("obteniendo home para socket de control: %w", err)
	}
	return filepath.Join(appdirs.Cache(home), "afirmauri-control.sock"), nil
}
