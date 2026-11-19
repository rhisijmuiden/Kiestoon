# Gebruikersportaal

Elke medewerker met een [gebruikersaccount](users.md) dat aan een toestel gekoppeld is, krijgt het
**gebruikersportaal**. Het adres is hetzelfde als dat van het beheerportaal: Kiestoon toont het menu dat bij de rol
past. Medewerkers regelen hier hun eigen telefoon, zonder de beheerder lastig te vallen.

| Pagina | Wat je daar doet |
| --- | --- |
| **Mijn toestel** | Niet storen, doorschakelen, volg mij, je eigen tijden |
| **Voicemail** | Berichten beluisteren, begroetingen opnemen, pincode wijzigen |
| **Wachtrijen & groepen** | Aan- en afmelden bij wachtrijen, je belgroepen zien |
| **Collega's** | Zien wie beschikbaar is, bellen en gesprekken doorverbinden met slepen |
| **Mijn gesprekken** | Je eigen gespreksgeschiedenis, terugbellen met één klik |
| **Softphone & toestellen** | Softphone in de browser, bel-app met QR-code, toetsen van je bureautoestel |

## Mijn toestel

![Mijn toestel](shot:my-phone)

| Instelling | Wat het doet |
| --- | --- |
| **Niet storen** | Bellers horen bezet of gaan naar je voicemail. Ook `*76` op de telefoon. |
| **Doorschakelen naar mijn mobiel** | Eén klik: elk gesprek gaat naar je mobiel tot je op **Stoppen** drukt. |
| **Altijd doorschakelen naar** | Je telefoon gaat helemaal niet over; gesprekken gaan direct naar dit nummer. |
| **Bij bezet doorschakelen naar** | Als je al in gesprek bent. |
| **Bij geen antwoord doorschakelen naar** | Na je beltijd. Komt in plaats van de voicemail. |

Doorschakelen kan naar een collega, een belgroep of een extern nummer. Externe doorschakelingen gebruiken een
uitgaande route, dus de grenzen van de [fraudebescherming](firewall.md#fraudebescherming) blijven gelden.

### Volg mij

Met **Ook andere nummers laten overgaan** gaat eerst je eigen telefoon over, daarna gaan ook de nummers over die je
toevoegt, bijvoorbeeld je mobiel en je telefoon thuis.

- **Na (seconden) laten overgaan**: `0` laat alles tegelijk overgaan.
- **Vragen om 1 te drukken**: wie opneemt moet eerst op 1 drukken. Zo neemt de voicemail van een mobiel het
  gesprek niet aan.

### Mijn tijden

Kies onder **Volg de tijden van** de [tijdconditie](time-conditions.md) die je volgt, bijvoorbeeld "Kantoortijden".
Buiten die tijden gaan je gesprekken naar **Buiten deze tijden doorschakelen naar**, of naar je voicemail als dat
leeg is. **Altijd bereikbaar** zet het uit.

## Voicemail

Zie [Voicemail](voicemail.md): berichten, begroetingen en de pincode regel je allemaal op deze pagina.

## Wachtrijen & groepen

![Wachtrijen & groepen](shot:my-queues)

Meld je aan bij de [wachtrijen](queues.md) waar je lid van bent, één voor één of met **Bij alle aanmelden** /
**Bij alle afmelden**. Aangemeld betekent dat je bellers uit die wachtrij krijgt. Vaste leden zijn altijd
aangemeld. Op het bureautoestel doet `*45` hetzelfde. Onder **Mijn belgroepen** zie je de
[belgroepen](ring-groups.md) waar je in zit en wie daarin beschikbaar is.

## Collega's en doorverbinden met slepen

![Collega's met live status](shot:colleagues)

De pagina Collega's toont elk toestel, elke belgroep en elke wachtrij met de live status: beschikbaar, gaat over,
in gesprek of niet storen. Klik op een collega om te bellen. **Een nummer bellen** belt elk nummer; met de softphone
aan begint het gesprek in de browser, anders gaat eerst je bureautoestel over en zet het het gesprek op zodra je
opneemt.

**Doorverbinden** werkt voor elke medewerker hetzelfde:

1. Neem het gesprek aan op je bureautoestel of de softphone. Het verschijnt onder **Mijn actieve gesprekken**.
2. Kies **Direct doorverbinden** of **Eerst overleggen**.
3. Sleep het gesprek naar een collega, belgroep of wachtrij. Of druk op **Doorverbinden** en kies een bestemming;
   `Esc` annuleert.
4. Bij **Eerst overleggen** wacht de beller in de wacht terwijl jij de collega spreekt. Druk daarna op
   **Doorverbinden voltooien**, of op **Terug naar beller** als de collega het niet kan aannemen.

!!! tip "Receptie"
    Het [schakelbord](switchboard.md) werkt op dezelfde manier, maar toont alle live gesprekken van het kantoor in
    plaats van alleen die van jou.

## Mijn gesprekken

![Mijn gesprekken](shot:my-calls)

Je eigen inkomende, uitgaande en gemiste gesprekken, met **Terugbellen** op elke regel. Beheerders zien alle
gesprekken onder [Gespreksgeschiedenis](history.md).

## Softphone & toestellen

![Softphone & toestellen](shot:devices)

**Softphone in de browser.** Druk op **Softphone aanzetten** en bel en neem op met elke headset, direct in de
browser. De browser vraagt één keer om de microfoon. De softphone blijft werken terwijl je de rest van het portaal
gebruikt en staat onderaan het scherm. Hij heeft **https** nodig: open het portaal via het
[internetadres](internet.md) of je [eigen reverse proxy](reverse-proxy.md). De beheerder kan de softphone per toestel
uitzetten.

**Bel-app.** **Mijn instellingen tonen** laat het SIP-account zien en een QR-code voor een app als Linphone, Zoiper
of Groundwire. Scan de code in de app en hij is ingesteld.

**Mijn bureautoestellen.** Telefoons die voor jouw toestel [geprovisioned](phones.md) zijn verschijnen hier.
Medewerkers stellen zelf hun [toetsen](phone-keys.md) in, bijvoorbeeld een snelkeuze of een BLF-lampje voor een
collega. De telefoon neemt de nieuwe toetsen over bij de volgende herstart of provisioning-controle.

**Windows.** Voor een volwaardige softphone in de taakbalk, zie de [Windows-softphone](softphone.md).

**Mobiele app.** Als je beheerder het toestaat, toont **Mobiele app koppelen** een QR-code voor de Kiestoon-app
op je telefoon, zie [Mobiele app](mobile-app.md).
