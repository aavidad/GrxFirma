// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// UnprotectDocumentUseCase orquesta el descifrado de un documento protegido.
type UnprotectDocumentUseCase struct {
	keys    ports.ProtectionKeyProvider
	engine  ports.ProtectorEngine
	auditor *AuditUseCase
	eventos ports.EventPublisher
}

func NuevoUnprotectDocumentUseCase(
	keys ports.ProtectionKeyProvider,
	engine ports.ProtectorEngine,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *UnprotectDocumentUseCase {
	return &UnprotectDocumentUseCase{
		keys:    keys,
		engine:  engine,
		auditor: auditor,
		eventos: eventos,
	}
}

func (uc *UnprotectDocumentUseCase) Ejecutar(ctx context.Context, cmd UnprotectCommand) (UnprotectResult, error) {
	if err := ctx.Err(); err != nil {
		return UnprotectResult{}, err
	}
	if cmd.ProtectedDocument.Size() == 0 {
		return UnprotectResult{}, errors.New("el documento protegido no puede estar vacio")
	}
	if uc == nil || uc.keys == nil {
		return UnprotectResult{}, errors.New("proveedor de claves de proteccion no configurado")
	}
	if uc.engine == nil {
		return UnprotectResult{}, errors.New("motor de proteccion no configurado")
	}
	keys, err := uc.keys.DecryptionKeys(ctx)
	if err != nil {
		uc.auditar(ctx, cmd, false, err)
		return UnprotectResult{}, fmt.Errorf("no se pudieron obtener claves de descifrado: %w", err)
	}
	defer func() {
		zeroTransientProtectionKeys(keys)
	}()
	mergedKeys, err := mergeTransientProtectionKeys(
		keys,
		cmd.Options,
		cmd.SymmetricKey,
	)
	if err != nil {
		uc.auditar(ctx, cmd, false, err)
		return UnprotectResult{}, err
	}
	keys = mergedKeys
	if len(keys) == 0 {
		err = errors.New("no hay claves de descifrado disponibles")
		uc.auditar(ctx, cmd, false, err)
		return UnprotectResult{}, err
	}
	_ = uc.publicarEvento(ctx, "desproteccion_iniciada", cmd.ProtectedDocument.Name)
	unprotected, err := uc.engine.Unprotect(ctx, cmd.ProtectedDocument, keys)
	if err != nil {
		uc.auditar(ctx, cmd, false, err)
		return UnprotectResult{}, fmt.Errorf("no se pudo desproteger el documento: %w", err)
	}
	_ = uc.publicarEvento(ctx, "desproteccion_completada", unprotected.Document.Name)
	uc.auditar(ctx, cmd, true, nil)
	return UnprotectResult{Unprotected: unprotected}, nil
}

func (uc *UnprotectDocumentUseCase) Execute(ctx context.Context, cmd UnprotectCommand) (UnprotectResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *UnprotectDocumentUseCase) auditar(ctx context.Context, cmd UnprotectCommand, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	resumen := ""
	if err != nil {
		resumen = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "desproteccion",
		DocumentName:  cmd.ProtectedDocument.Name,
		DocumentData:  cmd.ProtectedDocument.Content,
		Success:       success,
		ErrorSummary:  resumen,
	})
}

func (uc *UnprotectDocumentUseCase) publicarEvento(ctx context.Context, tipo, nombre string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{Type: tipo, Payload: []byte(nombre)})
}

func mergeTransientProtectionKeys(
	keys []domain.ProtectionKeyMaterial,
	options map[string]string,
	symmetricKey []byte,
) ([]domain.ProtectionKeyMaterial, error) {
	if len(symmetricKey) > 0 {
		if len(symmetricKey) != 32 {
			return nil, errors.New("la clave simetrica transitoria debe contener exactamente 32 bytes para AES-256-GCM")
		}
		secret := append([]byte(nil), symmetricKey...)
		out := append([]domain.ProtectionKeyMaterial(nil), keys...)
		out = append(out, domain.ProtectionKeyMaterial{
			RecipientID:  "cms-encrypted-transient",
			SymmetricKey: secret,
		})
		return out, nil
	}
	if len(options) == 0 {
		return keys, nil
	}
	secretB64, provided := options["secret_b64"]
	if !provided {
		return keys, nil
	}
	secret, err := decodeTransientProtectionSecret(secretB64)
	if err != nil {
		return nil, err
	}
	out := append([]domain.ProtectionKeyMaterial(nil), keys...)
	// Transferimos la única copia decodificada al ciclo de vida de keys;
	// Unprotect la limpia mediante zeroTransientProtectionKeys.
	out = append(out, domain.ProtectionKeyMaterial{
		RecipientID:  "cms-encrypted-transient",
		SymmetricKey: secret,
	})
	return out, nil
}

func decodeTransientProtectionSecret(secretB64 string) ([]byte, error) {
	secret, err := base64.StdEncoding.Strict().DecodeString(secretB64)
	if err != nil {
		clear(secret)
		return nil, errors.New("secret_b64 no es valido")
	}
	if len(secret) != 32 {
		clear(secret)
		return nil, errors.New("secret_b64 debe decodificar exactamente 32 bytes para AES-256-GCM")
	}

	canonical := make([]byte, base64.StdEncoding.EncodedLen(len(secret)))
	base64.StdEncoding.Encode(canonical, secret)
	isCanonical := len(canonical) == len(secretB64)
	for i := 0; isCanonical && i < len(canonical); i++ {
		isCanonical = canonical[i] == secretB64[i]
	}
	clear(canonical)
	if !isCanonical {
		clear(secret)
		return nil, errors.New("secret_b64 no es Base64 canonico")
	}
	return secret, nil
}

func zeroTransientProtectionKeys(keys []domain.ProtectionKeyMaterial) {
	for i := range keys {
		clear(keys[i].RSAOAEP256PrivateKeyPKCS8)
		keys[i].RSAOAEP256PrivateKeyPKCS8 = nil
		clear(keys[i].MLKEM768Seed)
		keys[i].MLKEM768Seed = nil
		clear(keys[i].X25519PrivateKey)
		keys[i].X25519PrivateKey = nil
		if keys[i].RecipientID == "cms-encrypted-transient" {
			clear(keys[i].SymmetricKey)
			keys[i].SymmetricKey = nil
		}
	}
}
