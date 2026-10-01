// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"os"
	"path/filepath"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	legacyws "grxfirma/internal/adapters/inbound/legacy/websocket"
	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/adapters/outbound/common/config"
	desktopdocumentpicker "grxfirma/internal/adapters/outbound/desktop/documentpicker"
	"grxfirma/internal/application"
)

func maybeRunDesktopService(context.Context, io.Writer) (bool, int) {
	return false, 0
}

func runExplicitDesktopService(ctx context.Context, stderr io.Writer) int {
	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)
	setProtocolLocalizer(configDir)
	cfg, err := config.Load(configDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", tl("Cargando configuración..."), err)
		return 1
	}
	if !cfg.PermiteWebSocket() {
		_, _ = fmt.Fprintln(stderr, tl("WebSocket deshabilitado por configuración o política"))
		return 1
	}
	cfg.WebsocketHabilitado = true
	if err := prepareStartupLocalTLSTrust(ctx, configDir, stderr); err != nil {
		return 1
	}

	p12Dir := cfg.DirectorioP12
	if p12Dir == "" {
		p12Dir = filepath.Join(configDir, "pkcs12")
	}
	if override := os.Getenv("GRXFIRMA_PKCS12_DIR"); override != "" {
		p12Dir = override
	}
	p12Password := os.Getenv("GRXFIRMA_PKCS12_PASSWORD")
	proxyDiag := diagnosticarClienteHTTPRuntimeSeguro(configDir)
	if msg := formatRuntimeProxyStartupNotice(proxyDiag); msg != "" {
		_, _ = fmt.Fprintf(stderr, "%s\n", msg)
	}

	websocketAdapter, _, err := construirServicioWebSocket(configDir, p12Dir, p12Password)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", tl("Preparando servicio local..."), err)
		return 1
	}

	srv, err := legacyws.StartTLSServer(ctx, cfg, websocketAdapter, filepath.Join(configDir, "tls"))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", tl("Preparando servicio local..."), err)
		return 1
	}
	if srv == nil {
		return 0
	}
	cleanupControl, err := startLegacyLaunchControlServer(ctx, nil, configDir, cfg, websocketAdapter)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: %v\n", err)
		cleanupControl = func() {}
	}
	defer cleanupControl()
	_, _ = fmt.Fprintf(stderr, "%s wss://%s\n", tl("GrxFirma — WebSocket activo"), srv.Addr)
	<-ctx.Done()
	return 0
}

type aprobacionHeadless struct{}

func (a aprobacionHeadless) Request(context.Context, string) (bool, error) {
	return false, errors.New("no hay interfaz gráfica disponible para confirmar la operación")
}

func newSignApproval() aprobacionHeadless {
	return aprobacionHeadless{}
}

func construirServicioWebSocket(configDir, p12Dir, p12Password string) (*legacyws.Adaptador, *application.AuditUseCase, error) {
	runtime, err := construirRuntimeAfirmaURI(configDir, p12Dir, p12Password)
	if err != nil {
		return nil, nil, err
	}
	auditLogger, err := auditlog.NewDefault()
	if err != nil {
		return nil, nil, err
	}
	auditor := application.NuevoAuditUseCase(relojReal{}, auditLogger)
	approval := newSignApproval()
	firmarDirecto := application.NuevoSignDocumentUseCase(
		runtime.catalogo,
		runtime.claves,
		runtime.motor,
		approval,
		auditor,
		nil,
	)
	if firmarDirecto == nil {
		return nil, nil, errors.New("no se pudo construir el caso de uso de firma para WebSocket")
	}
	batchUC := application.NuevoProcessBatchUseCase(
		runtime.catalogo,
		runtime.claves,
		runtime.motor,
		approval,
		auditor,
		nil,
	)
	triphaseExec := triphase.New(runtime.clienteHTTP)
	legacyHandler := newLegacyWebSocketHandler(
		firmarDirecto,
		batchUC,
		runtime.catalogo,
		runtime.selector,
		desktopdocumentpicker.NewHeadless(),
		runtime.claves,
		triphaseExec,
		triphase.NewBatch(triphaseExec),
	).withApproval(approval)
	return legacyws.New(runtime.parser, firmarDirecto, runtime.orquestador).
		WithTrustPolicy(runtime.trustPolicy).
		WithLegacyHandler(legacyHandler), auditor, nil
}
