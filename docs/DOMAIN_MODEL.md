<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Modelo de Dominio — GrxFirma

## Introducción

El paquete `internal/domain` contiene los conceptos centrales del negocio de firma digital, independientemente de si se trata de una extensión de navegador, CLI o App móvil.

---

## Entidades de Dominio

### 1. Document
Representa el contenido que se va a firmar.
- **Atributos**: `Content` ([]byte), `MimeType` (string), `Filename` (string), `Metadata` (map).
- **Invariante**: No debe contener la firma; solo el material original.

### 2. ExchangeSession (antes llamado TransportTicket)
Representa el estado de una operación asíncrona que requiere intercambio de datos externo.
- **Atributos**: `SessionID` (UUID), `TargetURL` (string), `Status` (Enum: Pending, Signed, Error, Cancelled), `AuditID` (string).
- **Contexto**: Se utiliza para rastrear operaciones que comienzan en un navegador (desktop) o app (mobile) y requieren respuesta a través de un servidor intermedio o archivo local.

### 3. SignatureResult
El resultado final de una operación de firma exitosa.
- **Atributos**: `Data` ([]byte), `Format` (string canónica del formato: `CAdES`, `XAdES`, `XMLdSig`, `PAdES`, etc.), `Algorithm` (SHA256withRSA, etc.), `Timestamp` (Time).
- **Sanitización**: Debe estar listo para ser enviado al adaptador de salida sin procesos adicionales de limpieza (la limpieza se realiza en el núcleo).

Nota importante:
- el dominio no mantiene una lista cerrada de formatos de firma;
- la validación estructural del formato concreto se resuelve en la aplicación y los adaptadores;
- esto evita tocar el núcleo cuando entra un formato nuevo como `XMLdSig`, `ODF`, `OOXML` o futuros formatos.

### 4. CertificateRef
Una referencia a un certificado disponible para firmar.
- **Atributos**: `ID` (string), `Subject` (string), `Issuer` (string), `ValidFrom`, `ValidTo`, `PlatformSource` (NSS, CertStore, P12, etc.).
- **Nota**: Esta entidad no contiene la clave privada; solo los metadatos públicos para permitir la selección por parte del usuario.

### 5. VerificationResult
El informe técnico de la validez de una firma preexistente.
- **Atributos**: `IsValid` (bool), `Status` (Valid, Invalid, Trusted, Untrusted), `Chain` (List de Certificados), `ErrorDetail` (string).

### 6. Identidad reforzada

`SolicitudRetoIdentidad`, `PruebaIdentidad` y `ResultadoVerificacionIdentidad`
representan el contrato neutral `identidad-reforzada/v1`. Los enlaces de tenant y
sesión son seudónimos; no existen roles ni decisiones de acceso. El resultado conserva
`accepted`, `rejected` e `indeterminate` y una aceptación exige seis dictámenes
conformes. El verificador CAdES comprueba además que los OID de `SignerInfo` declaran
SHA-256 y el algoritmo compatible con la clave RSA o ECDSA. Los nombres JSON
pertenecen a los adaptadores, no al dominio.

---

## DTOs Internos vs DTOs de Integración

El dominio utiliza sus propios tipos de datos. La capa **Anti-Corrupción (ACL)** es la responsable de transformar los datos de entrada al modelo de dominio:

| Tipo Externo (Adapter) | Entidad Interna (Domain) |
|-------------------------|--------------------------|
| `afirma://?fileid=...` | `ExchangeSession` + `Document` |
| `intent:open?url=...`   | `ExchangeSession` |
| `JSON Request`          | `SignCommand` |

---

## Servicios de Dominio (Domain Services)

Lógica compleja que involucra múltiples entidades pero no requiere IO:
- **BatchManager**: Lógica para agrupar documentos en una operación de firma múltiple coherente.
- **PolicyChecker**: Validación de si el certificado seleccionado cumple con las restricciones de la política de firma requerida por el emisor.
