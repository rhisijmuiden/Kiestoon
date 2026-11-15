# Windows-softphone

**Kiestoon Softphone** is een desktop-app voor Windows 10 en 11. Hij zet de telefoon in de taakbalk: bel met je
headset, met je collega's, telefoonboek en wachtrijen bij de hand, en verbind gesprekken door door ze naar een
collega te slepen.

<div class="grid" markdown>

![Contacten met live status](shot:softphone-contacts){ .kt-phone }

![Wachtrijen](shot:softphone-queues){ .kt-phone }

![Gesprekken](shot:softphone-history){ .kt-phone }

![Voicemail](shot:softphone-voicemail){ .kt-phone }

</div>

## Wat hij kan

- **Contacten**: collega's met live status (beschikbaar, gaat over, in gesprek, niet storen), het gedeelde
  [telefoonboek](phonebook.md) en je favorieten. Zoek op naam, nummer of bedrijf.
- **Gesprekken**: opnemen, weigeren, dempen, in de wacht, toetsen en doorverbinden. Sleep het gesprek naar een
  collega om door te verbinden, **Direct** of **Eerst overleggen**, net als in het
  [gebruikersportaal](portal.md#collegas-en-doorverbinden-met-slepen).
- **Queues**: per wachtrij aanmelden, afmelden of pauzeren, met het aantal wachtende bellers.
- **Voicemail** en **gespreksgeschiedenis** met terugbellen.
- **Headsetknoppen**: opnemen, ophangen en dempen met de knoppen van Jabra, Poly/Plantronics, EPOS en andere
  USB-headsets. Geluid werkt op elke headset.
- **Geluid**: kies de microfoon, de luidspreker voor gesprekken en waar hij overgaat, bijvoorbeeld op je speakers
  terwijl je via de headset praat.
- **Windows-integratie**: pictogram in het systeemvak, pop-up bij inkomende gesprekken, meldingen bij gemiste
  gesprekken en nieuwe voicemail, `tel:`- en `sip:`-links openen in de softphone, en **Starten met Windows**.
- **Updates**: onder **Info** zoekt de softphone naar nieuwe versies en werkt zichzelf bij, via het kanaal
  **Productie** of **Test**.

## Installeren en koppelen

1. Open het gebruikersportaal en ga naar **Softphone & toestellen**.
2. Klik op **Download voor Windows** en start de installer (`.msi`). Elke Kiestoon-server levert de installer die
   bij zijn eigen versie hoort, op `/download/kiestoon-softphone.msi`.
3. Klik op **Deze computer koppelen**. De softphone gaat open en koppelt zichzelf aan je account. Ging hij niet
   vanzelf open? Vul dan de code uit het portaal in de softphone in. De code is 10 minuten geldig.

Je kunt de softphone ook vanuit de app zelf instellen: vul op het welkomstscherm het **Serveradres** in (en een
**Koppelcode** uit het portaal of de welkomstmail, als je die hebt) en klik op **Verbinden**. Log daarna in met je
gebruikersnaam en wachtwoord, of kies **Inloggen via je browser (SSO)** voor [single
sign-on](users.md#single-sign-on) en passkeys. Je browser geeft de koppeling dan door aan de softphone.

Een gekoppelde softphone blijft 30 dagen na het laatste gebruik ingelogd, en kan alleen de functies van het
gebruikersportaal voor je eigen toestel gebruiken. Koppelen zet de [softphone in de
browser](portal.md#softphone-toestellen) (WebRTC) aan voor je toestel.

!!! info "Servers met een eigen certificaat"
    Heeft de server nog geen publiek certificaat, dan toont het portaal naast de code de vingerafdruk van het
    certificaat. De softphone onthoudt precies dat certificaat, zodat de verbinding versleuteld blijft en niet
    overgenomen kan worden. Met [internettoegang](internet.md) aan wordt het Let's Encrypt-certificaat gebruikt en
    hoeft er niets onthouden te worden.

## Voor beheerders

- De softphone gebruikt dezelfde WebRTC-verbinding als de softphone in de browser: de server heeft **https** met een
  hostnaam nodig (zie [Internettoegang](internet.md) of [eigen reverse proxy](reverse-proxy.md)).
- De MSI kan uitgerold worden met Intune, Group Policy of andere softwaredistributie.
- Elke release op GitHub bevat de MSI en een bestand `SHA256SUMS`; de softphone controleert de checksum voordat
  hij bijwerkt.
