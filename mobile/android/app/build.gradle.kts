// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

plugins {
    id("com.android.application")
}

val repositoryRoot = layout.projectDirectory.dir("../../..").asFile.canonicalFile
val repositoryVersion = repositoryRoot.resolve("VERSION.txt").readText().trim()
val configuredVersionName = providers.environmentVariable("GRXFIRMA_ANDROID_VERSION_NAME")
    .orElse(repositoryVersion)
    .get()
require(configuredVersionName == repositoryVersion) {
    "GRXFIRMA_ANDROID_VERSION_NAME debe coincidir con VERSION.txt ($repositoryVersion)."
}
val semanticVersion = Regex("""^(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?$""")
    .matchEntire(configuredVersionName)
    ?: error("VERSION.txt no contiene una versión SemVer Android válida.")
val derivedVersionCode = semanticVersion.groupValues.drop(1).take(3).map(String::toInt).let {
    require(it[0] <= 2_000 && it[1] <= 999 && it[2] <= 999) {
        "La versión supera el rango admitido para versionCode Android."
    }
    it[0] * 1_000_000 + it[1] * 1_000 + it[2]
}
val configuredVersionCode = providers.environmentVariable("GRXFIRMA_ANDROID_VERSION_CODE")
    .orElse(derivedVersionCode.toString())
    .get()
    .toIntOrNull()
    ?.takeIf { it in 1..2_100_000_000 }
    ?: error("GRXFIRMA_ANDROID_VERSION_CODE debe ser un entero Android positivo.")
val sourceCommit = providers.environmentVariable("GRXFIRMA_ANDROID_SOURCE_COMMIT")
    .orElse("development")
    .get()
require(sourceCommit == "development" || sourceCommit.matches(Regex("[0-9a-fA-F]{40}"))) {
    "GRXFIRMA_ANDROID_SOURCE_COMMIT debe ser un commit Git completo."
}

val coreAar = providers.gradleProperty("grxfirmaCoreAar")
    .orElse(providers.environmentVariable("GRXFIRMA_ANDROID_CORE_AAR"))
    .orElse(layout.projectDirectory.file("core/grxfirma.aar").asFile.absolutePath)

val coreSha256 = providers.gradleProperty("grxfirmaCoreSha256")
    .orElse(providers.environmentVariable("GRXFIRMA_ANDROID_CORE_SHA256"))
    .orElse("")

val releaseKeystore = providers.environmentVariable("GRXFIRMA_ANDROID_KEYSTORE")
val releaseKeystorePassword = providers.environmentVariable("GRXFIRMA_ANDROID_KEYSTORE_PASSWORD")
val releaseKeyAlias = providers.environmentVariable("GRXFIRMA_ANDROID_KEY_ALIAS")
val releaseKeyPassword = providers.environmentVariable("GRXFIRMA_ANDROID_KEY_PASSWORD")
val releaseSigningConfigured = listOf(
    releaseKeystore,
    releaseKeystorePassword,
    releaseKeyAlias,
    releaseKeyPassword,
).all { it.isPresent }
val pythonExecutable = providers.environmentVariable("PYTHON").orElse("python3")
val androidScripts = layout.projectDirectory.dir("../../../scripts/mobile/android")

fun quoted(value: String): String = "\"${value.replace("\\", "\\\\").replace("\"", "\\\"")}\""

