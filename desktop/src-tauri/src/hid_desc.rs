#![cfg_attr(not(windows), allow(dead_code))]
//! A small HID report descriptor parser: just enough to find where a headset reports its hook
//! switch and mute button, and where its off-hook, ring and mute lights are.

/// Usage pages and usages (HID Usage Tables 1.4).
pub const PAGE_GENERIC_DESKTOP: u16 = 0x01;
pub const PAGE_LED: u16 = 0x08;
pub const PAGE_CONSUMER: u16 = 0x0C;
pub const PAGE_TELEPHONY: u16 = 0x0B;
pub const TEL_HOOK_SWITCH: u16 = 0x20;
pub const TEL_PHONE_MUTE: u16 = 0x2F;
pub const TEL_RINGER: u16 = 0x9E;
pub const LED_MUTE: u16 = 0x09;
pub const LED_OFF_HOOK: u16 = 0x17;
pub const LED_RING: u16 = 0x18;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Kind {
    Input,
    Output,
}

/// One bit field of a report.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Field {
    pub kind: Kind,
    pub report_id: u8,
    /// Bit offset in the report, after the report ID byte.
    pub bit: usize,
    pub size: usize,
    pub page: u16,
    pub usage: u16,
}

#[derive(Debug, Default, Clone)]
pub struct Layout {
    pub fields: Vec<Field>,
    /// Report length in bytes (without report ID) per (kind, report ID).
    pub lengths: Vec<(Kind, u8, usize)>,
}

impl Layout {
    pub fn find(&self, kind: Kind, page: u16, usage: u16) -> Option<Field> {
        self.fields
            .iter()
            .copied()
            .find(|f| f.kind == kind && f.page == page && f.usage == usage)
    }

    pub fn len(&self, kind: Kind, report_id: u8) -> usize {
        self.lengths
            .iter()
            .find(|(k, id, _)| *k == kind && *id == report_id)
            .map(|x| x.2)
            .unwrap_or(0)
    }
}

#[derive(Clone, Copy, Default)]
struct Globals {
    page: u16,
    report_size: usize,
    report_count: usize,
    report_id: u8,
}

/// Parses a report descriptor. Unknown items are skipped; long items are ignored.
pub fn parse(desc: &[u8]) -> Layout {
    let mut out = Layout::default();
    let mut g = Globals::default();
    let mut stack: Vec<Globals> = Vec::new();
    let mut usages: Vec<(u16, u16)> = Vec::new();
    let mut usage_min: Option<u32> = None;
    // Bit offsets per (kind, report id).
    let mut offsets: Vec<(Kind, u8, usize)> = Vec::new();
    let offset_of = |offsets: &mut Vec<(Kind, u8, usize)>, k: Kind, id: u8| -> usize {
        if let Some(o) = offsets.iter().find(|(kk, ii, _)| *kk == k && *ii == id) {
            return o.2;
        }
        offsets.push((k, id, 0));
        0
    };
    let mut i = 0;
    while i < desc.len() {
        let prefix = desc[i];
        if prefix == 0xFE {
            // Long item: skip it.
            let size = *desc.get(i + 1).unwrap_or(&0) as usize;
            i += 3 + size;
            continue;
        }
        let size = match prefix & 0x03 {
            3 => 4,
            n => n as usize,
        };
        if i + 1 + size > desc.len() {
            break;
        }
        let data = &desc[i + 1..i + 1 + size];
        let mut v: u32 = 0;
        for (n, b) in data.iter().enumerate() {
            v |= (*b as u32) << (8 * n);
        }
        let typ = (prefix >> 2) & 0x03;
        let tag = prefix >> 4;
        match (typ, tag) {
            // Main items
            (0, 0x8) | (0, 0x9) => {
                let kind = if tag == 0x8 { Kind::Input } else { Kind::Output };
                let constant = v & 0x01 != 0;
                let variable = v & 0x02 != 0;
                let start = offset_of(&mut offsets, kind, g.report_id);
                if !constant && variable {
                    let mut list = usages.clone();
                    if let Some(min) = usage_min {
                        if list.is_empty() {
                            for n in 0..g.report_count as u32 {
                                let u = min + n;
                                list.push((if u > 0xFFFF { (u >> 16) as u16 } else { g.page }, u as u16));
                            }
                        }
                    }
                    for n in 0..g.report_count {
                        let Some(&(page, usage)) = list.get(n).or(list.last()) else { break };
                        out.fields.push(Field {
                            kind,
                            report_id: g.report_id,
                            bit: start + n * g.report_size,
                            size: g.report_size,
                            page,
                            usage,
                        });
                    }
                }
                let end = start + g.report_size * g.report_count;
                if let Some(o) = offsets.iter_mut().find(|(k, id, _)| *k == kind && *id == g.report_id) {
                    o.2 = end;
                }
                usages.clear();
                usage_min = None;
            }
            (0, _) => {
                // Feature, Collection, End Collection: the local items end here.
                usages.clear();
                usage_min = None;
            }
            // Global items
            (1, 0x0) => g.page = v as u16,
            (1, 0x7) => g.report_size = v as usize,
            (1, 0x8) => g.report_id = v as u8,
            (1, 0x9) => g.report_count = v as usize,
            (1, 0xA) => stack.push(g),
            (1, 0xB) => {
                if let Some(p) = stack.pop() {
                    g = p;
                }
            }
            // Local items
            (2, 0x0) => {
                if size == 4 {
                    usages.push(((v >> 16) as u16, v as u16));
                } else {
                    usages.push((g.page, v as u16));
                }
            }
            (2, 0x1) => usage_min = Some(v),
            _ => {}
        }
        i += 1 + size;
    }
    out.lengths = offsets.into_iter().map(|(k, id, bits)| (k, id, bits.div_ceil(8))).collect();
    out
}

