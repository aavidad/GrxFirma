<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Ayuda contextual de las opciones técnicas

Este documento recoge las opciones de GrxFirma cuyo significado no resulta
evidente para una persona sin conocimientos técnicos, la clave de su texto de
ayuda y el control en el que se muestra en cada interfaz. Sirve para colocar
los botones «?» y para mantener los textos cuando cambie una pantalla.

## Dónde están los textos

- Catálogos comunes: `internal/adapters/outbound/common/localizador/locales/*.json`
  (ca, de, en, es, eu, fr, gl, it, pt, va, zh). Todas las claves empiezan por
  `ayuda.`.
- `ayuda.boton_nombre` («Ayuda sobre {0}») es el nombre accesible del botón
  «?». `{0}` se sustituye por la etiqueta visible del control, ya traducida.
  En QML hay que sustituir `{0}` a mano, porque `tr()` no lo hace.
- WinUI y Qt leen el catálogo común directamente.
- Android lee sus textos de interfaz de `res/values*/strings.xml`. Los
  `assets/locales/*.json` solo traducen mensajes del motor y se generan con
  `scripts/mobile/android/sync_verification_locales.py`. Al poner los «?» en
  Android, copie cada texto del catálogo común a `strings.xml` y a sus diez
  traducciones con el nombre `ayuda_<concepto>`, cambiando los puntos por
  guiones bajos (`ayuda.formato.pades` pasa a `ayuda_formato_pades`). No se
  añaden antes porque el lint de Android trata los recursos sin uso como
  error.
- Las ayudas son breves: qué es, cuándo usarlo y el valor recomendado. No
  sustituyen al manual.

## Criterios para la fase 2

- El «?» va junto a la etiqueta del control, siempre en la misma posición, y
  se puede usar con teclado y lector de pantalla (WCAG 2.2 AA). Al pulsarlo se
  abre una ventana flotante o un mensaje con el texto; no se muestra solo al
  pasar el ratón.
- Si el control ya tiene `AutomationProperties.HelpText`, `ToolTip` o
  `helperText`, el «?» no lo sustituye. Cuando digan lo mismo, se puede retirar
  el texto antiguo para no repetirlo.
- En los selectores de opciones (operación, formato) un único «?» muestra el
  texto general. Si la plataforma lo permite, también puede mostrar el texto
  de la opción elegida.
- Perfil de firma Baseline B/T/LT/LTA: lo prepara otra rama
  (`feat/ayuda-perfil-baseline`). Aquí no tiene clave.
- WinUI: el botón es `Controls/HelpButton.cs` y `Controls/HelpRow.cs` lo
  coloca justo después de su control. En XAML se escribe una
  `<controls:HelpRow>` con el control y un
  `<controls:HelpButton Topic="…" HelpKey="ayuda.…" />`; en ENI se usa
  `WithHelp(…)`. `Topic` es la etiqueta visible del control. El contrato
  `cmd/gui-winui/tests/test_contextual_help_contract.py` comprueba que cada
  clave asignada a WinUI se usa y cuántos «?» tiene cada pantalla.

## Abreviaturas de rutas

- `W/` = `cmd/gui-winui/src/GrxFirma.WinUI/Views/`
- `Q/` = `cmd/gui-qml/qml/`
- `A/` = `mobile/android/app/src/main/res/layout/`
- `AJ/` = `mobile/android/app/src/main/java/io/github/aavidad/grxfirma/android/`

Las líneas de `main.qml` pueden desplazarse uno o dos puestos con cada
cambio. Busque el `id` indicado.

