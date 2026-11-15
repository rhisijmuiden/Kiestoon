# Mail server

Kiestoon sends e-mail for voicemail, faxes, welcome mails, fraud alerts and failed backups. It uses
your own SMTP server, so the mail comes from your own domain.

![Mail server](shot:mail)

## Set it up

1. Go to **Mail server** and pick a **Quick setup**: **Microsoft 365**, **Google Workspace** or **Other server**.
2. Check the **Host**, **Port** and **Encryption**:

    | Encryption | Port | Use |
    | --- | --- | --- |
    | **STARTTLS** | 587 | Most servers, Microsoft 365 |
    | **TLS** | 465 | Gmail, many hosting providers |
    | **None** | 25 | Only an internal relay. Passwords and messages travel readable over the network. |

3. Choose how to **Sign in**:
    - **Username and password**: classic SMTP AUTH. Use an app password where the provider offers one.
    - **OAuth2 · Microsoft 365**: for tenants where basic authentication is switched off. Register an app in
      Microsoft Entra with the application permission `SMTP.SendAsApp` and admin consent, give it access to the
      mailbox in Exchange Online, and fill in the **Mailbox**, **Tenant ID**, **Client ID** and **Client secret**.
    - **OAuth2 · Google Workspace**: an OAuth client in Google Cloud with a **Refresh token** for the mailbox
      (scope `https://mail.google.com/`).
    - **None**: for a relay that accepts mail from this server's address.
4. Fill in the **From address** and **From name**, for example `pbx@example.com` and "Kiestoon".
5. **Send alerts to**: the address for failed backups, fraud alerts and other problems.
6. Switch **Sending on** and save.
7. Under **Send a test**, fill in a recipient and click **Send test**.

**Do not check the certificate** is only for an internal server with its own certificate.

## The mail log

**Recent e-mails** shows every message with time, recipient and result. While sending is off, or when the server
cannot be reached, e-mails wait in a queue and are sent as soon as it works again; the page shows how many are
waiting.

## What Kiestoon e-mails

| E-mail | Set up in |
| --- | --- |
| Voicemail with the audio, and a transcription with speech-to-text | [Voicemail](voicemail.md) |
| Received faxes as PDF | [Fax](fax.md) |
| Welcome mail with a set-password link | [Users](users.md#add-employees) |
| Fraud alerts | [Fraud protection](firewall.md#fraud-protection) |
| Failed backups | [Backups](backups.md) |
