# AI-samenvattingen

Kiestoon kan opgenomen gesprekken en voicemailberichten uitschrijven en samenvatten: waar het gesprek over ging,
wat er is afgesproken en welke actiepunten er liggen. Open **AI** in het menu Systeem.

![AI: wat samenvatten en met welke engine](shot:ai)

## Zo werkt het

1. **De transcriptie wordt altijd op de server gemaakt** door Whisper (zie [Spraak](prompts.md#spraak)). Audio
   verlaat de telefooncentrale nooit.
2. **De samenvatting** komt van de engine die u kiest:

| Engine | Wat het betekent |
| --- | --- |
| **Lokaal model** (standaard) | Een taalmodel draait op de server zelf met llama.cpp. Gratis en privé: er verlaat niets de server. Het is trager en heeft vrij geheugen nodig (ongeveer 4 GB voor Qwen3 4B, 2 GB voor Qwen3 1.7B). Het model start zodra er werk is en stopt na 5 minuten zonder werk |
| **Claude (cloud)** | Snellere en betere samenvattingen van Claude van Anthropic, met uw **eigen API-sleutel** (aan te maken op console.anthropic.com). Alleen de tekst van de transcriptie wordt verstuurd; Anthropic rekent het gebruik af met uw account |

De API-sleutel wordt los van de andere instellingen bewaard en na opslaan nooit meer getoond (u ziet alleen de
laatste vier tekens). Verwijder hem met **Sleutel verwijderen**.

## Wat samenvatten

| Instelling | Wat het doet |
| --- | --- |
| **Opgenomen gesprekken** | Nieuwe opnames (van gesprekken die toch al worden opgenomen) worden op de achtergrond uitgeschreven en samengevat, nieuwste eerst, één tegelijk |
| **Voicemail** | Voicemailberichten krijgen een samenvatting en actiepunten in het gebruikersportaal, de Windows-softphone en de voicemailmail |
| **Taal van de samenvattingen** | *Zelfde als het gesprek*, of altijd Nederlands of Engels |
| **Samenvattingen van gesprekken bewaren** | Verwijdert samenvattingen en transcripties van gesprekken na dit aantal dagen. 0 bewaart ze zolang de opname bestaat |

Druk op **Uitproberen** om een voorbeeldvoicemail samen te vatten met de huidige instellingen, nog voor u ze opslaat.

## Samenvattingen bij opnames

Onder **Opnames** (en **Mijn gesprekken** in het portaal) toont een gesprek met een samenvatting het onderwerp en de
stemming van de beller. Klik op de sterretjesknop voor de samenvatting, de actiepunten en de volledige transcriptie.
Receptie en beheerders kunnen een losse opname ook direct **Uitschrijven en samenvatten**, ook als automatische
samenvattingen uit staan.

![Een opname met de AI-samenvatting](shot:recording-ai)

!!! note "Spraakherkenning kan fouten maken"
    Een samenvatting is zo goed als de transcriptie. Controleer namen en nummers met de opname.
    Het model krijgt de opdracht niets te verzinnen dat niet in de transcriptie staat.

## Vereisten

- Het pakket `kiestoon-speech` (versie 1.1.0 of nieuwer bevat llama.cpp voor het lokale model).
- Spraak-naar-tekst aan onder **Spraak**, met een gedownload Whisper-model.
- Voor het lokale model: het model gedownload op de AI-pagina (vaste SHA-256-controlesommen).