## Firmar

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Operación: firma | `ayuda.operacion.firma` | `W/SignPage.xaml:290` desplegable «Operación» | `Q/main.qml:9278` `signActionCombo`; preferencias `:14771` `settingsSignActionCombo` | `A/activity_main.xml:390` `signatureAction` | Ninguna; en Qt, ToolTip solo en preferencias |
| Operación: cofirma | `ayuda.operacion.cofirma` | igual que la anterior | igual que la anterior | igual, y aviso `cosignNotice` `A/activity_main.xml:337` | Android explica la opción en su etiqueta |
| Operación: contrafirma | `ayuda.operacion.contrafirma` | igual que la anterior | igual que la anterior | igual que la anterior | Android explica la opción en su etiqueta |
| Cofirma múltiple guiada | `ayuda.cofirma_multiple` | `W/SignPage.xaml:446` | `Q/main.qml:9350` `multiCosignCheckBox` | no aparece | HelpText (WinUI), ToolTip (Qt) |
| Firma por lotes | `ayuda.lote` | `W/SignPage.xaml:220` casilla | `Q/main.qml:9087` «Seleccionar varios» | `A/section_tools.xml:24` `toggleBatchButton`; `A/section_batch_wave4.xml:14` `batchAction` | HelpText (WinUI); `batch_helper` (Android) |
| Formato (selector) | `ayuda.formato` | `W/SignPage.xaml:297` `FormatCombo`; `W/SettingsPage.xaml:187` | `Q/main.qml:9295` `signFormatCombo`; preferencias `:14796` | `A/activity_main.xml:450` `signatureFormat`; `A/dialog_preferences.xml:36` `prefFormat` | Ninguna |
| Formato PAdES | `ayuda.formato.pades` | opción de `FormatCombo` | opción de `signFormatCombo` | opción de `signatureFormat` | Ninguna |
| Formato CAdES | `ayuda.formato.cades` | igual | igual | igual | Ninguna |
| Formato XAdES | `ayuda.formato.xades` | igual | igual | igual | Ninguna |
| Formato XMLDSig | `ayuda.formato.xmldsig` | igual | igual | no aparece | Ninguna |
| Formato ODF | `ayuda.formato.odf` | igual | igual | no aparece | Ninguna |
| Formato OOXML | `ayuda.formato.ooxml` | igual | igual | no aparece | Ninguna |
| Formato Facturae | `ayuda.formato.facturae` | igual | igual | no aparece | Ninguna |
| Formato ASiC | `ayuda.formato.asic` | igual («ASiC-XAdES») | igual («ASiC-XAdES») | no aparece | Ninguna |
| Formato Veri*Factu | `ayuda.formato.verifactu` | opción dinámica de `FormatCombo` | opción dinámica de `signFormatCombo` | opción de `signatureFormat` | `verifactu_sign_hint` (Android) |
| Sobrescritura | `ayuda.sobrescribir` | no aparece | `Q/main.qml:9318` `signOverwriteCombo`; preferencias `:14827` | no aparece | ToolTip solo en preferencias (Qt) |
| Compatibilidad estricta | `ayuda.compatibilidad_estricta` | `W/SettingsPage.xaml:195` | `Q/main.qml:9407` `signStrictCompatCheckBox`; preferencias `:15406` | no aparece | Ninguna (WinUI); ToolTip (Qt) |
| Validar al terminar | `ayuda.validar_al_terminar` | `W/SignPage.xaml:1019` | no aparece | no aparece | HelpText técnico |
| Firma remota (CSC) | `ayuda.firma_remota` | `W/SignPage.xaml:421` `RemoteSigningButton`; `W/RemoteSigningDialogs.cs:68` | `Q/main.qml:7281` diálogo `cscRemoteDialog` | no aparece | Texto visible en Qt; HelpText en WinUI |
| Motivo del PDF | `ayuda.pdf.motivo` | `W/SignPage.xaml:794` | `Q/main.qml:9814` `signReasonField`; preferencias `:15048` | no aparece | Texto de sección en Qt (`:9750`) |
| Lugar del PDF | `ayuda.pdf.lugar` | `W/SignPage.xaml:800` | `Q/main.qml:9826` `signLocationField` | no aparece | HelpText (WinUI) |
| Contacto del PDF | `ayuda.pdf.contacto` | `W/SignPage.xaml:807` | `Q/main.qml:9840` `signContactField` | no aparece | Ninguna |

