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

![De lijst met keuzemenu's](shot:ivr-list)

## Bouwstenen

| Bouwsteen | Wat het doet | Lijnen naar buiten |
| --- | --- | --- |
| **Binnenkomend gesprek** | Begin van het menu; speelt de welkomsttekst | Naar de toetskeuzes en *Geen invoer* |
| **Toetskeuze** | "Toets 1": de beller drukte deze toets | Eén |
| **Geen invoer** | De beller drukte niets of een ongebruikte toets, nadat het menu herhaald is | Eén |
| **Audio afspelen** | Speelt een bericht en volgt dan de lijn | Eén |
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

## Audio per stap

Elke stap die iets afspeelt (binnenkomend gesprek, toetskeuze, geen invoer, audio afspelen) heeft een eigen
audiobericht, met een golfvormspeler om het te beluisteren. Kies de bron in het zijpaneel:

| Bron | Hoe |
| --- | --- |
| **Genereren** | Typ de tekst, kies een stem en klik **Audio genereren**. Ingesproken door de lokale spraakengine (Piper); niets verlaat de server. Vereist dat [Spraak](prompts.md#spraak) aan staat |
| **Opnemen** | Neem op met de microfoon in de browser (vereist https) |
| **Uploaden** | Sleep een MP3-, WAV-, M4A- of OGG-bestand (max 10 MB). Wordt omgezet naar telefoonkwaliteit |
| **Bibliotheek** | Kies een bestaande [gesproken tekst](prompts.md) |
| **Geen** | Geen audio |

Gegenereerde, opgenomen en geüploade audio wordt bewaard onder **Gesproken teksten**, zodat je hem opnieuw kunt
gebruiken in wachtrijen en andere menu's.

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
