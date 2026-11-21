# Problemen oplossen

Begin bij het **dashboard**: dat laat zien of de telefooncentrale draait, welke trunks online zijn, hoeveel
telefoons geregistreerd zijn en elke waarschuwing die aandacht nodig heeft.

## Telefoons

??? question "Een telefoon registreert niet"
    - Controleer het toestelnummer en het SIP-wachtwoord (**Toestellen › Bewerken**, of de SIP-gegevens in het
      gebruikersportaal).
    - Staat de telefoon buiten kantoor? Dan moet zijn adres toegestaan zijn in de [firewall](firewall.md): gebruik de
      SIP-whitelist, **SIP vanaf lokale netwerken toestaan** of een [Edge](edge.md) voor vestigingen.
    - Kijk onder **Firewall › Geblokkeerde adressen**: na te veel verkeerde wachtwoorden blokkeert fail2ban het adres.
      **Deblokkeren** heft dat op.
    - Geo-blokkering kan een telefoon in het buitenland buitensluiten.

??? question "Een telefoon wordt niet automatisch ingesteld"
    - Laat de telefoon wijzen naar `http://<server>:8080/prov/` (DHCP-optie 66, of de provisioning-instelling van de
      telefoon zelf).
    - Nieuwe telefoons verschijnen onder **Wachten op een extensie**: kies een toestel en klik op **Koppelen**.
    - Controleer de gebruikersnaam en het wachtwoord van provisioning onder **Telefoons › Provisioning-instellingen**.
    - **Configuratie bekijken** toont precies het bestand dat de telefoon krijgt.

??? question "Geluid maar één kant op, of helemaal geen geluid"
    Bijna altijd NAT. Vul het **Publiek adres** en de **Lokale netwerken** in onder [Instellingen](settings.md), en
    stuur de audiopoorten (standaard UDP 10000–20000) door naar de server. Zet "SIP ALG" op de router uit.

## Gesprekken

??? question "Uitgaand bellen lukt niet"
    - Is de trunk online op het dashboard? Controleer de inloggegevens van de trunk en de IP-whitelist bij de
      provider.
    - Past er een [uitgaande route](routes.md) bij het gekozen nummer, en mag het toestel het bellen (**Uitgaand
      bellen** bij het toestel)?
    - Heeft de fraudebescherming uitgaand bellen geblokkeerd? Dan tonen het dashboard en **Belfuncties & fraude** dat,
      met een knop **Deblokkeren**.

??? question "Inkomende gesprekken komen niet aan"
    - Heeft de [inkomende route](routes.md) het nummer precies zoals de provider het stuurt? De trunkinstelling
      **Gebeld nummer komt uit** bepaalt waar Kiestoon het nummer leest.
    - Controleer de [tijdconditie](time-conditions.md): misschien gaat het gesprek naar de gesloten bestemming, of
      staat de modus geforceerd op nacht.

??? question "Bellers horen Engelse teksten"
    Zet **Taal van gesproken berichten** op Nederlands onder [Instellingen](settings.md), en controleer de taal van
    de teksten van keuzemenu's en wachtrijen onder [Gesproken teksten](prompts.md).

## Portaal en softphone

??? question "De softphone in de browser zegt 'Heeft https nodig'"
    Browsers geven alleen via https toegang tot de microfoon. Open het portaal via het [internetadres](internet.md)
    of een [reverse proxy](reverse-proxy.md), niet via `http://<ip>:8080`.

??? question "Passkeys toevoegen lukt niet"
    Passkeys hebben https en een hostnaam nodig, geen IP-adres.

??? question "Ik heb mezelf buitengesloten met de firewall"
    De veilige modus draait een wijziging binnen 90 seconden terug als je het portaal niet kunt bereiken. Toch
    buitengesloten? Op de console van de server verwijdert `nft delete table inet kiestoon` de regels van Kiestoon
    tot de volgende wijziging in het portaal.

??? question "Mijn telefoon met de MFA-app is kwijt"
    Log in met een van je herstelcodes. Een andere beheerder kan ook **MFA resetten** gebruiken onder
    [Gebruikers & rollen](users.md).

## Mail en back-ups

??? question "Geen voicemailmails"
    Stuur een test onder [Mailserver](mail.md) en zoek de fout bij **Recente e-mails**. Controleer of het toestel een
    e-mailadres heeft en **Voicemail naar e-mail sturen** aan staat.

??? question "Een back-up is mislukt"
    **Recente back-ups** toont de fout. Klik op **Testen** bij de locatie: vaak is het een gewijzigd wachtwoord, een
    volle schijf of een firewall ertussen.

## Logs op de server

| Wat | Opdracht |
| --- | --- |
| Portaal | `journalctl -u kiestoon -f` |
| Telefooncentrale | `journalctl -u asterisk -f`, of `asterisk -rvvv` voor de live console |
| SIP-registraties | `asterisk -rx "pjsip show contacts"` |
| Updates | `journalctl -u kiestoon-updater` |
| Firewall | `nft list ruleset` |
| fail2ban | `fail2ban-client status` |
| Versie | `kiestoon version` |
| Eerste gebruikersnaam en wachtwoord | `sudo kiestoon initial-admin` |

Kom je er niet uit? Open een [issue op GitHub](https://github.com/rhisijmuiden/Kiestoon/issues) met de versie en wat
je in de logs ziet. Haal er eerst wachtwoorden en telefoonnummers uit.
