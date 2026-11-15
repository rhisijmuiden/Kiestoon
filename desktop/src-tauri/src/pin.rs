//! Finding out who a Kiestoon server is before the softphone trusts it. A server with a public
//! certificate (internet mode) is checked the normal way. A server on the LAN uses its own
//! self-signed certificate: then the softphone pins exactly that certificate (its SHA-256
//! fingerprint, the same one the portal shows) and trusts nothing else for that address.

use base64::Engine;
use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::client::WebPkiServerVerifier;
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{DigitallySignedStruct, Error, SignatureScheme};
use serde::Serialize;
use sha2::{Digest, Sha256};
use std::sync::{Arc, Mutex};
use std::time::Duration;

#[derive(Debug, Clone, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct Probe {
    pub origin: String,
    /// The certificate is valid for this address and signed by a trusted authority.
    pub trusted: bool,
    /// Hex SHA-256 of the certificate (DER).
    pub fingerprint: String,
    /// Base64 SHA-256 of the certificate's public key.
    pub spki: String,
    /// Version of the Kiestoon server.
    pub version: String,
}

pub fn fingerprint(der: &[u8]) -> String {
    hex::encode(Sha256::digest(der))
}

pub fn spki_hash(der: &[u8]) -> Option<String> {
    let (_, cert) = x509_parser::parse_x509_certificate(der).ok()?;
    Some(base64::engine::general_purpose::STANDARD.encode(Sha256::digest(cert.tbs_certificate.subject_pki.raw)))
}

/// Accepts any certificate, but records it and whether normal verification would have passed.
#[derive(Debug)]
struct Recorder {
    inner: Arc<WebPkiServerVerifier>,
    seen: Mutex<Option<(Vec<u8>, bool)>>,
    /// When set, only a certificate with this fingerprint is accepted if it is not trusted.
    pin: Option<String>,
}

impl ServerCertVerifier for Recorder {
    fn verify_server_cert(
        &self,
        end: &CertificateDer<'_>,
        inter: &[CertificateDer<'_>],
        name: &ServerName<'_>,
        ocsp: &[u8],
        now: UnixTime,
    ) -> Result<ServerCertVerified, Error> {
        let trusted = self.inner.verify_server_cert(end, inter, name, ocsp, now).is_ok();
        *self.seen.lock().unwrap() = Some((end.to_vec(), trusted));
        if let Some(pin) = &self.pin {
            if !trusted && !pin.eq_ignore_ascii_case(&fingerprint(end)) {
                return Err(Error::General("certificate does not match the pinned fingerprint".into()));
            }
        }
        Ok(ServerCertVerified::assertion())
    }
    fn verify_tls12_signature(
        &self,
        m: &[u8],
        c: &CertificateDer<'_>,
        d: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, Error> {
        rustls::crypto::verify_tls12_signature(m, c, d, &rustls::crypto::ring::default_provider().signature_verification_algorithms)
    }
    fn verify_tls13_signature(
        &self,
        m: &[u8],
        c: &CertificateDer<'_>,
        d: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, Error> {
        rustls::crypto::verify_tls13_signature(m, c, d, &rustls::crypto::ring::default_provider().signature_verification_algorithms)
    }
    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        rustls::crypto::ring::default_provider()
            .signature_verification_algorithms
            .supported_schemes()
    }
}

fn roots() -> rustls::RootCertStore {
    let mut store = rustls::RootCertStore::empty();
    for c in rustls_native_certs::load_native_certs().certs {
        let _ = store.add(c);
    }
    store
}

/// Connects to the server, records its certificate and asks it for /api/desktop/info.
pub async fn probe(origin: &str, pin: Option<String>) -> Result<Probe, String> {
    let provider = Arc::new(rustls::crypto::ring::default_provider());
    let inner = WebPkiServerVerifier::builder_with_provider(Arc::new(roots()), provider.clone())
        .build()
        .map_err(|e| e.to_string())?;
    let rec = Arc::new(Recorder {
        inner,
        seen: Mutex::new(None),
        pin,
    });
    let tls = rustls::ClientConfig::builder_with_provider(provider)
        .with_safe_default_protocol_versions()
        .map_err(|e| e.to_string())?
        .dangerous()
        .with_custom_certificate_verifier(rec.clone())
        .with_no_client_auth();
    let client = reqwest::Client::builder()
        .use_preconfigured_tls(tls)
        .timeout(Duration::from_secs(10))
        .user_agent(concat!("KiestoonSoftphone/", env!("CARGO_PKG_VERSION")))
        .build()
        .map_err(|e| e.to_string())?;
    let res = client.get(format!("{origin}/api/desktop/info")).send().await.map_err(|e| {
        if e.is_connect() || e.is_timeout() {
            "unreachable".to_string()
        } else {
            format!("tls: {e}")
        }
    })?;
    if !res.status().is_success() {
        return Err(if res.status().as_u16() == 404 {
            "not_kiestoon".into()
        } else {
            format!("http {}", res.status())
        });
    }
    let info: serde_json::Value = res.json().await.map_err(|_| "not_kiestoon".to_string())?;
    let (der, trusted) = rec.seen.lock().unwrap().clone().ok_or("no certificate")?;
    Ok(Probe {
        origin: origin.to_string(),
        trusted,
        fingerprint: fingerprint(&der),
        spki: spki_hash(&der).unwrap_or_default(),
        version: info.get("version").and_then(|v| v.as_str()).unwrap_or_default().to_string(),
    })
}

/// "ab12cd…" -> "AB:12:CD:…", as the portal shows it.
pub fn pretty(fp: &str) -> String {
    fp.as_bytes()
        .chunks(2)
        .map(|c| String::from_utf8_lossy(c).to_uppercase())
        .collect::<Vec<_>>()
        .join(":")
}

#[cfg(test)]
mod tests {
    #[test]
    fn pretty_fingerprint() {
        assert_eq!(super::pretty("ab12cd"), "AB:12:CD");
    }
}
