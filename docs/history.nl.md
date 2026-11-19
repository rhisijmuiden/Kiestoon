# Gespreksgeschiedenis en opnames

## Gespreksgeschiedenis

**Gespreksgeschiedenis** (menu Receptie) toont elk gesprek: tijd, beller, gebeld nummer, richting, resultaat, duur
en trunk. Zoek op een nummer, filter op periode, richting (inkomend, uitgaand, intern) en resultaat (beantwoord,
geen gehoor, bezet, mislukt), en klik **CSV exporteren** voor een spreadsheet. Klik op een gesprek voor het
[gespreksverloop](call-journey.md): elke stap die het doorliep, getekend als flow.

![Gespreksgeschiedenis](shot:history)

Medewerkers zien hun eigen gesprekken in het gebruikersportaal onder **Mijn gesprekken**, met klik-om-te-bellen om
iemand terug te bellen.

## Opnames

Gesprekken worden opgenomen als:

- een toestel opnemen op **Altijd** heeft staan,
- een toestel opnemen op **Op verzoek** heeft en de gebruiker tijdens het gesprek `*1` kiest (nogmaals om te
  stoppen),
- een [wachtrij](queues.md) **Wachtrijgesprekken opnemen** aan heeft,
- een [vergaderruimte](conferences.md) **Vergaderingen opnemen** aan heeft.

**Opnames** (menu Receptie) toont ze met datum, van, naar en lengte. Beluister in de browser, download of
verwijder. Medewerkers vinden hun eigen opnames in het gebruikersportaal.

![Opnames](shot:recordings)

### Bewaartermijn en de AVG

Onder **Belfuncties & fraude › Opnemen**:

| Instelling | |
| --- | --- |
| **Opnames bewaren (dagen)** | Oudere opnames worden automatisch verwijderd. 0 bewaart ze altijd; let op de bewaarregels van de AVG |
| **Piep tijdens opnames** | Bellers horen elke 15 seconden een korte piep |

!!! info "Vertel bellers dat je opneemt"
    Volgens de AVG moet je bellers vertellen dat gesprekken opgenomen kunnen worden en waarom. Een zin in de
    welkomsttekst van je [keuzemenu](ivr.md) of [wachtrij](queues.md) is de gebruikelijke manier.
