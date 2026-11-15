<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Idiomas de los instaladores Windows

Los cuatro instaladores NSIS (suite, AfirmaURI, CLI y Qt) usan los catálogos
`i18n/{es,en,ca,va,gl,eu,fr,de,it,pt,zh}.json`. Los JSON son la fuente; los
`LangString` de cada `.nsh` y los metadatos de `version.nsh` se generan con:

```sh
python3 packaging/windows/i18n/generate.py
python3 packaging/windows/i18n/generate.py --check
python3 -m unittest discover -s packaging/windows/tests -p 'test_*.py'
```

NSIS propone el idioma del sistema y muestra un selector antes de las páginas
de instalación. En `/S`, `MUI_LANGDLL_DISPLAY` omite el diálogo. Se mantienen
los códigos de salida, los argumentos `/AFIRMA=`, la preferencia
`HKCU\Software\GrxFirma\AfirmaProtocolHandler` y la restauración de AutoFirma.
No se guarda una nueva preferencia de idioma en el registro.

NSIS trae diez de los idiomas solicitados. Para valenciano se incluye una base
`Valencian.nlf`/`Valencian.nsh` derivada de los controles estándar Catalan de
NSIS 3.10, con LCID 2051 (`ca-ES-valencia`) y nombre «Valencià» en el selector.
Conserva los créditos del catálogo de NSIS y su licencia zlib/libpng. Los avisos,
errores, preguntas y descripciones propios de GrxFirma usan `va.json`, separado
de `ca.json`. Los demás controles estándar proceden de los idiomas de NSIS;
el chino utiliza SimpChinese y el portugués, Portuguese.

Los metadatos PE que ya se emitían con idioma neutro conservan su descripción
en castellano, ahora leída del catálogo. Las páginas y los avisos se resuelven
en el idioma elegido. La página de elección de AutoFirma reserva más altura
para las traducciones largas.

`tests/test_installer_i18n.py` comprueba las claves de los once idiomas, los
marcadores de ejecución y la correspondencia entre JSON e inclusiones. También
compila los cuatro instaladores reales con una carga sintética temporal y
comprueba las once tablas de idioma sin advertencias de NSIS. La suite se
compila con y sin los dos frontends opcionales. Requiere `makensis` instalado;
no descarga dependencias ni ejecuta los instaladores.

`i18n/code-exceptions.json` enumera las excepciones exactas a la comprobación de
prosa en el código: identidad del editor, rutas históricas de accesos y el
error de compilación por falta de `STAGE_DIR`. Las rutas se conservan para
actualizar y desinstalar sin dejar accesos antiguos; no se traducen las claves
del registro ni los nombres y argumentos internos. Los comentarios y registros
técnicos de los scripts PowerShell quedan fuera de los catálogos de páginas
NSIS.
