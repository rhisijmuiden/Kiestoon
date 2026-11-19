---
title: Kiestoon manual
hide:
  - navigation
  - toc
---

<div class="kt-hero" markdown>

# Kiestoon

The open source phone system for small and medium businesses. One Debian 13 machine or Proxmox container runs
Asterisk 22 LTS with a modern web portal: extensions, desk phones that configure themselves, a visual IVR builder,
queues, a switchboard, a user portal with drag-and-drop transfer, branch offices, high availability and more.
In English and Dutch.

[Install Kiestoon](install.md){ .md-button .md-button--primary }
[First setup](first-setup.md){ .md-button }
[What can it do?](#what-can-kiestoon-do){ .md-button }

</div>

![The Kiestoon dashboard with live calls](shot:dashboard)

## What can Kiestoon do?

<div class="grid cards" markdown>

-   :material-phone-classic: **Telephony**

    ---

    Extensions with voicemail, ring groups, call queues, conference rooms, time conditions with Dutch holidays,
    call recording, fax-to-mail and mail-to-fax.

    [:octicons-arrow-right-24: Extensions](extensions.md)

-   :material-sitemap: **Visual IVR builder**

    ---

    Drag steps onto a canvas and draw lines between them. Every step has its own audio: generated from text,
    recorded in the browser or uploaded. Test it with one click.

    [:octicons-arrow-right-24: IVR builder](ivr.md)

-   :material-deskphone: **Phones that set themselves up**

    ---

    Yealink, Snom, Grandstream, Fanvil, Gigaset, Poly, Htek, Akuvox and Cisco. Visual key editor, expansion
    modules, key templates, firmware and zero-touch provisioning.

    [:octicons-arrow-right-24: Phones](phones.md)

-   :material-account-group: **Reception and user portal**

    ---

    A live switchboard and wallboard, a user portal with forwarding, voicemail, colleagues with presence and
    transfer by drag and drop, a browser softphone and a Windows softphone.

    [:octicons-arrow-right-24: User portal](portal.md)

-   :material-office-building-marker: **Multi-site and high availability**

    ---

    Branch offices through Kiestoon Edge (no port forwarding), linked Kiestoon systems with one phone book, and
    an active/passive pair that takes over when one node fails.

    [:octicons-arrow-right-24: Branch offices](edge.md)

-   :material-shield-lock: **Secure by default**

    ---

    Mandatory MFA for admins, passkeys, SSO (Entra ID, Google, Keycloak, SAML), a managed firewall with safe
    mode, fail2ban, geo-blocking and outbound fraud protection.

    [:octicons-arrow-right-24: Security](users.md)

-   :material-backup-restore: **Backups and updates**

    ---

    Encrypted full and incremental backups to FTP, FTPS, SFTP, SCP, S3, WebDAV, SMB or OneDrive, restore from the
    remote location, and one-click updates on a production or testing channel.

    [:octicons-arrow-right-24: Backups](backups.md)

-   :material-api: **Integrations**

    ---

    A REST API with tokens, signed webhooks (incoming call to your CRM), an XML and LDAP phone book, Prometheus
    metrics and a custom SMTP server with OAuth2 for Microsoft 365 and Google.

    [:octicons-arrow-right-24: API and webhooks](api.md)

</div>

## Where to start

1. [Install Kiestoon](install.md) on a Proxmox container or a Debian 13 machine.
2. Follow the [first setup](first-setup.md): administrator, number plan, trunk, extensions and phones.
3. Read [how Kiestoon works](concepts.md) for the ideas behind destinations, roles and the number plan.

!!! tip "Help from inside the portal"
    Every page in the Kiestoon portal has a **Help** button at the top. It opens the matching page of this manual,
    in the language you use the portal in.
