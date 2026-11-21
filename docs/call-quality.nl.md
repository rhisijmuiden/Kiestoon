# Gesprekskwaliteit

**Gesprekskwaliteit** (menu Telefonie) laat zien hoe gesprekken klinken: MOS, jitter, pakketverlies en round trip,
per gesprek, per trunk en per toestel. Gaat er iets mis met een trunk of toestel, dan krijg je een mail en zie je aan
welke kant van de verbinding het probleem zit.

![Gesprekskwaliteit](shot:call-quality)

## Waar de cijfers vandaan komen

Tijdens een gesprek wisselt Asterisk ongeveer elke 5 seconden een RTCP-rapport uit met elk toestel en elke provider.
Kiestoon leest beide richtingen van elk deel van het gesprek:

- **Ontvangen door Kiestoon**: de audio die Kiestoon van het toestel of de provider krijgt, door Kiestoon zelf gemeten.
- **Gemeld door het toestel / de provider**: de audio die Kiestoon verstuurt, zoals de andere kant meldt dat die aankwam.

Uit jitter, pakketverlies, round trip en codec schat Kiestoon de **MOS** (mean opinion score) met het E-model van de
ITU, op een schaal van 1 tot 4,5. Een normaal telefoongesprek scoort ongeveer 4,4. De MOS van een deel van het gesprek
is die van de slechtste richting, en de MOS van een gesprek is die van het slechtste deel. Je hoeft niets in te
stellen: elk gesprek wordt gemeten.

| MOS | Oordeel |
| --- | --- |
| 4,0 en hoger | Goed |
| 3,6 tot 4,0 | Redelijk: mensen merken het, maar kunnen praten |
| onder 3,6 | Slecht: woorden vallen weg, mensen herhalen zichzelf |

## De pagina

- **Meldingen** bovenaan: trunks en toestellen die nu over hun drempel zitten.
- **Gemiddelde MOS**, **Slechte gesprekken**, **Zwakste trunk** en **Actieve meldingen** over de gekozen periode
  (laatste 24 uur, 7 dagen of 30 dagen).
- **Lopende gesprekken**: de gesprekken van nu met hun huidige MOS, jitter, verlies en round trip, elke 5 seconden
  bijgewerkt. Klik op een gesprek voor het [gespreksverloop](call-journey.md).
- **MOS per trunk**: een lijn per trunk over de periode; de gekleurde band is het slechte bereik.
- **Toestellen met de zwakste kwaliteit**: de tien toestellen met de laagste MOS en waar het misgaat, bijvoorbeeld
  "Verlies richting het toestel (wifi of lokaal netwerk)".
- **Trunks**: per trunk het aantal gesprekken, de gemiddelde MOS, het aandeel slechte gesprekken, jitter en verlies
  in beide richtingen (ontvangen door Kiestoon / gemeld door de provider), round trip en het verloop.

Waarden boven een drempel zijn rood. De kolom **Waar het misgaat** leest de richting: verlies op de audio *richting*
een toestel wijst op het netwerk van dat toestel; verlies op de audio *van* een provider wijst op de provider of je
internetlijn.

## In de gespreksgeschiedenis

De [gespreksgeschiedenis](history.md) heeft een kolom **MOS**, en het [gespreksverloop](call-journey.md) van een
gesprek toont een kaart **Gesprekskwaliteit**: een korte diagnose, elk deel van het gesprek in beide richtingen, de
round trip en een strook met de MOS van elk rapport, zodat je ziet wanneer het tijdens het gesprek misging. Het
dashboard toont de MOS van de lopende gesprekken en de actieve meldingen.

## Meldingen

Onder **Gesprekskwaliteit › Instellingen** stel je de drempels in en wanneer je een mail krijgt.

![Instellingen gesprekskwaliteit](shot:call-quality-settings)

- **Drempels**: goed vanaf en slecht onder (MOS), en de drempels voor jitter, pakketverlies en round trip.
- **Trunk met slechte kwaliteit**: de gemiddelde MOS van de laatste 15 minuten ligt onder de slecht-drempel, bij
  minstens 3 gesprekken.
- **Toestel met slechte gesprekken**: minstens 3 slechte gesprekken op één toestel in de laatste 60 minuten.
- **Elk slecht gesprek**: een mail voor elk gesprek onder de slecht-drempel. Bij drukke installaties levert dit veel
  mails op.
- **Weer in orde**: een mail als de trunk of het toestel weer binnen de drempels zit.
- **Mail naar**, de **Rustperiode** voordat dezelfde melding opnieuw wordt gemaild, en hoe lang metingen bewaard
  blijven (standaard 90 dagen). **Testmail versturen** stuurt een voorbeeldmelding.

Meldingen gaan via de [mailserver](mail.md) van Kiestoon en komen in het auditlog.

## Prometheus

Met [metrics](api.md) aan heeft `/metrics` ook, over de laatste 15 minuten:

| Metric | Wat |
| --- | --- |
| `kiestoon_call_mos` | Gemiddelde MOS van de gespreksdelen |
| `kiestoon_call_poor_total` | Gesprekken onder de slecht-drempel |
| `kiestoon_trunk_mos{trunk}` | Gemiddelde MOS per trunk |
| `kiestoon_trunk_jitter_ms{trunk}`, `kiestoon_trunk_packet_loss_percent{trunk}`, `kiestoon_trunk_rtt_ms{trunk}` | Jitter, verlies en round trip per trunk |
| `kiestoon_active_call_mos_min` | Laagste huidige MOS van de lopende gesprekken |
| `kiestoon_quality_alerts_active` | Actieve meldingen gesprekskwaliteit |

## Goed om te weten

- Toestellen en providers die geen RTCP-rapporten sturen, krijgen alleen de richting die Kiestoon zelf meet.
- Een gesprek dat nog loopt, toont de cijfers tot nu toe.
- De MOS is een schatting uit netwerkcijfers, geen opname van wat mensen hoorden. Het is een betrouwbare wegwijzer
  naar waar problemen zitten en hoe erg ze zijn.
