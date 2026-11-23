# Instellingen

**Instellingen** bevat de keuzes voor het hele systeem: de taal voor bellers, het nummerplan, alarmnummers, het
netwerk en de telefooncentrale zelf.

![Instellingen](shot:settings)

## Taal voor bellers { #taal-voor-bellers }

Eén instelling voor alles wat bellers horen en toestellen tonen, in het Engels of het Nederlands:

- meldingen in wachtrijen: de hoeveelste wachtende de beller is ("u bent nummer 3") en de wachttijd,
- voicemailmenu's en begroetingen, sterretjescodes en andere systeemmeldingen,
- uitgesproken getallen, datums en tijden,
- de standaardtaal van [uitgesproken tekst](prompts.md) voor keuzemenu's en gesproken teksten,
- voicemail-, fax- en andere systeemmails,
- nieuwe gebruikers en hun welkomstmail.

**Schermen van toestellen volgen de taal voor bellers** geeft [automatisch ingestelde toestellen](phones.md)
dezelfde taal bij hun volgende configuratie-update. Zet het uit om toestellen een eigen taal te geven bij
**Telefoons & provisioning > Provisioning-instellingen**.

De taal van de webinterface hoort er niet bij: elke gebruiker kiest die zelf bij **Account**. Een beheerder kan
dus in het Engels werken terwijl bellers Nederlands horen.

Een losse [wachtrij](queues.md) of een [keuzemenu](ivr.md) kan in zijn eigen instellingen een andere taal krijgen
(**Taal voor bellers**), bijvoorbeeld een Engelse lijn voor buitenlandse klanten. De taal die je bij de eerste
installatie kiest, is meteen de eerste taal voor bellers.

## Nummerplan

Kiestoon gebruikt vaste reeksen per doel, te zien in de tabel: toestellen, belgroepen, wachtrijen,
vergaderruimtes, voicemail en functies, en de alarmnummers.

**Nummerlengte wijzigen** wisselt tussen interne nummers van 3, 4 of 5 cijfers. Bestaande toestellen krijgen
automatisch een nieuw nummer. Geprovisionede telefoons nemen hun nieuwe nummer zelf
over; andere telefoons moet je met de hand aanpassen. Kiestoon toont vooraf de nummers die niet korter kunnen.

## Alarmnummers { #alarmnummers }

112 en 911 werken altijd, vanaf elk toestel, zonder route en zelfs als een trunklimiet bereikt is of de
[fraudebescherming](firewall.md#fraudebescherming) uitgaand bellen blokkeert.

- **Nummer voor alarmoproepen**: een nummer waarop je teruggebeld kunt worden, gekoppeld aan je adres. Leeg: het
  nummer van de trunk.
- **Trunkvolgorde voor alarmoproepen**: welke trunk als eerste geprobeerd wordt. Geen volgorde: elke trunk wordt
  geprobeerd.

## Netwerk en NAT

| Instelling | Wat het doet |
| --- | --- |
| **Publiek adres** | Publiek IP-adres of hostnaam, als deze server achter NAT staat. Leeg: geen NAT. |
| **Lokale netwerken** | Netwerken die niet achter NAT zitten, één per regel. |
| **SIP-poort (UDP/TCP)** | Standaard 5060. |
| **SIP TLS-poort** | Standaard 5061. |
| **Audiopoorten vanaf / tot** | RTP-reeks, standaard 10000–20000. |

Een poort wijzigen vraagt een herstart van de telefooncentrale.

## Gesprekken

| Instelling | Wat het doet |
| --- | --- |
| **Tijdzone** | Voor tijdcondities, gespreksgeschiedenis en planningen. |
| **Landcode** | Zonder `+`; `31` is Nederland. |
| **Codecs voor toestellen** | Op volgorde van voorkeur, bijvoorbeeld `g722,alaw,ulaw`. |
| **Nummers in nationaal formaat tonen** | Toont `+31 20 …` als `020 …`, zodat terugbellen werkt. |

## De telefooncentrale herstarten

Sommige wijzigingen werken pas na een herstart van de telefooncentrale (Asterisk); het portaal toont dan een
melding. **Als het rustig is** herstart zodra er geen gesprekken zijn. **Nu herstarten** beëindigt alle lopende
gesprekken.
