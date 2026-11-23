# Importeren uit een andere telefooncentrale

**Systeem › Importeren** neemt de instellingen van je oude telefooncentrale over: toestellen,
belgroepen, wachtrijen, keuzemenu's met hun opnames, openingstijden, trunks en
telefoonnummers. Je uploadt een back-up of de configuratiebestanden, controleert een
overzicht, vinkt aan wat je wilt en Kiestoon maakt het aan. Er verandert niets in Kiestoon
totdat je op **Importeren** drukt.

![Importeren](shot:import)

| Systeem | Wat je uploadt |
|---|---|
| [UniFi Talk](import/unifi-talk.md) | De Talk-back-up (`.unifi`) uit UniFi OS |
| [Asterisk](#asterisk) | `/etc/asterisk` als `.tar.gz` of zip, of de losse `.conf`-bestanden |
| [FreePBX, Issabel, PBXact](#freepbx-issabel-pbxact) | Een FreePBX-back-up (`.tar.gz` of `.tgz`) of een mysqldump van de database `asterisk` |
| [FreeSWITCH en FusionPBX](#freeswitch-en-fusionpbx) | `/etc/freeswitch` als archief, of een FusionPBX-back-up met de databasedump |
| [3CX](#3cx) | Een 3CX-back-up (`.zip`) zonder wachtwoord, of de toestellenlijst als CSV-export |

## De stappen

1. **Uploaden.** Kies het systeem (of laat het op *Automatisch herkennen* staan) en sleep de
   bestanden erin. Je kunt tot 50 losse bestanden tegelijk uploaden, of één archief tot 2 GB.
2. **Overzicht.** Kiestoon toont alles wat het gevonden heeft, per soort, met voor elk onderdeel
   een vrij nummer in je [nummerplan](concepts.md#het-nummerplan). Het oude nummer blijft
   staan als het in het nummerplan past. Je kunt elk nummer aanpassen en uitvinken wat je niet
   wilt. Keuzemenu's staan als boom met hun toetsen, en elke opname kun je afspelen.
   Opmerkingen over wat niet overgenomen kon worden staan onder **Opmerkingen**.
3. **Importeren.** De opnames worden in je browser omgezet naar telefoonkwaliteit (8 kHz) en
   als prompt opgeslagen. Daarna worden de onderdelen aangemaakt: eerst de toestellen,
   belgroepen, wachtrijen, trunks en openingstijden, dan de keuzemenu's en inkomende routes
   die ernaar verwijzen.

Is een bestemming niet aangevinkt, dan wordt het gesprek op dat punt beëindigd; het overzicht
meldt dat. Wat al in Kiestoon bestaat met dezelfde naam wordt hergebruikt, dus je kunt de
import opnieuw doen zonder dubbele onderdelen. De geüploade bestanden worden hooguit twee uur
bewaard en verwijderd zodra je de import sluit.

!!! warning "Back-ups bevatten wachtwoorden"
    Een back-up van een telefooncentrale bevat de SIP-wachtwoorden van toestellen en trunks.
    Kiestoon houdt een SIP-wachtwoord aan als het sterk genoeg is, zodat een toestel alleen het
    nieuwe serveradres nodig heeft. Zwakke wachtwoorden worden vervangen door een nieuw
    wachtwoord, dat in het overzicht staat.

## Asterisk

Upload de map `/etc/asterisk` als `.tar.gz` of zip. Wil je de opnames meenemen, voeg dan
`/var/lib/asterisk/sounds/custom` (of waar je opnames staan) aan hetzelfde archief toe, of
upload ze als losse bestanden naast de configuratie.

```bash
tar czf asterisk-config.tar.gz /etc/asterisk /var/lib/asterisk/sounds/custom
```

Kiestoon leest:

| Uit Asterisk | Wordt in Kiestoon |
|---|---|
| Endpoints in `pjsip.conf` (ook de pjsip-wizard), peers in `sip.conf`, `users.conf` | Toestellen met hun SIP-wachtwoord |
| Registraties en endpoints met een externe host in `pjsip.conf`, peers met `register =>` in `sip.conf` | Trunks |
| `voicemail.conf` | Voicemail aan/uit, pincode en e-mail per toestel |
| `queues.conf` | Wachtrijen met hun leden en instellingen |
| `extensions.conf`: `Dial` naar meerdere toestellen | Belgroepen |
| `extensions.conf`: `Background` + `WaitExten` met toetsen | Keuzemenu's met hun opnames |
| `extensions.conf`: `GotoIfTime` | Tijdcondities |
| `extensions.conf`: nummers in de context van de trunk | Inkomende routes |

Sjablonen (`[naam](!)`), `#include`-bestanden en `(+)`-aanvullingen worden gevolgd.
Dialplanstappen waarvoor Kiestoon geen tegenhanger heeft (AGI-scripts, databaseopzoekingen,
eigen macro's) worden bij de opmerkingen genoemd. Opnames in `.wav`, `.ulaw`, `.alaw`, `.sln`
en `.gsm` worden herkend; GSM kan de browser niet omzetten, dus upload die opnieuw als WAV of
MP3 onder **Gesproken teksten**.

## FreePBX, Issabel, PBXact

Ga in FreePBX 15, 16 of 17 naar **Admin › Backup & Restore**, maak een back-up met alle
modules en download de `.tar.gz`. Back-ups van FreePBX 2.x tot en met 14 (`.tgz`) en van
Issabel of Elastix werken ook; die worden gelezen uit de databasedump erin. Een losse
mysqldump van de database `asterisk` werkt ook:

```bash
mysqldump asterisk > asterisk.sql
```

Kiestoon leest de toestellen (met voicemailinstellingen), belgroepen, wachtrijen, IVR's met
hun systeemopnames, aankondigingen, tijdcondities met hun tijdgroepen, overige bestemmingen,
trunks en inkomende routes. Functiecodes worden niet geïmporteerd, omdat Kiestoon zijn eigen
codes heeft (zie [Functiecodes](feature-codes.md)).

## FreeSWITCH en FusionPBX

Upload voor **FreeSWITCH** de map `/etc/freeswitch` als archief. Kiestoon leest de gebruikers
en groepen uit de directory, de gateways, de callcenter-wachtrijen, de IVR-menu's met hun
geluidsbestanden en het dialplan: de context `public` levert de inkomende nummers, voorwaarden
op `wday`/`time-of-day` worden tijdcondities. Voeg de map met opnames (bijvoorbeeld
`/usr/share/freeswitch/sounds/custom`) aan het archief toe om ze mee te nemen.

Upload voor **FusionPBX** een FusionPBX-back-up, of een databasedump naast de opnames:

```bash
pg_dump -U fusionpbx fusionpbx > fusionpbx.sql
tar czf fusionpbx.tar.gz fusionpbx.sql /var/lib/freeswitch/recordings
```

Zowel het tekstformaat (`.sql`) als het custom-formaat (`pg_dump -Fc`) wordt gelezen.
Kiestoon neemt de toestellen, voicemail, belgroepen, callcenter-wachtrijen, IVR-menu's,
gateways, bestemmingen (inkomende nummers) en opnames van het eerste domein over.
Tijdcondities uit FusionPBX worden niet geïmporteerd; stel ze opnieuw in op de pagina
[Tijdcondities](time-conditions.md).

## 3CX

Ga in 3CX naar **Admin › Backup**, maak een back-up **zonder wachtwoord** en download de
`.zip`. Pak hem niet uit en weer in. Met een wachtwoord versleutelt 3CX de configuratie in de
back-up, en die kan alleen 3CX zelf lezen; Kiestoon vraagt dan om een back-up zonder
wachtwoord.

3CX publiceert de opbouw van zijn back-ups niet. Kiestoon zoekt daarom de toestellen,
belgroepen, wachtrijen, digitale receptionisten, trunks en inkomende regels op naam, en neemt
de opnames uit de map `httpprompts`. Controleer het overzicht goed voordat je importeert.

De toestellenlijst uit **Users › Export** (een `.csv`-bestand) kun je ook uploaden, los of
naast de back-up. Sinds 3CX 20 staan de SIP-wachtwoorden niet meer in die export, dus de
toestellen krijgen een nieuw wachtwoord.

## Na de import

- Controleer de keuzemenu's in de [IVR-bouwer](ivr.md) en doe een testgesprek.
- Voeg de toestellen toe op de pagina **Telefoons & provisioning**; telefoons worden niet geïmporteerd,
  omdat Kiestoon ze zelf inricht.
- Zet de telefoonnummers bij je provider over naar de nieuwe trunk.
