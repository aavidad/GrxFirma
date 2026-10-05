<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma Suite para Linux

Este paquete agrupa los componentes principales de GrxFirma para Linux:

- CLI
- lanzador desktop IPC
- frontend desktop Qt/QML cuando la suite se construye con Qt6
- fallback desktop headless/Fyne
- handler `afirma://`
- `nativehost` para navegadores

## Qué incluye

- `grxfirma`
- `grxfirma-gui`
- `grxfirma-desktop`
- `grxfirma-gui-qml` cuando la build dispone de Qt6
- `grxfirma-afirmauri`
- `grxfirma-nativehost`
- `grxfirma-pkcs11-worker` (auxiliar aislado; capacidad PKCS#11 desactivada)
- `install-suite.sh`
- `configure-browsers.sh`
- `check-runtime-dependencies.sh`
- `runtime-dependencies.qml`
- `README_LINUX_SUITE.md`
- `VERSION.txt`
- `extensions/` con artefactos Firefox y Chromium
- ficheros `.desktop`
- páginas man

La distribución pública no contiene accesos directos de depuración. Sus
binarios Go se compilan con `-tags production` e ignoran cualquier intento de
activar `DEBUG` mediante variables de entorno.

## Artefactos de salida

La build de la suite genera:

- un bundle `.tar.gz` instalable por usuario;
- opcionalmente un paquete `.deb` para instalación del sistema.

Con `VERSION.txt` en `0.0.90`, los ficheros son
`GrxFirma-0.0.90-linux-amd64.tar.gz` y `grxfirma_0.0.90_amd64.deb`.
El paquete Debian se llama `grxfirma` y su versión coincide exactamente
con `VERSION.txt`.

## Instalación manual por usuario

```bash
chmod +x install-suite.sh
./install-suite.sh
```

Antes de modificar `~/.local`, el instalador ejecuta un preflight de todos los
ELF del bundle y de los módulos QML declarados. Si falta una biblioteca o un
módulo Qt, falla sin instalar parcialmente. Los binarios Go estáticos se
reconocen y no se confunden con dependencias ausentes.

Esto instala en:

- `~/.local/bin`
- `~/.local/lib/grxfirma/bin`
- `~/.local/lib/grxfirma/gui-qml/qml` y `assets` cuando la suite incluye Qt
- `~/.local/lib/grxfirma/gui-qml/help` cuando el bundle incluye ayuda PDF localizada
- `~/.local/share/applications`
- manifests de Native Messaging en el perfil del usuario
  para Chrome, Chromium, Edge, Brave, Vivaldi, Opera y Firefox
- registro `afirma://` y reparación de perfiles Firefox detectados, incluyendo
  perfiles Snap o Flatpak cuando existan; la extensión solo se copia si el XPI
  está firmado por Mozilla y coincide con su metadato SHA-256

Además:

- si la suite trae Qt, instala `~/.local/bin/grxfirma-gui-qml` y sus recursos
  `qml/assets/help` cuando existan;
- si la suite no trae Qt, limpia un `grxfirma-gui-qml` anterior y su
  `~/.local/lib/grxfirma/gui-qml` para que el lanzador no reutilice restos
  obsoletos;
- las preferencias del frontend Qt se guardan en
  `~/.config/grxfirma/settings.json` y no se tocan al reinstalar.
- la política local de confianza/TOFU del usuario se conserva en:
  - `~/.config/grxfirma/trusted-domains.json`
  - `~/.config/grxfirma/trusted-domains.seeded`
- los scripts de mantenimiento actualizan las bases XDG que existan. La
  ausencia normal de `/usr/local/share/applications` no genera una advertencia
  espuria durante la instalación o retirada del paquete de sistema.

### CA local para portales en Linux

La CA `GrxFirma Local Root CA` permite a los portales conectar con el servidor
TLS local de `afirma://` en `wss://127.0.0.1`. Se genera para cada usuario al
primer arranque de la aplicación Qt o de `grxfirma-afirmauri`, si este se abre
antes. GrxFirma la instala en `~/.pki/nssdb` y en los perfiles Firefox del
usuario que encuentre en `profiles.ini`: instalación normal, Snap y Flatpak.
En arranques posteriores comprueba los perfiles nuevos. La entrada Firefox
solo recibe confianza para servidores TLS (`C,,`), no para correo ni firma de
código. La aplicación avisa del resultado; si Firefox estaba abierto, hay que
cerrar todas sus ventanas y volver a abrirlo para que cargue la CA. Si una
base NSS estaba ocupada y no pudo actualizarse, cierre Firefox y vuelva a
iniciar GrxFirma. El `postinst` del `.deb` no crea ni instala esta CA como root.

Antes de desinstalar el `.deb`, cada usuario que haya usado GrxFirma puede
retirar únicamente las entradas de CA que acredita su inventario de propiedad:

```bash
grxfirma-afirmauri --remove-local-tls-trust
```

Al retirar el paquete con `sudo`, el `prerm` intenta ejecutar ese comando para
`SUDO_USER` como ese usuario, mientras el binario aún está instalado. `dpkg`
no conoce a los demás usuarios del equipo y en una retirada desatendida puede
no existir `SUDO_USER`: en esos casos cada usuario debe ejecutar el comando
antes de retirar el paquete. Si la base está ocupada, cierre Firefox y repita
el comando; si falla la retirada automática, el `.deb` conserva el binario y
permite repetir la desinstalación. Reinicie Firefox tras retirar la CA. Una
entrada ajena sin huella inventariada permanece intacta.

La preferencia empresarial `security.enterprise_roots` no resuelve este caso
en Linux; GrxFirma usa las bases NSS de los perfiles Firefox y no cambia esa
preferencia.

## Construcción

Desde Linux:

```bash
./packaging/linux/build-suite.sh
./packaging/linux/build-suite.sh --deb
```

### Reproducibilidad

Los binarios Go se construyen con dependencias de solo lectura, PGO desactivado,
`-trimpath`, metadatos VCS desactivados y `buildid` vacío. El versionado visible
se mantiene mediante `main.version`. La etiqueta `production` impide habilitar
trazas `DEBUG` en los binarios distribuidos. El `tar.gz` ordena las entradas y normaliza
propietario, grupo, modos y tiempos; `dpkg-deb` recibe el mismo
`SOURCE_DATE_EPOCH`.

Si `SOURCE_DATE_EPOCH` no está definido se usa la fecha del commit `HEAD`; fuera
de un checkout Git se usa `0`. Para comparar dos builds deben coincidir también
las versiones exactas de Go, C/C++, Qt, navegador usado para generar el CRX,
Python/zlib y `dpkg-deb`. Los binarios Qt y los artefactos generados por el
navegador no se reconstruyen de forma reproducible por estos helpers.

El `.deb` calcula su campo `Depends` durante la build con `dpkg-shlibdeps` para
los ELF y con el paquete Debian propietario de cada `qmldir` importado. La build
falla si un módulo QML no existe o no pertenece a un paquete instalado. El
bundle `tar.gz` no redistribuye Qt: conserva el manifiesto QML y valida el
runtime de la distribución antes de empaquetar y antes de instalar.

El frontend Qt se compila fuera del árbol fuente para que ficheros `moc` o
objetos de una build anterior no contaminen el artefacto.

## Observaciones

El auxiliar PKCS#11 se compila con CGo real y se comprueban sus metadatos de
compilación y dependencias ELF. El TAR lo contiene en su raíz; el DEB lo instala
en `/usr/lib/grxfirma/bin/grxfirma-pkcs11-worker` y la instalación por
usuario en `~/.local/lib/grxfirma/bin/grxfirma-pkcs11-worker`.
Incluirlo no activa la capacidad, no configura módulos ni pide un PIN. Las
etiquetas de los procesos principales permanecen sin PKCS#11. La activación
requiere completar la QA y validación de hardware previstas por el ADR; no se
declara soporte de DNIe/tokens de producción por la presencia del binario.
No se redistribuyen controladores nativos de dispositivos.

En compilaciones experimentales `pkcs11_preview` (nunca `production`), la
entrada segura de PIN requiere una sesión gráfica y `pinentry-qt` o
`pinentry-gnome3` instalado por la distribución. Es una dependencia opcional
del experimento, no un requisito ni una activación de tokens en la suite
normal. Si no hay diálogo seguro disponible, la operación se cancela sin
recurrir a PIN por consola, argumentos, variables de entorno o almacenamiento.

El cliente de tarjetas experimental exige además `/usr/bin/bwrap` administrado
por root, sin setuid, con las opciones de montajes por descriptor y bloqueo de
user namespaces anidados. La base de soporte comprobada es bubblewrap 0.11.1.
El kernel debe permitir la creación de los namespaces y ofrecer seccomp TSYNC
y `statx(STATX_MNT_ID)`; amd64 se ha ejecutado, arm64 requiere prueba propia.
No se cambian AppArmor ni sysctl para forzar la compatibilidad. Sin estas
condiciones se rechaza la operación, nunca se carga el controlador sin aislamiento.
El DEB declara estas herramientas en `Suggests`, no en `Depends`: la firma
ordinaria con certificados/P12 no usa el auxiliar desactivado de tarjetas.

Cada operación monta solo el auxiliar/controlador seleccionados, bibliotecas
de sistema de solo lectura y, si existe, el socket fijo de PC/SC; no monta los
hogares del host. Los recursos adicionales son una política local interna,
no opciones de URI/navegador. SoftHSM solo admite su backend de archivos con
configuración fija de sistema y un directorio privado 0700 del usuario; no se
heredan `HOME`, `SOFTHSM2_CONF` ni `LD_LIBRARY_PATH`. Los almacenes de grupo,
los HSM de red y las dependencias privadas no concedidas necesitan perfiles y
validación independientes. Este aislamiento no acredita ningún lector físico.

- El bundle `tar.gz` está pensado para instalación por usuario sin privilegios.
- El paquete `.deb` instala una disposición de sistema con manifests globales.
- Con `GRXFIRMA_BUILD_CHROMIUM_CRX=1` y un navegador Chromium disponible, la
  `.deb` añade también un `CRX` empaquetado y descriptores de extensión externa.
  La firma CRX3 usa aleatoriedad y por eso esta salida queda fuera de la
  reproducibilidad byte a byte.
- Si la suite incluye `help/`, `Abrir ayuda` intenta primero el PDF del idioma
  seleccionado con estas variantes:
  - `help/ayuda-<locale>.pdf`
  - `help/ayuda-<lang>.pdf`
  - `help/ayuda.pdf`
  - `help/<locale>/ayuda.pdf`
  - `help/<lang>/ayuda.pdf`
- Si no existe PDF compatible, el frontend cae al HTML local embebido.
- Si arrancas la REST local del backend en `127.0.0.1:63118`, dispones de:
  - `https://127.0.0.1:63118/` como consola técnica local para firma, verificación, certificados, diagnóstico y firma múltiple por `/sign-batch`;
  - `https://127.0.0.1:63118/signer` como firmador web local con sello visible y firma múltiple.
- El sello visible expuesto por esas superficies web soporta:
  - una página concreta;
  - rangos como `1,3-5`;
  - todas las páginas con `all`.
- El lanzador desktop prioriza Qt/QML y, si ese frontend no va incluido, usa el fallback desktop disponible en el paquete.
- Si el usuario tiene perfiles Firefox locales, Snap o Flatpak, el instalador
  despliega un XPI firmado y fuerza la delegación `afirma://` al manejador
  externo registrado por XDG. En Firefox confinado, Native Messaging depende
  además del portal WebExtensions disponible en la distribución.
- Para Chrome, Chromium, Edge, Brave, Vivaldi y Opera se incluyen los artefactos
  de extensión. El `CRX` local y su ID solo se generan cuando se solicita con
  `GRXFIRMA_BUILD_CHROMIUM_CRX=1`.
- La instalación automática final en navegadores Chromium sigue dependiendo de distribución firmada o de políticas corporativas del sistema.
- Chromium/Brave/Opera empaquetados como Snap o Flatpak pueden impedir el acceso
  al host externo. Vivaldi Snap no admite Native Messaging; para esos casos se
  requiere un paquete nativo del navegador o una política/portal soportado.
- El host nativo pide confirmación gráfica antes de firmar. En equipos Linux
  debe existir `zenity`, `qarma` o `kdialog`; el modo automático solo debe
  activarse en despliegues gestionados con `GRXFIRMA_NATIVEHOST_AUTO_APPROVE=1`.

## Avisos de nuevas versiones

Qt/QML consulta la última Release estable de GitHub una vez al arrancar, salvo
que el usuario desactive **Configuración > Avisar de nuevas versiones**. La
consulta manual permanece disponible en **Acerca de**. Se necesita salida
HTTPS a `api.github.com:443`; abrir la Release requiere
`github.com:443`.

La Suite no descarga, instala ni ejecuta actualizaciones. El repositorio debe
publicar una Release estable accesible sin autenticación y etiquetada con la
versión del paquete; no se incluye ningún token de GitHub. Un fallo de red,
proxy o publicación no bloquea las operaciones locales y la interfaz explica
cómo revisar la conexión y reintentar.

## Validación de navegadores

Los arneses E2E crean perfiles temporales, empaquetan la extensión del árbol
ensayado y comprueban el recorrido real Extensión → Native Messaging → host
Go. No usan el perfil habitual del usuario:

```bash
packaging/browser-extensions/tests/chromium/run-e2e.sh \
  --browser brave --timeout 90

packaging/browser-extensions/tests/chromium/run-e2e.sh \
  --browser chrome --timeout 90

packaging/browser-extensions/tests/firefox/run-e2e.sh --timeout 90
```

La prueba de Chrome requiere Chrome for Testing, porque Chrome estable puede
bloquear el sideload por línea de órdenes de una extensión unpacked. El arnés
lo detecta y devuelve un resultado limitado en vez de confundir esa política
del navegador con un fallo de Native Messaging.

Firefox Snap necesita `geckodriver`, el portal WebExtensions y el cliente
`flatpak` para autorizar temporalmente la extensión durante el ensayo. El
arnés restaura al terminar el manifiesto y los permisos del portal, incluso si
la prueba falla.

La campaña del 29-07-2026 superó este E2E en:

- Brave `150.1.92.144`;
- Chrome for Testing `151.0.7922.47`;
- Firefox Snap `144.0.2`.

En los tres casos se comprobó `connectNative`, `ping`, consulta de
certificados, identidad/ruta del ejecutable y limpieza sin hosts residuales.
Esto valida la integración técnica, no sustituye la firma/publicación del XPI
ni la distribución de Chromium mediante tienda o política empresarial.

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autoría: Alberto Avidad Fernández
