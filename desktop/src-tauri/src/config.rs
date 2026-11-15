//! Settings of the softphone, in %APPDATA%\nl.rhis.kiestoon.softphone\config.json.

use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
#[serde(default, rename_all = "camelCase")]
pub struct Config {
    /// Origin of the Kiestoon portal, for example https://pbx.example.com or https://10.0.0.5:8443.
    pub server: String,
    /// Hex SHA-256 of the server certificate (DER), when it is not publicly trusted (self-signed).
    pub fingerprint: String,
    /// Base64 SHA-256 of that certificate's public key, for the WebView's certificate exception.
    pub spki: String,
    /// "production" or "testing".
    pub channel: String,
    /// A pairing code to use once the app has restarted with the new certificate exception.
    pub pending_code: String,
    /// GitHub repository with the releases ("owner/name").
    pub repo: String,
}

impl Config {
    pub fn channel(&self) -> &str {
        if self.channel == "testing" {
            "testing"
        } else {
            "production"
        }
    }

    pub fn repo(&self) -> &str {
        if self.repo.is_empty() {
            "rhisijmuiden/kiestoon"
        } else {
            &self.repo
        }
    }

    pub fn load(path: &PathBuf) -> Config {
        std::fs::read(path).ok().and_then(|b| serde_json::from_slice(&b).ok()).unwrap_or_default()
    }

    pub fn save(&self, path: &PathBuf) -> Result<(), String> {
        if let Some(dir) = path.parent() {
            std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
        }
        let tmp = path.with_extension("tmp");
        std::fs::write(&tmp, serde_json::to_vec_pretty(self).map_err(|e| e.to_string())?).map_err(|e| e.to_string())?;
        std::fs::rename(&tmp, path).map_err(|e| e.to_string())
    }
}

/// Turns what a user typed into an https origin: "pbx.example.com" -> "https://pbx.example.com".
pub fn normalize_origin(input: &str) -> Result<String, String> {
    let s = input.trim().trim_end_matches('/');
    if s.is_empty() {
        return Err("empty".into());
    }
    let with_scheme = if s.contains("://") { s.to_string() } else { format!("https://{s}") };
    let u = url::Url::parse(&with_scheme).map_err(|_| "invalid".to_string())?;
    if u.scheme() != "https" {
        return Err("https_required".into());
    }
    let host = u.host_str().ok_or("invalid")?;
    if !u.username().is_empty() || u.password().is_some() {
        return Err("invalid".into());
    }
    let host = if host.contains(':') && !host.starts_with('[') { format!("[{host}]") } else { host.to_string() };
    Ok(match u.port() {
        Some(p) => format!("https://{host}:{p}"),
        None => format!("https://{host}"),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn origins() {
        assert_eq!(normalize_origin("pbx.example.com").unwrap(), "https://pbx.example.com");
        assert_eq!(normalize_origin(" https://10.0.0.5:8443/ ").unwrap(), "https://10.0.0.5:8443");
        assert_eq!(normalize_origin("https://pbx.example.com/desktop").unwrap(), "https://pbx.example.com");
        assert!(normalize_origin("http://pbx.example.com").is_err());
        assert!(normalize_origin("").is_err());
        assert!(normalize_origin("https://user:pw@pbx").is_err());
    }

    #[test]
    fn defaults() {
        let c = Config::default();
        assert_eq!(c.channel(), "production");
        assert_eq!(c.repo(), "rhisijmuiden/kiestoon");
    }
}
