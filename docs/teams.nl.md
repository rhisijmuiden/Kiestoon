# Microsoft Teams

Met **Direct Routing** bellen mensen die in Microsoft Teams werken gewoon met telefoonnummers, via Kiestoon en je eigen
trunks. Kiestoon is daarbij de **SBC** (session border controller) van je Microsoft 365-tenant: Teams stuurt zijn
gesprekken naar Kiestoon, en Kiestoon laat Teams-gebruikers overgaan net als elk ander toestel.

![Overzicht van Microsoft Teams](shot:teams)

## Wat je nodig hebt

- **Internettoegang** aan, met **Certificaat ook voor SIP gebruiken**. Microsoft accepteert alleen een openbaar
  certificaat (Let's Encrypt is prima) met een **RSA**-sleutel; Kiestoon vraagt Let's Encrypt om RSA zodra het
  certificaat voor SIP gebruikt wordt.
- Een **in Microsoft 365 geverifieerd domein** voor de SBC-naam (bijvoorbeeld `pbx.voorbeeld.nl` op het geverifieerde
  domein `voorbeeld.nl`), met minstens één gebruiker met licentie op dat domein.
- Een **Teams Phone**-licentie voor elke Teams-gebruiker die via Kiestoon belt (in Microsoft 365 E5, of als add-on).
- De **PowerShell-module MicrosoftTeams** en een Teams-beheerdersaccount.

## Inrichten

1. Ga naar **Telefonie > Microsoft Teams** en zet **Microsoft Teams Direct Routing** aan. De domeinnaam van de SBC
   wordt ingevuld vanuit Internettoegang.
2. Controleer de **SIP TLS-poort** (standaard 5067). Die staat los van de TLS-poort van de toestellen, omdat Microsoft
   de domeinnaam in elk SIP-bericht wil zien. De firewall zet deze poort alleen open voor de adressen van Microsoft
   (`52.112.0.0/14` en `52.122.0.0/15`).
3. Kies hoe **gesprekken naar buiten vanuit Teams** gaan: via de normale uitgaande routes, of via één trunk. Kies de
   **nummerweergave**: het eigen Teams-nummer van de gebruiker, of het nummer van de route of trunk.
4. Sla op en **herstart de telefooncentrale** eenmalig (Instellingen): Asterisk opent de nieuwe poort pas na een
   herstart.
5. Voeg de Teams-gebruikers toe (hieronder) en voer daarna het PowerShell-script uit van het tabblad
   **Microsoft 365 instellen**.

**Voortgang inrichting** op het overzicht laat zien wat nog ontbreekt. **SIP-proxy's van Microsoft** laat zien of
Kiestoon de drie proxy's van Microsoft bereikt (`sip`, `sip2` en `sip3.pstnhub.microsoft.com`); valt er een uit, dan
gaan gesprekken naar de volgende.

## Teams-gebruikers

![Teams-gebruikers](shot:teams-users)

Elke Teams-gebruiker krijgt een **telefoonnummer** (E.164, bijvoorbeeld `+31 20 123 4567`) en eventueel een **toestel**:

| Gaat over op | Wat er gebeurt |
| --- | --- |
| **Teams + bureautoestel** | Gesprekken naar het toestel laten het bureautoestel, de softphone en Teams tegelijk rinkelen. Wie het eerst opneemt heeft het gesprek. |
| **Alleen Teams** | Gesprekken naar het toestel gaan alleen in Teams over. |
| Geen toestel | Gesprekken naar het nummer gaan in Teams over, tenzij een inkomende route dat nummer ergens anders heen stuurt. |

Met een toestel bellen Teams-gebruikers collega's op toestelnummer en zien anderen hen als dat toestel; wachtrijen,
belgroepen en het schakelbord bereiken hen net als elk ander toestel. Hun belrechten (nationaal, internationaal) zijn
die van het toestel. Teams-nummers zonder toestel mogen nationaal naar buiten bellen. 112 werkt altijd.

## Het PowerShell-script

![Microsoft 365 instellen](shot:teams-setup)

Kiestoon schrijft het script met je domeinnaam, poort en gebruikers. Kopieer of download het en voer het uit in
PowerShell:

```powershell
Install-Module MicrosoftTeams     # eenmalig
.\kiestoon-teams.ps1
```

Het script voegt Kiestoon toe als **PSTN-gateway**, maakt een **voice route** die elk nummer naar Kiestoon stuurt en
het belbeleid (**voice routing policy**) "Kiestoon", en geeft elke gebruiker zijn nummer en dat beleid. Je kunt het
gerust opnieuw uitvoeren na het toevoegen of wijzigen van gebruikers. Microsoft heeft soms tot een uur nodig voordat
een nieuwe gateway of nummer werkt.

## Problemen oplossen

- **Certificaat niet klaar**: controleer Internettoegang. Het certificaat moet de SBC-domeinnaam dekken en RSA
  gebruiken. Gebruikt het nog ECDSA, sla Internettoegang dan opnieuw op zodat Let's Encrypt een RSA-certificaat geeft.
- **Proxy's onbereikbaar**: controleer of poort 5067 (of jouw poort) in je router naar Kiestoon wordt doorgestuurd, en
  of de telefooncentrale na het aanzetten van Teams is herstart.
- In het **Teams-beheercentrum**, onder *Spraak > Direct Routing*, toont Microsoft de TLS- en SIP OPTIONS-status van
  de gateway.
- Audio loopt altijd via de mediaservers van Microsoft (geen media bypass); de RTP-poorten moeten bereikbaar zijn.
