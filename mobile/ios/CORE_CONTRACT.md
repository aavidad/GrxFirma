<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Contrato XCFramework iOS v1

El único núcleo aceptado es el generado con `gomobile bind ./mobilebind`. La
app no contiene una implementación criptográfica Swift alternativa y Release
no puede degradarse a un mock.

## Fábrica requerida

La API Go debe exportar una función bind-friendly equivalente a:

```go
func NewIOSFacade(appSupportDir, appGroupDir, keychainAccessGroup string) (*Facade, error)
```

La cabecera Objective-C generada debe exponer `MobilebindNewIOSFacade`. La
fábrica debe montar servicios reales para firma, verificación, selección e
importación de certificados y perfil iOS. No es válido
devolver una fachada con servicios `nil`.

`appSupportDir` y `appGroupDir` son rutas ya creadas dentro del sandbox. El
adaptador Go debe rechazar rutas que escapen de esos directorios. El grupo de
Keychain procede de los entitlements firmados; no debe sustituirse por un
valor enviado en un enlace.

## Contrato operativo

La fachada debe incluir estos métodos públicos:

```text
MobileContractJSON() -> String
ClearSession()
SelectCertificateJSON(String) -> (String, error)
ImportCertificateJSON(String) -> (String, error)
SignJSON(String) -> (String, error)
VerifyJSON(String) -> (String, error)
ResolvePlatformProfileJSON() -> (String, error)
```

`MobileContractJSON()` debe devolver como mínimo:

```json
{
  "contract_version": 1,
  "platform": "ios",
  "services": {
    "sign": true,
    "verify": true,
    "select_certificate": true,
    "import_certificate": true,
    "remote_exchange": false
  },
  "identity_store": {"persistent": false},
  "approval": "native_ui_explicit_action",
  "verification": {"cryptographic_integrity": true},
  "limits": {
    "document_bytes": 33554432,
    "signed_output_bytes": 50331648,
    "certificate_bytes": 4194304,
    "password_bytes": 1024
  }
}
```

Cada `true` afirma que el servicio está configurado y operativo. La app exige
aprobación en UI nativa, integridad criptográfica y límites del núcleo iguales
o superiores a los suyos. Si falta un campo crítico, la plataforma no es
`ios`, un servicio es `false` o un límite es insuficiente, el núcleo queda
deshabilitado. El adaptador actual no enlaza métodos remotos bind-friendly, por
lo que fuerza esa capacidad a `false` incluso si un contrato futuro la
anunciase; habilitarla exigirá ampliar y volver a validar el bridge nativo.

Los JSON de operación son los ya expuestos por `mobilebind/facade.go`; la shell
Swift no cambia nombres ni semántica. El algoritmo de un enlace directo se
propaga en `options.algorithm` y aparece de nuevo en la confirmación. Se aplican
además estos límites nativos:

El motor actual informa en PAdES una etiqueta de perfil en el campo de salida
`algorithm`; por ello los enlaces PAdES se limitan a SHA-256. En CAdES y XAdES
la shell exige que el algoritmo devuelto coincida con el confirmado.

| Dato | Límite |
|---|---:|
| documento local/remoto | 25 MiB |
| PKCS#12 | 2 MiB |
| firma devuelta | 40 MiB |
| JSON bind | 56 MiB |
| URL `afirma://` | 8 KiB |
| documento Base64 en URL | 5 KiB |

## Gate binario

`scripts/mobile/ios/validate_core_xcframework.py` exige:

1. una slice `ios-arm64` para dispositivo;
2. una slice de simulador con `arm64` y `x86_64`;
3. cabeceras idénticas y todos los símbolos del contrato;
4. `module.modulemap`, plist y binarios no vacíos;
5. arquitecturas reales mediante `lipo` y fábrica mediante `nm` en macOS;
6. checksum canónico SHA-256 aprobado.

El contrato concurrente actual ya exporta `NewIOSFacade`, `MobileContractJSON`
y `ClearSession`, pero declara identidad en memoria y `remote_exchange=false`.
La shell no persiste la identidad ni simula transporte: limpia la sesión al
pasar a segundo plano y limita `afirma://` operativo a documentos incrustados.
