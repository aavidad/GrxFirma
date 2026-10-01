// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import "context"

type runnerCall struct {
	name  string
	args  []string
	stdin string
}

type fakeRunner struct {
	calls   []runnerCall
	outputs [][]byte
	errs    []error
}

func (f *fakeRunner) Run(_ context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runnerCall{name: name, args: append([]string(nil), args...), stdin: string(stdin)})
	idx := len(f.calls) - 1
	var out []byte
	if idx < len(f.outputs) {
		out = f.outputs[idx]
	}
	var err error
	if idx < len(f.errs) {
		err = f.errs[idx]
	}
	return out, err
}
