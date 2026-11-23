# Gesproken teksten en spraak

**Gesproken teksten** zijn de aankondigingen in keuzemenu's en wachtrijen: "Welkom bij …", "Al onze collega's zijn
in gesprek …". De ingebouwde systeemteksten (voicemail, positie in de wachtrij, getallen, datums) zijn er in het
Nederlands en Engels; eigen teksten maak je op deze pagina of direct in de [keuzemenu-bouwer](ivr.md).

![Gesproken teksten met hun golfvorm](shot:prompts)

## Een tekst toevoegen

Klik **Nieuwe tekst**, geef een naam en kies:

- **Opnemen** in de browser met je microfoon (vereist het portaal via https),
- **Uploaden** van een audiobestand (MP3, WAV, M4A, OGG),
- **Typ de tekst**, kies de engine en stem en klik **Audio genereren** (als tekst-naar-spraak aan staat, zie hieronder).

Audio wordt in de browser omgezet naar telefoonkwaliteit (8 kHz mono WAV). Om alleen de naam te wijzigen laat je de
audio leeg.

## Taal van de gesproken teksten

De taal van de ingebouwde teksten stel je in onder **Instellingen › Gesprekken › Taal van gesproken berichten**.
Nederlandse teksten komen uit het pakket `kiestoon-sounds-nl`, ingesproken door een open (CC0) Piper-stem. Wat geen
Nederlandse versie heeft, valt terug op het Engels.

## Spraak

