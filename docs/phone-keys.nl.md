# Toetsen en uitbreidingsmodules

Elke bureautelefoon heeft een **visuele toetseneditor**: een afbeelding van de telefoon met de toetsen links en
rechts van het display, in pagina's, plus eventuele uitbreidingsmodules. Klik op een toets om hem in te stellen;
sleep een toets op een andere om ze te wisselen, ook naar een andere pagina of module.

![De toetseneditor van een Yealink T54W met een EXP50-module](shot:phone-keys)

## De editor openen

- Beheerders: **Toestellen & provisioning**, klik **Bewerken** bij een telefoon.
- Medewerkers: in het gebruikersportaal onder **Softphone & toestellen** stellen ze de toetsen van hun eigen
  telefoon en modules in.

## Soorten toetsen

| Soort | Wat de toets doet | Waarde |
| --- | --- | --- |
| **Lijn** | Een lijn (account) op de telefoon | Lijnnummer |
| **BLF (collega)** | Laat zien of een collega vrij is, overgaat of in gesprek is; druk om te bellen, of om op te nemen als het overgaat | Toestel |
| **Snelkeuze** | Belt een nummer | Telefoonnummer |
| **Gesprek parkeren** | Parkeert het gesprek; de beller hoort muziek tot iemand het oppakt op een parkeerplaats (71-79) | |
| **Parkeerplek** | Licht op als daar een gesprek geparkeerd staat; druk om het op te pakken | 71 … 79 |
| **Intercom** | Belt een collega die opneemt via de luidspreker | Toestel |
| **Niet storen** | Zet niet storen aan en uit (`*76`), het lampje toont de stand | |
| **Voicemail** | Belt je voicemail (`*97`) | |
| **Leeg** | Doet niets | |

Elke toets heeft ook een **label** voor het display.

## Extra lijnen

Lijn 1 is het toestel van de telefoon. Bureautelefoons van Yealink, Snom en Grandstream kunnen **extra lijnen**
dragen (accounts 2 en hoger), bijvoorbeeld een gedeelde verkooplijn. Kies de toestellen onder **Extra lijnen** en
zet ze op lijntoetsen.

## Uitbreidingsmodules

Klik **Uitbreidingsmodule** en kies de module. Alle modules op een telefoon zijn van hetzelfde type.

| Module | Toetsen | Voor | Max |
| --- | --- | --- | --- |
| Yealink EXP40 | 40 (2 pagina's van 20) | T46U, T48U | 6 |
| Yealink EXP43 | 60 (3 pagina's van 20) | T43U, T46U, T48U | 3 |
| Yealink EXP50 | 60 (3 pagina's van 20) | T53W, T54W, T57W | 3 |
| Snom D7 | 18 | D717, D735, D785 | 3 |
| Grandstream GBX20 | 40 (2 pagina's van 20) | GXP2170, GRP2616 | 4 |

Moduletoetsen stel je precies zo in als telefoontoetsen. Een receptiemodule vol BLF-toetsen laat het hele kantoor in
één oogopslag zien.

## Toetssjablonen

Een **toetssjabloon** is een toetsindeling voor één telefoonmodel, inclusief uitbreidingsmodules. Gebruik het om
alle receptietelefoons, of alle telefoons van een team, dezelfde toetsen te geven.

1. Stel één telefoon in en klik **Opslaan als sjabloon**, of open het tabblad **Toetssjablonen** en klik **Nieuw
   sjabloon**.
2. Klik bij het sjabloon **Toepassen op telefoons**, kies de telefoons van dat model en bevestig. Hun toetsen en
   modules worden vervangen.
3. Telefoons die online zijn halen hun nieuwe toetsen direct op; de rest bij de volgende provisioningcontrole.

**Sjabloon laden…** in de editor van één telefoon kopieert een sjabloon naar die telefoon voordat je opslaat.

## Intercom

Intercom (`*80<toestel>` of een intercomtoets) bereikt alleen bekende toestellen die vrij zijn en niet op niet
storen staan. Alle apparaten gaan over, ook de browsersoftphone. Per telefoon kies je hoe hij opneemt:

| Optie | Effect |
| --- | --- |
| **Intercomgesprekken automatisch aannemen** | De telefoon neemt op via de luidspreker |
| **Eerst een waarschuwingstoon laten horen** | Een korte piep voordat de beller te horen is |
| **Beginnen met de microfoon uit** | De gebelde is pas te horen na het uitzetten van mute |
| **Ook tijdens een ander gesprek** | Het intercomgesprek wordt aangenomen terwijl de gebruiker aan het bellen is |

Het formulier van de telefoon toont alleen de opties die het merk ondersteunt:

| Merk | Opties |
| --- | --- |
| Yealink, Snom, Fanvil | Alle vier |
| Poly | Automatisch aannemen en gedempte microfoon |
| Grandstream, Htek, Cisco MPP, Akuvox-deurstations | Automatisch aannemen aan of uit |

## Wanneer krijgen telefoons hun toetsen?

Na het opslaan vraagt Kiestoon een geregistreerde telefoon om zijn configuratie op te halen (check-sync). De toetsen
veranderen binnen enkele seconden. Een telefoon die niet geregistreerd is, neemt de wijziging over bij de volgende
provisioningcontrole of na een herstart.
