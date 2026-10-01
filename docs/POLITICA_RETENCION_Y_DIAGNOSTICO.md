<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Política técnica de retención y diagnóstico

Vigente desde: 29 de julio de 2026.

Esta política fija el comportamiento predeterminado de GrxFirma para los
artefactos locales que crea y controla la propia aplicación. No sustituye las
decisiones jurídicas, contractuales o de archivo que deba aprobar el
responsable de cada despliegue.

## Retención predeterminada

| Artefacto | Ubicación habitual | Límite temporal | Límite adicional | Borrado |
|---|---|---:|---:|---|
| Incidencias Qt/QML | directorio privado `incidents` del estado de la app | 30 días | 50 incidencias | antes de preparar una incidencia nueva |
| Incidencias `afirma://` | `~/.local/state/grxfirma/incidents` o equivalente | 30 días | 50 incidencias | antes de persistir un fallo nuevo |
| Log Qt/QML | directorio privado `logs` del estado de la app | 30 días | activo + una rotación de 2 MiB cada una | al iniciar la GUI y al rotar |
| Auditoría JSONL | `~/.local/share/grxfirma/audit.jsonl` o equivalente | 90 días | activo + una rotación de 10 MiB cada una | antes de registrar una evidencia nueva |

Una incidencia cuenta como un grupo: JSON principal, resumen, manifiesto,
previsualización y cola de log comparten la misma fecha y se eliminan juntos.
Si se supera el máximo, se conservan los grupos más recientes.

## Garantías de borrado

- Solo se reconocen nombres generados por GrxFirma.
- No se siguen ni se eliminan enlaces simbólicos o puntos de reanálisis.
- Un fichero ajeno colocado en el mismo directorio queda intacto.
- Los directorios y ficheros se mantienen privados para el usuario.
- Si un artefacto reconocido deja de ser regular o no puede retirarse, la
  aplicación falla cerrada para esa nueva persistencia y no borra otra ruta.
- El borrado es local. Una copia que el usuario haya exportado o enviado a un
  canal de soporte queda bajo la política del destino correspondiente.

## Paquetes de producción

Los empaquetadores oficiales compilan los binarios Go con la etiqueta
`production`. En esos binarios:

- `GRXFIRMA_DEBUG` no activa trazas `DEBUG`;
- `GRXFIRMA_LOG_LEVEL=DEBUG` se limita a `INFO`;
- las sondas y ficheros activados por `GRXFIRMA_DEBUG_ENABLED` o
  `GRXFIRMA_DESKTOP_DEBUG` no se ejecutan;
- los instaladores no crean lanzadores de depuración.

El log operativo Qt/QML sigue disponible, pero aplica saneado final, permisos
privados, límite de tamaño y la retención de esta política. No es un volcado de
payloads IPC ni de identidad.

## Desarrollo y soporte controlado

Un binario compilado desde el código fuente sin la etiqueta `production` puede
admitir `DEBUG` para reproducir una incidencia. Debe hacerse únicamente:

1. con datos sintéticos o expresamente autorizados;
2. durante el tiempo imprescindible;
3. en un directorio privado y con el saneado común activo;
4. sin copiar documentos, certificados, PIN, contraseñas o payloads;
5. retirando el fichero al cerrar la investigación.

`GRXFIRMA_LOG_FILE` permite que un desarrollador elija una ruta arbitraria.
La aplicación no elimina automáticamente rutas elegidas por un operador,
porque hacerlo podría borrar un fichero que no controla. Quien configure esa
salida debe aplicar acceso, rotación y borrado equivalentes.

## Responsabilidad del despliegue

La retención técnica anterior es el máximo local predeterminado, no una
obligación de conservar datos durante todo ese plazo. El organismo responsable
debe decidir y documentar, según finalidad y base jurídica:

- si necesita una retención menor;
- si una evidencia debe exportarse a un archivo regulado antes de caducar;
- quién puede acceder a incidencias enviadas;
- cuánto conserva el endpoint de soporte y cómo atiende un borrado;
- y si el tratamiento exige un análisis de impacto o medidas adicionales.

Mientras no exista un endpoint aprobado, GrxFirma no envía incidencias por
sí sola: muestra una previsualización y exige una acción y consentimiento
explícitos para cada envío HTTPS.

## Evidencia técnica

La política queda cubierta por pruebas que verifican:

- caducidad y límite de cantidad;
- borrado conjunto de los ficheros de una incidencia;
- conservación de artefactos recientes y ficheros ajenos;
- rechazo de enlaces simbólicos;
- rotación privada de auditoría y logs;
- y anulación de `DEBUG` al compilar con `-tags production`.

Los gates de empaquetado deben comprobar además que todas las builds públicas
incluyen la etiqueta y que no contienen lanzadores de depuración.