De pagina **Spraak** (menu Systeem) voegt lokale spraaktechniek toe: Piper en Whisper draaien op de server en **er verlaat geen audio de server**. Optionele cloudstemmen staan onder [ElevenLabs-stemmen](#elevenlabs-stemmen).

![Spraak: stemmen en Whisper-modellen](shot:speech)

| Functie | Wat het doet |
| --- | --- |
| **Tekst-naar-spraak (Piper)** | Typ de tekst van een menu of mededeling en Kiestoon spreekt die in met een natuurlijke stem. Download minstens één stem per taal en kies de **Engelse stem** en **Nederlandse stem** |
| **Spraak-naar-tekst (Whisper)** | Voicemailberichten worden uitgeschreven en getoond in het gebruikersportaal en in de voicemail-e-mail (de e-mail kan tot een minuut later komen) |

Stemmen en modellen download je vanuit het portaal, met vastgelegde SHA-256-controlesommen. Grotere
Whisper-modellen zijn nauwkeuriger maar trager; *small* is een goede keuze vanaf 2 CPU-kernen. Spraak vereist het
pakket `kiestoon-speech` (`apt install kiestoon-speech`, ongeveer 66 MB).

Samenvattingen van gesprekken en voicemail met een lokaal model of Claude staan op de pagina [AI-samenvattingen](ai.md).

## ElevenLabs-stemmen

Naast de lokale Piper-stemmen kan Kiestoon teksten laten inspreken door [ElevenLabs](https://elevenlabs.io), een
clouddienst met zeer natuurlijke stemmen. Het staat standaard uit en vereist een eigen ElevenLabs-account.

1. Maak op elevenlabs.io een API-sleutel aan onder **Developers › API keys**, met toegang tot text-to-speech en voices.
2. Open **Spraak**, zet **ElevenLabs gebruiken** aan, plak de sleutel en klik **Sleutel controleren en opslaan**. De
   sleutel wordt bij ElevenLabs gecontroleerd, versleuteld op de server opgeslagen en daarna nooit meer getoond.
3. Kies onder **Stemprofiel** de **Nederlandse stem** en **Engelse stem**, en zet **Engine** op **ElevenLabs** om het
   standaard te gebruiken. Zet **Stemprofiel vastzetten** aan om de stem voor elke tekst vast te leggen (zie hieronder).

![ElevenLabs-instellingen op de pagina Spraak](shot:speech)

**Kies een stem** toont de stemmenbibliotheek van ElevenLabs en de stemmen in je account. Er wordt gefilterd op de taal
van de centrale (**Instellingen › Gesprekken › Taal van gesproken berichten**), dus een Nederlandse centrale toont eerst
Nederlandse stemmen. Filter op geslacht, toepassing en accent, en klik op de afspeelknop voor een gratis voorbeeld. Een
stem uit de bibliotheek wordt aan je ElevenLabs-account toegevoegd als je hem gebruikt.

![Een stem kiezen met voorbeelden](shot:voice-picker)

In de [keuzemenu-bouwer](ivr.md) en op deze pagina begint **Genereren** met de stem van het profiel en kun je voor
één tekst een andere engine of stem kiezen, tenzij het profiel vaststaat. De
teller onder de tekst toont hoeveel tekens het kost en hoeveel er deze periode nog over zijn.

![Een menutekst genereren met ElevenLabs](shot:ivr-tts)

Goed om te weten:

- Alleen de getypte tekst gaat naar ElevenLabs. Bellers, opnames en voicemail verlaten de server nooit.
- De audio wordt omgezet naar 8 kHz WAV en op de server bewaard, dus gesprekken wachten nooit op ElevenLabs en blijven
  werken zonder internet.
- Dezelfde tekst met dezelfde stem en instellingen wordt hergebruikt en kost geen nieuwe tekens.
- **Piper gebruiken als ElevenLabs niet bereikbaar is** laat genereren doorwerken als de sleutel, het tegoed of internet
  wegvalt (vereist dat Piper aan staat met een stem).
- De beheerders krijgen een mail (naar het meldingsadres van de [mailserver](mail.md)) als 80% van de tekens van de
  periode is gebruikt.
- **Model**: *Multilingual v2* klinkt in het Nederlands het natuurlijkst; *Flash v2.5* is sneller en kost de helft van de
  tekens.

### Stemprofiel

De kaart **Stemprofiel** op de pagina **Spraak**, onder de API-sleutel, bevat alles wat ElevenLabs gebruikt om een
tekst in te spreken: de **Nederlandse stem** en **Engelse stem**, het **Model** en de steminstellingen.

| Instelling | Wat het doet |
| --- | --- |
| **Stabiliteit** | Hoger klinkt rustiger en gelijkmatiger; lager klinkt levendiger. |
| **Gelijkenis** | Hoe dicht het resultaat bij de originele stem blijft. 75% is een goed begin. |
| **Stijl overdrijven** | Geeft meer expressie. Houd 0% aan voor rustige menumededelingen. |
| **Snelheid** | 95% is iets trager dan normaal, prettig voor menukeuzes. |
| **Speaker boost** | Maakt de stem iets helderder. Aanbevolen aan de telefoon. |

Zet **Stemprofiel vastzetten** aan zodat alle teksten als dezelfde spreker klinken. Elke **Genereren**-knop (de
keuzemenu-bouwer, deze pagina en opnieuw inspreken) gebruikt dan de stem van het profiel voor de taal van de tekst,
met deze instellingen; de keuze van engine en stem maakt plaats voor het vastgezette profiel met een link hierheen.
Vastzetten vereist dat ElevenLabs aan staat, een opgeslagen sleutel en minstens één stem. Heeft een taal geen stem in
het profiel, dan spreekt de andere stem het in. Zet je ElevenLabs uit, dan vervalt het vastzetten.

![Het stemprofiel, vastgezet](shot:voice-profile)

### Een andere stem voor alle teksten

Teksten die van tekst zijn gemaakt onthouden hun tekst en stem (de kolom **Bron**). Vink ze aan op deze pagina, kies een
engine en stem, en klik **Opnieuw genereren met deze stem** om ze allemaal opnieuw in te spreken, bijvoorbeeld na het
kiezen van een nieuwe huisstem. Opgenomen en geüploade teksten kun je niet aanvinken.
