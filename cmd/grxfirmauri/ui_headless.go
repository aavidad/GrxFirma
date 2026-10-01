// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/desktop/truststore"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
	"grxfirma/presentation/desktop/progressdialog"
)

func newCertSelector() certpicker.CertSelector {
	return certpicker.NewHeadless()
}

func newProgressProvider() progressdialog.ProgressProvider {
	return &progressdialog.HeadlessProvider{}
}

func newTrustPolicy(configDir string) (ports.TrustPolicy, error) {
	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, err
	}
	policy, err := config.LoadPolicy(config.DirPolicyDefecto)
	if err != nil {
		return nil, err
	}
	return truststore.NewWithOptions(configDir, truststore.Options{
		SystemAllowlistFile: truststore.SystemAllowlistPath,
		Headless:            true,
		TOFUEnabled:         cfg.TofuHabilitado,
		ExtraAllowed:        policy.DominiosDeConfianza,
	})
}

func updateProtocolUI(status, detail string) {}

func setProtocolCompletionUI(bool) {}
