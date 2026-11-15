# Eerste inrichting

Deze handleiding brengt een verse Kiestoon van de setup-token naar het eerste echte telefoongesprek. Reken op
ongeveer een half uur. Elke stap verwijst naar het volledige hoofdstuk.

## 1. De eerste beheerder aanmaken

Open het portaal (`http://<server>:8080`). De installatiewizard vraagt om:

- **Taal** voor de wizard. Elke gebruiker kiest later zijn eigen taal.
- **Setup-token**: staat op de console en aan het eind van het installatiescript (`sudo kiestoon setup-token`).
- **Naam van de organisatie**: zichtbaar in het portaal en later op de displays van telefoons.
- **Gebruikersnaam, volledige naam en wachtwoord** (minstens 12 tekens; een zinnetje van een paar woorden werkt
  goed).

Beheerders gebruiken altijd **tweestapsverificatie**. Scan direct na het aanmaken de QR-code met een
authenticator-app (Microsoft Authenticator, Google Authenticator, 1Password …), vul de 6-cijferige code in en
bewaar de **herstelcodes**. Elke herstelcode werkt één keer, voor als je je telefoon kwijt bent.

!!! tip "Aansluiten als standby-node"
    De wizard heeft ook *Aansluiten bij een bestaande Kiestoon als standby-node*. Gebruik dat alleen voor de tweede
    machine van een [HA-paar](high-availability.md).

## 2. Basisinstellingen

Open **Instellingen** (onderaan het linkermenu).

![Instellingen: nummerplan, noodnummers en netwerk](shot:settings)

1. **Nummerplan**: interne nummers hebben standaard 3 cijfers (toestellen 200-599, belgroepen 600-699, wachtrijen
   700-799, vergaderruimtes 800-899). Kies voor 4 of 5 cijfers; bestaande toestellen worden automatisch
   hernummerd.
2. **Gesprekken**: taal van de gesproken teksten (Nederlands of Engels, ook voor voicemailmails), tijdzone en
   landcode (`31` voor Nederland).
3. **Alarmnummers**: 112 en 911 werken altijd, vanaf elk toestel. Zet het *nummer voor alarmoproepen* op een
   nummer waarop je teruggebeld kunt worden en dat aan je adres gekoppeld is.
4. **Netwerk en NAT**: staat de server achter NAT en bellen toestellen van buitenaf, vul dan het *publieke adres* en
   de *lokale netwerken* in.

Zie [Instellingen](settings.md) voor alle opties.

## 3. Mailserver

Voicemail naar e-mail, welkomstmails, fax en waarschuwingen hebben een mailserver nodig. Open **Mailserver**, vul
je SMTP-server in (of log in met Microsoft 365 / Google OAuth2) en stuur een testmail. Zie [Mailserver](mail.md).

## 4. Je telecomprovider koppelen

Open **Trunks & routes**:

1. **Trunks › Trunk toevoegen**: kies het sjabloon van je provider (bijvoorbeeld [Voys](providers/voys.md) of
   [Cheapconnect](providers/cheapconnect.md)) en vul de accountgegevens in. De status wordt *Aangemeld*.
2. **Inkomende routes**: stuur elk telefoonnummer ergens heen: een toestel, belgroep, wachtrij, keuzemenu of
   tijdconditie.
3. **Uitgaande routes**: maak een route met het voorbeeld *Nederlandse nummers* en kies de trunk(s).

Zie [Trunks](trunks.md) en [Inkomende en uitgaande routes](routes.md).

## 5. Toestellen toevoegen

Open **Toestellen › Toestel toevoegen**. Kiestoon kiest het volgende vrije nummer en maakt een sterk SIP-wachtwoord.
Vul het e-mailadres in en laat *Portalaccount aanmaken en welkomstmail versturen* aan staan: de medewerker krijgt een
link om een wachtwoord te kiezen, het adres van het portaal en de gegevens voor een softphone-app. Zie
[Toestellen](extensions.md).

## 6. Telefoons aansluiten

Open **Telefoons & provisioning**:

1. Zet **provisioning aan** en kopieer de provisioning-URL.
2. Zet die URL in DHCP-optie 66 van je router, of vul hem in op de webinterface van de telefoon.
3. Sluit de telefoons aan. Ze verschijnen onder *Wachten op een extensie*: kies een toestel en klik **Koppelen**.

Zie [Telefoons en provisioning](phones.md) en [Toetsen en uitbreidingsmodules](phone-keys.md).

## 7. Beveiligen en back-uppen

- Zet de **firewall** aan met de SIP-toegangslijst van je provider en fail2ban. De veilige modus draait een
  wijziging terug als je de toegang verliest. Zie [Firewall](firewall.md).
- Voeg een **back-uplocatie** toe (SFTP, FTPS, S3, OneDrive …) met een schema, en noteer de versleutelingssleutel.
  Zie [Back-ups](backups.md).
- Kijk bij **Info & updates** (onderaan het linkermenu) naar het kanaal en de nieuwste versie. Zie
  [Updates](updates.md).

## Controlelijst

- [ ] Beheerder met MFA en bewaarde herstelcodes
- [ ] Nummerplan, tijdzone, taal van gesproken teksten, nummer voor alarmoproepen
- [ ] Mailserver getest
- [ ] Trunk geregistreerd, inkomende en uitgaande route
- [ ] Toestellen met welkomstmail
- [ ] Telefoons gekoppeld en bellen
- [ ] Firewall aan, back-ups ingepland, versleutelingssleutel veilig bewaard
