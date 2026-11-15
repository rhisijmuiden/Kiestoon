//! Updates from the GitHub releases of Kiestoon. Production gets stable releases; testing also
//! gets release candidates. The MSI is checked against the release's SHA256SUMS before it runs.

use futures_util::StreamExt;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::path::PathBuf;
use std::time::Duration;

#[derive(Debug, Deserialize)]
struct GhAsset {
    name: String,
    browser_download_url: String,
    size: u64,
}

#[derive(Debug, Deserialize)]
struct GhRelease {
    tag_name: String,
    #[serde(default)]
    body: Option<String>,
    #[serde(default)]
    published_at: Option<String>,
    html_url: String,
    draft: bool,
    prerelease: bool,
    assets: Vec<GhAsset>,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Release {
    pub version: String,
    pub notes: String,
    pub published_at: String,
    pub url: String,
    #[serde(skip)]
    pub msi_url: String,
    #[serde(skip)]
    pub msi_name: String,
    #[serde(skip)]
    pub msi_size: u64,
    #[serde(skip)]
    pub sums_url: String,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Check {
    pub current: String,
    pub channel: String,
    pub latest: Option<Release>,
    pub available: bool,
}

fn client() -> Result<reqwest::Client, String> {
    reqwest::Client::builder()
        .user_agent(concat!("KiestoonSoftphone/", env!("CARGO_PKG_VERSION")))
        .timeout(Duration::from_secs(600))
        .connect_timeout(Duration::from_secs(15))
        .build()
        .map_err(|e| e.to_string())
}

pub fn parse_version(v: &str) -> Option<semver::Version> {
    semver::Version::parse(v.trim().trim_start_matches('v')).ok()
}

/// Is this the softphone installer of a release?
fn is_msi(name: &str) -> bool {
    let n = name.to_ascii_lowercase();
    n.starts_with("kiestoon-softphone") && n.ends_with("_x64.msi")
}

fn pick(releases: Vec<GhRelease>, channel: &str) -> Option<Release> {
    releases
        .into_iter()
        .filter(|r| !r.draft && (channel == "testing" || !r.prerelease))
        .filter_map(|r| {
            let v = parse_version(&r.tag_name)?;
            let msi = r.assets.iter().find(|a| is_msi(&a.name))?;
            let sums = r.assets.iter().find(|a| a.name == "SHA256SUMS")?;
            Some((v, Release {
                version: r.tag_name.clone(),
                notes: r.body.clone().unwrap_or_default(),
                published_at: r.published_at.clone().unwrap_or_default(),
                url: r.html_url.clone(),
                msi_url: msi.browser_download_url.clone(),
                msi_name: msi.name.clone(),
                msi_size: msi.size,
                sums_url: sums.browser_download_url.clone(),
            }))
        })
        .max_by(|a, b| a.0.cmp(&b.0))
        .map(|(_, r)| r)
}

pub async fn check(repo: &str, channel: &str, current: &str) -> Result<Check, String> {
    let res = client()?
        .get(format!("https://api.github.com/repos/{repo}/releases?per_page=30"))
        .header("Accept", "application/vnd.github+json")
        .send()
        .await
        .map_err(|e| format!("network: {e}"))?;
    if !res.status().is_success() {
        return Err(format!("github {}", res.status()));
    }
    let list: Vec<GhRelease> = res.json().await.map_err(|e| e.to_string())?;
    let latest = pick(list, channel);
    let cur = parse_version(current);
    let available = match (&latest, &cur) {
        (Some(l), Some(c)) => parse_version(&l.version).is_some_and(|v| v > *c),
        (Some(_), None) => true,
        _ => false,
    };
    Ok(Check { current: current.to_string(), channel: channel.to_string(), latest, available })
}

/// Finds the checksum of a file in a SHA256SUMS file ("<hex>  <name>" per line).
pub fn sum_for(sums: &str, name: &str) -> Option<String> {
    sums.lines().find_map(|l| {
        let mut parts = l.split_whitespace();
        let sum = parts.next()?;
        let file = parts.next()?.trim_start_matches('*');
        (file == name && sum.len() == 64).then(|| sum.to_ascii_lowercase())
    })
}

/// Downloads the MSI of a release and checks it. Reports progress in percent.
pub async fn download(r: &Release, progress: impl Fn(u8)) -> Result<PathBuf, String> {
    let c = client()?;
    let sums = c.get(&r.sums_url).send().await.map_err(|e| e.to_string())?.text().await.map_err(|e| e.to_string())?;
    let want = sum_for(&sums, &r.msi_name).ok_or("no checksum for the installer")?;
    let res = c.get(&r.msi_url).send().await.map_err(|e| e.to_string())?;
    if !res.status().is_success() {
        return Err(format!("download {}", res.status()));
    }
    let total = res.content_length().unwrap_or(r.msi_size).max(1);
    let mut stream = res.bytes_stream();
    let mut data: Vec<u8> = Vec::with_capacity(total as usize);
    let mut last = 0u8;
    while let Some(chunk) = stream.next().await {
        let chunk = chunk.map_err(|e| e.to_string())?;
        data.extend_from_slice(&chunk);
        let pct = ((data.len() as u64 * 99) / total).min(99) as u8;
        if pct != last {
            last = pct;
            progress(pct);
        }
    }
    let got = hex::encode(Sha256::digest(&data));
    if got != want {
        return Err("the installer does not match its checksum".into());
    }
    let path = std::env::temp_dir().join(&r.msi_name);
    tokio::fs::write(&path, &data).await.map_err(|e| e.to_string())?;
    progress(100);
    Ok(path)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rel(tag: &str, pre: bool) -> GhRelease {
        GhRelease {
            tag_name: tag.into(), body: None, published_at: None, html_url: String::new(), draft: false, prerelease: pre,
            assets: vec![
                GhAsset { name: format!("Kiestoon-Softphone_{tag}_x64.msi"), browser_download_url: "u".into(), size: 1 },
                GhAsset { name: "SHA256SUMS".into(), browser_download_url: "s".into(), size: 1 },
            ],
        }
    }

    #[test]
    fn channels() {
        let list = || vec![rel("v0.7.0-rc.2", true), rel("v0.6.0", false), rel("v0.7.0-rc.1", true)];
        assert_eq!(pick(list(), "production").unwrap().version, "v0.6.0");
        assert_eq!(pick(list(), "testing").unwrap().version, "v0.7.0-rc.2");
    }

    #[test]
    fn skips_releases_without_installer() {
        let mut r = rel("v0.8.0", false);
        r.assets.retain(|a| a.name == "SHA256SUMS");
        assert_eq!(pick(vec![r, rel("v0.6.0", false)], "production").unwrap().version, "v0.6.0");
    }

    #[test]
    fn versions_and_sums() {
        assert!(parse_version("v0.7.0").unwrap() > parse_version("v0.7.0-rc.9").unwrap());
        assert!(parse_version("v0.7.0-rc.10").unwrap() > parse_version("v0.7.0-rc.9").unwrap());
        let sums = format!("{}  kiestoon_0.7.0_amd64.deb\n{}  Kiestoon-Softphone_v0.7.0_x64.msi\n", "a".repeat(64), "B".repeat(64));
        assert_eq!(sum_for(&sums, "Kiestoon-Softphone_v0.7.0_x64.msi"), Some("b".repeat(64)));
        assert_eq!(sum_for(&sums, "other"), None);
    }
}
