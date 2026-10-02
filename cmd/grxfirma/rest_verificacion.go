// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	restin "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/outbound/common/localizador"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/verificacionlocal"
	"grxfirma/internal/application"
)

// fuentesVerificacionLocal agrupa anclas y evaluador construidos a partir de
// las banderas de verificación.
type fuentesVerificacionLocal struct {
	anclas    *verificacionlocal.ProveedorAnclas
	evaluador *verificacionlocal.Evaluador
	conCRL    bool
}

// construirFuentesVerificacion carga las anclas (si se indicaron) y el
// directorio de CRL. Las extensiones remotas (OCSP/TSA) no se activan aquí:
// quedan desactivadas por defecto y solo pueden componerse en código.
func construirFuentesVerificacion(cfg restFlags) (fuentesVerificacionLocal, error) {
	var out fuentesVerificacionLocal
	if ruta := strings.TrimSpace(cfg.anclasVerificacion); ruta != "" {
		anclas, err := verificacionlocal.CargarAnclas(ruta)
		if err != nil {
			return out, err
		}
		out.anclas = anclas
	}
	evaluacion := verificacionlocal.Configuracion{}
	if dir := strings.TrimSpace(cfg.crlVerificacion); dir != "" {
		almacen, err := verificacionlocal.NuevoAlmacenCRL(dir)
		if err != nil {
			return out, err
		}
		evaluacion.CRL = almacen
		out.conCRL = true
	}
	out.evaluador = verificacionlocal.Nuevo(evaluacion)
	return out, nil
}

// configurarVerificacionLocal añade el dictamen al /verify del servidor REST
// completo. Con anclas locales, estas sustituyen al almacén del sistema.
func configurarVerificacionLocal(uc *application.VerifySignatureUseCase, cfg restFlags) error {
	if uc == nil {
		return nil
	}
	fuentes, err := construirFuentesVerificacion(cfg)
	if err != nil {
		return err
	}
	if fuentes.anclas != nil {
		uc.ConAnclas(fuentes.anclas)
	}
	uc.ConEvaluador(fuentes.evaluador)
	return nil
}

// runRESTSoloVerificacion arranca el validador autónomo: solo GET /health y
// POST /verify, sin catálogos de certificados, claves, firma, protección ni
// gestión, y sin ninguna conexión de red saliente.
func runRESTSoloVerificacion(ctx context.Context, logger *slog.Logger, cfg restFlags) int {
	loc := localizador.Detectar()
	if cfg.lifetime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.lifetime)
		defer cancel()
	}
	if err := validarPoliticaSoloVerificacion(cfg); err != nil {
		logger.ErrorContext(ctx, "política del validador insegura", "op", "rest-verify-security", "error", err, "addr", cfg.addr)
		fmt.Fprintln(os.Stderr, loc.T("rest.verify_only.error.security", err))
		return 1
	}
	fuentes, err := construirFuentesVerificacion(cfg)
	if err != nil {
		logger.ErrorContext(ctx, "fuentes de verificación inválidas", "op", "rest-verify-config", "error", err)
		fmt.Fprintln(os.Stderr, loc.T("rest.verify_only.error.config", err))
		return 1
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, loc.T("rest.verify_only.error.config_dir"))
		return 1
	}
	configDir := filepath.Join(home, ".config", "grxfirma")

	credentialFile := ""
	if strings.TrimSpace(cfg.token) == "" {
		token, err := generarTokenREST()
		if err != nil {
			fmt.Fprintln(os.Stderr, loc.T("rest.verify_only.error.token", err))
			return 1
		}
		cfg.token = token
		var cleanup func()
		credentialFile, cleanup, err = escribirCredencialRESTPrivada(configDir, cfg.token)
		if err != nil {
			fmt.Fprintln(os.Stderr, loc.T("rest.error.private_credential"))
			return 1
		}
		defer cleanup()
	}

	verificar := application.NuevoVerifySignatureUseCase(fuentes.anclas, commonsigner.NewMultiVerifierOffline(), nil).
		ConEvaluador(fuentes.evaluador)
	adaptador := restin.New(nil, verificar, nil).WithBearerToken(cfg.token)
	adaptador.MaxV2Firmas = cfg.v2MaxFirmas
	adaptador.MaxV2Revisiones = cfg.v2MaxRevisiones
	adaptador.MaxV2PDFBytes = cfg.v2MaxPDFMiB << 20
	if cfg.v2MaxCuerpoMiB > 0 {
		adaptador.MaxBodyBytes = int64(cfg.v2MaxCuerpoMiB) << 20
	}
	srv, err := restin.StartTLS13Server(ctx, cfg.addr, adaptador.RoutesSoloVerificacion(), filepath.Join(configDir, "tls"))
	if err != nil {
		logger.ErrorContext(ctx, "no se pudo abrir el validador", "op", "rest-verify-listen", "error", err)
		fmt.Fprintln(os.Stderr, loc.T("rest.verify_only.error.listen", err))
		return 1
	}
	fmt.Fprintln(os.Stdout, loc.T("rest.verify_only.output.active", srv.Addr))
	fmt.Fprintln(os.Stdout, loc.T("rest.verify_only.output.routes"))
	fmt.Fprintln(os.Stdout, loc.T("rest.verify_only.output.anchors", fuentes.anclas.Cantidad()))
	if fuentes.conCRL {
		fmt.Fprintln(os.Stdout, loc.T("rest.verify_only.output.crl_local"))
	} else {
		fmt.Fprintln(os.Stdout, loc.T("rest.verify_only.output.crl_none"))
	}
	fmt.Fprintln(os.Stdout, loc.T("rest.verify_only.output.remote_disabled"))
	if credentialFile != "" {
		fmt.Fprintln(os.Stdout, loc.T("rest.output.bearer_file", credentialFile))
	}
	<-ctx.Done()
	return 0
}

// validarPoliticaSoloVerificacion exige anclas locales y token Bearer. La
// autenticación por certificado del modo completo no se publica aquí.
func validarPoliticaSoloVerificacion(cfg restFlags) error {
	loc := localizador.Detectar()
	if strings.TrimSpace(cfg.anclasVerificacion) == "" {
		return errors.New(loc.T("rest.verify_only.error.anchors_required"))
	}
	if strings.TrimSpace(cfg.certFingerprintsCSV) != "" {
		return errors.New(loc.T("rest.verify_only.error.fingerprints"))
	}
	if esDireccionLoopback(cfg.addr) {
		return nil
	}
	if !cfg.permitirRemoto {
		return errors.New(loc.T("rest.verify_only.error.not_loopback", cfg.addr))
	}
	if strings.TrimSpace(cfg.token) == "" {
		return errors.New(loc.T("rest.verify_only.error.token_required"))
	}
	return nil
}
