// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

module grxfirma

go 1.26.8

require (
	fyne.io/fyne/v2 v2.7.3
	github.com/Microsoft/go-winio v0.6.2
	github.com/deatil/go-cryptobin v1.1.1013
	github.com/digitorus/pdf v0.1.2
	github.com/digitorus/pdfsign v0.0.0-20260102145623-a2800162ff5c
	github.com/digitorus/pkcs7 v0.0.0-20230818184609-3a137a874352
	github.com/digitorus/timestamp v0.0.0-20231217203849-220c5c2851b7
	github.com/gowebpki/jcs v1.0.1
	github.com/miekg/pkcs11 v1.1.2
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/crypto v0.56.0
	golang.org/x/image v0.45.0
	golang.org/x/net v0.57.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	software.sslmate.com/src/go-pkcs12 v0.7.2
)

replace github.com/digitorus/pdfsign => ./third_party/pdfsign

require (
	fyne.io/systray v1.12.0 // indirect
	github.com/BurntSushi/toml v1.5.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fredbi/uri v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fyne-io/gl-js v0.2.0 // indirect
	github.com/fyne-io/glfw-js v0.3.0 // indirect
	github.com/fyne-io/image v0.1.1 // indirect
	github.com/fyne-io/oksvg v0.2.0 // indirect
	github.com/go-gl/gl v0.0.0-20231021071112-07e5d0ea2e71 // indirect
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20240506104042-037f3cc74f2a // indirect
	github.com/go-text/render v0.2.0 // indirect
	github.com/go-text/typesetting v0.3.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/hack-pad/go-indexeddb v0.3.2 // indirect
	github.com/hack-pad/safejs v0.1.0 // indirect
	github.com/jeandeaual/go-locale v0.0.0-20250612000132-0ef82f21eade // indirect
	github.com/jsummers/gobmp v0.0.0-20230614200233-a9de23ed2e25 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattetti/filebuffer v1.0.1 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/nicksnyder/go-i18n/v2 v2.5.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rymdport/portal v0.4.2 // indirect
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/yuin/goldmark v1.7.17 // indirect
	golang.org/x/text v0.41.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/digitorus/pdf => ./third_party/pdf
