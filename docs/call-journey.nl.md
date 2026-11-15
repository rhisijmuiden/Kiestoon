# Gespreksverloop

Klik op een gesprek in **Gespreksgeschiedenis** (menu Receptie) om te zien wat er stap voor stap mee gebeurde: op
welk nummer het binnenkwam, de tijdsconditie, welke keuze de beller maakte in het keuzemenu, hoe lang hij in de
wachtrij stond, welke toestellen overgingen en wie hem miste, wie opnam en hoe lang, doorverbinden, voicemail en wie
ophing. Het werkt voor inkomende, uitgaande en interne gesprekken.

![Gespreksverloop](shot:call-journey)

## Wat je ziet

- **De samenvatting** bovenaan: beller en gebeld nummer, het resultaat (beantwoord, voicemail, opgehangen tijdens
  wachten, gemist, bezet, mislukt), de totale tijd, de wachttijd tot iemand opnam en de gesprekstijd.
- **De flow**: één blok per stap, in de stijl van de [keuzemenu-bouwer](ivr.md). Op de lijnen staat hoe lang de beller
  in de vorige stap zat. Zoom met de knoppen of het muiswiel; op een breed scherm staan de blokken naast elkaar.
- **Stappen**: hetzelfde verloop als lijst met de kloktijd van elke stap, ook handig op een telefoon.
- **Opnames** van het gesprek, direct af te spelen (zie [opnames](history.md#opnames)).

| Stap | Toont |
| --- | --- |
| **Binnenkomend gesprek** | Beller, gebeld nummer, trunk en [inkomende route](routes.md) |
| **Gesprek vanaf een toestel** / **Uitgaande route** | Wie belde, het gekozen nummer, de gebruikte route en trunk |
| **Tijdsconditie** | Open, gesloten of feestdag op dat moment |
| **Keuzemenu** | Elke gekozen toets, en elke keer dat de beller geen of een ongeldige keuze maakte |
| **Wachtrij** | Plek in de rij, wachttijd, elke medewerker die werd gebeld (opgenomen, niet opgenomen, bezet) en waarom de beller de wachtrij verliet (beantwoord, opgehangen, maximale wachttijd, niemand aangemeld) |
| **Belgroep** / **Toestel** | Elk toestel dat overging en hoe dat afliep; niet storen en buiten het rooster |
| **In gesprek met** | Wie opnam en de gesprekstijd |
| **Doorverbonden** | Direct of na overleg, door wie en naar wie (ook bij doorverbinden met slepen) |
| **Voicemail**, **Doorgeschakeld naar**, **Geparkeerd**, **Overgenomen door**, **Vergadering**, **Fax** | Waar het gesprek heen ging |
| **Gesprek beëindigd** | Wie ophing: de beller, de andere partij, een toestel of de telefooncentrale |

![Een gesprek dat in de voicemail eindigde](shot:call-journey-voicemail)

Medewerkers zien het verloop van hun eigen gesprekken door in het gebruikersportaal onder **Mijn gesprekken** op een
gesprek te klikken. Ze zien alleen gesprekken waaraan ze deelnamen, en alleen hun eigen opnames.

## Oudere gesprekken

Kiestoon registreert het verloop vanaf versie 0.6.0-rc.16. Voor oudere gesprekken, en voor gesprekken waarvan het
verloop na de bewaartermijn is verwijderd, toont de pagina een korte versie op basis van het gespreksrecord: wie
belde, welke toestellen overgingen en wie opnam.

## Bewaartermijn en de AVG

Het verloop bevat telefoonnummers en namen en is dus een persoonsgegeven. Onder **Belfuncties & fraude ›
Gespreksverloop**:

| Instelling | |
| --- | --- |
| **Gespreksverloop bewaren (dagen)** | Ouder verloop wordt elke nacht verwijderd. Standaard 90 dagen. **0** registreert helemaal geen verloop (de gespreksgeschiedenis zelf blijft) |

Kies de kortste termijn die je nodig hebt, en noem die in je privacyverklaring.

## Hoe het werkt

Asterisk meldt elke stap van een gesprek aan het portaal via de manager-interface: Channel Event Logging (CEL) voor
kanalen, opnemen, bridges, doorverbinden en ophangen, dial- en wachtrij-events voor de toestellen die overgingen, en
markeringen die het gegenereerde dialplan bij elke stap toevoegt (route, tijdsconditie, toets in het keuzemenu,
wachtrij, belgroep, voicemail). Het portaal bewaart ze per gesprek (de Asterisk-*linkedid*) in de eigen database; er
wordt niets naar logbestanden op schijf geschreven.
