# Provider: Speakup

[Speakup](https://speakup.nl/telefonie/trunk/) levert SIP-trunks voor je eigen centrale. Kiestoon heeft een
kant-en-klaar sjabloon **Speakup**.

## Wat het sjabloon instelt

| Instelling | Waarde | Waarom |
|---|---|---|
| Authenticatie | Registratie | Speakup-trunks werken met een loginnaam en wachtwoord |
| Host / poort | `sip.speakup.nl`, `5060` | Het SIP-domein van Speakup; geen aparte proxy of registrar |
| Transport | UDP | |
| Codecs | `alaw,ulaw` | Speakup ondersteunt G.711 A-law, µ-law en G.729 en transcodeert niet |
| DTMF | RFC 2833 / 4733 | Standaard van Kiestoon |
| Gebeld nummer komt uit | To-header | Nummers komen in formaat `31...`; Kiestoon herkent ook `0...` en `+31...` |

!!! warning "Speakup publiceert geen handleiding voor een eigen centrale"
    Deze waarden komen uit koppelhandleidingen van Swyx en Tuxis (een Speakup-partner). Gebruik altijd de
    gegevens die Speakup je voor je trunk geeft: die gaan voor deze pagina.

## 1. Bij Speakup

1. Vraag Speakup om de trunkgegevens: **loginnaam**, **wachtwoord** en de **nummers** van de trunk (`31...`).
2. Optioneel, veiliger: vraag Speakup de trunk alleen te accepteren vanaf het publieke IP-adres van je Kiestoon-server.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Trunk toevoegen**, provider **Speakup**.
2. **Gebruikersnaam**: het hoofdnummer van de trunk, bijvoorbeeld `31201234567`.
3. **Authenticatie-gebruikersnaam**: de Speakup-loginnaam, als die afwijkt van het nummer. **Wachtwoord**: het
   trunkwachtwoord.
4. **Nummerweergave**: een van de trunknummers. CLIP no screening (een nummer tonen dat niet van jou is) mag niet
   in Nederland.
5. **T.38-fax**: Speakup geeft T.38 van begin tot eind door. Voor een aparte faxtrunk gebruikt Speakup
   `t38-sbc.speakup.nl`: maak een tweede trunk met die host en zet T.38 aan.
6. Opslaan, en maak [inkomende routes](../routes.md) voor de nummers en een [uitgaande route](../routes.md).

## Als het niet werkt

| Symptoom | Oplossing |
| --- | --- |
| Status **Geweigerd** | Controleer loginnaam en wachtwoord; zet de loginnaam bij Authenticatie-gebruikersnaam |
| Inkomende gesprekken komen op de verkeerde route | Zet **Gebeld nummer komt uit** op **Request-URI** |
| Geen geluid | NAT: controleer het publieke adres en de RTP-portforward |

Bronnen: [Swyx: SIP Provider SpeakUp (NL)](https://service.swyx.net/hc/en-gb/articles/360010409039-SIP-Provider-SpeakUp-NL),
[Tuxis: Speakup trunk instellen](https://support.tuxis.nl/en/knowledgebase/article/speakup-trunk-instellen-kerio-operator).
