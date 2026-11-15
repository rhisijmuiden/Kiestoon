---
title: Handleiding Kiestoon
hide:
  - navigation
  - toc
---

<div class="kt-hero" markdown>

# Kiestoon

De open source telefooncentrale voor het mkb. Eén Debian 13-machine of Proxmox-container draait Asterisk 22 LTS
met een modern webportaal: toestellen, bureautelefoons die zichzelf instellen, een visuele keuzemenu-bouwer,
wachtrijen, een schakelbord, een gebruikersportaal met doorverbinden door slepen, vestigingen, hoge
beschikbaarheid en meer. In het Nederlands en Engels.

[Kiestoon installeren](install.md){ .md-button .md-button--primary }
[Eerste inrichting](first-setup.md){ .md-button }
[Wat kan het?](#wat-kan-kiestoon){ .md-button }

</div>

![Het Kiestoon-dashboard met live gesprekken](shot:dashboard)

## Wat kan Kiestoon?

<div class="grid cards" markdown>

-   :material-phone-classic: **Telefonie**

    ---

    Toestellen met voicemail, belgroepen, wachtrijen, vergaderruimtes, tijdcondities met Nederlandse feestdagen,
    gespreksopname, fax-naar-mail en mail-naar-fax.

    [:octicons-arrow-right-24: Toestellen](extensions.md)

-   :material-sitemap: **Visuele keuzemenu-bouwer**

    ---

    Sleep stappen op een canvas en trek lijnen ertussen. Elke stap heeft eigen audio: gegenereerd uit tekst,
    opgenomen in de browser of geüpload. Test het met één klik.

    [:octicons-arrow-right-24: Keuzemenu (IVR)](ivr.md)

-   :material-deskphone: **Telefoons die zichzelf instellen**

    ---

    Yealink, Snom, Grandstream, Fanvil, Gigaset, Poly, Htek, Akuvox en Cisco. Visuele toetseneditor,
    uitbreidingsmodules, toetsensjablonen, firmware en zero-touch provisioning.

    [:octicons-arrow-right-24: Telefoons](phones.md)

-   :material-account-group: **Receptie en gebruikersportaal**

    ---

    Een live schakelbord en wallboard, een gebruikersportaal met doorschakelen, voicemail, collega's met
    beschikbaarheid en doorverbinden door slepen, een browser-softphone en een Windows-softphone.

    [:octicons-arrow-right-24: Gebruikersportaal](portal.md)

-   :material-office-building-marker: **Meerdere locaties en hoge beschikbaarheid**

    ---

    Vestigingen via Kiestoon Edge (zonder port forwarding), gekoppelde Kiestoon-centrales met één telefoonboek, en
    een actief/passief-paar dat overneemt als een node uitvalt.

    [:octicons-arrow-right-24: Vestigingen](edge.md)

-   :material-shield-lock: **Standaard veilig**

    ---

    Verplichte MFA voor beheerders, passkeys, SSO (Entra ID, Google, Keycloak, SAML), een beheerde firewall met
    veilige modus, fail2ban, geoblokkering en bescherming tegen belfraude.

    [:octicons-arrow-right-24: Beveiliging](users.md)

-   :material-backup-restore: **Back-ups en updates**

    ---

    Versleutelde volledige en incrementele back-ups naar FTP, FTPS, SFTP, SCP, S3, WebDAV, SMB of OneDrive, herstel
    vanaf de externe locatie, en updates met één klik op een productie- of testkanaal.

    [:octicons-arrow-right-24: Back-ups](backups.md)

-   :material-api: **Koppelingen**

    ---

    Een REST-API met tokens, ondertekende webhooks (inkomend gesprek naar je CRM), een XML- en LDAP-telefoonboek,
    Prometheus-metrics en een eigen SMTP-server met OAuth2 voor Microsoft 365 en Google.

    [:octicons-arrow-right-24: API en webhooks](api.md)

</div>

## Waar begin je?

1. [Installeer Kiestoon](install.md) in een Proxmox-container of op een Debian 13-machine.
2. Volg de [eerste inrichting](first-setup.md): beheerder, nummerplan, trunk, toestellen en telefoons.
3. Lees [hoe Kiestoon werkt](concepts.md) voor de ideeën achter bestemmingen, rollen en het nummerplan.

!!! tip "Hulp vanuit het portaal"
    Elke pagina in het Kiestoon-portaal heeft bovenaan een knop **Help**. Die opent de bijbehorende pagina van deze
    handleiding, in de taal waarin je het portaal gebruikt.
