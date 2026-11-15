//! Kiestoon Softphone: a small Windows shell around the /desktop pages of a Kiestoon portal.
//!
//! The calling itself (SIP over a secure WebSocket, WebRTC audio) happens in those pages, served by
//! the user's own Kiestoon server, so the softphone always matches the server it talks to. This shell
//! adds what a browser tab cannot: linking to a server (with certificate pinning for servers with a
//! self-signed certificate), the tray icon, the incoming-call popup, notifications, headset buttons,
//! tel: links, starting with Windows and updating itself.

mod config;
mod headset;
mod hid_desc;
mod pin;
mod tray;
mod updater;
#[cfg(windows)]
mod winview;

use config::Config;
use serde::Serialize;
use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use tauri::ipc::CapabilityBuilder;
use tauri::{AppHandle, Emitter, Manager, Runtime, State, Url, WebviewUrl, WebviewWindowBuilder, WindowEvent};
use tauri_plugin_autostart::ManagerExt as _;
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_notification::NotificationExt;
use tauri_plugin_opener::OpenerExt;

/// The Kiestoon release this build belongs to (set by CI), for example "v0.7.0-rc.1".
pub const VERSION: &str = match option_env!("KIESTOON_VERSION") {
    Some(v) => v,
    None => concat!("v", env!("CARGO_PKG_VERSION")),
};

/// Commands the pages may call. The remote (server) capability lists them explicitly.
const COMMANDS: &[&str] = &[
    "app_info", "set_channel", "check_update", "install_update", "notify", "incoming_show", "incoming_hide", "set_status",
    "headset_state", "headset_list", "open_external", "unpair", "focus_main", "set_autostart",
];

struct AppState {
    cfg: Mutex<Config>,
    path: PathBuf,
    headsets: headset::Shared,
    labels: Mutex<HashMap<String, String>>,
    release: Mutex<Option<updater::Release>>,
}

impl AppState {
    fn cfg(&self) -> Config {
        self.cfg.lock().unwrap().clone()
    }
    fn save(&self, c: Config) -> Result<(), String> {
        c.save(&self.path)?;
        *self.cfg.lock().unwrap() = c;
        Ok(())
    }
}

/// A page of the app itself (the setup screen, the incoming-call popup).
fn local_url(page: &str) -> Url {
    let base = if cfg!(windows) { "http://tauri.localhost/" } else { "tauri://localhost/" };
    Url::parse(base).unwrap().join(page).unwrap()
}

fn server_url(origin: &str, code: &str) -> Result<Url, String> {
    let mut u = Url::parse(&format!("{origin}/desktop")).map_err(|e| e.to_string())?;
    if !code.is_empty() {
        u.query_pairs_mut().append_pair("pair", code);
    }
    Ok(u)
}

/// Lets the pages of this server (and only this server) call the shell.
fn allow_server<R: Runtime>(app: &AppHandle<R>, origin: &str) -> Result<(), String> {
    let id = format!("server-{}", &pin::fingerprint(origin.as_bytes())[..12]);
    let mut cap = CapabilityBuilder::new(id).remote(format!("{origin}/*")).window("main").permission("core:event:default");
    for c in COMMANDS {
        cap = cap.permission(format!("allow-{}", c.replace('_', "-")));
    }
    app.add_capability(cap).map_err(|e| e.to_string())
}

