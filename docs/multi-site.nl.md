# Gekoppelde systemen

Twee of meer Kiestoon-systemen (bijvoorbeeld een kantoor in Nederland en een in de VS) kun je aan elkaar koppelen.
Ze houden hun eigen toestellen, trunks en instellingen, en krijgen:

- één telefoonboek: de nummers van de andere locatie verschijnen met een **locatieprefix** (bijvoorbeeld `8` + `201`);
- gratis interne gesprekken via een versleutelde WireGuard-tunnel, met de beller als prefix + nummer zodat
  terugbellen werkt;
- BLF-toetsen (bezetlampjes) voor collega's op de andere locatie (waarde van de toets: prefix + nummer);
- wachtrijen met medewerkers op beide locaties;
- **follow the sun**: een nummer gaat naar de locatie die nu open is, elk in zijn eigen tijdzone.

![Gekoppelde systemen](shot:federation)

## Twee systemen koppelen

1. Op systeem A: **Vestigingen**, zet de tunnelhub aan en vul het publieke adres in. Stuur UDP 51820 (en TCP 8443
   voor het koppelen) ernaartoe door.
2. Op systeem A: **Gekoppelde systemen › Systeem uitnodigen**. Vul de naam in en de prefix die de nummers van B op A
   krijgen.
3. Op systeem B: **Gekoppelde systemen › Aansluiten met code**. Plak de code en kies de prefix die de nummers van A
   op B krijgen.

B verbindt met de hub van A; alleen A heeft open poorten nodig. Een ster werkt ook: meerdere systemen sluiten aan bij
één hub. De code legt het certificaat van A vast, net als een koppelcode van een edge (zie
[Vestigingen](edge.md)).

Een prefix mag niet overlappen met de nummers op het systeem (toestellen, belgroepen, wachtrijen,
vergaderruimtes, parkeren 70–79) of met een andere koppeling. Prefixen beginnen met 1–9; 0 blijft de buitenlijn.

## Hoe het werkt

- Elke koppeling is een PJSIP-endpoint `fed-<id>` via TCP naar het tunneladres van het andere systeem, herkend aan
  dat adres. Kiezen van `<prefix><nummer>` gaat naar het andere systeem, dat het alleen naar zijn eigen interne
  nummers stuurt (nooit naar zijn trunks), dus een koppeling kan niet gebruikt worden om aan de andere kant naar
  buiten te bellen.
- Elke minuut haalt elke kant het telefoonboek van de ander op (`/api/fed/v1/directory` door de tunnel, met een token
  per koppeling). Wijzigingen in bezetstatus worden direct doorgegeven, en elke minuut volledig.
- Bezetlampjes van nummers op afstand zijn custom device states (`Custom:ktfed<id>-<nummer>`), bijgewerkt door het
  portaal.
- Medewerkers op afstand in een wachtrij zijn `Local/<nummer>@kiestoon-fedout-<id>`-leden met die custom device
  state, zodat de wachtrij weet wanneer ze in gesprek zijn.
- **Zonder prefix bellen als het nummer uniek is** (per koppeling): nummers van die locatie die lokaal niet bestaan
  kun je zonder prefix kiezen.
- Bestemmingen: *Gekoppeld systeem* (een nummer op de andere locatie) en *Follow the sun* kun je overal kiezen waar
  je een bestemming kiest (inkomende routes, keuzemenu's, tijdcondities, overloop van wachtrijen).

## Follow the sun { #follow-the-sun }

**Follow the sun**: een lijst regels, elk met een tijdzone, dagen en uren en een bestemming. De eerste regel die past
bij de huidige tijd in zijn eigen tijdzone krijgt het gesprek; anders gaat het naar de terugvalbestemming.
Voorbeeld: Amsterdam 08:00–17:00 (Europe/Amsterdam) naar de Nederlandse supportwachtrij, New York 09:00–17:00
(America/New_York) naar de Amerikaanse supportwachtrij op het gekoppelde systeem, anders voicemail.

![Follow the sun](shot:follow-the-sun)

## Ontkoppelen

Een koppeling aan één kant verwijderen laat de andere kant dat weten, verwijdert het endpoint en maakt bestemmingen
leeg die ernaar wezen.

## Poorten

| Poort | Waar | Wat |
| --- | --- | --- |
| UDP 51820 | hub, vanaf de andere systemen | WireGuard |
| TCP 8443 | hub, één keer tijdens het koppelen | portaal (HTTPS) |
