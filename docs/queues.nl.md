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

Je kunt een wachtrij ook wijzigen of een nieuwe maken vanuit de [IVR-bouwer](ivr.nl.md#wachtrij-instellingen-in-de-editor): klik op een wachtrij-stap in het canvas.

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

## Terugbellen vanuit de wachtrij

Zet **Terugbellen vanuit de wachtrij** aan en wachtende bellers horen regelmatig dat ze op **1** kunnen drukken om
teruggebeld te worden. Ze bevestigen hun nummer (of toetsen een ander nummer in, gevolgd door #) en hangen op. Ze
houden hun plaats in de wachtrij: een plaatshouder wacht voor ze. Zodra er een medewerker vrij is, gaat eerst diens
toestel over (het scherm toont `CB` en de beller), de medewerker hoort een korte melding en Kiestoon belt de beller
via de uitgaande routes.

| Instelling | Betekenis |
| --- | --- |
| **Aanbieden elke** | Hoe vaak wachtende bellers het aanbod horen |
| **Extra pogingen** | Hoe vaak Kiestoon het nog probeert als de beller niet opneemt (een nieuwe poging komt vooraan in de wachtrij) |
| **Tijd tussen pogingen** | Wachttijd voor de volgende poging |
| **Opgeven na** | Minuten na het verzoek waarna een onbeantwoord terugbelverzoek stopt |
| **Nummerweergave bij terugbellen** | Het nummer dat de beller ziet; leeg gebruikt de uitgaande route |

De openstaande verzoeken staan op het [schakelbord](switchboard.md#terugbelverzoeken), waar de receptie een verzoek
kan annuleren, opnieuw kan proberen of met de hand kan toevoegen (bijvoorbeeld na een gemiste oproep).

!!! note
    Bellers met een afgeschermd nummer wordt gevraagd hun nummer in te toetsen. Verzoeken die wachtten toen
    Kiestoon of Asterisk herstartte, komen weer in de wachtrij; een verzoek dat op dat moment gebeld werd, krijgt
    *Niet bereikt* en kan opnieuw geprobeerd worden.

## Supervisie: meeluisteren, influisteren, inbreken

Met **Supervisie** aan kunnen gebruikers met de rol *Receptie* of hoger die een eigen toestel hebben, aansluiten bij
gesprekken van deze wachtrij:

| Modus | Schakelbord | Toestel | Wie hoort de supervisor |
| --- | --- | --- | --- |
| **Meeluisteren** | oor-knop | `*33<toestel>` | niemand |
| **Influisteren** (coachen) | spraak-knop | `*34<toestel>` | alleen de medewerker |
| **Inbreken** | personen-knop | `*35<toestel>` | medewerker en beller |

Op het schakelbord staan de knoppen naast een medewerker die in een wachtrijgesprek zit; je eigen toestel gaat over
en je wordt verbonden. Tijdens het gesprek druk je **4** om mee te luisteren, **5** om in te fluisteren en **6** om in
te breken. Alleen wachtrijgesprekken zijn bereikbaar, nooit andere gesprekken van de medewerker, en elk gebruik
staat in het auditlog.

!!! warning "Privacy"
    Meeluisteren met gesprekken is een verwerking van persoonsgegevens. Informeer je medewerkers (en waar nodig
    bellers) dat gesprekken in deze wachtrij beluisterd kunnen worden, bijvoorbeeld in de begroeting en in je
    privacyverklaring.

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
