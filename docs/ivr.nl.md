# Keuzemenu (IVR)

De **keuzemenu-bouwer** is je digitale receptionist: "Welkom bij Noordkade. Voor verkoop toets 1, voor support
toets 2." Je bouwt het menu visueel: sleep stappen op een canvas, verbind ze door lijnen te trekken, en geef elke
stap eigen audio.

![Het IVR-canvas: toetskeuzes, audio, doorverbinden en een herhaalstap](shot:ivr-canvas)

## Een menu maken

1. Open **Keuzemenu's (IVR)** en klik **Nieuw keuzemenu**. Het canvas opent met één stap: **Binnenkomend
   gesprek**.
2. Klik **Binnenkomend gesprek** en geef het menu een **naam** in het zijpaneel. Stel in:
    - **Wachten op een toets**: seconden dat het menu wacht na de aankondiging,
    - **Menu herhalen**: hoe vaak het menu opnieuw speelt voordat het stopt,
    - **Toestelnummer intoetsen toestaan**: bellers die een toestelnummer kennen, kunnen het direct kiezen.
3. Geef het binnenkomende gesprek een **audiobericht** (de welkomsttekst), zie [Audio per stap](#audio-per-stap).
4. Sleep bouwstenen uit de balk boven het canvas (of klik erop om ze toe te voegen) en verbind ze.
5. Klik **Opslaan** en daarna **Testgesprek**.

Blokken tonen live gegevens: of een tijdconditie nu open is, hoeveel medewerkers van een wachtrij online zijn en of een
toestel beschikbaar is. Met de knoppen rechts in de balk wissel je tussen vloeiende en hoekige lijnen en zet je de
bewegende stipjes op de lijnen (de richting van het gesprek) aan of uit.

![De lijst met keuzemenu's](shot:ivr-list)

## Bouwstenen

| Bouwsteen | Wat het doet | Lijnen naar buiten |
| --- | --- | --- |
| **Binnenkomend gesprek** | Begin van het menu; speelt de welkomsttekst | Naar de toetskeuzes en *Geen invoer*, of één startstap |
| **Keuzemenu** | Een volgend menu met een eigen tekst en toetsen, bijvoorbeeld één per tak van een tijdconditie | Naar zijn toetskeuzes en *Geen invoer* |
| **Toetskeuze** | "Toets 1": de beller drukte deze toets | Eén, of meerdere bestemmingen die [tegelijk overgaan](#meerdere-bestemmingen-tegelijk-laten-overgaan) |
| **Geen invoer** | De beller drukte niets of een ongebruikte toets, nadat het menu herhaald is | Eén |
| **Audio afspelen** | Speelt een bericht en volgt dan de lijn | Eén |
| **Spraakvraag** | Vraagt waarvoor de beller belt en luistert naar het antwoord (zie [hieronder](#spraakvraag)) | Eén per antwoord, plus *Niet begrepen* |
| **Toestel, Belgroep, Wachtrij, Voicemail** | Verbindt het gesprek daarheen door | Eindpunt |
| **Submenu** | Een ander keuzemenu | Eindpunt |
| **Extern nummer** | Schakelt door naar een nummer buiten | Eindpunt |
| **Tijdconditie** | Controleert de openingstijden en de dag/nachtschakelaar | Twee: *open* en *gesloten* |
| **Herhaal menu** | Terug naar het begin van het menu | Eindpunt |
| **Ophangen** | Beëindigt het gesprek | Eindpunt |

Gekoppelde centrales voegen **Andere locatie** toe, en [follow-the-sun](multi-site.md#follow-the-sun)-routes kunnen
ook een doel zijn.

## Lijnen trekken

Sleep vanaf het bolletje rechts van een stap naar de volgende stap. Een toetskeuze hoort alleen bij het menu als er
een lijn vanaf **Binnenkomend gesprek** naartoe loopt. Een tijdconditie heeft twee bolletjes: **open** en
**gesloten**.

Het canvas helpt je:

- Stappen met een probleem krijgen een waarschuwing: *Hoort niet bij het menu*, *Er loopt geen lijn naar deze
  stap*, *Er loopt geen lijn uit: het gesprek stopt hier*, *Kies waar het gesprek heen gaat*.
- **Ongedaan maken** / **Opnieuw** (ook <kbd>Ctrl</kbd>+<kbd>Z</kbd> / <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Z</kbd>),
  **Automatisch ordenen** zet de flow netjes neer, de minimap en zoomknoppen helpen bij grote menu's.
- **Dupliceren** in het zijpaneel kopieert een stap.
- Toetsenbord: druk <kbd>Enter</kbd> op een stap om hem te selecteren en verplaats hem met de pijltjestoetsen.

## Eerst de tijd controleren

Wil je dat de tijd bepaalt in welk menu bellers komen? Trek de lijn vanaf **Binnenkomend gesprek** naar een
**Tijdconditie** in plaats van naar toetsen. Die stap draait dan direct na de welkomsttekst. Trek **open** en
**gesloten** elk naar een **Keuzemenu** en geef elk menu eigen toetsen en *Geen invoer*. Een keuzemenu speelt zijn
eigen tekst (bijvoorbeeld "Toets 1 voor support") en wacht op een toets, net als het begin van het menu. Dezelfde
toets mag in verschillende menu's voorkomen.

Nieuwe toetsen uit het palet komen in het geselecteerde keuzemenu (of het menu van de geselecteerde toets). Toetsen
die aan **Binnenkomend gesprek** hangen terwijl dat een startstap heeft, worden overgeslagen; het canvas waarschuwt
daarvoor.

## Kopiëren en plakken

Klik met rechts op een stap voor **Kopiëren**, **Kopiëren met de stappen erna**, **Plakken**, **Dupliceren** en
**Verwijderen**; dezelfde knoppen staan onderaan het zijpaneel. Selecteer meerdere stappen met <kbd>Ctrl</kbd>-klik
of door met <kbd>Shift</kbd> een kader te slepen, en gebruik dan <kbd>Ctrl</kbd>+<kbd>C</kbd> en
<kbd>Ctrl</kbd>+<kbd>V</kbd>. Het klembord blijft in je browser, dus je kunt ook in een ander menu plakken.

Waar de kopie terechtkomt hangt af van waar je hem op plakt:

- **Een gekopieerd keuzemenu op een ander menu** (een keuzemenu of *Binnenkomend gesprek*): dat menu krijgt dezelfde
  toetsen, geen-invoer-stap en alles daarna. Typisch: kopieer het menu voor kantoortijden met de stappen erna, klik
  met rechts op het lege menu *Gesloten* en kies **Plakken in dit menu**, en pas daarna aan wat anders moet. Een
  toets waarvan het menu het cijfer al heeft, blijft los staan en krijgt een waarschuwing.
- **Gekopieerde toetsen op een menu**: die komen in dat menu.
- **Eén gekopieerde stap op een stap met een vrij bolletje**: die wordt erachter gekoppeld.
- **Op het lege canvas**: de kopie komt waar je klikte, zonder lijnen.

## Telefoonnummers

Klik op **Binnenkomend gesprek** om te zien welke telefoonnummers naar dit menu gaan. **Nummer toevoegen** toont de
vrije nummers van je trunks (en nummers die nu elders heen gaan; die verhuizen dan naar dit menu), of typ een ander
nummer. Dat maakt meteen de inkomende route aan, dezelfde die je ziet onder **Trunks & routes › Inkomende routes**;
met het × haal je hem weer weg. De nummers staan op de stap *Binnenkomend gesprek*. Sla een nieuw menu eerst één
keer op voordat je nummers koppelt.

## Meerdere bestemmingen tegelijk laten overgaan

Een toetskeuze (of *Geen invoer*, *Audio afspelen*, een tijdconditie of spraakantwoord) kan tot vijf bestemmingen
tegelijk laten overgaan: trek een tweede lijn vanaf hetzelfde bolletje, bijvoorbeeld naar een wachtrij én een extern
nummer. Wie het eerst opneemt, krijgt het gesprek; de rest stopt met overgaan. Dit werkt voor toestellen,
belgroepen, wachtrijen en externe nummers. Zolang de toestellen overgaan hoort de beller de muziek van de wachtrij
(of het overgaan als er geen wachtrij bij zit). Neemt niemand op binnen de maximale wachttijd van de wachtrij (zonder
wachtrij 60 seconden), dan volgt het gesprek de overloop van de wachtrij.

## Wachtrij-instellingen in de editor

Klik op een **Wachtrij**-stap om de wachtrij zelf te wijzigen in het zijpaneel: medewerkers, verdeling, overgaan- en
wachttijden, begroeting, muziek, positiemeldingen, terugbellen en overloop. **Wachtrij opslaan** slaat dat meteen
op, los van het menu, en de wijziging geldt overal waar de wachtrij gebruikt wordt. **Nieuwe wachtrij** maakt ter
plekke een wachtrij aan en laat de stap ernaartoe gaan.

## Audio per stap

Elke stap die iets afspeelt (binnenkomend gesprek, toetskeuze, geen invoer, audio afspelen) heeft een eigen
audiobericht, met een golfvormspeler om het te beluisteren. Kies de bron in het zijpaneel:

| Bron | Hoe |
| --- | --- |
| **Genereren** | Typ de tekst, kies de engine (**Lokaal (Piper)** of **ElevenLabs**) en een stem, en klik **Audio genereren**. Met een [vastgezet stemprofiel](prompts.md#stemprofiel) ligt de stem vast en typ je alleen de tekst. Piper draait op de server; ElevenLabs geeft stemmen in studiokwaliteit (zie [ElevenLabs-stemmen](prompts.md#elevenlabs-stemmen)). Vereist dat [Spraak](prompts.md#spraak) aan staat |
| **Opnemen** | Neem op met de microfoon in de browser (vereist https) |
| **Uploaden** | Sleep een MP3-, WAV-, M4A- of OGG-bestand (max 10 MB). Wordt omgezet naar telefoonkwaliteit |
| **Bibliotheek** | Kies een bestaande [gesproken tekst](prompts.md) |
| **Geen** | Geen audio |

Gegenereerde, opgenomen en geüploade audio wordt bewaard onder **Gesproken teksten**, zodat je hem opnieuw kunt
gebruiken in wachtrijen en andere menu's.

## Spraakvraag

Met een **Spraakvraag** zeggen bellers waarvoor ze bellen in plaats van een toets te kiezen
("Vertel ons in een paar woorden waarvoor u belt").

![Een menu met een spraakvraag](shot:ivr-speech)

1. De stap speelt de vraag en neemt het antwoord op (maximaal 12 seconden; 3 seconden stilte beëindigt het).
2. Het antwoord wordt **op deze server** uitgeschreven door Whisper; de beller hoort zolang even wachtmuziek.
3. Elk **antwoord** van de stap heeft een naam en trefwoorden. Het antwoord met de meeste trefwoorden in wat de
   beller zei wint. Een trefwoord past ook op langere woorden (*factu* past op *factuur* en *facturen*);
   hoofdletters en accenten maken niet uit, en een trefwoord van meer woorden (*werkt niet*) moet in die volgorde voorkomen.
4. Met **Laat AI kiezen als geen trefwoord past** kiest de [AI-engine](ai.md) het antwoord dat het best past.
   Claude doet daar een seconde of twee over; een lokaal model kan de eerste keer veel langer nodig hebben.
5. Past er niets, of staat spraak-naar-tekst uit: het gesprek volgt de lijn **Niet begrepen**.

Elk antwoord heeft een eigen poort rechts aan de stap; trek van elk een lijn naar waar het gesprek heen moet.
Een handige opzet: verbind **Geen invoer** met de spraakvraag en zet **Menu herhalen** op 0. Wie niets drukt, mag
het zeggen; wie de toets kent, drukt gewoon. Het [gespreksverloop](call-journey.md) toont hoe het antwoord van
de beller is herkend.

!!! note "Vereisten"
    Spraak-naar-tekst moet aan staan onder [Spraak](prompts.md#spraak) met een gedownload Whisper-model. Het
    model *base* of *small* houdt de wachttijd kort.

## Testgesprek

Klik **Testgesprek**, vul je toestelnummer in en klik **Bel mij**. Je telefoon gaat over; neem op en hoor het menu
zoals een beller het hoort. Sla het menu eerst op.

## Het menu gebruiken

Een keuzemenu is een [bestemming](concepts.md#bestemmingen). Gebruik het in een inkomende route, achter een
tijdconditie (open › menu, gesloten › voicemail), of als submenu van een ander menu.

!!! example "Een compleet hoofdnummer"
    Inkomende route `050 700 1234` › tijdconditie *Kantooruren* › **open**: keuzemenu *Hoofdmenu*
    (1 › intro verkoop › belgroep Verkoop, 2 › wachtrij Support, 3 › Daan, geen invoer › herhalen), **gesloten**:
    voicemail.

!!! note "Oudere menu's"
    Menu's van vóór het canvas (een lijst met toetsen) worden de eerste keer dat je ze opent automatisch omgezet
    naar een flow.