## Sello visible

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Sello visible | `ayuda.sello.visible` | `W/SignPage.xaml:509`; `W/SettingsPage.xaml:221` | `Q/main.qml:9387` `signVisibleSealCheckBox`; preferencias `:14881` | `A/activity_main.xml:318` `visibleSealCheck` | HelpText técnico (WinUI); ToolTip (Qt) |
| Estilo del sello | `ayuda.sello.estilo` | `W/SignPage.xaml:537` | `Q/main.qml:9434` botones de estilo | `AJ/seal/SealEditorDialog.kt:236` | HelpText (WinUI) |
| Opacidad | `ayuda.sello.opacidad` | `W/SignPage.xaml:577` | `Q/main.qml:9464` `signSealLogoOpacitySlider`; preferencias `:14933` | `AJ/seal/SealEditorDialog.kt:186` | ToolTip en WinUI y Qt; ninguna en Android |
| Imagen | `ayuda.sello.imagen` | `W/SignPage.xaml:681` | `Q/main.qml:9764` «Usar logo GrxFirma» y siguientes | `AJ/seal/SealEditorDialog.kt:245` | Texto (Qt `:9798`) |
| Datos en el sello | `ayuda.sello.texto` | `W/SignPage.xaml:597` | `Q/main.qml:9780` `signSealKeepTextCheckBox` | `AJ/seal/SealEditorDialog.kt:248` | HelpText (WinUI) |
| Código QR | `ayuda.sello.qr` | `W/SignPage.xaml:604` y `:610` | `Q/main.qml:9484` `signQREnabledCheckBox` | `AJ/seal/SealEditorDialog.kt:200` | HelpText (WinUI) |
| CSV de cotejo | `ayuda.sello.csv` | `W/SignPage.xaml:624` | `Q/main.qml:9510` `csvEnabledCheck` | `AJ/seal/SealEditorDialog.kt:491` | Aviso `csv_notice` (Qt y Android) |
| Páginas | `ayuda.sello.paginas` | `W/SignPage.xaml:649` y `:669` | `Q/main.qml:9551` `signSealPagesField` | `AJ/seal/SealEditorDialog.kt:263` | HelpText (WinUI); marcador (Qt) |
| Posición, tamaño y giro | `ayuda.sello.posicion` | `W/SignPage.xaml:703` a `:781` | `Q/main.qml:9639` `signSealRotationCombo`, `:9670` `signSealXField` | se coloca con el dedo | HelpText parcial (WinUI) |
| Idioma del sello | `ayuda.sello.idioma` | `W/SettingsPage.xaml:232` `SealLanguageCombo` | `Q/main.qml:14956` `settingsSealLanguageCombo` | `A/dialog_preferences.xml:111` `prefSealLanguage` | Sí en las tres; el «?» es opcional |

## Certificado

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Importar P12/PFX | `ayuda.certificado.importar_p12` | `W/SignPage.xaml:416`; `W/CertificatesPage.xaml:125` | `Q/main.qml:11109` «Importar certificado» y diálogo `:7552` | `A/activity_main.xml:189` `selectCertificateFileButton` | ToolTip (WinUI); `certificate_helper` (Android) |
| DNIe o tarjeta | `ayuda.certificado.tarjeta` | `W/CertificatesPage.xaml:136` | `Q/main.qml:11268` «Usar DNIe o tarjeta» | no aparece | Bloque `SigningMethodsHelp` (WinUI) |
| Tipo de certificado | `ayuda.certificado.tipo` | `W/CertificatesPage.xaml:167` filtros | `Q/main.qml:14615` `certTypeFisica` y siguientes | `A/section_certificate.xml:47` `certificateKindFilter` | Ninguna |
| Módulos PKCS#11 | `ayuda.pkcs11.modulos` | no aparece | `Q/TokenSettingsPanel.qml:134` `enableCheck` | no aparece | `diagnose_help` |
| DNIe por NFC | `ayuda.dnie_nfc` | no aparece | no aparece | `A/activity_main.xml:198` `selectDnieNfcButton` | `dnie_hint_*`; el «?» es opcional |
| Cierre del certificado | `ayuda.sesion_certificado` | no aparece | no aparece | `A/dialog_preferences.xml:142` `prefSessionTimeout` | Ninguna |

