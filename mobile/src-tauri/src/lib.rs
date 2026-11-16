//! Kiestoon for iPhone and Android: a small shell around the /desktop pages of a Kiestoon portal,
//! like the Windows softphone. The pages do the calling (SIP over a secure WebSocket, WebRTC audio);
//! the shell adds linking to a server, push wake-up and the system call screen (plugin
//! kiestoon-call: PushKit + CallKit on iPhone, Firebase + the Android call notification).

use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::sync::Mutex;
use std::time::Duration;
use tauri::ipc::CapabilityBuilder;
use tauri::{AppHandle, Manager, Runtime, State, Url, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_opener::OpenerExt;

/// The Kiestoon release this build belongs to (set by CI), for example "v0.7.0-rc.7".
pub const VERSION: &str = match option_env!("KIESTOON_VERSION") {
    Some(v) => v,
    None => concat!("v", env!("CARGO_PKG_VERSION")),
};

/// Commands the pages of the server may call (besides the kiestoon-call plugin).
const COMMANDS: &[&str] = &["app_info", "unpair", "open_external"];
/// Commands of the kiestoon-call plugin the pages may call.
const PLUGIN_COMMANDS: &[&str] = &["registration", "set_secret", "call_update", "take_pending", "register_listener", "remove_listener"];

#[derive(Default, Clone, Serialize, Deserialize)]
struct Config {
    /// The origin of the linked Kiestoon portal, for example https://pbx.example.com.
    #[serde(default)]
    server: String,
}

struct AppState {
    cfg: Mutex<Config>,
    path: PathBuf,
}

impl AppState {
    fn cfg(&self) -> Config {
        self.cfg.lock().unwrap().clone()
    }
    fn save(&self, c: Config) -> Result<(), String> {
        if let Some(dir) = self.path.parent() {
            std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
        }
        std::fs::write(&self.path, serde_json::to_vec(&c).unwrap()).map_err(|e| e.to_string())?;
        *self.cfg.lock().unwrap() = c;
        Ok(())
    }
}

/// A page of the app itself (the setup screen).
fn local_url(page: &str) -> Url {
    let base = if cfg!(target_os = "android") { "http://tauri.localhost/" } else { "tauri://localhost/" };
    Url::parse(base).unwrap().join(page).unwrap()
}

fn server_url(origin: &str, code: &str) -> Result<Url, String> {
    let mut u = Url::parse(&format!("{origin}/desktop")).map_err(|e| e.to_string())?;
    if !code.is_empty() {
        u.query_pairs_mut().append_pair("pair", code);
    }
    Ok(u)
}

/// The origin of an address the user typed: https only (a phone cannot trust a self-signed
/// certificate, so the portal needs a real one).
fn origin_of(input: &str) -> Result<String, String> {
    let mut s = input.trim().trim_end_matches('/').to_string();
    if s.is_empty() {
        return Err("empty".into());
    }
    if !s.contains("://") {
        s = format!("https://{s}");
    }
    let u = Url::parse(&s).map_err(|_| "invalid".to_string())?;
    if u.scheme() != "https" {
        return Err("https_required".into());
    }
    let host = u.host_str().ok_or("invalid")?;
    Ok(match u.port() {
        Some(p) => format!("https://{host}:{p}"),
        None => format!("https://{host}"),
    })
}

/// Lets the pages of this server (and only this server) call the shell and the plugin.
fn allow_server<R: Runtime>(app: &AppHandle<R>, origin: &str) -> Result<(), String> {
    let mut cap = CapabilityBuilder::new("server")
        .remote(format!("{origin}/*"))
        .window("main")
        .permission("core:event:default");
    for c in COMMANDS {
        cap = cap.permission(format!("allow-{}", c.replace('_', "-")));
    }
    for c in PLUGIN_COMMANDS {
        cap = cap.permission(format!("kiestoon-call:allow-{}", c.replace('_', "-")));
    }
    app.add_capability(cap).map_err(|e| e.to_string())
}

fn navigate<R: Runtime>(app: &AppHandle<R>, url: Url) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.navigate(url);
    }
}

// ---- commands for the setup screen -----------------------------------------

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct SetupInfo {
    version: String,
    server: String,
}

