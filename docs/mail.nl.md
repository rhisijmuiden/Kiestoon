# Mailserver

Kiestoon stuurt e-mail voor voicemail, faxen, welkomstmails, meldingen bij fraude en mislukte back-ups. Het gebruikt
je eigen SMTP-server, dus de mail komt van je eigen domein.

![Mailserver](shot:mail)

## Instellen

1. Ga naar **Mailserver** en kies bij **Snel instellen**: **Microsoft 365**, **Google Workspace** of **Andere
   server**.
2. Controleer **Host**, **Poort** en **Versleuteling**:

    | Versleuteling | Poort | Gebruik |
    | --- | --- | --- |
    | **STARTTLS** | 587 | De meeste servers, Microsoft 365 |
    | **TLS** | 465 | Gmail, veel hostingproviders |
    | **Geen** | 25 | Alleen een interne relay. Wachtwoorden en berichten gaan leesbaar over het netwerk. |

3. Kies hoe je **Inloggen** wilt:
    - **Gebruikersnaam en wachtwoord**: klassieke SMTP AUTH. Gebruik een app-wachtwoord als de provider dat biedt.
    - **OAuth2 · Microsoft 365**: voor tenants waar basisverificatie uit staat. Registreer een app in Microsoft
      Entra met de toepassingsmachtiging `SMTP.SendAsApp` en beheerderstoestemming, geef hem toegang tot de mailbox
      in Exchange Online, en vul **Mailbox**, **Tenant-ID**, **Client-ID** en **Clientgeheim** in.
    - **OAuth2 · Google Workspace**: een OAuth-client in Google Cloud met een **Refresh token** voor de mailbox
      (scope `https://mail.google.com/`).
    - **Geen**: voor een relay die mail van het adres van deze server accepteert.
4. Vul het **Afzenderadres** en de **Afzendernaam** in, bijvoorbeeld `pbx@example.com` en "Kiestoon".
5. **Meldingen sturen naar**: het adres voor mislukte back-ups, fraudemeldingen en andere problemen.
6. Zet **Versturen aan** en sla op.
7. Vul onder **Test versturen** een ontvanger in en klik op **Test versturen**.

**Certificaat niet controleren** is alleen voor een interne server met een eigen certificaat.

## Het maillog

**Recente e-mails** toont elk bericht met tijd, ontvanger en resultaat. Staat versturen uit, of is de server
onbereikbaar, dan wachten e-mails in een wachtrij en gaan ze weg zodra het weer werkt; de pagina toont hoeveel er
wachten.

## Wat Kiestoon mailt

| E-mail | In te stellen bij |
| --- | --- |
| Voicemail met audio, en uitgeschreven tekst met spraak-naar-tekst | [Voicemail](voicemail.md) |
| Ontvangen faxen als PDF | [Fax](fax.md) |
| Welkomstmail met link om een wachtwoord te kiezen | [Gebruikers](users.md#medewerkers-toevoegen) |
| Fraudemeldingen | [Fraudebescherming](firewall.md#fraudebescherming) |
| Mislukte back-ups | [Back-ups](backups.md) |
