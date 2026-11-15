//! WebView2 settings that Tauri does not expose: the microphone is allowed for the linked server
//! without a prompt, and a self-signed server certificate is accepted when it is exactly the pinned
//! one (the --ignore-certificate-errors-spki-list browser argument covers the rest).

use base64::Engine;
use tauri::{Runtime, WebviewWindow};
use webview2_com::Microsoft::Web::WebView2::Win32::*;
use webview2_com::{take_pwstr, PermissionRequestedEventHandler, ServerCertificateErrorDetectedEventHandler};
use windows::core::{Interface, PWSTR};

fn origin_of(uri: &str) -> String {
    url::Url::parse(uri).map(|u| u.origin().ascii_serialization()).unwrap_or_default()
}

fn der_from_pem(pem: &str) -> Option<Vec<u8>> {
    let body: String = pem.lines().filter(|l| !l.starts_with("-----")).collect();
    base64::engine::general_purpose::STANDARD.decode(body.trim()).ok()
}

pub fn install<R: Runtime>(win: &WebviewWindow<R>, origin: String, fingerprint: String) {
    if origin.is_empty() {
        return;
    }
    let _ = win.with_webview(move |wv| unsafe {
        let Ok(core) = wv.controller().CoreWebView2() else { return };
        let mut token = Default::default();

        let o = origin.clone();
        let _ = core.add_PermissionRequested(
            &PermissionRequestedEventHandler::create(Box::new(move |_, args| {
                let Some(args) = args else { return Ok(()) };
                let mut uri = PWSTR::null();
                args.Uri(&mut uri)?;
                let uri = take_pwstr(uri);
                let mut kind = COREWEBVIEW2_PERMISSION_KIND::default();
                args.PermissionKind(&mut kind)?;
                if origin_of(&uri) == o
                    && (kind == COREWEBVIEW2_PERMISSION_KIND_MICROPHONE || kind == COREWEBVIEW2_PERMISSION_KIND_AUTOPLAY)
                {
                    args.SetState(COREWEBVIEW2_PERMISSION_STATE_ALLOW)?;
                }
                Ok(())
            })),
            &mut token,
        );

        if fingerprint.is_empty() {
            return;
        }
        let Ok(core14) = core.cast::<ICoreWebView2_14>() else { return };
        let o = origin.clone();
        let _ = core14.add_ServerCertificateErrorDetected(
            &ServerCertificateErrorDetectedEventHandler::create(Box::new(move |_, args| {
                let Some(args) = args else { return Ok(()) };
                let mut uri = PWSTR::null();
                args.RequestUri(&mut uri)?;
                if origin_of(&take_pwstr(uri)) != o {
                    return Ok(());
                }
                let cert = args.ServerCertificate()?;
                let mut pem = PWSTR::null();
                cert.ToPemEncoding(&mut pem)?;
                let ok = der_from_pem(&take_pwstr(pem)).is_some_and(|der| crate::pin::fingerprint(&der).eq_ignore_ascii_case(&fingerprint));
                if ok {
                    args.SetAction(COREWEBVIEW2_SERVER_CERTIFICATE_ERROR_ACTION_ALWAYS_ALLOW)?;
                }
                Ok(())
            })),
            &mut token,
        );
    });
}
