// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
for (const variant of ['chromium', 'firefox']) {
  test(`${variant}: valida y normaliza sitios HTTPS`, async () => {
    const context = vm.createContext({ URL });
    vm.runInContext(await readFile(path.join(root, variant, 'trusted_sites.js'), 'utf8'), context);
    const normalize = (value) => vm.runInContext(`grxfirmaTrustedSites.normalizeSite(${JSON.stringify(value)})`, context);
    assert.equal(normalize('  Ejemplo.ES  '), 'ejemplo.es');
    assert.equal(normalize('*.Sede.Ejemplo.ES'), '*.sede.ejemplo.es');
    assert.equal(normalize('MÜNCHEN.de'), 'xn--mnchen-3ya.de');
    for (const bad of ['*', '*.es', '*.gob.es', '*.co.uk', '*.com.es', '*.github.io', 'github.io', 'com', 'localhost', 'a.localhost', '127.0.0.1', '127.1', '[::1]', 'http://ejemplo.es', 'https://ejemplo.es', 'ejemplo.es/ruta', 'ejemplo.es:443', 'ejemplo.es?x=1', 'ejemplo%2ees', 'a..es', '-ejemplo.es']) {
      assert.throws(() => normalize(bad), /invalid_site/, bad);
    }
    assert.equal(vm.runInContext("grxfirmaTrustedSites.matches('https://sub.ejemplo.es/doc.pdf', '*.ejemplo.es')", context), true);
    assert.equal(vm.runInContext("grxfirmaTrustedSites.matches('https://ejemplo.es.evil.org/', '*.ejemplo.es')", context), false);
  });
}
