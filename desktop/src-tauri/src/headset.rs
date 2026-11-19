//! USB headset buttons and lights (Jabra, Poly/Plantronics, EPOS, Yealink, Logitech and other
//! headsets with a HID telephony interface). The hook switch answers and hangs up, the mute button
//! mutes; the off-hook, ring and mute lights follow the call.

#![cfg_attr(not(windows), allow(dead_code))]

use serde::Serialize;
use std::sync::{Arc, Mutex};

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct LineState {
    pub ringing: bool,
    pub offhook: bool,
    pub muted: bool,
}

#[derive(Debug, Clone, Serialize)]
pub struct HeadsetInfo {
    pub name: String,
    pub vendor: String,
    pub buttons: bool,
}

/// Shared between the commands and the headset thread.
#[derive(Default)]
pub struct Headsets {
    pub state: Mutex<LineState>,
    pub found: Mutex<Vec<HeadsetInfo>>,
}

pub type Shared = Arc<Headsets>;

/// USB vendor IDs of headset makers whose devices are always headsets (Jabra, Poly/Plantronics,
/// EPOS/Sennheiser, Yealink). Logitech also makes mice and keyboards, so it is matched by name.
const HEADSET_VENDORS: [u16; 5] = [0x0B0E, 0x047F, 0x1395, 0x6993, 0x3542];

/// Whether a HID collection belongs to a headset, judged by vendor and product name. Used for
/// headsets without telephony buttons (or with buttons Windows keeps for itself), so they still
/// show up as a headset.
pub fn looks_like_headset(vendor_id: u16, product: &str) -> bool {
    let p = product.to_lowercase();
    if HEADSET_VENDORS.contains(&vendor_id) {
        return true;
    }
    if ["headset", "head set", "hands-free", "handsfree", "speakerphone"]
        .iter()
        .any(|w| p.contains(w))
    {
        return true;
    }
    // Logitech: Zone, H-series (H390, H570e, H650e) and PRO X. Gaming headsets (G435, G733) carry
    // "Headset" in their name; G-numbers alone are also mice, so they do not count.
    if vendor_id == 0x046D {
        let model = p.trim_start_matches("logitech").trim_start_matches("logi").trim();
        return model.starts_with("zone")
            || p.contains("headset")
            || p.contains("pro x")
            || model
                .split(|c: char| !c.is_ascii_alphanumeric())
                .any(|w| w.len() >= 4 && w.starts_with('h') && w[1..4].chars().all(|c| c.is_ascii_digit()));
    }
    false
}

/// Which HID collections are worth opening: telephony, consumer control (some headsets put their
/// telephony buttons inside it) and vendor pages of known headsets. Keyboards and mice are skipped.
pub fn candidate(page: u16, vendor_id: u16, product: &str) -> bool {
    match page {
        crate::hid_desc::PAGE_GENERIC_DESKTOP => false,
        crate::hid_desc::PAGE_TELEPHONY | crate::hid_desc::PAGE_CONSUMER | crate::hid_desc::PAGE_LED => true,
        p if p >= 0xFF00 => looks_like_headset(vendor_id, product),
        _ => false,
    }
}

/// Adds a found headset to the list, once per name; buttons count when any collection has them.
pub fn add_info(infos: &mut Vec<HeadsetInfo>, name: String, vendor: String, buttons: bool) {
    if let Some(i) = infos.iter_mut().find(|i| i.name == name) {
        i.buttons |= buttons;
    } else {
        infos.push(HeadsetInfo { name, vendor, buttons });
    }
}

/// A button press, sent to the softphone page as the "headset" event.
pub fn action_name(hook_from: bool, hook_to: bool) -> Option<&'static str> {
    match (hook_from, hook_to) {
        (false, true) => Some("hook-off"),
        (true, false) => Some("hook-on"),
        _ => None,
    }
}

