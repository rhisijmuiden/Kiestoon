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
            if d.usage_page() != hid_desc::PAGE_TELEPHONY {
                continue;
            }
            let name = d.product_string().unwrap_or("USB headset").to_string();
            let vendor = d.manufacturer_string().unwrap_or("").to_string();
            let Ok(dev) = d.open_device(api) else { continue };
            let mut buf = [0u8; 4096];
            let Ok(n) = dev.get_report_descriptor(&mut buf) else { continue };
            let layout = hid_desc::parse(&buf[..n]);
            let hook = layout.find(Kind::Input, hid_desc::PAGE_TELEPHONY, hid_desc::TEL_HOOK_SWITCH);
            let mute = layout.find(Kind::Input, hid_desc::PAGE_TELEPHONY, hid_desc::TEL_PHONE_MUTE);
            let buttons = hook.is_some() || mute.is_some();
            if !infos.iter().any(|i: &HeadsetInfo| i.name == name) {
                infos.push(HeadsetInfo { name, vendor, buttons });
            }
            let _ = dev.set_blocking_mode(false);
            opened.push(Open {
                led_off: layout.find(Kind::Output, hid_desc::PAGE_LED, hid_desc::LED_OFF_HOOK),
                led_ring: layout.find(Kind::Output, hid_desc::PAGE_LED, hid_desc::LED_RING),
                led_mute: layout.find(Kind::Output, hid_desc::PAGE_LED, hid_desc::LED_MUTE),
                ringer: layout.find(Kind::Output, hid_desc::PAGE_TELEPHONY, hid_desc::TEL_RINGER),
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
}
