# pkcs7 (copia interna de firma)

Copia de `github.com/digitorus/pkcs7` (v0.0.0-20230818184609-3a137a874352,
licencia MIT, ver `LICENSE`) reducida a la firma y verificación CMS que usa el
paquete `sign`.

Cambios respecto al original:

- `SignerInfoConfig.SkipSigningTime` permite omitir el atributo firmado
  `signing-time`, prohibido en PAdES (`ETSI.CAdES.detached`, ETSI EN 319 142-1).
  Los validadores @firma rechazan la firma si aparece.
- La codificación de longitudes DER se reescribe con comprobaciones de rango
  explícitas (sin conversiones enteras que puedan desbordar).
- Se han retirado el cifrado y descifrado (`encrypt.go`, `decrypt.go`), que no
  se usan y contienen algoritmos obsoletos como DES.