## Verificar

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Original de una firma separada | `ayuda.verificar.original` | `W/VerifyPage.xaml:131` | `Q/main.qml:11833` «Seleccionar original…» | `A/activity_main.xml:141` `selectOriginalDocumentButton` | HelpText (WinUI); `original_document_helper` (Android) |
| Integridad | `ayuda.verificar.integridad` | `W/VerifyPage.xaml:251` `IntegritySummary` | `Q/main.qml:12380` fila «Integridad» | `AJ/ui/VerificationCard.kt` | Resultado en lenguaje claro (Android) |
| Confianza | `ayuda.verificar.confianza` | `W/VerifyPage.xaml:259` `TrustResult` | `Q/main.qml:12382` fila «Confianza» | `AJ/ui/VerificationCard.kt` | Resultado en lenguaje claro (Android) |
| Revocación | `ayuda.verificar.revocacion` | sección «Detalles» de `W/VerifyPage.xaml` | `Q/main.qml:6015` «Revocación online» | `A/section_certificate.xml:108` `checkCertificateOnlineButton` | `revocation_scope` (Android) |
| Cobertura | `ayuda.verificar.cobertura` | `W/VerifyPage.xaml:265` `FormatSummary` | `Q/main.qml:12367` «Cobertura:» | `AJ/ui/VerificationCard.kt` | Ninguna |
| Informe de verificación | `ayuda.verificar.informe` | `W/VerifyPage.xaml:183` y `:189` | `Q/main.qml:11869` exportar | `A/activity_main.xml:596` y `:604` | Ninguna |

## Huellas de ficheros

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Huella (crear y comprobar) | `ayuda.huella` | `W/HashPage.xaml:94` y `:102` | `Q/main.qml:12088` «Crear huella» | `A/section_tools.xml:159` `createHashButton` | Texto de sección (Qt `:11938`, Android `hash_helper`) |
| Huella de una carpeta | `ayuda.huella.carpeta` | `W/HashPage.xaml:122` y `:225` | `Q/main.qml:11967` y `:12070` «Recursivo» | no aparece | HelpText de subcarpetas (WinUI) |
| Algoritmo | `ayuda.huella.algoritmo` | `W/HashPage.xaml:202` `HashAlgorithmCombo` | `Q/main.qml:12031`; preferencias `:14242` | `A/section_tools.xml:135` `hashAlgorithm` | ToolTip solo en preferencias (Qt) |
| Formato de la huella | `ayuda.huella.formato` | `W/HashPage.xaml:212` `HashFormatCombo` | `Q/main.qml:12049` | `A/section_tools.xml:151` `hashFormat` | Ninguna |