android {
    namespace = "es.dipgra.grxfirma.android"
    compileSdk = 36
    buildToolsVersion = "36.0.0"

    dependenciesInfo {
        // El APK directo queda cubierto por el SBOM externo de la release.
        // El bloque cifrado de AGP no es reproducible y no aporta verificación
        // al instalador fuera de Play. El AAB conserva la información para Play.
        includeInApk = false
        includeInBundle = true
    }

    defaultConfig {
        applicationId = "es.dipgra.grxfirma"
        minSdk = 26
        targetSdk = 36
        versionCode = configuredVersionCode
        versionName = configuredVersionName
        manifestPlaceholders["sourceCommit"] = sourceCommit.lowercase()
        manifestPlaceholders["coreSha256"] = coreSha256.get().lowercase()

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        testInstrumentationRunnerArguments["clearPackageData"] = "true"
        vectorDrawables.useSupportLibrary = true

        ndk {
            abiFilters += setOf("armeabi-v7a", "arm64-v8a", "x86_64")
        }
    }

    flavorDimensions += "backend"
    productFlavors {
        create("verification") {
            dimension = "backend"
            applicationIdSuffix = ".verification"
            versionNameSuffix = "-verification"
            buildConfigField("String", "CORE_MODE", quoted("verification"))
            buildConfigField("String", "CORE_EXPECTED_SHA256", quoted(""))
            buildConfigField("String", "SOURCE_COMMIT", quoted(sourceCommit.lowercase()))
        }
        create("production") {
            dimension = "backend"
            buildConfigField("String", "CORE_MODE", quoted("production"))
            buildConfigField("String", "CORE_EXPECTED_SHA256", quoted(coreSha256.get()))
            buildConfigField("String", "SOURCE_COMMIT", quoted(sourceCommit.lowercase()))
        }
    }

    signingConfigs {
        if (releaseSigningConfigured) {
            create("officialRelease") {
                storeFile = file(releaseKeystore.get())
                storePassword = releaseKeystorePassword.get()
                keyAlias = releaseKeyAlias.get()
                keyPassword = releaseKeyPassword.get()
                enableV1Signing = false
                enableV2Signing = true
                enableV3Signing = true
                enableV4Signing = false
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            if (releaseSigningConfigured) {
                signingConfig = signingConfigs.getByName("officialRelease")
            }
        }
    }

    buildFeatures {
        buildConfig = true
        viewBinding = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        jniLibs.useLegacyPackaging = false
        resources.excludes += setOf(
            "META-INF/AL2.0",
            "META-INF/LGPL2.1",
            "META-INF/LICENSE.md",
            "META-INF/NOTICE.md",
        )
    }

    testOptions {
        execution = "ANDROIDX_TEST_ORCHESTRATOR"
        unitTests.isIncludeAndroidResources = true
    }

    lint {
        abortOnError = true
        checkDependencies = true
        checkReleaseBuilds = true
        htmlReport = true
        sarifReport = true
        warningsAsErrors = true
        // API 37 is preview-only; API 36 is the latest stable SDK available to sdkmanager.
        // Gradle 9.4.1 is the version explicitly supported by AGP 9.2.x.
        disable += setOf(
            "AndroidGradlePluginVersion",
            "GradleDependency",
            "ObsoleteSdkInt",
            "OldTargetApi",
        )
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.appcompat:appcompat:1.7.1")
    implementation("androidx.activity:activity-ktx:1.12.4")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.10.0")
    implementation("androidx.lifecycle:lifecycle-viewmodel-ktx:2.10.0")
    implementation("com.google.android.material:material:1.13.0")
    implementation("androidx.constraintlayout:constraintlayout:2.2.1")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")

    add("productionImplementation", files(coreAar))

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20250517")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.10.2")

    androidTestImplementation("androidx.test.ext:junit:1.3.0")
    androidTestImplementation("androidx.test:core-ktx:1.7.0")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test:rules:1.7.0")
    androidTestImplementation("androidx.test.espresso:espresso-core:3.7.0")
    androidTestImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.10.2")
    androidTestUtil("androidx.test:orchestrator:1.6.1")
}

dependencyLocking {
    lockAllConfigurations()
}

val verifyProductionCore by tasks.registering(Exec::class) {
    group = "verification"
    description = "Requires the pinned, production-capable gomobile AAR."
    executable(pythonExecutable.get())
    args(
        androidScripts.file("validate_core_aar.py").asFile.absolutePath,
        "--aar",
        coreAar.get(),
        "--sha256",
        coreSha256.get(),
    )
}

val verifyProductionSigning by tasks.registering(Exec::class) {
    group = "verification"
    description = "Requires non-repository credentials for the official Android release."
    executable(pythonExecutable.get())
    args(androidScripts.file("validate_release_signing.py").asFile.absolutePath)
}

tasks.configureEach {
    if (name == "preProductionDebugBuild" || name == "preProductionReleaseBuild") {
        dependsOn(verifyProductionCore)
    }
    if (name == "preProductionReleaseBuild") {
        dependsOn(verifyProductionSigning)
    }
}
