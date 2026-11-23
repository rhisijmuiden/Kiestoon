# Trunks (SIP-providers)

Een **trunk** verbindt Kiestoon met je telecomprovider, zodat je naar buiten kunt bellen en gesprekken op je
telefoonnummers ontvangt. Je kunt meerdere trunks hebben, bijvoorbeeld een hoofdprovider en een reserve.

![Trunks & routes](shot:trunks)

## Een trunk toevoegen

1. Open **Trunks & routes › Trunks** en klik **Trunk toevoegen**.
2. Kies de **provider**. Een sjabloon vult de server, poort, transport, codecs en de IP-reeksen van de provider in,
   en toont een stap-voor-stap-uitleg in het formulier als die er is.
3. Vul de accountgegevens van je provider in en sla op. Binnen een minuut wordt de status **Aangemeld**
   (registratie) of **Bereikbaar** (IP-authenticatie).

![De Voys-trunk bewerken, met de ingebouwde uitleg](shot:trunk-edit)

### Sjablonen

| Sjabloon | Authenticatie | Opmerkingen |
| --- | --- | --- |
| **Voys** | Registratie | `ha.voys.nl` met DNS SRV-failover. [Volledige uitleg](providers/voys.md) |
| **VoIPGRID platform** (Spitsgroep, partners) | Registratie | Maak een VoIP-trunk in hun portaal en gebruik de server die je krijgt |
| **KPN SIP trunk** | IP-adres | KPN-SBC `213.162.171.164` (+ `.180`), TCP. [Volledige uitleg](providers/kpn.md) |
| **Cheapconnect** | Registratie | `sip.cheapconnect.net`. [Volledige uitleg](providers/cheapconnect.md) |
| **Speakup** | Registratie | `sip.speakup.nl`. [Volledige uitleg](providers/speakup.md) |
| **Vodafone Ziggo Zakelijk** | Registratie | `trunk.sipnl.net`. [Volledige uitleg](providers/ziggo.md) |
| **Telnyx** | Registratie | Credential connection; TLS op poort 5061 aangeraden |
| **Twilio Elastic SIP** | IP-adres | Zet het publieke IP van deze server in de toegangslijst van de trunk |
| **Generic (registration)** | Registratie | Elke provider die een gebruikersnaam en wachtwoord geeft |
| **Generic (IP authentication)** | IP-adres | Vraag de provider vanaf welke IP-adressen ze gesprekken sturen |

## Velden

| Veld | Wat |
| --- | --- |
| **Authenticatie** | *Registratie*: Kiestoon meldt zich aan met gebruikersnaam en wachtwoord. *IP-adres*: de provider herkent je server aan het publieke IP |
| **Host / Poort** | De SIP-server van de provider. Poort `0` volgt de DNS SRV-records van de host (aangeraden voor Voys) |
| **Transport** | UDP, TCP of TLS |
| **Gebruikersnaam / Wachtwoord** | Van je provider. **Authenticatie-gebruikersnaam** alleen als de provider een aparte geeft |
| **From-domein / From-gebruiker** | Alleen als de provider erom vraagt |
| **IP-adressen van de provider** | Gesprekken vanaf deze adressen horen bij deze trunk, en de [firewall](firewall.md) laat ze door |
| **Gebeld nummer komt uit** | *Request-URI* of *To-header*: waar de provider je nummer zet bij inkomende gesprekken |
| **Codecs** | Op volgorde van voorkeur, bijvoorbeeld `alaw,g722` |
| **Maximaal aantal gelijktijdige gesprekken** | 0 is geen limiet |
| **Nummerweergave** | Standaardnummer dat de gebelde ziet. Moet een van je eigen nummers zijn |
| **Audio versleutelen (SRTP)** | Als de provider het ondersteunt, met TLS |
| **T.38-fax** | Faxen ontvangen en versturen met T.38 als de provider het biedt; anders gaat fax over G.711 |

## Status

De trunklijst en het dashboard tonen de status van elke trunk: *Aangemeld*, *Bereikbaar*, *Niet aangemeld*,
*Geweigerd* (verkeerde gebruikersnaam of wachtwoord) of *Onbereikbaar*. Een trunk die offline is geeft een
waarschuwing bovenaan het dashboard.

Is een trunk niet aangemeld of niet bereikbaar, dan staat de reden uit het logboek van de telefooncentrale onder de
status, bijvoorbeeld *Aanmelden geweigerd (401): de gebruikersnaam of het wachtwoord klopt niet* of *De provider
antwoordt niet*. **Logregels tonen** opent wat je kunt controleren en de logregels over de trunk. Zie
[Logboeken](logs.md).

![Waarom een trunk niet werkt](shot:trunk-why)

### Registratie vernieuwen

Een trunk die zich aanmeldt met een gebruikersnaam en wachtwoord heeft in de trunklijst een knop **Registratie
vernieuwen** (de ronde pijlen). Daarmee meldt Kiestoon zich meteen opnieuw aan bij de provider, in plaats van te wachten
op de volgende poging. Gebruik hem nadat je iets bij de provider hebt aangepast, na een storing, of als de status op
*Geweigerd* of *Niet aangemeld* blijft staan. Kiestoon wacht een paar seconden op het antwoord van de provider en toont
dat onder de status:

| Melding | Betekenis |
| --- | --- |
| *Zojuist opnieuw aangemeld* | De provider heeft de registratie geaccepteerd |
| *De provider weigert de registratie* | Controleer de gebruikersnaam en het wachtwoord bij je provider |
| *Verstuurd, de provider heeft nog niet geantwoord* | De status in de lijst werkt zichzelf bij zodra het antwoord komt |

![Registratie vernieuwen](shot:trunk-refresh)

De knop staat er niet bij een trunk met IP-authenticatie (daar valt niets aan te melden) of een trunk die uit staat.

## Telefoonnummers

Onder **Telefoonnummers** in het trunkformulier leg je vast welke nummers de provider op deze trunk aflevert. Je kunt
toevoegen:

- **Een nummerblok**: vul het eerste en het laatste nummer in (`050 700 1200` t/m `050 700 1259`) of het eerste nummer
  en een aantal (`050 700 1200` en `60`). Het formulier laat zien hoeveel nummers dat zijn voordat je ze toevoegt.
- **Eén of meer nummers**: plak een lijst, één per regel of gescheiden door komma's.

Nummers houden hun voorloopnul of `+`. Een nummer hoort bij één trunk, en een trunk heeft hooguit 10.000 nummers.
Verwijder één nummer met het kruisje, of alles met **Alles verwijderen**.

![Telefoonnummers van een trunk](shot:trunk-numbers)

De nummers kun je daarna overal kiezen waar je een nummer invult:

- onder **Inkomende routes**, waar nummers zonder route bovenaan staan en je ze in één keer naar een toestel,
  keuzemenu, wachtrij, belgroep of andere [bestemming](concepts.md#bestemmingen) stuurt ([inkomende routes](routes.md#inkomende-routes));
- als suggestie voor de **nummerweergave** van een trunk, een uitgaande route en een toestel.

## Achter NAT

Heeft Kiestoon een privé-IP-adres, vul dan het **publieke adres** en de **lokale netwerken** in onder
[Instellingen › Netwerk en NAT](settings.md), en stuur de SIP-poort en de RTP-reeks (10000-20000/udp) door naar de
server. Zonder dat komen gesprekken wel tot stand, maar hoor je niets of maar één kant.

## Verder

Gesprekken op je nummers hebben een [inkomende route](routes.md) nodig; naar buiten bellen een
[uitgaande route](routes.md).
