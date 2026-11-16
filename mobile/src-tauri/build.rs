fn main() {
    tauri_build::try_build(tauri_build::Attributes::new().app_manifest(
        tauri_build::AppManifest::new().commands(&["setup_info", "connect", "app_info", "unpair", "open_external"]),
    ))
    .expect("failed to run tauri-build");
}
