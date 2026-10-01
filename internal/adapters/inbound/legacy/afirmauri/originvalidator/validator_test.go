// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package originvalidator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/domain"
)

// mockPolicy es una implementación stub de ports.TrustPolicy para tests.
type mockPolicy struct {
	// decisions mapea origen → TrustStatus. Si no existe la entrada, devuelve TrustPending.
	decisions map[string]domain.TrustStatus
	// evaluateErr, si no es nil, se devuelve como error en Evaluate.
	evaluateErr error
}

func (m *mockPolicy) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	if m.evaluateErr != nil {
		return domain.TrustDecision{}, m.evaluateErr
	}
	status, ok := m.decisions[origin]
	if !ok {
		status = domain.TrustPending
	}
	return domain.TrustDecision{
		Origin: origin,
		Status: status,
		Reason: "mock",
	}, nil
}

func (m *mockPolicy) Allow(_ context.Context, _ string) error  { return nil }
func (m *mockPolicy) Deny(_ context.Context, _ string) error   { return nil }
func (m *mockPolicy) Remove(_ context.Context, _ string) error { return nil }

// TestValidate_FormatoValido comprueba que orígenes bien formados pasan la validación
// de formato y son aprobados por la política mock.
func TestValidate_FormatoValido(t *testing.T) {
	t.Parallel()

	origenes := []string{
		"https://app.example.com",
		"https://app.example.com:443",
		"https://sub.dominio.example.org",
		"http://localhost",
		"http://localhost:8080",
		"http://127.0.0.1",
		"http://127.0.0.1:3000",
	}

	policy := &mockPolicy{
		decisions: make(map[string]domain.TrustStatus),
	}
	for _, o := range origenes {
		policy.decisions[o] = domain.TrustAllowed
	}

	v := originvalidator.New(policy)

	for _, origen := range origenes {
		origen := origen
		t.Run(origen, func(t *testing.T) {
			t.Parallel()
			if err := v.Validate(context.Background(), origen); err != nil {
				t.Errorf("se esperaba nil, se obtuvo: %v", err)
			}
		})
	}
}

// TestValidate_FormatoInvalido comprueba que los orígenes con formato incorrecto
// devuelven el error específico correspondiente sin llegar a consultar la política.
func TestValidate_FormatoInvalido(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		origen string
		errEsp error
	}{
		{
			nombre: "vacio",
			origen: "",
			errEsp: originvalidator.ErrOrigenVacio,
		},
		{
			nombre: "solo_espacios",
			origen: "   ",
			errEsp: originvalidator.ErrOrigenVacio,
		},
		{
			nombre: "esquema_ftp",
			origen: "ftp://files.example.com",
			errEsp: originvalidator.ErrEsquemaNoPermitido,
		},
		{
			nombre: "esquema_ws",
			origen: "ws://app.example.com",
			errEsp: originvalidator.ErrEsquemaNoPermitido,
		},
		{
			nombre: "sin_esquema",
			origen: "app.example.com",
			errEsp: originvalidator.ErrEsquemaNoPermitido,
		},
		{
			nombre: "http_google",
			origen: "http://google.com",
			errEsp: originvalidator.ErrHTTPSoloLocalhost,
		},
		{
			nombre: "http_ip_publica",
			origen: "http://8.8.8.8",
			errEsp: originvalidator.ErrHTTPSoloLocalhost,
		},
		{
			nombre: "http_ip_publica_con_puerto",
			origen: "http://172.16.1.1:8080",
			errEsp: originvalidator.ErrHTTPSoloLocalhost,
		},
		{
			nombre: "con_path",
			origen: "https://app.example.com/api",
			errEsp: originvalidator.ErrOrigenConPath,
		},
		{
			nombre: "con_path_y_query",
			origen: "https://app.example.com/api?foo=bar",
			errEsp: originvalidator.ErrOrigenConPath,
		},
		{
			nombre: "con_query",
			origen: "https://app.example.com?foo=bar",
			errEsp: originvalidator.ErrOrigenConPath,
		},
		{
			nombre: "con_fragment",
			origen: "https://app.example.com#seccion",
			errEsp: originvalidator.ErrOrigenConPath,
		},
		{
			nombre: "con_credenciales",
			origen: "https://user:pass@app.example.com",
			errEsp: originvalidator.ErrOrigenConCredenciales,
		},
		{
			nombre: "con_usuario_sin_password",
			origen: "https://user@app.example.com",
			errEsp: originvalidator.ErrOrigenConCredenciales,
		},
		{
			nombre: "demasiado_largo",
			origen: "https://" + strings.Repeat("a", 246) + ".com",
			errEsp: originvalidator.ErrOrigenDemasiadoLargo,
		},
	}

	// La política no debería ser consultada para orígenes con formato inválido,
	// pero si lo fuera, aprobaría todo para no enmascarar errores de formato.
	policy := &mockPolicy{
		decisions: map[string]domain.TrustStatus{},
	}
	v := originvalidator.New(policy)

	for _, tc := range casos {
		tc := tc
		t.Run(tc.nombre, func(t *testing.T) {
			t.Parallel()
			err := v.Validate(context.Background(), tc.origen)
			if err == nil {
				t.Fatalf("se esperaba error %v, se obtuvo nil", tc.errEsp)
			}
			if !errors.Is(err, tc.errEsp) {
				t.Errorf("se esperaba error %v, se obtuvo: %v", tc.errEsp, err)
			}
		})
	}
}

