<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Puertos y Adaptadores — GrxFirma

## Introducción

En GrxFirma, los **puertos** son las interfaces de Go que el núcleo (`internal/application`) utiliza para interactuar con el mundo exterior. Los **adaptadores** son las implementaciones concretas que cambian según la plataforma (escritorio, móvil, servidor).

---

## Puertos (Interfaces)

### 1. Salida (Outbound) — Servicios que el núcleo utiliza

#### `SignerEngine`
Encargado de la firma criptográfica real.
- **Implementaciones de producción**: los motores de firma conectados por cada
  plataforma. `SidecarSigner` y el adaptador PKCS#11 son diseños o bases
  experimentales y no están cableados desde los binarios de producción.

#### `ResultTransport` (antes `IntermediateServer`)
Encargado de enviar los resultados de vuelta al origen (navegador o servidor intermedio).
- **Implementaciones**: `HTTPTransport`, `LocalFileTransport`.

#### `SecureStorage`
Almacenamiento de configuraciones y valor-pruebas (tokens, URLs de confianza).
- **Implementaciones**: `SecretService` (Linux), `Keychain` (macOS/iOS), `WindowsCertStore`, `AndroidKeystore`.

#### `CertificateCatalog`
Busca certificados aprovechando las capacidades del sistema.
- **Implementaciones**: `NSSScanner` (Linux), `SyscallCertStore` (Windows), `P12ManualImporter` (Mobile).

#### `CertificateImporter`
Importa credenciales externas reutilizables por distintos entry points sin duplicar parsing.
- **Implementaciones**: `pkcs12importer` (P12/PFX y PEM detras de un adaptador comun).

#### `ProtectionRecipientCatalog` y `ProtectionKeyProvider`

Separan el material público anunciado para cifrado de las claves privadas que
pueden desproteger. `LocalCombinedKeyring` agrega:

- `LocalStrongKeyring`, con identidad local `ML-KEM-768 + X25519`;
- `LocalCompatKeyring`, con identidades RSA-OAEP procedentes de los catálogos
  de certificados.

El catálogo compatible falla cerrado. Antes de anunciar una identidad comprueba
certificado RSA, `KeyUsage` de `KeyEncipherment`, coincidencia entre la clave
pública del certificado y la del firmante, y que el firmante sea una
`*rsa.PrivateKey` exportable. Esta consulta no serializa la clave. Solo
`DecryptionKeys` crea el PKCS#8 necesario para el motor, y el caso de uso lo
zeroiza después de desproteger junto con las semillas ML-KEM/X25519. De este
modo un `crypto.Signer` opaco de Windows CertStore, PKCS#11 o hardware sigue
disponible para firma, pero no se ofrece como destinatario RSA hasta que su
adaptador proporcione capacidad de descifrado. Un P12/PFX RSA cargado en memoria
sí puede satisfacer el contrato.

#### `EventPublisher`
Notifica al borde (UI, adaptadores de entrada) sobre el progreso asíncrono.
- **Implementaciones**: `ChannelPublisher` (Go nativo), `IPCEventForwarder` (Sidecars).

#### Puertos de identidad reforzada

`AutorizadorSolicitudIdentidad`, `CanonicalizadorIdentidad`,
`RepositorioRetosIdentidad`, `VerificadorPruebaIdentidad` y
`SelectorCertificadoIdentidad` mantienen el contrato genérico fuera de REST, Native
Messaging y cualquier aplicación integradora. El selector recibe candidatos públicos
y devuelve uno, pero su adaptador de sistema nunca serializa el catálogo al portal.
El repositorio separa `Reservar` de
`Finalizar`: nunca mantiene una transacción mientras el verificador consulta cadena,
políticas u OCSP/CRL. `identityjcs` implementa ya el canonicalizador con el perfil
`rfc8785-jcs-v1`; `identitypolicy` autoriza combinaciones exactas,
`identitymemory` consume retos efímeros concurrentes, `identityverifier` aplica el
perfil CAdES/X.509 con revocación trivalente e `identityevidence` cifra y encadena
la evidencia durable. Véase
[IDENTIDAD_REFORZADA_V1.md](IDENTIDAD_REFORZADA_V1.md).

---

## Adaptadores (Implementaciones)

### 1. Inbound (Entrada) - Iniciadores de operaciones

| Adaptador | Plataformas | Protocolo | Notas |
|-----------|-------------|------------|-------|
| `cli`     | Todas       | Consola    | Headless / MVP |
| `rest`    | Desktop     | HTTP Local | Integración navegador |
| `nativehost` | Desktop | Native Messaging | Firma y prueba de identidad con caller acreditado |
| `identity_bridge` | Chromium/Firefox | `window.postMessage` acotado | Solo canon y correlación desde portales HTTPS gestionados |
| `intent`  | Android     | Intent     | URI filter |
| `deeplink`| iOS/Android | URL Scheme | `afirma://` |

### 2. Outbound (Salida) - Proveedores de capacidades

| Adaptador | Plataformas | Puerto implementado | Tecnología |
|-----------|-------------|---------------------|------------|
| `nss`     | Linux       | `CertificateCatalog`| `p11-kit`  |
| `pkcs12importer` | Todas | `CertificateImporter` | `go-pkcs12` encapsulado |
| `keystore`| Android     | `SecureStorage`     | Java API   |
| `auditlog`| Todas       | `EvidenceLogger`    | Filesystem |
| `identityevidence` | Servidor | `RegistroEvidenciaIdentidad` | AES-256-GCM + HMAC-SHA-256 |
| `identityverifier` | Servidor | `VerificadorPruebaIdentidad` | CAdES, X.509, EKU, OID y OCSP/CRL |
| `selectorCertificadoIdentidadSistema` | Desktop | `SelectorCertificadoIdentidad` | Diálogo local e i18n por plataforma |

---

## Gestión de Capacidades (CapabilityProfileProvider)

Debido a la naturaleza multiplataforma, no todos los puertos estarán disponibles siempre.

- **Detección**: Al arrancar, el caso de uso `ResolvePlatformProfile` consulta este puerto para saber qué adaptadores cargar.
- **Degradación**: si no hay un almacén del SO conectado, GrxFirma opera
  con certificados P12/PFX aportados por el usuario. PKCS#11/DNIe no forma
  parte del perfil productivo actual.
