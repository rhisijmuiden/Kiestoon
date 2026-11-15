# Wachtrijen

Een **wachtrij** laat bellers in de rij wachten, met muziek en positiemeldingen, tot een medewerker vrij is.
Gesprekken worden over de medewerkers verdeeld volgens een verdeling. Nummers liggen standaard in de 700-reeks.

![Wachtrijen met live wachtende bellers](shot:queues)

## Een wachtrij maken

1. Open **Wachtrijen** en klik **Nieuwe wachtrij**.
2. Kies een **verdeling**:

    | Verdeling | Wie gaat over |
    | --- | --- |
    | **Om de beurt** | Medewerkers om de beurt, beginnend na de laatste die een gesprek kreeg |
    | **Allemaal** | Alle vrije medewerkers tegelijk |
    | **Langst vrij** | De medewerker die het langst vrij is |
    | **Minste gesprekken** | De medewerker met de minste gesprekken vandaag |
    | **Willekeurig** | Een willekeurige vrije medewerker |
    | **Op volgorde** | Medewerkers in een vaste volgorde |
    | **Willekeurig op prioriteit** | Willekeurig, een hogere prioriteitswaarde geeft minder kans |

3. Voeg **medewerkers** toe. **Dynamische** medewerkers melden zich zelf aan en af (gebruikersportaal,
   desktop-softphone of `*45`); medewerkers op **Altijd aan** zijn altijd aangemeld. Een hogere **prioriteit**
   zet een medewerker verder naar achteren.
4. Optioneel: een **begroeting** (een [gesproken tekst](prompts.md) die één keer speelt bij binnenkomst),
   **wachtmuziek** en **positie omroepen** elke N seconden.
5. Stel de tijden en limieten in, en **als de wachtrij vol of te traag is**: de overloopbestemming.

![De wachtrij Support bewerken](shot:queue-edit)

## Tijden en limieten

| Instelling | Betekenis |
| --- | --- |
| **Medewerker laten overgaan** | Seconden dat een medewerker overgaat voordat de volgende geprobeerd wordt |
| **Wachten voor volgende medewerker** | Pauze tussen twee pogingen |
| **Nawerktijd** | Seconden na een gesprek voordat de medewerker het volgende krijgt |
| **Maximale wachttijd** | Daarna gaat de beller naar de overloopbestemming |
| **Maximum aantal bellers** | Meer bellers gaan direct naar de overloop (0 is onbeperkt) |
| **Bellers toelaten als niemand is aangemeld** | Uit: bellers gaan direct naar de overloopbestemming |
| **Wachtrijgesprekken opnemen** | Elk beantwoord gesprek in deze wachtrij wordt opgenomen |

## Medewerkers op andere locaties

Met [gekoppelde centrales](multi-site.md) kunnen medewerkers van een andere Kiestoon in de wachtrij
(**Agents op gekoppelde systemen**). Hun toestellen gaan via de tunnel over en hun bezetlampje vertelt de wachtrij
wanneer ze in gesprek zijn.

## Een wachtrij live volgen

- De pagina **Wachtrijen** toont wachtende bellers en aangemelde medewerkers.
- Het [wallboard](switchboard.md#wallboard) is een weergave voor een groot scherm voor het team.
- Op het [schakelbord](switchboard.md) meldt de receptie medewerkers aan en af en pauzeert ze.
- Medewerkers gebruiken **Wachtrijen & groepen** in het [gebruikersportaal](portal.md) of `*45` / `*45<wachtrij>` op
  de telefoon.
