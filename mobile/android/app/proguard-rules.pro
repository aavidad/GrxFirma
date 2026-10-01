# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

-keep class mobilebind.Mobilebind { public *; }
-keep class mobilebind.Facade { public *; }
-keepclasseswithmembers,includedescriptorclasses class * {
    native <methods>;
}
-dontwarn go.**
