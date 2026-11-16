// The commands the pages may call on the native plugin (Kotlin / Swift).
const COMMANDS: &[&str] = &[
    "registration",
    "set_secret",
    "call_update",
    "take_pending",
    "register_listener",
    "remove_listener",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS)
        .android_path("android")
        .ios_path("ios")
        .build();
}
