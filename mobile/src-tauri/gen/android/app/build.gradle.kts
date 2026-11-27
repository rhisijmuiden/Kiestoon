import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("rust")
}

val tauriProperties = Properties().apply {
    val propFile = file("tauri.properties")
    if (propFile.exists()) {
        propFile.inputStream().use { load(it) }
    }
}

android {
    compileSdk = 36
    namespace = "nl.rhis.kiestoon"
    defaultConfig {
        manifestPlaceholders["usesCleartextTraffic"] = "false"
        applicationId = "nl.rhis.kiestoon"
        minSdk = 29
        targetSdk = 36
        // Store builds set both from VERSION (see .github/workflows/mobile-release.yml): Google Play needs a
        // higher versionCode for every upload, also between release candidates of the same version.
        versionCode = (System.getenv("KIESTOON_VERSION_CODE")
            ?: tauriProperties.getProperty("tauri.android.versionCode", "1")).toInt()
        versionName = System.getenv("KIESTOON_VERSION_NAME")
            ?: tauriProperties.getProperty("tauri.android.versionName", "1.0")
    }
    // The upload key for Google Play. It is never in the repository: CI writes it from the secret
    // ANDROID_UPLOAD_KEYSTORE to a temporary file and passes the path and passwords as environment
    // variables. Without them the release build is unsigned (fine for a test build, not for Play).
    val uploadKeystore = System.getenv("KIESTOON_UPLOAD_KEYSTORE")?.takeIf { it.isNotBlank() }?.let { file(it) }
    signingConfigs {
        if (uploadKeystore != null) {
            create("upload") {
                storeFile = uploadKeystore
                storePassword = System.getenv("KIESTOON_UPLOAD_STORE_PASSWORD")
                keyAlias = System.getenv("KIESTOON_UPLOAD_KEY_ALIAS") ?: "upload"
                keyPassword = System.getenv("KIESTOON_UPLOAD_KEY_PASSWORD") ?: System.getenv("KIESTOON_UPLOAD_STORE_PASSWORD")
            }
        }
    }
    buildTypes {
        getByName("debug") {
            manifestPlaceholders["usesCleartextTraffic"] = "true"
            isDebuggable = true
            isJniDebuggable = true
            isMinifyEnabled = false
            packaging {                jniLibs.keepDebugSymbols.add("*/arm64-v8a/*.so")
                jniLibs.keepDebugSymbols.add("*/armeabi-v7a/*.so")
                jniLibs.keepDebugSymbols.add("*/x86/*.so")
                jniLibs.keepDebugSymbols.add("*/x86_64/*.so")
            }
        }
        getByName("release") {
            if (uploadKeystore != null) signingConfig = signingConfigs.getByName("upload")
            isMinifyEnabled = true
            proguardFiles(
                *fileTree(".") { include("**/*.pro") }
                    .plus(getDefaultProguardFile("proguard-android-optimize.txt"))
                    .toList().toTypedArray()
            )
        }
    }
    kotlinOptions {
        jvmTarget = "1.8"
    }
    buildFeatures {
        buildConfig = true
    }
}

rust {
    rootDirRel = "../../../"
}

dependencies {
    implementation("androidx.webkit:webkit:1.14.0")
    implementation("androidx.appcompat:appcompat:1.7.1")
    implementation("androidx.activity:activity-ktx:1.10.1")
    implementation("com.google.android.material:material:1.12.0")
    testImplementation("junit:junit:4.13.2")
    androidTestImplementation("androidx.test.ext:junit:1.1.4")
    androidTestImplementation("androidx.test.espresso:espresso-core:3.5.0")
}

apply(from = "tauri.build.gradle.kts")

// Push needs the Firebase project of the published app. Builds without google-services.json work,
// just without push.
if (file("google-services.json").exists()) {
    apply(plugin = "com.google.gms.google-services")
}