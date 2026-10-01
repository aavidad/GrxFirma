<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Control accesible de GrxFirma en Linux

`atspi_control.py` permite inspeccionar y accionar la interfaz Qt/QML mediante
AT-SPI. Está pensado para campañas de QA en una máquina virtual con sesión
gráfica iniciada. No forma parte del paquete de usuario.

El controlador trabaja sobre el árbol accesible de la aplicación y evita usar
coordenadas para botones, campos y barras de desplazamiento. Para mover o
redimensionar el sello dentro del lienzo PDF sigue siendo necesaria una
herramienta de entrada gráfica, porque ese gesto no está expuesto como acción
AT-SPI.

## Salvaguardas

- La aplicación objetivo se selecciona por nombre y debe estar visible en la
  sesión del mismo usuario.
- Los valores de los campos de texto se reciben por la entrada estándar. No se
  pasan contraseñas, PIN ni secretos en argumentos.
- La herramienta no toma capturas, no enumera ficheros personales y no guarda
  el contenido introducido.
- `--include-offscreen` y `--include-invisible` amplían deliberadamente el árbol
  consultado. Deben combinarse con nombre, rol e índice comprobados en una
  ejecución previa de `list`.
- Las pruebas de firma deben usar documentos sintéticos y certificados
  oficiales de ensayo. No se debe automatizar con certificados personales.

## Requisitos

- Python 3.
- bindings AT-SPI para Python (`python3-pyatspi` en Debian/Ubuntu).
- una sesión gráfica con el bus de accesibilidad disponible.
- `NO_AT_BRIDGE=0`.

En una sesión SSH hay que reutilizar las variables de la sesión gráfica real;
los valores siguientes son solo un ejemplo y no deben copiarse sin
comprobarlos:

```bash
export DISPLAY=:0
export XDG_RUNTIME_DIR=/run/user/"$(id -u)"
export DBUS_SESSION_BUS_ADDRESS=unix:path="$XDG_RUNTIME_DIR/bus"
export NO_AT_BRIDGE=0
```

## Uso

Listar controles visibles:

```bash
python3 scripts/linux-qa/atspi_control.py \
  --application GrxFirma \
  list
```

Incluir controles que están fuera del área desplazada:

```bash
python3 scripts/linux-qa/atspi_control.py \
  --application GrxFirma \
  --include-offscreen \
  list
```

Activar un botón por nombre:

```bash
python3 scripts/linux-qa/atspi_control.py \
  --application GrxFirma \
  activate --name "Firmar"
```

Ejecutar cinco veces la acción `Increase` de la segunda barra de
desplazamiento accesible sin nombre:

```bash
python3 scripts/linux-qa/atspi_control.py \
  --application GrxFirma \
  --include-offscreen \
  activate --name "" --role "scroll bar" --index 1 \
  --action Increase --repeat 5
```

Escribir un valor no secreto sin exponerlo en la línea de órdenes:

```bash
printf '%s' "$BUSQUEDA_QA" |
  python3 scripts/linux-qa/atspi_control.py \
    --application GrxFirma \
    set-text --name "Buscar certificado" --role "entry"
```

Algunos controles protegidos de Qt, como una contraseña PKCS#12, rechazan
deliberadamente `EditableText.setTextContents`. El controlador lo comunica sin
mostrar el valor. En una campaña desatendida ese campo debe introducirse con
una herramienta de entrada gráfica que lea el secreto por `stdin`, nunca
incluyéndolo en argumentos, variables de entorno, capturas o logs.

La salida de `list` incluye la ruta AT-SPI, el rol, los estados, las acciones y
las dimensiones del control. La ruta y el índice solo son estables durante esa
ejecución; hay que volver a inspeccionar el árbol después de navegar a otra
pantalla.