// TestValidate_IntegracionTrustPolicy comprueba la integración con la política:
// - origen permitido → nil
// - origen denegado → ErrOrigenRechazado
// - origen pendiente → ErrOrigenRechazado
// - error en Evaluate → ErrOrigenRechazado
func TestValidate_IntegracionTrustPolicy(t *testing.T) {
	t.Parallel()

	const origenValido = "https://app.example.com"
	const origenDenegado = "https://denied.example.com"
	const origenPendiente = "https://pending.example.com"

	policy := &mockPolicy{
		decisions: map[string]domain.TrustStatus{
			origenValido:   domain.TrustAllowed,
			origenDenegado: domain.TrustDenied,
			// origenPendiente no está registrado → TrustPending
		},
	}

	v := originvalidator.New(policy)
	ctx := context.Background()

	t.Run("origen_permitido_retorna_nil", func(t *testing.T) {
		t.Parallel()
		if err := v.Validate(ctx, origenValido); err != nil {
			t.Errorf("se esperaba nil, se obtuvo: %v", err)
		}
	})

	t.Run("origen_denegado_retorna_ErrOrigenRechazado", func(t *testing.T) {
		t.Parallel()
		err := v.Validate(ctx, origenDenegado)
		if !errors.Is(err, originvalidator.ErrOrigenRechazado) {
			t.Errorf("se esperaba ErrOrigenRechazado, se obtuvo: %v", err)
		}
	})

	t.Run("origen_pendiente_retorna_ErrOrigenRechazado", func(t *testing.T) {
		t.Parallel()
		err := v.Validate(ctx, origenPendiente)
		if !errors.Is(err, originvalidator.ErrOrigenRechazado) {
			t.Errorf("se esperaba ErrOrigenRechazado, se obtuvo: %v", err)
		}
	})

	t.Run("error_en_evaluate_retorna_ErrOrigenRechazado", func(t *testing.T) {
		t.Parallel()
		errPolicy := errors.New("fallo interno de politica")
		policyConError := &mockPolicy{evaluateErr: errPolicy}
		vConError := originvalidator.New(policyConError)

		err := vConError.Validate(ctx, "https://app.example.com")
		if !errors.Is(err, originvalidator.ErrOrigenRechazado) {
			t.Errorf("se esperaba ErrOrigenRechazado, se obtuvo: %v", err)
		}
		if !errors.Is(err, errPolicy) {
			t.Errorf("se esperaba que el error envolviera el error de politica, se obtuvo: %v", err)
		}
	})
}

// TestValidate_LocalhostIPv6Loopback comprueba que ::1 (loopback IPv6) es aceptado con http.
func TestValidate_LocalhostIPv6Loopback(t *testing.T) {
	t.Parallel()

	// La URL con IPv6 requiere brackets según RFC 3986.
	origen := "http://[::1]:8080"
	policy := &mockPolicy{
		decisions: map[string]domain.TrustStatus{
			origen: domain.TrustAllowed,
		},
	}
	v := originvalidator.New(policy)
	if err := v.Validate(context.Background(), origen); err != nil {
		t.Errorf("se esperaba nil para loopback IPv6, se obtuvo: %v", err)
	}
}
