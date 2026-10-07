<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Accesibilidad de GrxFirma: revisión técnica WCAG 2.1 AA

Fecha: 2026-10-07.
Versión revisada: 0.0.124, rama `fix/accesibilidad-wcag`.
Autoría: Alberto Avidad Fernández
Estado: autoevaluación técnica. No sustituye a la revisión formal con
personas y lectores de pantalla, que se describe en el apartado 7.

## 1. Para qué sirve

Este documento recoge cómo se ha revisado la accesibilidad de las tres
aplicaciones de GrxFirma, qué resultado ha dado cada criterio, qué se ha
corregido y qué queda. Sirve de base para la revisión humana y para redactar la
declaración de accesibilidad (borrador en el apartado 8).

La norma de referencia es la que exige el Real Decreto 1112/2018 a las
aplicaciones del sector público: UNE-EN 301 549 (cláusula 11, software), que
para estos criterios equivale a WCAG 2.1 nivel AA. Los objetivos táctiles se han
medido también con el criterio 2.5.8 de WCAG 2.2 (24 × 24 px), como pide la
guía de usabilidad del proyecto.

La lista de comprobación anterior, más general, sigue en
[CHECKLIST_ACCESIBILIDAD.md](CHECKLIST_ACCESIBILIDAD.md).

## 2. Alcance

| Aplicación | Código | Plataforma probada |
|---|---|---|
| Escritorio Windows (WinUI 3) | `cmd/gui-winui` | Windows 10 (VM `win10`), versión 0.0.124 compilada desde esta rama |
| Escritorio Qt/QML (Linux y la variante Qt de Windows) | `cmd/gui-qml` | Linux, Qt 6.10, bajo Xvfb con el gestor de ventanas metacity |
| Android (Views) | `mobile/android` | Emulador `medium_phone` (Android 16), compilación `verification` |

Quedan fuera:

- la extensión de navegador y la web del proyecto (`docs/sitio`);
- los informes HTML de verificación y los documentos que firma la persona,
  porque su contenido es suyo;
- la capa del sello para portales (`afirma://`) del Qt, que otra rama está
  cambiando ahora mismo. Sus hallazgos están en el apartado 6.

## 3. Método

Para cada aplicación se revisó el código criterio por criterio (XAML y C#, QML y
JavaScript, layouts y Kotlin) y después se probó la aplicación en marcha. Los
contrastes se calcularon con la fórmula de WCAG sobre los colores reales de
cada tema: 2 temas en WinUI más alto contraste, 14 temas en Qt y claro y oscuro
en Android.

Pruebas en ejecución:

- Qt: árbol AT-SPI en vivo bajo Xvfb con `scripts/accessibility/run-atspi-audit.sh`.
  La auditoría recorre todas las secciones, comprueba que cada campo tiene
  nombre propio (no vale el texto de ejemplo), que ningún botón se anuncia solo
  con un símbolo, que al arrancar hay un control con el foco y que Tab lo mueve.
  También se probó la aplicación con escala de pantalla al 200 %
  (`QT_SCALE_FACTOR=2`) y con la ventana reducida a 640 px.
- Android: volcados de `uiautomator` de toda la pantalla principal, letra al
  200 %, modo oscuro y la batería instrumentada completa (64 pruebas, 6 de
  ellas nuevas de accesibilidad: anillo de foco, contraste real del icono ⋮ en
  los dos temas, lienzo del sello, encabezados, foco en el primer error y grupo
  de certificados).
- WinUI: inventario con UI Automation de las diez secciones, antes y
  después de las correcciones, con
  `scripts/windows-qa/Invoke-GrxFirmaUiaAccessibilityAudit.ps1`: nombres,
  tamaños, encabezados, campos obligatorios y capturas.

Limitaciones de esta revisión:

- No se han usado lectores de pantalla reales. Los nombres, roles y avisos se
  han comprobado en el árbol de accesibilidad, que es lo que leen NVDA, JAWS,
  Narrador, Orca y TalkBack, pero no cómo los pronuncia cada uno.
