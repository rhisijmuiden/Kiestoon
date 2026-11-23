# Inkomende en uitgaande routes

Routes bepalen waar gesprekken heen gaan: **inkomende routes** sturen elk van je telefoonnummers naar een
bestemming, **uitgaande routes** kiezen de trunk voor nummers die je gebruikers bellen. Beide staan op **Trunks &
routes**.

## Inkomende routes

1. Open **Trunks & routes › Inkomende routes** en klik **Inkomende route toevoegen**.
2. **Telefoonnummer**: je nummer zoals de provider het stuurt. `0201234567`, `31201234567` en `+31201234567` gelden
   als hetzelfde nummer. Laat het leeg voor een vangnetroute: elk nummer dat geen andere route heeft.
3. **Sturen naar**: elke [bestemming](concepts.md#bestemmingen), bijvoorbeeld een tijdconditie voor je hoofdnummer,
   een wachtrij voor een supportnummer, een toestel voor een direct nummer of een faxbox voor je faxnummer.
4. **Voorvoegsel bellernaam** (optioneel): staat voor de naam van de beller op het display, bijvoorbeeld
   `Support: `, zodat wie opneemt weet welk nummer gebeld is.

!!! tip "Een gangbare indeling"
    | Nummer | Sturen naar |
    | --- | --- |
    | Hoofdnummer | Tijdconditie *Kantooruren* › keuzemenu › belgroep / wachtrij |
    | Supportnummer | Wachtrij *Support* |
    | Directe nummers | Het toestel van de medewerker |
    | Faxnummer | Faxbox |
    | Leeg (vangnet) | Belgroep receptie |

Bij bellers die in het [telefoonboek](phonebook.md) staan, verschijnt de naam van het contact.

### Veel nummers tegelijk routeren

Als je trunks [telefoonnummers](trunks.md#telefoonnummers) hebben, staan de nummers die nog geen route hebben boven de
routes. Vink de nummers aan (of **Alles selecteren**), klik **Nummers routeren** en kies één bestemming. Elk nummer
krijgt een eigen route, genoemd naar de naam die je opgeeft plus het nummer, zodat je er later één ergens anders heen
kunt sturen. Bij het intypen van een telefoonnummer in een route worden de trunknummers als suggestie getoond.

![Nummers zonder route](shot:inbound-numbers)

## Uitgaande routes

Uitgaande routes worden **van boven naar beneden** geprobeerd; de eerste route met een passend kiespatroon wordt
gebruikt. Met de pijltjes verander je de volgorde.

1. Open **Uitgaande routes** en klik **Uitgaande route toevoegen**.
2. Klik een voorbeeld: **Nederlandse nummers** (`0[1-9]XXXXXXXX`) of **Internationaal** (`00X.`), of schrijf eigen
   patronen.
3. Voeg een of meer **trunks** toe. Ze worden in deze volgorde geprobeerd; de volgende trunk wordt gebruikt als er
   een uitvalt of vol zit.
4. **Nummerweergave** (optioneel) gaat voor de nummerweergave van de trunk. Het eigen uitgaande nummer van een
   toestel en het nummer van zijn [toestelgroep](extension-groups.md) gaan voor.
5. **Wie mag deze route gebruiken** (optioneel): kies [toestelgroepen](extension-groups.md#eigen-uitgaande-route-per-groep)
   om ze een eigen route te geven, bijvoorbeeld over een eigen trunk. Anderen slaan de route over en nemen de
   volgende die past. Standaard: iedereen.

### Kiespatronen

| Teken | Betekent |
| --- | --- |
| `X` | Elk cijfer 0-9 |
| `Z` | 1-9 |
| `N` | 2-9 |
| `[1-5]` | Een cijfer in de reeks |
| `.` | Een of meer cijfers |

Elk patroon heeft drie delen: **prefix** (wordt weggehaald voor het kiezen), **patroon** en **voorvoegen** (wordt
toegevoegd). Voorbeeld: prefix `9`, patroon `0XXXXXXXXX` laat gebruikers een `9` kiezen voor een buitenlijn.

### Wie mag wat bellen

Routes bepalen hoe een nummer gekozen wordt; het toestel bepaalt of het gekozen mag worden. Elk toestel heeft een
**uitgaand-belniveau** (alleen intern, nationaal, internationaal) en een schakelaar voor betaalnummers. Daarbovenop
beperkt de [fraudebescherming](firewall.md#fraudebescherming) het aantal gelijktijdige uitgaande gesprekken en
stuurt waarschuwingen.

## Noodnummers

**112 en 911 werken altijd**, vanaf elk toestel, zonder route, en zelfs als een trunklimiet of de fraudeblokkade
actief is. Onder **Instellingen › Alarmnummers** stel je het nummer voor alarmoproepen in (een nummer dat aan je
adres gekoppeld is) en de volgorde waarin trunks geprobeerd worden.