#[tauri::command]
fn setup_info(state: State<'_, AppState>) -> SetupInfo {
    SetupInfo { version: VERSION.into(), server: state.cfg().server }
}

/// Checks that a Kiestoon portal answers at the address, then opens it with the pairing code.
#[tauri::command]
async fn connect<R: Runtime>(app: AppHandle<R>, state: State<'_, AppState>, url: String, code: String) -> Result<(), String> {
    let origin = origin_of(&url)?;
    let client = reqwest::Client::builder().timeout(Duration::from_secs(10)).build().map_err(|e| e.to_string())?;
    let res = client.get(format!("{origin}/api/desktop/info")).send().await.map_err(|_| "unreachable".to_string())?;
    let info: serde_json::Value = res.json().await.map_err(|_| "not_kiestoon".to_string())?;
    if info.get("version").is_none() {
        return Err("not_kiestoon".into());
    }
    state.save(Config { server: origin.clone() })?;
    allow_server(&app, &origin)?;
    navigate(&app, server_url(&origin, code.trim())?);
    Ok(())
}

// ---- commands for the server pages -----------------------------------------

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct AppInfo {
    version: String,
    channel: String,
    os: String,
    autostart: bool,
    server: String,
    fingerprint: String,
}

#[tauri::command]
fn app_info(state: State<'_, AppState>) -> AppInfo {
    AppInfo {
        version: VERSION.into(),
        channel: "production".into(),
        os: std::env::consts::OS.into(),
        autostart: false,
        server: state.cfg().server,
        fingerprint: String::new(),
    }
}

#[tauri::command]
fn unpair<R: Runtime>(app: AppHandle<R>, state: State<'_, AppState>) -> Result<(), String> {
    state.save(Config::default())?;
    navigate(&app, local_url("index.html"));
    Ok(())
}

#[tauri::command]
fn open_external<R: Runtime>(app: AppHandle<R>, url: String) -> Result<(), String> {
    let u = Url::parse(&url).map_err(|e| e.to_string())?;
    if u.scheme() != "https" && u.scheme() != "http" {
        return Err("only web links".into());
    }
    app.opener().open_url(u.as_str(), None::<&str>).map_err(|e| e.to_string())
}

/// kiestoon://pair?server=…&code=… (the QR code in the portal): link to that server.
fn on_link<R: Runtime>(app: &AppHandle<R>, link: &Url) {
    if link.scheme() != "kiestoon" || link.host_str() != Some("pair") {
        return;
    }
    let mut setup = local_url("index.html");
    {
        let mut q = setup.query_pairs_mut();
        for (k, v) in link.query_pairs() {
            if k == "server" || k == "code" {
                q.append_pair(&k, &v);
            }
        }
        q.append_pair("auto", "1");
    }
    navigate(app, setup);
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_kiestoon_call::init())
        .invoke_handler(tauri::generate_handler![setup_info, connect, app_info, unpair, open_external])
        .setup(|app| {
            let path = app.path().app_config_dir()?.join("config.json");
            let cfg: Config = std::fs::read(&path).ok().and_then(|b| serde_json::from_slice(&b).ok()).unwrap_or_default();
            let start = if cfg.server.is_empty() {
                local_url("index.html")
            } else {
                allow_server(app.handle(), &cfg.server)?;
                server_url(&cfg.server, "")?
            };
            app.manage(AppState { cfg: Mutex::new(cfg), path });
            WebviewWindowBuilder::new(app, "main", WebviewUrl::External(start)).build()?;

            let h = app.handle().clone();
            app.deep_link().on_open_url(move |e| {
                for u in e.urls() {
                    on_link(&h, &u);
                }
            });
            if let Ok(Some(urls)) = app.deep_link().get_current() {
                for u in urls {
                    on_link(app.handle(), &u);
                }
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running Kiestoon");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn origins() {
        assert_eq!(origin_of("pbx.example.com/").unwrap(), "https://pbx.example.com");
        assert_eq!(origin_of("https://pbx.example.com:8443/desktop").unwrap(), "https://pbx.example.com:8443");
        assert_eq!(origin_of("http://pbx.example.com").unwrap_err(), "https_required");
        assert_eq!(origin_of(" ").unwrap_err(), "empty");
    }
}