## Proteger

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Proteger | `ayuda.proteger` | `W/ProtectPage.xaml:278` `ProtectButton` | `Q/main.qml:13133` | `A/section_tools.xml:311` `protectButton` | `protect_helper` (Android) |
| Desproteger | `ayuda.desproteger` | `W/ProtectPage.xaml:400` `UnprotectButton` | `Q/main.qml:13227` `unprotectCard` | `A/section_tools.xml:336` `unprotectButton` | `unprotect_helper` (Android) |
| Destinatarios | `ayuda.proteger.destinatarios` | `W/ProtectPage.xaml:186` `RecipientsList` | `Q/main.qml:13051` `protectionRecipientsColumn` | `A/section_tools.xml:231` `addRecipientButton` | Texto visible (WinUI) |
| Nivel de protección | `ayuda.proteger.nivel` | `W/ProtectPage.xaml:136` | `Q/main.qml:12932` «Perfil» | no aparece | Ninguna |
| Contenedor | `ayuda.proteger.contenedor` | `W/ProtectPage.xaml:144` `ContainerCombo` | `Q/main.qml:12950` «Contenedor» | `A/section_tools.xml:215` `protectionContainer` | Texto (Qt `:12974`) |
| Clave compartida | `ayuda.proteger.clave` | InfoBar `W/ProtectPage.xaml:161` | `Q/main.qml:13008` | `A/section_tools.xml:269` `protectKey` | `protect_key_helper` (Android) |
| Proteger y firmar | `ayuda.proteger.firmar` | `W/ProtectPage.xaml:250` | `Q/main.qml:13170` | `A/section_tools.xml:319` `protectSignButton` | HelpText técnico (WinUI) |

## Preferencias

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Sello de tiempo | `ayuda.sellado_tiempo` | `W/SettingsPage.xaml:201` `TsaEnabledCheckBox` | `Q/main.qml:15439` `tsaEnabledSwitch` | `A/activity_main.xml:414` `tsaEnabled`; `A/dialog_preferences.xml:59` `prefTsaEnabled` | ToolTip en el título (Qt) |
| Servidor TSA | `ayuda.tsa.servidor` | `W/SettingsPage.xaml:207` `TsaUrlTextBox` | `Q/main.qml:15460` `tsaCombo` | `A/activity_main.xml:430` `tsaUrl`; `A/dialog_preferences.xml:75` `prefTsaUrl` | `tsa_helper` (Android) |
| Formato automático por tipo | `ayuda.formato.automatico_por_tipo` | no aparece | `Q/main.qml:15279` `autoFormatPdfCombo` y siguientes | no aparece | Texto de sección con jerga (`:15265`) |
| Subfiltro PAdES | `ayuda.pades.subfiltro` | no aparece | `Q/main.qml:15019` `settingsPadesSubFilterCombo` | no aparece | ToolTip |
| Proxy | `ayuda.proxy` | `W/SettingsPage.xaml:255` | `Q/main.qml:15527` `proxyEnabledSwitch` | no aparece | HelpText parcial |
| Servicio local para otros programas | `ayuda.rest_local` | `W/SettingsPage.xaml:427` `RestTokenText` | `Q/main.qml:15858` `restPortField` | no aparece | ToolTip en el título (Qt) |
| Confianza local con el navegador | `ayuda.confianza_local` | no aparece | `Q/main.qml:14718` «Reinstalar certificados locales» | no aparece | ToolTip |
| Conectores del navegador | `ayuda.conectores_navegador` | no aparece | `Q/main.qml:14710` «Reinstalar conectores de navegadores» | no aparece | ToolTip |

## ENI, Facturae y Veri*Factu