/// Reads a field from an input report (`report` starts after the report ID byte).
pub fn get(report: &[u8], f: &Field) -> u32 {
    let mut v = 0u32;
    for n in 0..f.size.min(32) {
        let bit = f.bit + n;
        if let Some(b) = report.get(bit / 8) {
            if b & (1 << (bit % 8)) != 0 {
                v |= 1 << n;
            }
        }
    }
    v
}

/// Sets a one-bit (or wider) field in an output report buffer (without report ID byte).
pub fn set(report: &mut [u8], f: &Field, value: u32) {
    for n in 0..f.size.min(32) {
        let bit = f.bit + n;
        if let Some(b) = report.get_mut(bit / 8) {
            if value & (1 << n) != 0 {
                *b |= 1 << (bit % 8);
            } else {
                *b &= !(1 << (bit % 8));
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Telephony collection of a typical USB headset (simplified from a Jabra Evolve descriptor).
    const DESC: &[u8] = &[
        0x05, 0x0B, // Usage Page (Telephony)
        0x09, 0x05, // Usage (Headset)
        0xA1, 0x01, // Collection (Application)
        0x85, 0x02, //   Report ID (2)
        0x15, 0x00, 0x25, 0x01, // Logical min 0, max 1
        0x09, 0x20, //   Usage (Hook Switch)
        0x09, 0x97, //   Usage (Line Busy Tone)
        0x75, 0x01, 0x95, 0x02, // Report size 1, count 2
        0x81, 0x22, //   Input (Data, Var, Abs, No Preferred)
        0x09, 0x2F, //   Usage (Phone Mute)
        0x95, 0x01, //   Report count 1
        0x81, 0x06, //   Input (Data, Var, Rel)
        0x95, 0x05, 0x81, 0x01, // 5 bits padding (Constant)
        0x05, 0x08, //   Usage Page (LED)
        0x85, 0x03, //   Report ID (3)
        0x09, 0x17, //   Usage (Off-Hook)
        0x09, 0x18, //   Usage (Ring)
        0x09, 0x09, //   Usage (Mute)
        0x95, 0x03, 0x91, 0x22, // Output (Data, Var, Abs)
        0x95, 0x05, 0x91, 0x01, // padding
        0xC0, // End Collection
    ];

    #[test]
    fn finds_hook_mute_and_leds() {
        let l = parse(DESC);
        let hook = l.find(Kind::Input, PAGE_TELEPHONY, TEL_HOOK_SWITCH).unwrap();
        assert_eq!((hook.report_id, hook.bit, hook.size), (2, 0, 1));
        let mute = l.find(Kind::Input, PAGE_TELEPHONY, TEL_PHONE_MUTE).unwrap();
        assert_eq!(mute.bit, 2);
        let off = l.find(Kind::Output, PAGE_LED, LED_OFF_HOOK).unwrap();
        let ring = l.find(Kind::Output, PAGE_LED, LED_RING).unwrap();
        let led_mute = l.find(Kind::Output, PAGE_LED, LED_MUTE).unwrap();
        assert_eq!((off.report_id, off.bit, ring.bit, led_mute.bit), (3, 0, 1, 2));
        assert_eq!(l.len(Kind::Output, 3), 1);
        assert_eq!(l.len(Kind::Input, 2), 1);
    }

    #[test]
    fn get_and_set_bits() {
        let l = parse(DESC);
        let mute = l.find(Kind::Input, PAGE_TELEPHONY, TEL_PHONE_MUTE).unwrap();
        assert_eq!(get(&[0b0000_0100], &mute), 1);
        assert_eq!(get(&[0b0000_0001], &mute), 0);
        let ring = l.find(Kind::Output, PAGE_LED, LED_RING).unwrap();
        let mut buf = [0u8; 1];
        set(&mut buf, &ring, 1);
        assert_eq!(buf[0], 0b10);
        set(&mut buf, &ring, 0);
        assert_eq!(buf[0], 0);
    }

    #[test]
    fn survives_garbage() {
        let _ = parse(&[0xFE, 0x10]);
        let _ = parse(&[0x27, 0x01]);
        let _ = parse(&[]);
    }
}
