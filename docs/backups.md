# Backups and restore

Kiestoon makes **encrypted** backups to your own server or cloud storage, and restores them from the portal. It uses
[restic](https://restic.net): everything is encrypted on the Kiestoon server before it leaves, and stored
deduplicated, so daily backups take little space.

![Backups](shot:backups)

## What is in a backup

Everything needed to rebuild the phone system: the database with all settings, extensions, routes, IVRs, users
and call history, voicemail, recordings, faxes, voice prompts, certificates and the configuration in
`/etc/kiestoon`. Downloaded speech models and phone firmware are left out, because they can be downloaded again.

## Backup locations

Under **Backups › Add location** choose the protocol:

| Protocol | Notes |
| --- | --- |
| **FTP** | Plain FTP sends the password unencrypted. The backup itself is always encrypted. |
| **FTPS** | FTP over TLS. **Implicit TLS** is usually port 990; off means explicit TLS (AUTH TLS) on port 21. |
| **SFTP** | Over SSH, with a password or an **SSH private key** (OpenSSH, without passphrase). |
| **SCP** | Uses the same SSH connection as SFTP. |
| **S3** | Amazon S3, MinIO, Wasabi, Ceph, DigitalOcean, Scaleway, Hetzner, Cloudflare R2 or any other S3-compatible storage. The bucket must exist. |
| **WebDAV** | Nextcloud, ownCloud, SharePoint (NTLM) or another WebDAV server. For Nextcloud, use an app password. |
| **SMB** | A Windows share or a NAS (Synology, QNAP), SMB 2 or 3. |
| **OneDrive** | Microsoft OneDrive or OneDrive for Business, connected with a sign-in code. |

Fill in the host, port, username, password and **Folder**, and click **Test**. The first test creates a new
encrypted backup store on the location.

!!! danger "Store the encryption key"
    Kiestoon creates a strong **Encryption key** for every location. Without this key a backup cannot be restored
    on a new server. Open it with the key button on the location and store it in a password manager.

### OneDrive

1. Register an app in Microsoft Entra (*App registrations*) with **Allow public client flows** on and the
   delegated permissions `Files.ReadWrite` and `offline_access`.
2. Fill in its **Application (client) ID** and save.
3. Click **Connect**, open the Microsoft page that Kiestoon shows, sign in with the account the backups go to and
   enter the code.

## Full and incremental

- An **incremental** backup only reads files that changed since the previous snapshot. Fast and small.
- A **full** backup reads every file again, so damaged or missed files are picked up as well.

Every snapshot is a complete picture: any snapshot can be restored on its own, full or incremental. Start a backup
by hand with **Full** or **Incremental** on a location.

## Schedule and retention

With **Automatic backups** on, Kiestoon makes a backup every day at the chosen **Time**, and a full backup on the day
under **Full backup on**. It runs for every location that is on.

**Keep daily**, **Keep weekly** and **Keep monthly** set how many snapshots stay. With 7 / 4 / 12 you can go back a
week day by day, a month week by week and a year month by month.

A failed backup sends an e-mail to the alert address under [Mail server](mail.md). **Recent backups** shows every run
with its type, trigger, size and result.

## Restore

1. Click **Snapshots** on a location.
2. Pick the snapshot and click **Restore**.
3. Type `RESTORE` to confirm and click **Restore now**.

Everything on this server is replaced with the backup. Calls are interrupted and the portal restarts. The current
data is kept on the server as a safety copy.

### Restore on a new server

After a broken server, install Kiestoon again ([Install](install.md)), add the same location and paste the key
under **Existing encryption key**, click **Test**, and restore the newest snapshot. The new server then has the same extensions, phones and
settings; provisioned phones simply connect again.
