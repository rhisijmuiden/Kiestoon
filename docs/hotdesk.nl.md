# Flexplekken

Een **flexplek** is een bureautelefoon die iedereen als de eigen telefoon kan gebruiken: je logt in met je
toestelnummer en pincode, en de telefoon laadt jouw account. Gesprekken naar jouw toestel gaan daar over, en wie je
belt ziet jouw nummer.

## Een flexplek instellen

1. Maak een toestel aan voor de plek, bijvoorbeeld **290 Flexplek 1**. Dat is het gastaccount dat de telefoon
   gebruikt zolang niemand is ingelogd. Je kunt het beperkte belrechten geven.
2. Ga naar **Toestellen & provisioning**, klik **Bewerken** bij de telefoon en kies dat toestel.
3. Zet onder **Flexplek** de optie **Deze telefoon is een flexplek** aan. Zet **Elke nacht uitloggen** aan als de
   plekken elke ochtend vrij moeten zijn (om 03:00 wordt iedereen uitgelogd).

Flexplekken werken op elke geprovisionde bureautelefoon. Ze zijn gebouwd voor Yealink.

## Inloggen en uitloggen

| Code | Wat het doet |
| --- | --- |
| `*55` | Inloggen: toets je toestelnummer en `#`, daarna je pincode en `#` |
| `*56` | Uitloggen: de telefoon gaat terug naar het gastaccount |

De pincode is de **voicemailpincode** van het toestel. Een toestel zonder pincode kan niet inloggen.

Na het in- of uitloggen laadt de telefoon binnen een paar seconden de nieuwe instellingen. Zet `*55` en `*56` op een
**snelkiestoets**, dan is het één druk op de knop.

- Een toestel is op **één flexplek tegelijk** ingelogd: inloggen op een andere plek logt het op de eerste uit.
- De toetsen van de telefoon blijven gelijk; het account op lijn 1 wisselt.
- Je eigen bureautelefoon en de softphone blijven werken. Mag het toestel maar één apparaat tegelijk hebben
  (**Apparaten tegelijk** 1), dan neemt de flexplek het over van het andere apparaat.
- Na 5 keer een verkeerde pincode is de plek 10 minuten geblokkeerd.

## In de beheeromgeving

De lijst met telefoons laat per flexplek zien wie er is ingelogd. Klik op het **uitlog**-icoon om een plek vrij te
maken. Elke in- en uitlog staat in het auditlog.
