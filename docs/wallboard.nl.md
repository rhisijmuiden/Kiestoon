# Wallboard, SLA-meldingen en rapportages

Het wallboard toont je wachtrijen groot leesbaar voor een scherm aan de muur: wie er wacht, hoe lang, het
servicelevel van vandaag en wat elke medewerker doet. Kiestoon bewaakt de drempels die je per wachtrij instelt en
waarschuwt het team als een wachtrij in de problemen komt.

![Het wallboard op een tv](shot:wallboard-tv)

## Wat het wallboard toont

Elke wachtrij krijgt een tegel met:

- **Wachtend**: bellers die nu in de wachtrij staan, ook terugbelverzoeken (in paars, met de tijd sinds hun verzoek).
- **Langste wacht**: de beller die het langst wacht. Wordt rood boven de maximale wachttijd van de wachtrij.
- **Servicelevel**: de ring toont het deel van de gesprekken van vandaag dat binnen de streeftijd is opgenomen.
  Groen op of boven de norm, oranje tot 10 punten eronder, rood daaronder.
- De bellers op volgorde, en onderaan de beantwoorde en opgehangen gesprekken, de gemiddelde wachttijd en de
  gemiddelde gespreksduur van vandaag.

Het paneel **Medewerkers** toont iedereen die bij de wachtrijen is aangemeld, met een timer sinds de laatste
wijziging: in gesprek (met wachtrij en beller), gaat over, intern gesprek, vrij (met het aantal gesprekken van
vandaag), pauze of offline. Onder de wachtrijen staat een grafiek met de beantwoorde en opgehangen gesprekken per
uur van vandaag.

!!! info "Hoe het servicelevel wordt geteld"
    Binnen de streeftijd beantwoord, gedeeld door beantwoorde plus opgehangen gesprekken. Bellers die binnen
    5 seconden ophangen tellen niet mee (die hebben zich waarschijnlijk verbeld), en bellers die naar de overloop
    gingen of om terugbellen vroegen ook niet. De cijfers beginnen om middernacht en blijven 400 dagen bewaard voor
    rapportages.

## Layouts

Het wallboard heeft drie layouts:

| Layout | Toont |
| --- | --- |
| **Overzicht** | Alle wachtrijen, de medewerkers en de gesprekken per uur |
| **Eén wachtrij groot** | Eén wachtrij tegelijk met zijn medewerkers; bij meer wachtrijen loopt hij ze langs |
| **Medewerkers** | Een kaart per medewerker met status en timer |

![De layout medewerkers](shot:wallboard-agents)

In het portaal kies je de layout bovenaan **Receptie › Wallboard**, of **Wisselen** om elke 30 seconden te wisselen.
Supervisors met een eigen toestel zien bij een medewerker in gesprek de knoppen meeluisteren, influisteren en
inbreken (voor wachtrijen met supervisie aan). Bij een tv-link horen de layouts en het wisselinterval bij de link.

## Tv-links

Een tv logt niet in. Open als beheerder **Receptie › Wallboard › Instellingen** en klik **Nieuwe tv-link**. Je kiest:

- een **naam**, bijvoorbeeld *Supportruimte*;
- de **wachtrijen** die het scherm toont (of alle wachtrijen);
- de **layouts** en hoe vaak het scherm **wisselt**;
- het **thema**, donker of licht (licht werkt beter in ruimtes met veel daglicht);
- **Namen van bellers verbergen**: toont alleen de laatste 4 cijfers van nummers, voor een scherm dat bezoekers
  kunnen zien.

![Wallboard-instellingen](shot:wallboard-settings)

Kopieer de link en open hem op de tv, een Chromecast, een Fire TV-stick of een mini-pc met een browser, op volledig
scherm. De link ziet eruit als `https://pbx.example.com/wall/k7Q2m-X9vLr-3e8sW-dPz4a`. De sleutel erin is het
geheim: iedereen met de link ziet het wallboard (en verder niets). **Nieuwe sleutel** vervangt de sleutel, waarna de
oude link niet meer werkt; **Intrekken** verwijdert de link. *Laatst getoond* vertelt wanneer een scherm voor het
laatst de cijfers ophaalde.

