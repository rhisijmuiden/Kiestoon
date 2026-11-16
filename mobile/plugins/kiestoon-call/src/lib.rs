//! The native half of the Kiestoon mobile app: push wake-up and the system call screen.
//!
//! Everything happens in Kotlin (Android) and Swift (iOS); this crate only registers those
//! plugins. The pages of the server call them as `plugin:kiestoon-call|<command>`:
//!
//! - `registration`: the push token, platform and an id of this installation of the app
//! - `set_secret`: the key that decrypts the contents of notifications (from the server)
//! - `call_update`: tells the system call screen what the softphone does (ringing, active, ended)
//! - `take_pending`: an answer or decline pressed on the call screen before the page was loaded
//!
//! and listen to the events `push` (a notification while the app runs), `callAction` (answer, end
//! or mute on the call screen) and `token` (the push token changed).

use tauri::{
    plugin::{Builder, TauriPlugin},
    Runtime,
};

#[cfg(target_os = "ios")]
tauri::ios_plugin_binding!(init_plugin_kiestoon_call);

pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("kiestoon-call")
        .setup(|_app, _api| {
            #[cfg(target_os = "android")]
            _api.register_android_plugin("nl.rhis.kiestoon.call", "CallPlugin")?;
            #[cfg(target_os = "ios")]
            _api.register_ios_plugin(init_plugin_kiestoon_call)?;
            Ok(())
        })
        .build()
}