#[cfg(windows)]
pub fn start(shared: Shared, emit: impl Fn(&'static str) + Send + 'static) {
    std::thread::Builder::new()
        .name("headset".into())
        .spawn(move || win::run(shared, emit))
        .ok();
}

#[cfg(not(windows))]
pub fn start(_shared: Shared, _emit: impl Fn(&'static str) + Send + 'static) {}

#[cfg(windows)]
mod win {
    use super::{action_name, HeadsetInfo, LineState, Shared};
    use crate::hid_desc::{self, Field, Kind, Layout};
    use hidapi::{HidApi, HidDevice};
    use std::time::{Duration, Instant};

    struct Open {
        dev: HidDevice,
        layout: Layout,
        hook: Option<Field>,
        mute: Option<Field>,
        led_off: Option<Field>,
        led_ring: Option<Field>,
        led_mute: Option<Field>,
        ringer: Option<Field>,
        last_hook: Option<bool>,
        last_mute: bool,
        written: Option<LineState>,
    }

    fn open_all(api: &mut HidApi) -> (Vec<Open>, Vec<HeadsetInfo>) {
        let _ = api.refresh_devices();
        let mut opened = Vec::new();
        let mut infos = Vec::new();
        for d in api.device_list() {
            let product = d.product_string().unwrap_or("").to_string();
            if !super::candidate(d.usage_page(), d.vendor_id(), &product) {
                continue;
            }
            let named = super::looks_like_headset(d.vendor_id(), &product);
            let name = if product.is_empty() { "USB headset".to_string() } else { product };
            let vendor = d.manufacturer_string().unwrap_or("").to_string();
            let Ok(dev) = d.open_device(api) else {
                // Some software keeps a headset to itself; it is still a headset, just without buttons here.
                if named || d.usage_page() == hid_desc::PAGE_TELEPHONY {
                    super::add_info(&mut infos, name, vendor, false);
                }
                continue;
            };
            let mut buf = [0u8; 4096];
            let layout = match dev.get_report_descriptor(&mut buf) {
                Ok(n) => hid_desc::parse(&buf[..n]),
                Err(_) => Layout::default(),
            };
            let hook = layout.find(Kind::Input, hid_desc::PAGE_TELEPHONY, hid_desc::TEL_HOOK_SWITCH);
            let mute = layout.find(Kind::Input, hid_desc::PAGE_TELEPHONY, hid_desc::TEL_PHONE_MUTE);
            let led_off = layout.find(Kind::Output, hid_desc::PAGE_LED, hid_desc::LED_OFF_HOOK);
            let led_ring = layout.find(Kind::Output, hid_desc::PAGE_LED, hid_desc::LED_RING);
            let led_mute = layout.find(Kind::Output, hid_desc::PAGE_LED, hid_desc::LED_MUTE);
            let ringer = layout.find(Kind::Output, hid_desc::PAGE_TELEPHONY, hid_desc::TEL_RINGER);
            let buttons = hook.is_some() || mute.is_some();
            let telephony = buttons || led_off.is_some() || led_ring.is_some() || ringer.is_some();
            if telephony || named || d.usage_page() == hid_desc::PAGE_TELEPHONY {
                super::add_info(&mut infos, name, vendor, buttons);
            }
            if !telephony {
                continue;
            }
            let _ = dev.set_blocking_mode(false);
            opened.push(Open {
                led_off,
                led_ring,
                led_mute,
                ringer,
                dev,
                layout,
                hook,
                mute,
                last_hook: None,
                last_mute: false,
                written: None,
            });
        }
        (opened, infos)
    }

    /// Writes the lights. Fields can live in different output reports, so each report is sent once.
    fn write(o: &mut Open, st: LineState) {
        let fields: Vec<(Field, bool)> = [
            (o.led_off, st.offhook),
            (o.led_ring, st.ringing),
            (o.led_mute, st.muted),
            (o.ringer, st.ringing),
        ]
        .into_iter()
        .filter_map(|(f, v)| f.map(|f| (f, v)))
        .collect();
        let mut ids: Vec<u8> = fields.iter().map(|(f, _)| f.report_id).collect();
        ids.dedup();
        for id in ids {
            let len = o.layout.len(Kind::Output, id);
            if len == 0 {
                continue;
            }
            let mut buf = vec![0u8; len + 1];
            buf[0] = id;
            for (f, v) in fields.iter().filter(|(f, _)| f.report_id == id) {
                hid_desc::set(&mut buf[1..], f, *v as u32);
            }
            let _ = o.dev.write(&buf);
        }
        o.written = Some(st);
    }

    pub fn run(shared: Shared, emit: impl Fn(&'static str)) {
        let Ok(mut api) = HidApi::new() else { return };
        let mut devs: Vec<Open> = Vec::new();
        let mut scanned = Instant::now() - Duration::from_secs(60);
        let mut buf = [0u8; 64];
        loop {
            if scanned.elapsed() > Duration::from_secs(5) {
                // Rescan: a headset can be plugged in or out at any time.
                let (d, infos) = open_all(&mut api);
                devs = d;
                *shared.found.lock().unwrap() = infos;
                scanned = Instant::now();
            }
            if devs.is_empty() {
                std::thread::sleep(Duration::from_millis(500));
                continue;
            }
            let st = *shared.state.lock().unwrap();
            let mut broken = false;
            for o in devs.iter_mut() {
                if o.written != Some(st) {
                    write(o, st);
                }
                loop {
                    match o.dev.read(&mut buf) {
                        Ok(0) => break,
                        Ok(n) => {
                            // Reports start with the report ID when the device uses IDs.
                            let (id, data) = if o.layout.fields.iter().any(|f| f.report_id != 0) {
                                (buf[0], &buf[1..n])
                            } else {
                                (0, &buf[..n])
                            };
                            if let Some(h) = o.hook.filter(|f| f.report_id == id) {
                                let now = hid_desc::get(data, &h) != 0;
                                if let Some(a) = o.last_hook.and_then(|was| action_name(was, now)) {
                                    emit(a);
                                } else if o.last_hook.is_none() && now && st.ringing {
                                    emit("hook-off");
                                }
                                o.last_hook = Some(now);
                            }
                            if let Some(m) = o.mute.filter(|f| f.report_id == id) {
                                let now = hid_desc::get(data, &m) != 0;
                                if now && !o.last_mute {
                                    emit("mute");
                                }
                                o.last_mute = now;
                            }
                        }
                        Err(_) => {
                            broken = true;
                            break;
                        }
                    }
                }
            }
            if broken {
                scanned = Instant::now() - Duration::from_secs(60);
            }
            std::thread::sleep(Duration::from_millis(40));
        }
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn hook_transitions() {
        assert_eq!(super::action_name(false, true), Some("hook-off"));
        assert_eq!(super::action_name(true, false), Some("hook-on"));
        assert_eq!(super::action_name(true, true), None);
    }

    #[test]
    fn recognises_headsets() {
        use super::looks_like_headset as h;
        assert!(h(0x046D, "Logitech USB Headset"));
        assert!(h(0x046D, "Logitech Zone Wired"));
        assert!(h(0x046D, "Zone Vibe 100"));
        assert!(h(0x046D, "Logitech H570e Stereo"));
        assert!(h(0x046D, "H390 USB Headset"));
        assert!(h(0x046D, "G435 Wireless Gaming Headset"));
        assert!(h(0x046D, "PRO X Wireless"));
        assert!(h(0x0B0E, "Jabra Evolve2 65"));
        assert!(h(0x1234, "Some Bluetooth Hands-Free"));
        assert!(!h(0x046D, "USB Receiver"));
        assert!(!h(0x046D, "MX Master 3S"));
        assert!(!h(0x046D, "G502 HERO Gaming Mouse"));
        assert!(!h(0x046D, "K120 Keyboard"));
        assert!(!h(0x1234, "USB Keyboard"));
    }

    #[test]
    fn picks_collections() {
        use super::candidate as c;
        assert!(c(0x0B, 0x1234, "anything"));
        assert!(c(0x0C, 0x046D, "Logitech USB Headset"));
        assert!(!c(0x01, 0x046D, "Logitech USB Headset"));
        assert!(c(0xFF43, 0x046D, "Logitech Zone Wired"));
        assert!(!c(0xFF00, 0x046D, "USB Receiver"));
    }

    #[test]
    fn merges_infos() {
        let mut l = Vec::new();
        super::add_info(&mut l, "Zone".into(), "Logitech".into(), false);
        super::add_info(&mut l, "Zone".into(), "Logitech".into(), true);
        super::add_info(&mut l, "Zone".into(), "Logitech".into(), false);
        assert_eq!(l.len(), 1);
        assert!(l[0].buttons);
    }
}
