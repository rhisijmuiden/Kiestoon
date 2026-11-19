//! The icon in the notification area: status colour, tooltip and a menu for presence and queues.

use std::collections::HashMap;
use tauri::image::Image;
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Emitter, Manager, Runtime};

const BASE: &[u8] = include_bytes!("../icons/tray.png");
pub const ID: &str = "main";

/// RGB of the status dot.
fn colour(status: &str) -> Option<[u8; 3]> {
    match status {
        "available" => Some([0x34, 0xD3, 0x99]),
        "dnd" | "call" => Some([0xF8, 0x71, 0x71]),
        "ringing" => Some([0xFB, 0xBF, 0x24]),
        "offline" => Some([0x7C, 0x8A, 0xA3]),
        _ => None,
    }
}

/// The app icon with a status dot in the lower right corner.
pub fn icon(status: &str) -> Image<'static> {
    let base = Image::from_bytes(BASE).expect("tray icon");
    let (w, h) = (base.width(), base.height());
    let mut px = base.rgba().to_vec();
    if let Some([r, g, b]) = colour(status) {
        let rad = w as f32 * 0.22;
        let (cx, cy) = (w as f32 - rad - 0.5, h as f32 - rad - 0.5);
        for y in 0..h {
            for x in 0..w {
                let d = ((x as f32 + 0.5 - cx).powi(2) + (y as f32 + 0.5 - cy).powi(2)).sqrt();
                let i = ((y * w + x) * 4) as usize;
                if d <= rad {
                    px[i..i + 4].copy_from_slice(&[r, g, b, 255]);
                } else if d <= rad + w as f32 * 0.07 {
                    // Dark ring so the dot stands out on light and dark taskbars.
                    px[i..i + 4].copy_from_slice(&[0x07, 0x0B, 0x14, 255]);
                }
            }
        }
    }
    Image::new_owned(px, w, h)
}

fn label<'a>(labels: &'a HashMap<String, String>, key: &str, default: &'a str) -> &'a str {
    labels.get(key).map(|s| s.as_str()).filter(|s| !s.is_empty()).unwrap_or(default)
}

pub fn menu<R: Runtime>(app: &AppHandle<R>, labels: &HashMap<String, String>) -> tauri::Result<Menu<R>> {
    let m = Menu::new(app)?;
    m.append(&MenuItem::with_id(
        app,
        "open",
        label(labels, "open", "Open Kiestoon"),
        true,
        None::<&str>,
    )?)?;
    m.append(&PredefinedMenuItem::separator(app)?)?;
    m.append(&MenuItem::with_id(
        app,
        "available",
        format!("● {}", label(labels, "available", "Available")),
        true,
        None::<&str>,
    )?)?;
    m.append(&MenuItem::with_id(
        app,
        "dnd",
        format!("● {}", label(labels, "dnd", "Do not disturb")),
        true,
        None::<&str>,
    )?)?;
    if labels.get("hasQueues").is_some_and(|v| !v.is_empty()) {
        m.append(&PredefinedMenuItem::separator(app)?)?;
        m.append(&MenuItem::with_id(
            app,
            "queues-in",
            label(labels, "queuesIn", "Sign in to all queues"),
            true,
            None::<&str>,
        )?)?;
        m.append(&MenuItem::with_id(
            app,
            "queues-out",
            label(labels, "queuesOut", "Sign out of all queues"),
            true,
            None::<&str>,
        )?)?;
    }
    m.append(&PredefinedMenuItem::separator(app)?)?;
    m.append(&MenuItem::with_id(app, "info", label(labels, "info", "Info"), true, None::<&str>)?)?;
    m.append(&MenuItem::with_id(app, "quit", label(labels, "quit", "Quit"), true, None::<&str>)?)?;
    Ok(m)
}

pub fn show_main<R: Runtime>(app: &AppHandle<R>) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
    }
}

pub fn create<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<()> {
    TrayIconBuilder::with_id(ID)
        .icon(icon("offline"))
        .tooltip("Kiestoon")
        .menu(&menu(app, &HashMap::new())?)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, e| match e.id().as_ref() {
            "open" => show_main(app),
            "quit" => app.exit(0),
            other => {
                if other == "info" {
                    show_main(app);
                }
                let _ = app.emit_to("main", "tray", serde_json::json!({ "action": other }));
            }
        })
        .on_tray_icon_event(|tray, e| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = e
            {
                let app = tray.app_handle();
                match app.get_webview_window("main") {
                    Some(w) if w.is_visible().unwrap_or(false) && w.is_focused().unwrap_or(false) => {
                        let _ = w.hide();
                    }
                    _ => show_main(app),
                }
            }
        })
        .build(app)?;
    Ok(())
}

pub fn update<R: Runtime>(app: &AppHandle<R>, status: &str, tooltip: &str, labels: Option<&HashMap<String, String>>) {
    if let Some(t) = app.tray_by_id(ID) {
        let _ = t.set_icon(Some(icon(status)));
        let _ = t.set_tooltip(Some(tooltip));
        if let Some(l) = labels {
            if let Ok(m) = menu(app, l) {
                let _ = t.set_menu(Some(m));
            }
        }
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn icons_render() {
        for s in ["available", "dnd", "call", "ringing", "offline", "other"] {
            let i = super::icon(s);
            assert_eq!(i.rgba().len(), (i.width() * i.height() * 4) as usize);
        }
    }
}
