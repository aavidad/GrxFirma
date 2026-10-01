<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR-001 — Tecnologia de integracion movil

Estado: Aprobado.  
Fecha: 2026-03-18

## Contexto

GrxFirma necesita ejecutar un nucleo Go comun en Android e iPhone con una interfaz nativa en cada plataforma:

- el nucleo debe seguir siendo Go nativo siempre que sea razonablemente posible,
- mobile no puede quedar como una extension tardia de escritorio,
- los adaptadores de plataforma deben quedar fuera del dominio y de `application`,
- la arquitectura debe ser viable en iPhone, no solo en Android.

La decision pendiente es como exponer el nucleo Go dentro de las aplicaciones moviles.

## Opciones evaluadas

### Opcion A — `gomobile bind` con interfaz nativa por plataforma

Descripcion:

- el nucleo Go se compila como libreria para Android e iOS,
- Android consume el resultado como AAR,
- iOS consume el resultado como framework/artefacto generado,
- la interfaz de usuario sigue siendo nativa por plataforma y llama al nucleo mediante enlaces generados.

Ventajas:

- es la via oficial del ecosistema Go para exponer paquetes Go a Java y Objective-C,
- mantiene el corazon en Go sin meter un segundo runtime de interfaz como requisito de arquitectura,
- encaja con el requisito de separar nucleo y adaptadores,
- permite que codigo nativo implemente interfaces Go y las pase al nucleo, lo que encaja bien con puertos como `SecureStorage`, `BiometricPrompt`, `CapabilityProfileProvider` o `EventPublisher`.

Desventajas:

- los tipos exportados al limite de integracion tienen restricciones claras,
- obliga a disenar una fachada de enlace especifica para mobile,
- la interfaz no es compartida entre Android e iOS; se comparte el nucleo, no la presentacion.

### Opcion B — Flutter + FFI/CGo

Descripcion:

- la interfaz se implementa en Flutter,
- el nucleo Go se expone como libreria nativa consumida desde Dart mediante FFI.

Ventajas:

- interfaz comun para varias plataformas,
- FFI en Flutter esta bien documentado para Android e iOS,
- puede servir si en el futuro se prioriza una experiencia visual unificada sobre la simplicidad de integracion.

Desventajas:

- introduce Dart/Flutter como segunda plataforma obligatoria del producto,
- incrementa la complejidad de compilacion, empaquetado y depuracion,
- aleja el proyecto de la preferencia de mantener el núcleo y sus integraciones cerca de Go nativo,
- mete una capa adicional entre la interfaz y los puertos de plataforma.

### Opcion C — REST local o proceso separado

Descripcion:

- el nucleo Go corre como proceso local o servicio separado,
- la app movil habla con ese proceso por HTTP local o socket.

Ventajas:

- desacoplamiento fuerte entre interfaz y nucleo,
- aparente simplicidad conceptual si se piensa con el modelo de escritorio.

Desventajas:

- no es una base fiable para iPhone,
- depende de un modelo de proceso residente o de servicio local que no es la base operativa de iOS,
- en Android tambien choca con restricciones modernas sobre trabajo en segundo plano y servicios.

Conclusion:

- se descarta como arquitectura base de V2.

### Opcion D — React Native + modulo nativo

Descripcion:

- la interfaz se implementa en React Native,
- el nucleo Go se encapsula detras de un modulo nativo.

Ventajas:

- interfaz potencialmente compartida.

Desventajas:

- introduce un runtime JavaScript adicional,
- la propia documentacion de React Native refleja transicion entre arquitecturas y APIs nativas, lo que aumenta el riesgo de mantenimiento,
- es menos directo que `gomobile bind` para un nucleo cuyo valor principal esta en Go.

## Decision

Se adopta la **Opcion A: `gomobile bind` con interfaz nativa por plataforma** como estrategia base para mobile en GrxFirma.

Esta decision no obliga a compartir interfaz entre Android e iPhone.  
Lo que se comparte es el nucleo Go, los casos de uso, el dominio y la mayor parte de la logica de firma y verificacion.

## Motivos de la decision

1. Es la opcion que mejor encaja con el principio de usar Go nativo.
2. Es la unica opcion evaluada que es razonablemente directa tanto para Android como para iPhone sin inventar un modelo de proceso local ajeno a mobile.
3. Permite que los puertos dependientes de plataforma sigan siendo realmente adaptadores y no obliguen a contaminar el nucleo.
4. Evita introducir como requisito estructural un runtime adicional de interfaz.
5. Reduce la distancia entre la arquitectura aprobada y la forma real de inyectar adaptadores nativos en el nucleo.

## Consecuencias arquitectonicas

### 1. Habra una fachada de enlace movil especifica

No se debe exponer `internal/domain` ni `internal/application` directamente al borde de `gomobile`.

Debe existir una capa de fachada especifica para mobile, con tipos simples y compatibles con las restricciones de `gobind`, por ejemplo:

- cadenas,
- booleanos,
- `[]byte`,
- structs sencillos,
- interfaces pequenas con firmas compatibles.