| Concepto | Clave | WinUI | Qt | Android | Ayuda actual |
|---|---|---|---|---|---|
| Documento ENI | `ayuda.eni.documento` | `W/EniPage.xaml.cs:87` | `Q/EniPanel.qml:195` | `A/section_documents.xml:212` `toggleEniButton` | `eni_helper` (Android) |
| Expediente ENI | `ayuda.eni.expediente` | `W/EniPage.xaml.cs:124` | `Q/EniPanel.qml:223` | `A/section_expediente.xml:15` `toggleExpedienteButton` | `expediente_helper` (Android) |
| Metadatos ENI | `ayuda.eni.metadatos` | `W/EniPage.xaml.cs:61` | `Q/EniPanel.qml:207` | `A/section_documents.xml:250` y siguientes | Parcial (órgano) |
| Origen ENI | `ayuda.eni.origen` | `W/EniPage.xaml.cs:99` | `Q/EniPanel.qml:208` `origin` | `A/section_documents.xml:269` `eniOrigin` | Ninguna |
| Estado de elaboración | `ayuda.eni.estado_elaboracion` | `W/EniPage.xaml.cs:182` | `Q/EniPanel.qml:210` `state` | `A/section_documents.xml:284` `eniState`; `A/section_expediente.xml:120` | Ninguna |
| Facturae | `ayuda.facturae` | `W/FacturaePage.xaml:163` | `Q/FacturaePanel.qml:258` | no aparece | `facturae.intro` (Qt) |
| Códigos DIR3 | `ayuda.facturae.dir3` | `W/FacturaePage.xaml:398` a `:415` | campos de `Q/FacturaePanel.qml:277` a `:341` | no aparece | Pista por campo (Qt) |
| Política de firma Facturae | `ayuda.facturae.politica` | no aparece | `Q/main.qml:15075` `settingsFacturaePolicyVersionCombo` | no aparece | ToolTip |
| Rol del firmante Facturae | `ayuda.facturae.rol` | no aparece | `Q/main.qml:15098` | no aparece | ToolTip |
| Veri*Factu | `ayuda.verifactu` | `W/FacturaePage.xaml:755` | `Q/FacturaePanel.qml:384` | `A/section_documents.xml:34` `toggleVerifactuButton` | `verifactu.scope`, `verifactu_helper` |

## Conceptos sin control en ninguna interfaz

Estos conceptos se pidieron, pero hoy no tienen ningún control en Windows,
Linux ni Android. No tienen clave para no dejar textos sin uso. Si en el
futuro se añade el control, hay que escribir su ayuda con el mismo criterio:

- alcance de la contrafirma (todas las firmas o solo las últimas);
- modo XAdES (envuelta, envolvente o separada) y firma CAdES implícita o
  explícita;
- política de firma general, salvo la de Facturae;
- certificar el PDF (MDP);
- zona horaria del sello;
- contraseña de un PDF protegido;
- preferencias de revocación y de anclas de confianza propias.

## Reparto de la fase 2

Número de «?» por pantalla (un «?» por fila de las tablas, contando una sola
vez los selectores con varias opciones):

| Plataforma | Pantalla | «?» |
|---|---|---|
| WinUI | Firmar (`SignPage.xaml`) | 19 |
| WinUI | Verificar (`VerifyPage.xaml`) | 6 |
| WinUI | Configuración (`SettingsPage.xaml`) | 8 |
| WinUI | Certificados (`CertificatesPage.xaml`) | 3 |
| WinUI | Proteger (`ProtectPage.xaml`) | 7 |
| WinUI | Huellas (`HashPage.xaml`) | 4 |
| WinUI | ENI (`EniPage.xaml.cs`) | 5 |
| WinUI | Facturae y Veri*Factu (`FacturaePage.xaml`) | 3 |
| Qt | Firmar (`main.qml`) | 19 |
| Qt | Verificar y huellas (`main.qml`) | 10 |
| Qt | Preferencias (`main.qml` y `TokenSettingsPanel.qml`) | 21 |
| Qt | Certificados (`main.qml`) | 2 |
| Qt | Proteger (`main.qml`) | 7 |
| Qt | ENI (`EniPanel.qml`) | 5 |
| Qt | Facturae y Veri*Factu (`FacturaePanel.qml`) | 3 |
| Android | Firmar o verificar (`activity_main.xml`) | 9 |
| Android | Resultado de la verificación (`VerificationCard.kt`) | 3 |
| Android | Editor del sello (`SealEditorDialog.kt`) | 7 |
| Android | Preferencias (`dialog_preferences.xml`) | 5 |
| Android | Otras herramientas (`section_tools.xml`) | 10 |
| Android | Documentos y expediente | 6 |
| Android | Certificado (`section_certificate.xml`) | 2 |

En Preferencias de Qt muchas filas ya tienen ToolTip. Ahí el «?» sirve para
que la ayuda esté también al alcance del teclado y de las pantallas táctiles.
