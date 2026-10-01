// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/ports"
)

const maxCertificateCredentialBytes = 16 * 1024 * 1024

type CertificateAccessUseCase struct {
	access ports.CertificateAccess
}

func NuevoCertificateAccessUseCase(access ports.CertificateAccess) *CertificateAccessUseCase {
	return &CertificateAccessUseCase{access: access}
}

func (uc *CertificateAccessUseCase) List(ctx context.Context, _ ListCertificateAccessOptionsCommand) (ListCertificateAccessOptionsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListCertificateAccessOptionsResult{}, err
	}
	if uc == nil || uc.access == nil {
		return ListCertificateAccessOptionsResult{}, errors.New("acceso a certificados no configurado")
	}
	options, err := uc.access.Options(ctx)
	if err != nil {
		return ListCertificateAccessOptionsResult{}, fmt.Errorf("no se pudieron consultar los gestores de certificados: %w", err)
	}
	return ListCertificateAccessOptionsResult{Options: options}, nil
}

func (uc *CertificateAccessUseCase) OpenManager(ctx context.Context, cmd OpenCertificateManagerCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uc == nil || uc.access == nil {
		return errors.New("acceso a certificados no configurado")
	}
	if strings.TrimSpace(cmd.ManagerID) == "" {
		return errors.New("debe seleccionar un gestor de certificados")
	}
	return uc.access.OpenManager(ctx, strings.TrimSpace(cmd.ManagerID))
}

func (uc *CertificateAccessUseCase) Import(ctx context.Context, cmd ImportCertificateToStoreCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uc == nil || uc.access == nil {
		return errors.New("acceso a certificados no configurado")
	}
	if strings.TrimSpace(cmd.TargetID) == "" {
		return errors.New("debe seleccionar el almacén de destino")
	}
	if len(cmd.Data) == 0 {
		return errors.New("el certificado que se va a importar está vacío")
	}
	if len(cmd.Data) > maxCertificateCredentialBytes {
		return fmt.Errorf("el certificado supera el tamaño máximo de %d MiB", maxCertificateCredentialBytes/(1024*1024))
	}
	return uc.access.Import(ctx, strings.TrimSpace(cmd.TargetID), cmd.Data, cmd.Password)
}

type TemporaryCertificateUseCase struct {
	store ports.TemporaryCertificateStore
}

func NuevoTemporaryCertificateUseCase(store ports.TemporaryCertificateStore) *TemporaryCertificateUseCase {
	return &TemporaryCertificateUseCase{store: store}
}

func (uc *TemporaryCertificateUseCase) Use(ctx context.Context, cmd UseTemporaryCertificateCommand) (UseTemporaryCertificateResult, error) {
	if err := ctx.Err(); err != nil {
		return UseTemporaryCertificateResult{}, err
	}
	if uc == nil || uc.store == nil {
		return UseTemporaryCertificateResult{}, errors.New("almacén temporal no configurado")
	}
	if len(cmd.Data) == 0 {
		return UseTemporaryCertificateResult{}, errors.New("la credencial temporal está vacía")
	}
	if len(cmd.Data) > maxCertificateCredentialBytes {
		return UseTemporaryCertificateResult{}, fmt.Errorf("la credencial temporal supera el tamaño máximo de %d MiB", maxCertificateCredentialBytes/(1024*1024))
	}
	ref, err := uc.store.Load(ctx, cmd.Data, cmd.Password)
	if err != nil {
		return UseTemporaryCertificateResult{}, err
	}
	return UseTemporaryCertificateResult{Certificate: ref}, nil
}

func (uc *TemporaryCertificateUseCase) Remove(ctx context.Context, cmd RemoveTemporaryCertificateCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uc == nil || uc.store == nil {
		return errors.New("almacén temporal no configurado")
	}
	id := strings.TrimSpace(cmd.CertificateID)
	if id == "" {
		return errors.New("debe indicar la credencial temporal")
	}
	uc.store.Remove(id)
	return nil
}

// Clear retira todas las identidades efímeras antes de que la aplicación
// entre en modo residente. No persiste ni devuelve material de credenciales.
func (uc *TemporaryCertificateUseCase) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uc == nil || uc.store == nil {
		return errors.New("almacén temporal no configurado")
	}
	uc.store.Clear()
	return nil
}