### 2. Los puertos afectados por esta decision

Los puertos que previsiblemente necesitaran implementacion nativa o paso explicito a traves del limite Go/mobile son:

- `SecureStorage`
- `BiometricPrompt`
- `CapabilityProfileProvider`
- `EventPublisher`
- `UserApproval`
- `MobilePushNotification`
- `CertificateImporter`
- `SigningKeyProvider`

Segun plataforma y capacidad disponible, tambien pueden verse afectados:

- `SmartCardAccess`
- `TempFileStore`

### 3. El camino canonico de certificados en mobile queda fijado

Para mobile, el camino base aprobado es:

`ImportP12` + `SecureStorage` + `SigningKeyProvider`

Eso significa:

- no asumir un almacen del sistema uniforme como en escritorio,
- no modelar mobile como si fuera una variante de NSS/Windows Store,
- tratar la disponibilidad de hardware, biometria y almacenamiento seguro como capacidades de plataforma.

### 4. La interfaz movil sigue siendo nativa

Android e iPhone tendran adaptadores de entrada y salida propios.

Eso afecta a:

- seleccion documental,
- deep links,
- share sheet,
- notificaciones,
- biometria,
- almacenamiento seguro,
- ciclo de vida de la app,
- publicacion de eventos al borde.

### 5. Se rechaza el modelo de servicio local como base de mobile

Para mobile no se debe disenar una arquitectura basada en:

- servidor REST local residente,
- sockets locales persistentes,
- procesos separados duraderos,
- ni equivalentes heredados del modelo de escritorio.

Esto queda descartado como estrategia principal de V2 para Android/iPhone.

## Consecuencias para el diseno de codigo

1. La Fase 3 y la tarea de contratos mobile deben definir una fachada de enlace movil estable y pequena.
2. Los eventos de progreso y resultado deben salir del nucleo por `EventPublisher`, no por canales acoplados a la interfaz concreta.
3. Los DTOs internos siguen siendo independientes de la integracion mobile; si hacen falta DTOs de enlace, seran DTOs especificos de la fachada movil.
4. Cualquier limitacion de tipos de `gobind` debe resolverse en la fachada, no dentro del dominio.
5. Las decisiones de interfaz visual no condicionan esta ADR; solo fija la tecnologia de integracion del nucleo Go.

## Opciones descartadas

### Flutter + FFI/CGo

Se deja como alternativa futura solo si el proyecto decide priorizar una interfaz compartida multiplataforma sobre la simplicidad de integracion y el principio de Go nativo.

Hoy no es la opcion base recomendada.

### REST local o proceso separado

Se rechaza.

La razon principal es arquitectonica: no es una base fiable ni natural para iPhone y ademas empuja el proyecto a reproducir en mobile un modelo de escritorio.

### React Native + modulo nativo

Se rechaza como opcion base.

No mejora la integracion del nucleo respecto a `gomobile bind` y anade complejidad de plataforma y mantenimiento.

## Riesgos y mitigaciones

### Riesgo 1 — Restricciones de tipos en `gobind`

Mitigacion:

- no exponer tipos ricos del dominio en el limite mobile,
- crear una fachada de enlace con contratos pequenos y estables,
- mantener `application` y `domain` internos.

### Riesgo 2 — Diferencias de capacidades entre Android e iPhone

Mitigacion:

- modelar todo mediante `CapabilityProfileProvider`,
- documentar las diferencias en la guía de cada plataforma,
- no asumir que una capacidad existe por igual en ambas plataformas.

### Riesgo 3 — Confundir shared core con shared UI

Mitigacion:

- dejar escrito que la decision aprobada comparte nucleo, no interfaz,
- tratar Android e iPhone como adaptadores distintos.

## Fuentes consultadas

Fuentes principales y oficiales:

- Go Packages, `golang.org/x/mobile/cmd/gobind`:
  https://pkg.go.dev/golang.org/x/mobile/cmd/gobind
- Go Packages, `golang.org/x/mobile/app`:
  https://pkg.go.dev/golang.org/x/mobile/app
- Flutter, integracion FFI en Android:
  https://docs.flutter.dev/platform-integration/android/c-interop
- Flutter, integracion FFI en iOS:
  https://docs.flutter.dev/platform-integration/ios/c-interop
- React Native, plataforma nativa:
  https://reactnative.dev/docs/native-platform
- Apple, `BGAppRefreshTask`:
  https://developer.apple.com/documentation/backgroundtasks/bgapprefreshtask
- Apple, actualizaciones en segundo plano por notificacion:
  https://developer.apple.com/documentation/usernotifications/pushing-background-updates-to-your-app
- Android, limites de ejecucion en segundo plano:
  https://developer.android.com/about/versions/oreo/background
- Android, restricciones de servicios en primer plano desde segundo plano:
  https://developer.android.com/about/versions/12/foreground-services

Inferencia arquitectonica derivada de esas fuentes:

- iPhone no es una plataforma adecuada para basar V2 mobile en un servicio local residente o proceso separado como mecanismo principal de integracion del nucleo.