fn browser_args(spki: &str) -> String {
    // Tauri's defaults, plus: ringing and the call audio may start without a click.
    let mut a = String::from("--disable-features=msWebOOUI,msPdfOOUI,msSmartScreenProtection --autoplay-policy=no-user-gesture-required");
    if !spki.is_empty() {
        a.push_str(&format!(" --ignore-certificate-errors-spki-list={spki}"));
    }
    a
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
fn app_info<R: Runtime>(app: AppHandle<R>, state: State<'_, AppState>) -> AppInfo {
    let c = state.cfg();
    AppInfo {
        version: VERSION.into(),
        channel: c.channel().into(),
        os: if cfg!(windows) { "Windows".into() } else { std::env::consts::OS.into() },
        autostart: app.autolaunch().is_enabled().unwrap_or(false),
        server: c.server.clone(),
        fingerprint: if c.fingerprint.is_empty() { String::new() } else { pin::pretty(&c.fingerprint) },
    }
}

#[tauri::command]
fn set_channel(state: State<'_, AppState>, channel: String) -> Result<(), String> {
    let mut c = state.cfg();
    c.channel = if channel == "testing" { "testing".into() } else { "production".into() };
    state.save(c)
}

#[tauri::command]
async fn check_update(state: State<'_, AppState>) -> Result<updater::Check, String> {
    let c = state.cfg();
    let res = updater::check(c.repo(), c.channel(), VERSION).await?;
    *state.release.lock().unwrap() = if res.available { res.latest.clone() } else { None };
    Ok(res)
}

#[tauri::command]
async fn install_update<R: Runtime>(app: AppHandle<R>, state: State<'_, AppState>) -> Result<(), String> {
    let rel = state.release.lock().unwrap().clone().ok_or("check for updates first")?;
    let a = app.clone();
    let msi = updater::download(&rel, move |p| {
        let _ = a.emit_to("main", "update-progress", serde_json::json!({ "percent": p }));
    })
    .await?;
    run_installer(&msi)?;
    app.exit(0);
    Ok(())
}

/// Starts the MSI (it asks for permission when needed) and starts the softphone again afterwards.
fn run_installer(msi: &std::path::Path) -> Result<(), String> {
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        let exe = std::env::current_exe().map_err(|e| e.to_string())?;
        let cmd = format!("msiexec /i \"{}\" /passive /norestart && start \"\" \"{}\"", msi.display(), exe.display());
        std::process::Command::new("cmd").args(["/C", &cmd]).creation_flags(0x0800_0000).spawn().map_err(|e| e.to_string())?;
        Ok(())
    }
    #[cfg(not(windows))]
    {
        Err(format!("updates are installed on Windows only ({})", msi.display()))
    }
}

#[tauri::command]
fn notify<R: Runtime>(app: AppHandle<R>, title: String, body: String) {
    let _ = app.notification().builder().title(title).body(body).show();
}

/// The incoming-call popup in the lower right corner, above other windows.
#[tauri::command]
fn incoming_show<R: Runtime>(app: AppHandle<R>, name: String, number: String, via: String, lang: String) -> Result<(), String> {
    let mut url = local_url("incoming.html");
    url.query_pairs_mut().append_pair("name", &name).append_pair("number", &number).append_pair("via", &via).append_pair("lang", &lang);
    if let Some(w) = app.get_webview_window("incoming") {
        w.navigate(url).map_err(|e| e.to_string())?;
        let _ = w.show();
        return Ok(());
    }
    let (w, h) = (360.0, 156.0);
    let mut b = WebviewWindowBuilder::new(&app, "incoming", WebviewUrl::External(url))
        .title("Kiestoon")
        .inner_size(w, h)
        .resizable(false)
        .decorations(false)
        .always_on_top(true)
        .skip_taskbar(true)
        .focused(false)
        .additional_browser_args(&browser_args(&state_spki(&app)));
    if let Ok(Some(m)) = app.primary_monitor() {
        let s = m.scale_factor();
        let area = m.work_area();
        let x = (area.position.x as f64 + area.size.width as f64) / s - w - 16.0;
        let y = (area.position.y as f64 + area.size.height as f64) / s - h - 16.0;
        b = b.position(x, y);
    }
    b.build().map_err(|e| e.to_string())?;
    Ok(())
}

fn state_spki<R: Runtime>(app: &AppHandle<R>) -> String {
    app.state::<AppState>().cfg().spki
}

#[tauri::command]
fn incoming_hide<R: Runtime>(app: AppHandle<R>) {
    if let Some(w) = app.get_webview_window("incoming") {
        let _ = w.close();
    }
}

/// From the popup: "answer" or "decline".
#[tauri::command]
fn incoming_action<R: Runtime>(app: AppHandle<R>, action: String) {
    let _ = app.emit_to("main", "incoming", serde_json::json!({ "action": action }));
    if action == "answer" {
        tray::show_main(&app);
    }
    incoming_hide(app);
}

#[tauri::command]
fn set_status<R: Runtime>(app: AppHandle<R>, state: State<'_, AppState>, status: String, tooltip: String, labels: HashMap<String, String>) {
    let changed = {
        let mut l = state.labels.lock().unwrap();
        let c = *l != labels;
        if c {
            *l = labels.clone();
        }
        c
    };
    tray::update(&app, &status, &tooltip, changed.then_some(&labels));
}

#[tauri::command]
fn headset_state(state: State<'_, AppState>, ringing: bool, offhook: bool, muted: bool) {
    *state.headsets.state.lock().unwrap() = headset::LineState { ringing, offhook, muted };
}

