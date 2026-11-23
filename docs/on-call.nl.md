# Storingsdienst

Een **storingsdienst** is een intern nummer dat de engineer belt die op dat moment dienst heeft, op zijn eigen
mobiele nummer. Kiestoon bepaalt zelf aan de hand van het rooster wie dat is: je hoeft niets om te zetten als de
dienst wisselt. Neemt de engineer niet op, dan loopt het gesprek door de **escalatie**: de back-up, de teamleider,
iedereen, een vast nummer en tot slot een bestemming zoals een voicemailbox. Nummers liggen in de reeks van de
belgroepen.

Engineers zien hun diensten onder **Mijn storingsdiensten** in het portaal en kunnen daar diensten ruilen of
overdragen aan collega's. Een ruiling gaat alleen voor die periode om het reguliere rooster heen.

![Overzicht storingsdienst](shot:on-call)

## Een storingsdienst maken

1. Open **Storingsdienst** en klik op **Nieuwe storingsdienst**. Geef de dienst een naam en een nummer.
2. Kies op het tabblad **Rooster** hoe de dienst rouleert:
    - **Wekelijks wisselen**: de engineers hebben om de beurt een week dienst. Stel de **overdrachtsdag** en de
      **tijd** in (bijvoorbeeld maandag 08:00) en de **startdatum**: de eerste engineer in de lijst heeft dienst vanaf
      de overdracht op of vóór die datum.
    - **Dagelijks wisselen**: hetzelfde, per dag.
    - **Geplande diensten**: geen rotatie; je plant elke dienst op het overzicht met **Tijdelijke overname**.
3. Voeg de **engineers** toe in de volgorde van de rotatie. Elke engineer wordt gebeld op het mobiele nummer in zijn
   eigen profiel. Ontbreekt het nummer, dan zie je een waarschuwing; klik op het nummer om het te wijzigen.
4. **Back-up (2e lijn)**: de volgende in de rotatie, een vaste persoon of niemand.
5. **Teamleider**: wordt gebeld in de stap teamleider, krijgt mail over gaten in het rooster en keurt ruilingen goed.
6. **Ruilen en overdragen**: direct geregeld zodra de collega akkoord geeft, of pas na goedkeuring door de
   teamleider of een beheerder.
7. **Herinneringen**: engineers krijgen de dag voordat hun dienst begint een mail.

![Rooster storingsdienst](shot:on-call-schedule)

Om bellers bij de storingsdienst te laten uitkomen kies je **Storingsdienst** als bestemming van een
[inkomende route](routes.md), een [tijdconditie](time-conditions.md) (bijvoorbeeld buiten kantoortijd) of een toets
in een [keuzemenu](ivr.md).

## Escalatie

Op het tabblad **Escalatie** staan de stappen die van boven naar beneden lopen tot iemand opneemt. Per stap kies je
wie er wordt gebeld en hoeveel seconden:

| Stap | Wie wordt gebeld |
|---|---|
| Dienstdoende engineer (1e lijn) | De engineer met dienst volgens het rooster, ruilingen meegerekend |
| Back-up (2e lijn) | De back-up op dit moment |
| Teamleider | De teamleider van de dienst |
| Iedereen | Alle engineers van de dienst tegelijk |
| Een gebruiker | Eén vaste persoon |
| Een nummer | Een vast telefoonnummer, bijvoorbeeld een externe servicedesk |

Een stap zonder iemand om te bellen (geen back-up, of een gat in het rooster) wordt overgeslagen. **Stappen
herhalen** loopt de hele lijst nog eens door, tot vijf keer. Na de laatste stap gaat het gesprek naar **Als niemand
opneemt**.

- **Engineer drukt 1 om aan te nemen**: de engineer hoort dat het een storingsmelding is en drukt op 1. Zo kan de
  voicemail van een mobiel het gesprek nooit aannemen; zonder de 1 volgt de volgende stap.
- **Ook toestel en app laten overgaan**: naast het mobiele nummer gaan ook het eigen toestel, de softphone en de
  Kiestoon-app van de engineer over.
- **Muziek tijdens het wachten**: de beller hoort wachtmuziek in plaats van overgaan.
- **Welkomstbericht voor de beller**: een [gesproken tekst](prompts.md) die eerst wordt afgespeeld, zoals "U wordt
  doorverbonden met de dienstdoende monteur".
- **Voorvoegsel nummerweergave**: staat voor de naam van de beller, zodat engineers zien dat het een storingsmelding
  is.

![Escalatie storingsdienst](shot:on-call-escalation)

Elk gesprek naar de storingsdienst staat in de [gespreksroute](call-journey.md): welke stap wie belde en wie het
gesprek aannam.

## Tijdelijke overname

Op het overzicht zet **Tijdelijke overname** iemand een tijd op dienst, bijvoorbeeld als een engineer ziek is. Een
overname gaat voor het reguliere rooster en voor eerdere ruilingen. Het tabblad **Ruilingen en wijzigingen** toont
alle overnames, ruilingen en overdrachten; daar kun je ze ook intrekken.

Het overzicht toont ook **Aandachtspunten**: gaten in de komende 14 dagen waarin niemand dienst heeft, en engineers
zonder mobiel nummer. De teamleider krijgt een mail over gaten.

## Agenda-link

Onder **Rooster** geeft **Agenda-link (iCal)** een link waarop je je abonneert in Outlook, Google of Apple Agenda.
Iedereen met de link kan het rooster lezen; **Nieuwe link** maakt de oude ongeldig. Engineers vinden dezelfde link
op hun eigen pagina.

## Mijn storingsdiensten

Engineers openen **Mijn storingsdiensten** in het portaal. Ze zien:

- hun **mobiele nummer**, dat ze zelf kunnen wijzigen;
- of ze nu dienst hebben, en hun **komende diensten**;
- het rooster van de komende 14 dagen.

**Ruilen** naast een dienst vraagt een collega om die over te nemen:

- **Overdragen**: de collega neemt je dienst over (of een deel ervan, van een tijd tot een tijd).
- **Ruilen**: de collega neemt jouw dienst en jij neemt er een van hem terug.

De collega geeft akkoord of weigert in zijn eigen portaal en krijgt een mail. Afhankelijk van de instelling van de
dienst keurt de teamleider het daarna goed. Tot die tijd geldt het reguliere rooster. **Nu overnemen** zet jezelf
direct op dienst, bijvoorbeeld als de dienstdoende engineer je dat telefonisch vroeg.

![Mijn storingsdiensten](shot:my-on-call)
