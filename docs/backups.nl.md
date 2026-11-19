# Back-ups en herstellen

Kiestoon maakt **versleutelde** back-ups naar je eigen server of cloudopslag en herstelt ze vanuit het portaal. Het
gebruikt [restic](https://restic.net): alles wordt op de Kiestoon-server versleuteld voordat het de deur uit gaat, en
ontdubbeld opgeslagen, zodat dagelijkse back-ups weinig ruimte innemen.

![Back-ups](shot:backups)

## Wat er in een back-up zit

Alles om de telefooncentrale opnieuw op te bouwen: de database met alle instellingen, toestellen, routes,
keuzemenu's, gebruikers en gespreksgeschiedenis, voicemail, opnames, faxen, gesproken teksten, certificaten en de
configuratie in `/etc/kiestoon`. Gedownloade spraakmodellen en firmware van telefoons blijven eruit, want die kun je
opnieuw downloaden.

## Back-uplocaties

Kies onder **Back-ups › Locatie toevoegen** het protocol:

| Protocol | Opmerkingen |
| --- | --- |
| **FTP** | Gewone FTP stuurt het wachtwoord onversleuteld. De back-up zelf is altijd versleuteld. |
| **FTPS** | FTP over TLS. **Impliciete TLS** is meestal poort 990; uit betekent expliciete TLS (AUTH TLS) op poort 21. |
| **SFTP** | Via SSH, met een wachtwoord of een **SSH-privésleutel** (OpenSSH, zonder wachtwoordzin). |
| **SCP** | Gebruikt dezelfde SSH-verbinding als SFTP. |
| **S3** | Amazon S3, MinIO, Wasabi, Ceph, DigitalOcean, Scaleway, Hetzner, Cloudflare R2 of andere S3-compatibele opslag. De bucket moet al bestaan. |
| **WebDAV** | Nextcloud, ownCloud, SharePoint (NTLM) of een andere WebDAV-server. Gebruik bij Nextcloud een app-wachtwoord. |
| **SMB** | Een Windows-share of een NAS (Synology, QNAP), SMB 2 of 3. |
| **OneDrive** | Microsoft OneDrive of OneDrive for Business, gekoppeld met een inlogcode. |

Vul host, poort, gebruikersnaam, wachtwoord en **Map** in en klik op **Testen**. De eerste test maakt een nieuwe
versleutelde back-upopslag aan op de locatie.

!!! danger "Bewaar de versleutelingssleutel"
    Kiestoon maakt voor elke locatie een sterke **Versleutelingssleutel**. Zonder deze sleutel kun je een back-up
    niet herstellen op een nieuwe server. Open hem met de sleutelknop bij de locatie en bewaar hem in een
    wachtwoordmanager.

### OneDrive

1. Registreer een app in Microsoft Entra (*App-registraties*) met **Allow public client flows** aan en de
   gedelegeerde machtigingen `Files.ReadWrite` en `offline_access`.
2. Vul de **toepassings-ID (client-ID)** in en sla op.
3. Klik op **Koppelen**, open de Microsoft-pagina die Kiestoon toont, log in met het account waar de back-ups heen
   gaan en vul de code in.

## Volledig en incrementeel

- Een **incrementele** back-up leest alleen bestanden die veranderd zijn sinds de vorige snapshot. Snel en klein.
- Een **volledige** back-up leest elk bestand opnieuw, zodat ook beschadigde of gemiste bestanden meegaan.

Elke snapshot is een compleet beeld: elke snapshot kun je los herstellen, volledig of incrementeel. Start een
back-up met de hand met **Volledig** of **Incrementeel** bij een locatie.

## Planning en bewaren

Met **Automatische back-ups** aan maakt Kiestoon elke dag op de gekozen **Tijd** een back-up, en een volledige op de
dag bij **Volledige back-up op**. Dat gebeurt voor elke locatie die aan staat.

**Dagelijks bewaren**, **Wekelijks bewaren** en **Maandelijks bewaren** bepalen hoeveel snapshots er blijven. Met
7 / 4 / 12 kun je een week per dag terug, een maand per week en een jaar per maand.

Een mislukte back-up stuurt een e-mail naar het meldingsadres onder [Mailserver](mail.md). **Recente back-ups** toont
elke run met type, aanleiding, grootte en resultaat.

## Herstellen

1. Klik op **Snapshots** bij een locatie.
2. Kies de snapshot en klik op **Herstellen**.
3. Typ `RESTORE` om te bevestigen en klik op **Nu herstellen**.

Alles op deze server wordt vervangen door de back-up. Gesprekken worden onderbroken en het portaal start opnieuw. De
huidige gegevens blijven als veiligheidskopie op de server staan.

### Herstellen op een nieuwe server

Na een kapotte server installeer je Kiestoon opnieuw ([Installeren](install.md)), voeg je dezelfde locatie toe en plak je de
sleutel bij **Bestaande versleutelingssleutel**, klik je op **Testen** en herstel je de nieuwste snapshot. De nieuwe server heeft dan dezelfde
toestellen, telefoons en instellingen; geprovisionede telefoons maken gewoon weer verbinding.