#[tauri::command]
fn headset_list(state: State<'_, AppState>) -> Vec<headset::HeadsetInfo> {
    state.headsets.found.lock().unwrap().clone()
}

#[tauri::command]
fn open_external<R: Runtime>(app: AppHandle<R>, url: String) -> Result<(), String> {
    let u = Url::parse(&url).map_err(|e| e.to_string())?;
    if u.scheme() != "https" && u.scheme() != "http" {
        return Err("only web links".into());
    }
    app.opener().open_url(u.as_str(), None::<&str>).map_err(|e| e.to_string())
}

#[tauri::command]
fn unpair<R: Runtime>(app: AppHandle<R>, state: State<'_, AppState>) -> Result<(), String> {
    let c = state.cfg();
    state.save(Config { channel: c.channel, repo: c.repo, ..Config::default() })?;
    app.restart();
}

#[tauri::command]
fn focus_main<R: Runtime>(app: AppHandle<R>) {
    tray::show_main(&app);
}

#[tauri::command]
fn set_autostart<R: Runtime>(app: AppHandle<R>, enabled: bool) -> Result<(), String> {
    let l = app.autolaunch();
    if enabled { l.enable() } else { l.disable() }.map_err(|e| e.to_string())
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

#[tauri::command]
async fn probe_server(url: String) -> Result<pin::Probe, String> {
    let origin = config::normalize_origin(&url)?;
    let mut p = pin::probe(&origin, None).await?;
    p.fingerprint = pin::pretty(&p.fingerprint);
    Ok(p)
}

/// Links the softphone to a server. `accepted` is the fingerprint the user confirmed (or the one
/// in a pairing link from the portal); it is only needed when the certificate is not trusted.
#[tauri::command]
async fn connect<R: Runtime>(app: AppHandle<R>, state: State<'_, AppState>, url: String, accepted: String, code: String) -> Result<(), String> {
    link(&app, &state, &url, &accepted, &code).await
}

/// Opens the portal in the normal browser to sign in there (for SSO); it returns with a pairing link.
#[tauri::command]
fn browser_login<R: Runtime>(app: AppHandle<R>, url: String) -> Result<(), String> {
    let origin = config::normalize_origin(&url)?;
    app.opener().open_url(format!("{origin}/desktop-link"), None::<&str>).map_err(|e| e.to_string())
}

async fn link<R: Runtime>(app: &AppHandle<R>, state: &AppState, url: &str, accepted: &str, code: &str) -> Result<(), String> {
    let origin = config::normalize_origin(url)?;
    let accepted: String = accepted.chars().filter(|c| c.is_ascii_hexdigit()).collect::<String>().to_ascii_lowercase();
    let probe = pin::probe(&origin, (!accepted.is_empty()).then(|| accepted.clone())).await?;
    if !probe.trusted {
        if accepted.is_empty() {
            return Err("untrusted".into());
        }
        if accepted != probe.fingerprint {
            return Err("fingerprint_mismatch".into());
        }
    }
    let old = state.cfg();
    let mut c = Config {
        server: origin.clone(),
        fingerprint: if probe.trusted { String::new() } else { probe.fingerprint.clone() },
        spki: if probe.trusted { String::new() } else { probe.spki.clone() },
        channel: old.channel.clone(),
        repo: old.repo.clone(),
        pending_code: String::new(),
    };
    // The WebView only takes a new certificate exception when it starts: restart for a new one.
    if c.spki != old.spki && !c.spki.is_empty() {
        c.pending_code = code.to_string();
        state.save(c)?;
        app.restart();
    }
    state.save(c)?;
    allow_server(app, &origin)?;
    let main = app.get_webview_window("main").ok_or("no window")?;
    #[cfg(windows)]
    winview::install(&main, origin.clone(), state.cfg().fingerprint);
    main.navigate(server_url(&origin, code)?).map_err(|e| e.to_string())?;
    tray::show_main(app);
    Ok(())
}

/// kiestoon://pair?server=…&code=…&fp=…, and tel:, sip: and callto: links.
fn open_urls<R: Runtime>(app: &AppHandle<R>, urls: Vec<Url>) {
    for u in urls {
        match u.scheme() {
            "kiestoon" => {
                let q: HashMap<String, String> = u.query_pairs().into_owned().collect();
                let (server, code, fp) = (q.get("server").cloned().unwrap_or_default(), q.get("code").cloned().unwrap_or_default(), q.get("fp").cloned().unwrap_or_default());
                if server.is_empty() {
                    continue;
                }
                let app = app.clone();
                tauri::async_runtime::spawn(async move {
                    let state = app.state::<AppState>();
                    if let Err(e) = link(&app, &state, &server, &fp, &code).await {
                        // Show the setup screen with the details, so the user can check and retry.
                        let mut page = local_url("index.html");
                        page.query_pairs_mut().append_pair("server", &server).append_pair("code", &code).append_pair("error", &e);
                        if let Some(w) = app.get_webview_window("main") {
                            let _ = w.navigate(page);
                        }
                        tray::show_main(&app);
                    }
                });
            }
            "tel" | "sip" | "sips" | "callto" => {
                let raw = u.as_str().split_once(':').map(|x| x.1).unwrap_or_default();
                let raw = url::form_urlencoded::parse(raw.as_bytes()).map(|(k, _)| k.into_owned()).collect::<String>();
                let number: String = raw.trim_start_matches('/').split(['@', ';', '?']).next().unwrap_or_default().chars().filter(|c| c.is_ascii_digit() || "+*#".contains(*c)).collect();
                if !number.is_empty() {
                    tray::show_main(app);
                    let _ = app.emit_to("main", "dial", serde_json::json!({ "number": number }));
                }
            }
            _ => {}
        }
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _argv, _cwd| tray::show_main(app)))
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_autostart::init(tauri_plugin_autostart::MacosLauncher::LaunchAgent, Some(vec!["--hidden"])))
        .invoke_handler(tauri::generate_handler![
            app_info, set_channel, check_update, install_update, notify, incoming_show, incoming_hide, incoming_action, set_status,
            headset_state, headset_list, open_external, unpair, focus_main, set_autostart, setup_info, probe_server, connect, browser_login
        ])
        .on_window_event(|w, e| {
            // Closing the main window keeps the softphone running in the tray.
            if let WindowEvent::CloseRequested { api, .. } = e {
                if w.label() == "main" {
                    api.prevent_close();
                    let _ = w.hide();
                }
            }
        })
        .setup(|app| {
            let path = app.path().app_config_dir()?.join("config.json");
            let mut cfg = Config::load(&path);
            let code = std::mem::take(&mut cfg.pending_code);
            if !code.is_empty() {
                let _ = cfg.save(&path);
            }
            let shared: headset::Shared = Arc::new(headset::Headsets::default());
            app.manage(AppState { cfg: Mutex::new(cfg.clone()), path, headsets: shared.clone(), labels: Mutex::new(HashMap::new()), release: Mutex::new(None) });

            let handle = app.handle().clone();
            let start = if cfg.server.is_empty() {
                WebviewUrl::App("index.html".into())
            } else {
                allow_server(&handle, &cfg.server).map_err(std::io::Error::other)?;
                WebviewUrl::External(server_url(&cfg.server, &code).map_err(std::io::Error::other)?)
            };
            let hidden = std::env::args().any(|a| a == "--hidden") && !cfg.server.is_empty();
            let main = WebviewWindowBuilder::new(app, "main", start)
                .title("Kiestoon")
                .inner_size(420.0, 720.0)
                .min_inner_size(380.0, 560.0)
                .visible(!hidden)
                .additional_browser_args(&browser_args(&cfg.spki))
                .build()?;
            #[cfg(windows)]
            winview::install(&main, cfg.server.clone(), cfg.fingerprint.clone());
            let _ = &main;

            tray::create(&handle)?;

            let h = handle.clone();
            headset::start(shared, move |action| {
                let _ = h.emit_to("main", "headset", serde_json::json!({ "action": action }));
            });

            let h = handle.clone();
            app.deep_link().on_open_url(move |e| open_urls(&h, e.urls()));
            #[cfg(all(debug_assertions, windows))]
            let _ = app.deep_link().register_all();
            if let Ok(Some(urls)) = app.deep_link().get_current() {
                open_urls(&handle, urls);
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running Kiestoon Softphone");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn urls() {
        assert_eq!(server_url("https://pbx:8443", "").unwrap().as_str(), "https://pbx:8443/desktop");
        assert_eq!(server_url("https://pbx", "AB CD").unwrap().as_str(), "https://pbx/desktop?pair=AB+CD");
        assert!(browser_args("abc=").ends_with("--ignore-certificate-errors-spki-list=abc="));
    }
}