![Een tv-link in het lichte thema, één wachtrij groot met verborgen namen](shot:wallboard-tv-light)

!!! tip "Scherm aan laten"
    De tv-pagina vraagt de browser om het scherm wakker te houden. Zet ook de tv of stick zelf zo dat hij niet in
    slaap valt.

## SLA-drempels

Onder **SLA-drempels per wachtrij** stel je per wachtrij in:

| Drempel | Melding als |
| --- | --- |
| **Streeftijd** | Zelf geen melding: de tijd waarbinnen een gesprek op tijd is opgenomen (standaard 20 s) |
| **Servicelevel-norm** | Het servicelevel van vandaag onder dit percentage zit (beoordeeld vanaf 5 gesprekken) |
| **Max. wachtend** | Er meer bellers wachten dan dit |
| **Max. wachttijd** | Een beller langer wacht dan dit |
| **Max. opgehangen** | Meer dan dit deel van de bellers van vandaag heeft opgehangen (beoordeeld vanaf 10 gesprekken) |

Laat een drempel leeg om hem niet te bewaken. De kolom **Nu** laat zien of de wachtrij binnen zijn drempels zit.

## Meldingen

Kiestoon controleert de drempels elke 5 seconden. Onder **Meldingen** kies je waar een melding heen gaat:

- **Op het wallboard en het schakelbord**: een rode balk bovenaan en een rode rand om de wachtrij.
- **Per e-mail** naar de ontvangers, via de [mailserver](mail.md). De **rustperiode** (standaard 15 minuten) zorgt
  dat dezelfde melding van dezelfde wachtrij in die tijd maar één keer wordt gemaild.
- **Alleen mailen** tijdens de openingstijden van een [tijdconditie](time-conditions.md), zodat niemand 's nachts
  mail krijgt.
- **Ook mailen als het weer goed is** stuurt een korte mail als de wachtrij weer binnen de drempel zit.

Pushmeldingen in de mobiele app volgen met de app.

## Geplande rapportages

Onder **Geplande rapportages** maak je een overzicht dat Kiestoon zelf mailt. Klik **Nieuwe rapportage** en kies:

| Periode | Wordt verstuurd | Gaat over | Vergeleken met |
| --- | --- | --- | --- |
| **Dagelijks** | elke dag (of alleen op werkdagen) op de tijd die je kiest | die dag | dezelfde dag een week eerder |
| **Wekelijks** | op de dag en tijd die je kiest | de vorige week, maandag tot en met zondag | de week ervoor |
| **Maandelijks** | op de 1e op de tijd die je kiest | de vorige maand | de maand ervoor |

Kies de wachtrijen, de inhoud (per wachtrij, per medewerker, per uur, en het verschil met de periode ervoor), de
ontvangers en de taal (Engels of Nederlands) van de mail.

![Voorbeeld van een weekrapportage](shot:report-preview)

De mail is een gewone HTML-mail die in elk mailprogramma werkt. Bovenaan staan de gesprekken, het servicelevel, de
gemiddelde wachttijd en het deel opgehangen, elk met het verschil met de periode ervoor. Daaronder de tabellen per
wachtrij, per medewerker en per uur, het drukste uur en het aantal SLA-meldingen. De cijfers per dag en wachtrij en
per medewerker zitten als CSV-bestand in de bijlage, dat opent in Excel.

- **Voorbeeld** toont de mail met de cijfers van nu.
- **Nu versturen naar mij** mailt hem naar je eigen adres (in te stellen onder [Gebruikers](users.md)).
- Een rapportage die niet bezorgd kan worden, wordt niet elke minuut opnieuw geprobeerd: hij wacht op zijn volgende
  verzendmoment. Het [maillogboek](mail.md) laat zien wat er misging.
