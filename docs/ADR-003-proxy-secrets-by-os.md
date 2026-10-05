<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR-003 — Secretos de proxy por almacen seguro del SO

## Estado
Aprobado.

## Contexto

AutoFirma 1.9 y la variante Go historica ya contemplaban configuracion de
proxy mas rica que la hoy expuesta en V2:

- tipo de proxy,
- usuario,
- password,
- exclusiones.

En V2 existe `proxy` no sensible tipado (`Enabled`, `Type`, `Host`, `Port`,
`ExcludedURLs`) dentro de `ConfiguracionUsuario`. La credencial se gestiona
separadamente mediante un identificador opaco y un almacén seguro por SO; no se
modela como una preferencia persistible. Guardarla en claro o introducir un
almacén ad hoc contradiría el hardening fijado para V2.

## Decision

El flujo de credenciales aplica estas decisiones:

1. existe un puerto especifico `ProxySecretStore` en `internal/ports`;
2. el puerto tiene adaptadores por SO:
   - Linux: Secret Service / `libsecret`
   - macOS: Keychain
   - Windows: Credential Manager / DPAPI
3. la configuracion de usuario seguira guardando solo metadatos no sensibles:
   - `proxyEnabled`
   - `proxyType`
   - `proxyHost`
   - `proxyPort`
   - `proxyExcludedUrls`
   - si hace falta, un `proxySecretId` y/o `realm` opacos
4. el material sensible (`username`, `password`) solo se resolvera justo antes
   de abrir la conexion de red y no se persistira en claro en `settings.json`,
   logs, evidencias ni trazas.

## Consecuencias

### Positivas

- se mantiene la arquitectura hexagonal;
- se evita introducir `proxyPassword` en DTOs o documentos de preferencias;
- se puede cerrar luego la paridad funcional de proxy por SO sin deuda de
  seguridad.

### Negativas

- la disponibilidad depende del almacén de sesión del usuario y puede requerir
  desbloqueo/interacción del SO;
- la validación final de Secret Service, Keychain y DPAPI sigue siendo una
  comprobación manual de release por plataforma.

## No objetivos de esta ADR

- no persiste ni devuelve `proxyPassword` mediante REST/CLI;
- no implementa NTLM/Kerberos ni convierte un proxy corporativo en una nueva
  autoridad de confianza;
- no sustituye la validación manual de los almacenes reales en cada SO.

## Estado de implementacion

Primer aterrizaje ya en repo:

- Linux dispone de un adaptador inicial en
  [proxysecretstore](../internal/adapters/outbound/desktop/proxysecretstore)
  que usa `secret-tool` sobre Secret Service;
- Windows dispone también de un adaptador inicial basado en `DPAPI`
  (`dpapi-user`);
- macOS dispone de adaptador CGo sobre Security.framework/Keychain;
- el backend seguro ya permite `Store/Load/Delete` de credenciales de proxy
  sin persistirlas en `settings.json`;
- el backend ya expone además un `Status()` mínimo para diagnosticar si el
  secret store local está disponible antes de intentar usar proxy autenticado;
- la REST local ya publica ese diagnóstico en
  `GET /settings/proxy/secret-store/status`;
- el firmador web local `/signer` muestra también ese estado visible del
  almacén seguro;
- `REST /settings` e `IPC save_settings` ya rechazan intentos de persistir
  `proxyUsername` o `proxyPassword` en claro;
- el resolvedor runtime local de `proxySecretId` alimenta el transporte HTTP
  compartido de `grxfirmauri`/trifásico, CLI, GUI, nativehost, TSA,
  revocación y verificadores, con fallo cerrado;
- Qt/QML expone creación, rotación y borrado mediante IPC local autenticado.
  La operación sensible no se difiere, se redacta completa en logs y zeroiza
  los buffers controlables; `save_settings` no puede alterar sus referencias;
- el código y las pruebas automáticas del proxy seguro están cerrados. La matriz manual
  de release conserva Secret Service, Keychain y DPAPI reales.
