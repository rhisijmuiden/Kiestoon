# Logboeken

**Systeem › Logboeken** toont wat de telefooncentrale en het portaal vastleggen. Gebruik het om uit te zoeken waarom
iets niet werkt: een trunk die niet aanmeldt, een toestel dat niet kan inloggen, een gesprek dat mislukt, een back-up
of mail die fout gaat.

![Logboeken](shot:logs)

## De logboeken

| Logboek | Wat erin staat |
| --- | --- |
| **Telefooncentrale** | Meldingen, waarschuwingen en fouten van Asterisk (`/var/log/asterisk/messages.log`): trunkregistraties, mislukte gesprekken, toestellen die niet kunnen aanmelden |
| **Beveiliging** | Aanmeldpogingen op telefoonaccounts, ook mislukte en geblokkeerde (`security.log`). De [firewall](firewall.md) (fail2ban) leest dit logboek |
| **Portaal** | Meldingen van het Kiestoon-portaal: mail, back-ups, updates, koppelingen |
| **Systeemhulp** | De dienst die als root de firewall, back-ups, internettoegang en updates toepast |
| **Dienst telefooncentrale** | Starten, stoppen en vastlopen van de Asterisk-dienst |

## Zoeken

- **Zoeken** toont alleen de regels met alle woorden die je typt, bijvoorbeeld een hostnaam, een telefoonnummer, een
  toestel of `trunk-3`. De woorden worden gemarkeerd.
- **Tonen** beperkt de regels tot *meldingen en erger*, *waarschuwingen en fouten* of *alleen fouten*.
- **Volgen** vernieuwt het logboek elke 5 seconden, handig terwijl je iets op een toestel probeert.
- **Downloaden** slaat het hele logboek op als tekstbestand, bijvoorbeeld om naar je leverancier te sturen.

De nieuwste regel staat bovenaan. De viewer leest het einde van het logboek (de laatste paar megabytes, ook de vorige
week als het logboek net is gewisseld); gebruik **Downloaden** voor oudere regels. De Asterisk-logboeken wisselen
wekelijks en blijven 8 weken bewaard.

## Waarom een trunk niet werkt

Staat een trunk op *Geweigerd*, *Niet aangemeld* of *Onbereikbaar*, dan toont de [trunklijst](trunks.md#status) de
reden onder de status, gelezen uit het logboek van de telefooncentrale, bijvoorbeeld *Aanmelden geweigerd (401): de
gebruikersnaam of het wachtwoord klopt niet*. Klik **Logregels tonen** voor wat je kunt controleren en de logregels
over die trunk, en **Openen in logboeken** om alles eromheen te zien.
