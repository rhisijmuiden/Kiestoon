# Tijdcondities en feestdagen

Een **tijdconditie** stuurt gesprekken tijdens openingstijden de ene kant op en daarbuiten de andere, met een
aparte bestemming voor feestdagen en een dag/nachtschakelaar voor het onverwachte.

![Tijdcondities met hun huidige stand](shot:time-conditions)

## Een tijdconditie maken

1. Open **Tijdcondities** en klik **Nieuwe tijdconditie**.
2. Geef hem een naam, bijvoorbeeld *Kantooruren*.
3. Voeg de **openingstijden** toe: dagen en een van/tot-tijd. Een lunchpauze is een tweede tijdvak (bijvoorbeeld
   08:30-12:00 en 13:00-17:30). Tijden volgen de tijdzone in [Instellingen](settings.md).
4. Kies de bestemmingen: **Als open**, **Als gesloten** en eventueel **Op feestdagen** (leeg: zelfde als gesloten).
5. Laat **Gesloten op feestdagen** aan staan om de feestdagenkalender te gebruiken.

![De tijdconditie Kantooruren bewerken](shot:time-condition-edit)

## Dag/nachtschakelaar

Elke tijdconditie heeft een **stand**:

| Stand | Effect |
| --- | --- |
| **Automatisch** | Volgt de openingstijden en feestdagen |
| **Geforceerd open** | Altijd open, bijvoorbeeld als je langer open bent |
| **Geforceerd dicht** | Altijd dicht, bijvoorbeeld bij een calamiteit of een bedrijfsuitje |

Je schakelt op de pagina Tijdcondities, op het schakelbord (receptie), of met de **schakelcode** `*28<id>` vanaf elke
telefoon; de code staat bij de tijdconditie. Nogmaals de code kiezen zet hem terug op automatisch.

## Feestdagen

Het tabblad **Feestdagen** bevat de kalender die alle tijdcondities gebruiken.

![De feestdagenkalender](shot:holidays)

- **Nederlandse feestdagen**: Nieuwjaarsdag, Goede Vrijdag, Pasen, Koningsdag, Bevrijdingsdag, Hemelvaartsdag,
  Pinksteren, Eerste en Tweede Kerstdag. Data schuiven mee met Pasen en worden elk jaar berekend. Zet de dagen uit
  waarop je wel werkt.
- **Eigen sluitingsdagen**: bijvoorbeeld een bedrijfsuitje of de dagen tussen kerst en oud en nieuw, eenmalig of
  **elk jaar**.

## Een tijdconditie gebruiken

Een tijdconditie is een [bestemming](concepts.md#bestemmingen): zet hem achter een inkomende route, in een
[keuzemenu](ivr.md) (een tijdconditiestap met de lijnen *open* en *gesloten*) of in een andere tijdconditie.
Medewerkers kunnen een tijdconditie ook als persoonlijk schema gebruiken in het gebruikersportaal (doorschakelen
als het gesloten is).
