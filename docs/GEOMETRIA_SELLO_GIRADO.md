<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Geometría del sello visible

`signSealX/Y/W/H` en Qt y `VisibleSealX/Y/Width/HeightPercent` en WinUI
describen siempre la tarjeta **antes de girarla**. X e Y sitúan su esquina
inferior izquierda; W y H son el ancho y el alto que eligió la persona. Las
interfaces convierten los porcentajes a puntos PDF usando las dimensiones de
la página visible. El IPC transmite esos puntos en `visibleSealRectX/Y/W/H` y
marca las coordenadas relativas al origen de `CropBox`; el motor suma dicho
origen al colocar la anotación.

La única conversión de tarjeta a rectángulo de anotación PDF está en
`cajaSelloGirado`: mantiene el centro y calcula la caja envolvente de la
tarjeta girada. A 90° y 270° intercambia ancho y alto; para cualquier otro
ángulo usa `|w cos θ| + |h sin θ|` y `|w sin θ| + |h cos θ|`. Si la caja cabe en
la página pero cruza un borde, se desplaza íntegra. Si supera el tamaño de la
página, la firma se detiene con una indicación para reducir la tarjeta.

La composición se genera siempre con W × H y después se gira sin cambiar su
escala. El PNG resultante ocupa exactamente la caja PDF; las esquinas libres
quedan transparentes. La firma aplica la opacidad una sola vez al conjunto;
`seal_preview` la aplica al PNG compuesto. Los editores Qt y WinUI piden para
su tarjeta la composición a 0° y giran visualmente el recuadro completo, de
modo que la previsualización no reciba un segundo giro. Al arrastrar el
tirador, el tamaño que cambia sigue siendo W × H antes del giro.
