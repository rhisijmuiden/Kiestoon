# Gesproken teksten en spraak

**Gesproken teksten** zijn de aankondigingen in keuzemenu's en wachtrijen: "Welkom bij …", "Al onze collega's zijn
in gesprek …". De ingebouwde systeemteksten (voicemail, positie in de wachtrij, getallen, datums) zijn er in het
Nederlands en Engels; eigen teksten maak je op deze pagina of direct in de [keuzemenu-bouwer](ivr.md).

![Gesproken teksten met hun golfvorm](shot:prompts)

## Een tekst toevoegen

Klik **Nieuwe tekst**, geef een naam en kies:

- **Opnemen** in de browser met je microfoon (vereist het portaal via https),
- **Uploaden** van een audiobestand (MP3, WAV, M4A, OGG),
- **Typ de tekst** en klik **Audio maken** (als tekst-naar-spraak aan staat, zie hieronder).

Audio wordt in de browser omgezet naar telefoonkwaliteit (8 kHz mono WAV). Om alleen de naam te wijzigen laat je de
audio leeg.

## Taal van de gesproken teksten

De taal van de ingebouwde teksten stel je in onder **Instellingen › Gesprekken › Taal van gesproken berichten**.
Nederlandse teksten komen uit het pakket `kiestoon-sounds-nl`, ingesproken door een open (CC0) Piper-stem. Wat geen
Nederlandse versie heeft, valt terug op het Engels.

## Spraak

De pagina **Spraak** (menu Systeem) voegt lokale spraaktechniek toe. **Geen audio verlaat de server.**

![Spraak: stemmen en Whisper-modellen](shot:speech)

| Functie | Wat het doet |
| --- | --- |
| **Tekst-naar-spraak (Piper)** | Typ de tekst van een menu of mededeling en Kiestoon spreekt die in met een natuurlijke stem. Download minstens één stem per taal en kies de **Engelse stem** en **Nederlandse stem** |
| **Spraak-naar-tekst (Whisper)** | Voicemailberichten worden uitgeschreven en getoond in het gebruikersportaal en in de voicemail-e-mail (de e-mail kan tot een minuut later komen) |

Stemmen en modellen download je vanuit het portaal, met vastgelegde SHA-256-controlesommen. Grotere
Whisper-modellen zijn nauwkeuriger maar trager; *small* is een goede keuze vanaf 2 CPU-kernen. Spraak vereist het
pakket `kiestoon-speech` (`apt install kiestoon-speech`, ongeveer 66 MB).

Samenvattingen van gesprekken en voicemail met een lokaal model of Claude staan op de pagina [AI-samenvattingen](ai.md).