- La sesión de la VM Windows se bloquea sola. UI Automation sigue funcionando
  con la sesión bloqueada, pero Windows no deja enviar teclas, así que el orden
  real de Tab en WinUI se ha comprobado en el código y en los contratos, no
  pulsando la tecla.
- El alto contraste de Windows se ha revisado en el código (recursos
  `HighContrast` con colores `SystemColor*`), sin activarlo en la VM.

## 4. Resultado por criterio

Estado después de las correcciones de esta rama. «Cumple» significa que no se
ha encontrado ningún fallo con los métodos del apartado 3; la confirmación
final corresponde a la revisión humana.

| Criterio | WinUI | Qt | Android |
|---|---|---|---|
| 1.1.1 Contenido no textual | Cumple | Cumple | Cumple |
| 1.3.1 Información y relaciones | Cumple | Cumple | Cumple |
| 1.4.1 Uso del color | Cumple | Cumple | Cumple |
| 1.4.3 Contraste del texto | Cumple | Cumple | Cumple |
| 1.4.4 Cambio de tamaño del texto | Cumple (por código; sin probar en la VM) | Parcial | Cumple |
| 1.4.10 Reajuste | Cumple | Cumple | Cumple |
| 1.4.11 Contraste de componentes | Cumple (alto contraste sin probar en vivo) | Cumple | Cumple |
| 2.1.1 Teclado | Cumple (por código) | Cumple | Cumple |
| 2.1.2 Sin trampas para el foco | Cumple | Cumple | Cumple |
| 2.4.3 Orden del foco | Cumple (por código) | Cumple | Cumple |
| 2.4.7 Foco visible | Cumple | Cumple | Cumple |
| 2.5.8 Tamaño del objetivo (WCAG 2.2) | Cumple, con la excepción de control equivalente | Cumple | Cumple |
| 3.3.1 Identificación de errores | Cumple | Cumple | Cumple |
| 3.3.2 Etiquetas o instrucciones | Cumple | Cumple | Cumple |
| 3.3.3 Sugerencias ante errores | Cumple | Cumple | Cumple |
| 4.1.2 Nombre, función y valor | Cumple | Cumple | Cumple |
| 4.1.3 Mensajes de estado | Parcial | Cumple | Cumple |

Notas a la tabla:

- 1.4.4 en Qt: con la escala de pantalla de Linux o Windows al 200 % la
  interfaz crece entera y se reorganiza bien. Lo que no sigue es el ajuste de
  «solo texto más grande» del sistema (factor de texto de GNOME, «Aumentar el
  tamaño del texto» de Windows), porque los tamaños de letra están fijados en
  píxeles. Pendiente P-Q1.
- 2.5.8 en WinUI: los tiradores del editor del sello se ven a escala del
  PDF y bajan de 24 px en ventanas estrechas. Todo lo que hacen se puede hacer
  con los campos numéricos de posición, tamaño y giro, que es la excepción que
  admite el criterio. Pendiente P-W1. El inventario UIA marcó otros dos
  controles pequeños, pero estaban medio tapados por el borde de la zona
  visible al hacer la medida; en el XAML miden 44 px de alto.
- 4.1.3 en WinUI: el avance de la firma por lotes no se anuncia mientras
  dura; sí el resultado final. Pendiente P-W2.

Contrastes más bajos que quedan, todos por encima del umbral:

- WinUI: «No comprobado» en tema claro (#5E5E5E sobre la capa #FBFBFC): 6,27:1.
- Qt: texto secundario en el tema Minimalista Luz: 5,05:1. El acento y los
  colores de veredicto se ajustan ahora a cada fondo con
  `ThemeContrast.js`.
- Android: borde de los campos en tema claro (#707974 sobre #F8FAF8): 4,28:1
  (umbral 3:1). El icono ⋮ en tema oscuro pasa de 1,28:1 a 7,93:1.

## 5. Qué se ha corregido

### WinUI

- Colores de estado (éxito, fallo, aviso y «No comprobado») definidos por tema,
  con variante de alto contraste, y leídos siempre según el tema real de la
  aplicación. «No comprobado» estaba a 3,32:1 en tema claro.
- Los textos que cambian (resultados de Proteger, errores de campo, mensajes de
  Configuración, Firmar, Facturae, Diagnóstico y Certificados) se anuncian al
  lector de pantalla con un ayudante común (`LiveAnnouncer`). Antes solo se
  anunciaban algunos.
- Facturae: el aviso de resultado se vuelve a anunciar en cada intento, aparece
  bajo el botón «Crear», y ante un error de datos el foco va al primer campo
  obligatorio vacío. Los 21 campos con «*» están marcados como obligatorios y
  hay una leyenda que explica el asterisco.
- Al marcar un error en un campo ya no se borra su ayuda: se lee el error y
  después la ayuda original.
- Orden del foco: el tirador de giro y «Otras formas de firmar» ya no quedan al
  final de la página. En la vista del sello para portales, el primer Tab va a
  las acciones, el título es encabezado de nivel 1 y los campos numéricos se
  pliegan sin desplazamiento horizontal.
- El tirador de giro anuncia los grados al girar.
- El rectángulo de «Dibujar área» tiene doble borde y el papel del editor usa el
  color de ventana en alto contraste. Antes quedaban a 1,8:1 y 1,46:1.
- Cada certificado de la lista se anuncia con su estado y su caducidad, además
  del nombre. Nombre y estado ya no se cortan con puntos suspensivos.
- El logotipo de «Acerca de» ya no se lee dos veces y el círculo de estado de
  Diagnóstico crece con el texto.

### Qt

- Al abrir la aplicación el foco está en la sección activa. Antes Tab no hacía
  nada hasta hacer clic dentro de la ventana.
- Todos los campos y desplegables tienen el nombre de su rótulo visible. Antes
  muchos se anunciaban por su valor («Compat») o por el texto de ejemplo
  («Granada» en lugar de «Ubicación»). Los campos que solo tenían texto de
  ejemplo tienen ahora un rótulo visible.
- Los diez botones «▶/▼» de los detalles se abren con el teclado (dos no
  respondían), se anuncian «Mostrar detalles» u «Ocultar detalles» y dicen si
  están desplegados.
- Un campo auxiliar invisible del portapapeles recibía el foco con Tab y se lo
  quedaba tras «Copiar». Ya no entra en el orden de tabulación y devuelve el
  foco.
- Colores del veredicto de verificación, de la insignia del lote y de los
  textos de acento ajustados a cada fondo. En los temas claros el veredicto
  llegaba a 1,77:1.
- Desplegables, casillas, campos numéricos y deslizadores con borde de 3:1 en
  todos los temas (antes entre 1,02:1 y 2:1).
- La tarjeta de certificado distingue foco y selección y anuncia si está
  elegida. La navegación anuncia la sección activa y no lee el icono.
- 25 títulos marcados como encabezado; barra de estado, resultado de
  verificación y errores de ENI anunciados como avisos.
- Ventana con tamaño mínimo de 640 × 480 y diálogos que caben en ella.
- La barra de estado ya no muestra errores técnicos de conexión
  («Error IPC: QLocalSocket…»): dice qué hacer y el detalle queda en el
  registro.
- La ayuda «?» se desplaza con las flechas y con Re Pág y Av Pág.

### Android

- La barra superior tiñe el icono ⋮ con el color de primer plano del tema
  (antes 1,28:1 en modo oscuro).
- Anillo de foco de 2 dp en botones, casillas, interruptores y botones de
  opción, visible solo con teclado o mando. Antes el foco era un velo del 10 %.
- La tarjeta de verificación muestra el resultado de cada comprobación junto a
  su «?» («Integridad: válida», «Confianza: no comprobada», «Cobertura:
  Documento completo»). Antes solo se veían las etiquetas.
- El editor del sello dice dónde está el sello (página, posición, tamaño y
  giro), lo anuncia tras cada ajuste, responde a las flechas y a + y −, y
  ofrece acciones de TalkBack para moverlo, redimensionarlo y girarlo.
- «Procesando la operación…» se anuncia al empezar.
- Los errores de la clave de protección, la dirección del sello de tiempo, ENI
  y expediente se marcan en su campo y llevan el foco allí. «Aplicar a todas
  las páginas» explica por qué no se puede con más de 128 páginas.
- La ayuda de «Páginas con sello» describe las opciones que hay en Android.
- Lista de certificados abiertos anunciada como grupo («1 de 3»), dos títulos
  más marcados como encabezado, tarjeta de resultado sin parada de foco vacía y
  logotipo de «Acerca de» fuera del lector.
- Mensajes de validación de ENI con trato de usted, como el resto de la
  aplicación.
- Paleta completa de Material con los verdes del proyecto, medida en los dos
  temas.

### Pruebas que evitan regresiones

- WinUI: `cmd/gui-winui/tests/test_wcag_audit_contract.py` y ajustes en los
  contratos de orden de foco y nombres de lista.
- Qt: `cmd/gui-qml/tests/test_accessibility_wcag_contract.py`,
  `cmd/gui-qml/tests/qml/tst_accessiblecontrols.qml` y la auditoría AT-SPI
  ampliada.
- Android: `AccessibilityContractTest`, `SealAdjustmentTest`, una prueba más en
  `VerificationCardTest` y `ToolsPolicyTest`, y la prueba instrumentada
  `AccessibilityUiTest`.

## 6. Qué queda pendiente

| Id | Aplicación | Qué falta | Prioridad |
|---|---|---|---|
| P-Q1 | Qt | Seguir el tamaño de texto del sistema o añadir un ajuste «Tamaño del texto» (100–200 %). Hoy hay 231 tamaños fijos en píxeles. | Alta |
| P-Q2 | Qt | Capa del sello para portales: la barra de estado sigue alcanzable con Tab bajo la capa, sus desplegables y campos numéricos no tienen el borde de 3:1 y el título no es encabezado. Se deja para después de la rama del selector de portales. | Media |
| P-Q3 | Qt | 116 textos traducidos empiezan por un emoji o símbolo (⌛, ✍, 🔑) que el lector pronuncia. | Baja |
| P-W1 | WinUI | Tiradores del sello de al menos 24 px a cualquier escala. | Baja |
| P-W2 | WinUI | Anunciar el avance de la firma por lotes cada pocos documentos. | Media |
| P-W3 | WinUI | Los colores puestos desde código no se repintan si se cambia de tema con la página abierta. | Baja |
| P-A1 | Android | Activar `AccessibilityChecks` de Espresso en las pruebas instrumentadas. Exige nuevas dependencias verificadas y regenerar los ficheros de bloqueo. | Media |
| P-A2 | Android | Comprobar en Android 15 o posterior que el modo de borde a borde no tapa los últimos botones con la barra de gestos. | Media |

## 7. Lo que necesita revisión humana

La revisión formal debe hacerla una persona con experiencia en accesibilidad y,
si es posible, con usuarios que usen estas tecnologías a diario.

1. Lectores de pantalla:
   - Windows (WinUI y Qt para Windows): NVDA y JAWS, más Narrador;
   - Linux (Qt): Orca;
   - Android: TalkBack, también con acceso por botón (Switch Access).
2. Recorrido completo solo con teclado en WinUI, con la sesión desbloqueada:
   orden de Tab, foco visible y salida de cada diálogo. El script de UI
   Automation lo registra si la sesión no está bloqueada.
3. Alto contraste de Windows (en Windows 10, los temas Negro y Blanco; en
   Windows 11, Acuático y Desierto) en Firmar, editor del sello, Certificados
   y Diagnóstico.
4. Ampliador de pantalla y texto al 200 % en las tres plataformas, y
   «Aumentar el tamaño del texto» de Windows en WinUI.
5. Accessibility Scanner de Google en Android y teclado físico conectado al
   móvil.
6. Que los mensajes de error y de estado se entienden a la primera, con
   personas que no conozcan la firma electrónica.
7. Los pasos críticos de la lista anterior: confianza en un dominio, elección de
   certificado y firma desde un portal (`afirma://`).

## 8. Borrador de declaración de accesibilidad

Modelo del artículo 15 del Real Decreto 1112/2018 y de la Decisión de Ejecución
(UE) 2018/1523. Los datos entre corchetes los completa el organismo que publique
o distribuya la aplicación.

> **Declaración de accesibilidad**
>
> [Nombre del organismo] se ha comprometido a hacer accesible su aplicación
> GrxFirma, de conformidad con el Real Decreto 1112/2018, de 7 de septiembre,
> sobre accesibilidad de los sitios web y aplicaciones para dispositivos
> móviles del sector público.
>
> La presente declaración de accesibilidad se aplica a GrxFirma [versión] para
> Windows, Linux y Android.
>
> **Situación de cumplimiento.** Esta aplicación es parcialmente conforme con
> el RD 1112/2018 debido a las excepciones y a la falta de conformidad de los
> aspectos que se indican a continuación.
>
> **Contenido no accesible.** Falta de conformidad con el RD 1112/2018:
>
> - En la versión Qt (Linux), el texto no aumenta con el ajuste de tamaño de
>   texto del sistema; sí con la escala de pantalla (criterio 1.4.4).
> - En la versión Qt, la pantalla de colocación del sello para portales tiene
>   controles con poco contraste de borde y la barra de estado queda alcanzable
>   con el tabulador bajo esa pantalla (criterios 1.4.11 y 2.4.3).
> - En Windows, el avance de la firma por lotes no se anuncia al lector de
>   pantalla hasta que termina (criterio 4.1.3).
>
> Carga desproporcionada: no aplica.
>
> Contenido que no entra en el ámbito de la legislación aplicable: los
> documentos que la persona usuaria firma o verifica, que son de su propiedad.
>
> **Preparación de la presente declaración.** Esta declaración se preparó el
> [fecha]. El método empleado es una autoevaluación técnica del desarrollador
> (revisión del código y pruebas automáticas con las interfaces de
> accesibilidad de cada sistema), descrita en `docs/ACCESIBILIDAD.md`. [Añadir
> la revisión con lectores de pantalla cuando se haga.] Última revisión:
> [fecha].
>
> **Observaciones y datos de contacto.** Puede realizar comunicaciones sobre
> requisitos de accesibilidad (artículo 10.2.a del RD 1112/2018), como
> informar de cualquier posible incumplimiento o transmitir otras dificultades
> de acceso al contenido, a través de [canal del organismo]. También puede
> presentar una solicitud de información accesible, queja o reclamación por
> [sede electrónica o registro del organismo].
>
> **Procedimiento de aplicación.** Si una solicitud de información accesible
> o queja ha sido desestimada, no está de acuerdo con la decisión adoptada o
> la respuesta no cumple los requisitos del artículo 12.5 del RD 1112/2018,
> puede iniciar una reclamación ante [unidad responsable de accesibilidad del
> organismo].

## 9. Cómo repetir las pruebas

```bash
# Contratos
python3 -m unittest discover -s cmd/gui-qml/tests -p 'test_*.py'
(cd cmd/gui-winui/tests && python3 -m unittest discover -p 'test_*.py')
QT_QPA_PLATFORM=offscreen /usr/lib/qt6/bin/qmltestrunner -input cmd/gui-qml/tests/qml

# Árbol AT-SPI del Qt (compila la aplicación; usa metacity y xdotool si están instalados)
TMPDIR=/tmp scripts/accessibility/run-atspi-audit.sh

# Android: validación del proyecto y pruebas instrumentadas de accesibilidad
bash scripts/mobile/android/validate-project.sh
adb shell am instrument -w -e class io.github.aavidad.grxfirma.android.AccessibilityUiTest \
  io.github.aavidad.grxfirma.verification.debug.test/androidx.test.runner.AndroidJUnitRunner
```

En Windows, el inventario de UI Automation se lanza como tarea programada
interactiva (véase `scripts/windows-qa/README.md`):

```powershell
.\scripts\windows-qa\Invoke-GrxFirmaUiaAccessibilityAudit.ps1 `
  -OutputFile C:\QA\a11y\uia.txt -ScreenshotDirectory C:\QA\a11y
```
