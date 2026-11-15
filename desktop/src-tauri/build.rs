fn main() {
    // Every command gets an allow-<name> permission. The setup screen and the incoming-call popup
    // (pages of the app) get theirs in capabilities/local.json; the pages of the linked server get
    // theirs at runtime (see allow_server in lib.rs).
    tauri_build::try_build(
        tauri_build::Attributes::new().app_manifest(tauri_build::AppManifest::new().commands(&[
            "app_info",
            "set_channel",
            "check_update",
            "install_update",
            "notify",
            "incoming_show",
            "incoming_hide",
            "incoming_action",
            "set_status",
            "headset_state",
            "headset_list",
            "open_external",
            "unpair",
            "focus_main",
            "set_autostart",
            "setup_info",
            "probe_server",
            "connect",
            "browser_login",
        ])),
    )
    .expect("tauri build");
}
