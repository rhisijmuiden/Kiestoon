# Toestelgroepen

Een **toestelgroep** is een set toestellen die op dezelfde manier naar buiten bellen en werken, bijvoorbeeld
*Verkoop* of *Support*. Eén groep regelt drie dingen in één keer voor al zijn leden:

- een **eigen nummer** meesturen bij externe gesprekken, en een **eigen uitgaande route** (bijvoorbeeld een eigen
  trunk),
- **instellingen vastzetten** zoals voicemail, opnemen en belrechten,
- de **toetsen van hun telefoons** gelijk houden met een toetssjabloon.

Open **Toestellen › Toestelgroepen**.

![Toestelgroepen met hun nummer, vastgezette instellingen en eigen routes](shot:extension-groups)

## Een groep maken

1. Klik **Nieuwe groep** en geef hem een **naam**.
2. Kies een **uitgaand nummer** (de nummers van je trunks worden voorgesteld), of laat het leeg.
3. Klik de **leden** aan. Een toestel zit in één groep tegelijk; kies je er een uit een andere groep, dan verhuist
   hij.
4. Vink de **instellingen voor alle leden** aan die je wilt vastzetten (zie hieronder) en kies eventueel
   **toetssjablonen**.
5. Klik **Opslaan**. Kiestoon bouwt de configuratie opnieuw op en stuurt de toetsen naar de telefoons van de leden.

![Een groep bewerken: nummer, leden, vastgezette instellingen en toetssjablonen](shot:extension-group-edit)

Je kunt ook één toestel in een groep zetten vanuit zijn eigen formulier, met **Toestelgroep**.

## Welk nummer gaat naar buiten

Bij een extern gesprek stuurt Kiestoon het eerste mee dat is ingevuld:

1. het **eigen** uitgaande nummer van het toestel,
2. het nummer van de **groep**,
3. het nummer van de **uitgaande route**,
4. het nummer van de **trunk**.

Het groepsnummer is dus de standaard voor iedereen in de groep, en één persoon (een manager met een direct nummer)
kan toch een eigen nummer hebben. Het toestelformulier laat zien welk nummer meegaat als het eigen veld leeg is.

## Eigen uitgaande route per groep

Onder **Trunks › Uitgaande routes** beperkt **Wie mag deze route gebruiken** een route tot een of meer groepen.
Leden van die groepen gebruiken hem; alle anderen slaan hem over en nemen de volgende route die past. Zet de
groepsroute **boven** de algemene route.

Voorbeeld: *Support line* alleen voor de groep Support, over een eigen trunk, boven *National and mobile* voor
iedereen.

![Een uitgaande route die alleen de groep Support gebruikt](shot:outbound-route-groups)

Een groep waartoe een route beperkt is kun je niet verwijderen; haal hem eerst uit de route. Is er voor een beller
geen route meer over, dan eindigt het gesprek met een melding "geen dienst".

## Instellingen voor alle leden

Vink een instelling aan om hem voor elk lid vast te zetten. Instellingen die je niet aanvinkt blijven **per
toestel**.

| Instelling | |
| --- | --- |
| **Voicemail**, **Voicemail naar e-mail sturen**, **Verwijderen na versturen** | Voicemail naar e-mail werkt alleen bij leden met een e-mailadres |
| **Belduur (seconden)**, **Apparaten tegelijk** | |
| **Uitgaand bellen**, **Betaalnummers toestaan** | |
| **Opnemen** | Nooit, op verzoek of altijd |
| **Opnamegroep** | |
| **Browsersoftphone** | |

Een vastgezette instelling staat in het toestelformulier op slot, met de groep en de waarde erbij:

![Een vastgezette instelling in het toestelformulier](shot:extension-locked)

Haal je een toestel uit de groep (of verwijder je de groep), dan houdt het de waarden die het heeft; je kunt ze
daarna weer per toestel wijzigen.

## Telefoontoetsen voor de hele groep

Kies per telefoonmodel een [toetssjabloon](phone-keys.md). Bij opslaan van de groep krijgt elke telefoon van dat
model van een lid de toetsen van het sjabloon en wordt gevraagd ze op te halen. Een telefoon die later aan een lid
gekoppeld wordt, of een toestel dat later in de groep komt, krijgt ze ook. **Toetsen opnieuw naar de telefoons
sturen** op de regel doet het nog een keer, bijvoorbeeld nadat je het sjabloon hebt aangepast.
