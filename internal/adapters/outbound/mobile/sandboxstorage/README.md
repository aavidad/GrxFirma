<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# sandbox-storage

Estado: implementacion base operativa para Fase 6.

Puerto implementado:
- `ports.TempFileStore`

Dependencias de plataforma:
- almacenamiento sandbox de Android/iOS
- capa nativa para resolver rutas seguras y políticas de expiración

Implementacion actual:
- el adaptador usa un directorio base inyectado por la capa nativa, ya dentro del sandbox de la app
- crea temporales con `os.CreateTemp` y restringe el borrado a rutas hijas de ese directorio
- no añade dependencias externas ni SDKs de plataforma al nucleo Go

Limitaciones:
- la resolucion del directorio sandbox correcto sigue siendo responsabilidad de Android/iOS
- la expiracion automatica y el borrado seguro dependen de las politicas reales del SO
- si se necesitan atributos especiales de proteccion de fichero, deben aplicarse en el bridge nativo
