// SPDX-License-Identifier: EUPL-1.2
//go:build android

package machinepolicy

import "testing"

func TestAndroidMachinePolicyIsAbsent(t *testing.T) {
	if !NoMachinePolicy() || Native() {
		t.Fatal("Android tomó una fuente de política del sistema")
	}
	for _, name := range []string{PermitirSHA1Legacy, PermitirDESLegacy, ProhibirCifradoCMSHeredado} {
		value, present, err := ReadOptIn(name)
		if err != nil || present || value {
			t.Fatalf("%s: valor=%v presente=%v error=%v", name, value, present, err)
		}
	}
	if _, present, err := Strings(AllowedDomains); err != nil || present {
		t.Fatal("Android expuso dominios de política de máquina")
	}
}
