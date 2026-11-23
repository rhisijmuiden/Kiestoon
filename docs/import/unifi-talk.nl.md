# Importeren uit UniFi Talk

Kiestoon kan een UniFi Talk-telefooncentrale overnemen uit een Talk-backup: de smart
attendants (keuzemenu's) **met hun bandjes**, de gebruikers met hun toestelnummer,
belgroepen, wachtrijen, de SIP-trunk en de telefoonnummers. De import gaat via de pagina
**Importeren**; zie het [importoverzicht](../import.md) voor de stappen die voor elke centrale
gelden.

## 1. Maak de backup in UniFi OS

1. Open de UniFi OS-console (UDM, UDM Pro, UCG, Cloud Key of UniFi OS Server) waarop Talk draait.
2. Ga naar **Settings → Control Plane → Backups** (oudere versies: **System → Backups**).
3. Kies **Download** en selecteer de applicatie **Talk**. Je krijgt een bestand zoals
   `unifi_os_backup_for_talk_1795438324390_97bfc7f6.unifi`.

Een volledige consolebackup werkt ook, zolang Talk erin zit.

!!! warning "Bewaar het bestand veilig"
    Een UniFi OS-backup bevat ook certificaten, privésleutels en SIP-wachtwoorden. UniFi
    versleutelt hem met een sleutel die op elke console hetzelfde is, dus iedereen die het
    bestand heeft kan het in feite lezen. Kiestoon leest alleen het Talk-deel, houdt het
    bestand alleen in het geheugen voor het voorbeeld en slaat het niet op.

## 2. Uploaden en het voorbeeld controleren

Upload het `.unifi`-bestand op de pagina **Importeren**. Kiestoon herkent het vanzelf en laat
zien wat er wordt aangemaakt:

| Uit Talk | Wordt in Kiestoon |
|---|---|
| Gebruiker (toestelnummer, naam, SIP-wachtwoord, voicemail aan/uit) | Toestel. Het SIP-wachtwoord blijft hetzelfde als Kiestoon het accepteert, dus een telefoon heeft alleen het nieuwe serveradres en nummer nodig |
| Belgroep (leden, tegelijk of na elkaar, actie bij niet opnemen) | Belgroep |
| Wachtrij (agents, strategie, maximale wachttijd, beltijd, nawerktijd, doorschakelen naar voicemail) | Wachtrij |
| SIP-gateway (provider, account, wachtwoord, nummers) | Trunk, met het passende providersjabloon van Kiestoon (bijvoorbeeld Voys) |
| Telefoonnummer en waar het overgaat | Inkomende route |
| Smart attendant met toetsen, submenu's en bandjes | Keuzemenu in de [IVR-bouwer](../ivr.md), één bandje per stap |
| Algemene voicemailbegroeting | Gesproken tekst |

Elk toestel, elke groep en elke wachtrij krijgt een vrij nummer in je Kiestoon-nummerplan.
Talk-nummers blijven behouden als ze passen (zelfde lengte, juiste reeks); in het voorbeeld kun
je elk nummer aanpassen. Wat in Kiestoon al bestaat met dezelfde naam wordt hergebruikt, dus
je kunt de import opnieuw draaien zonder dubbele onderdelen.

### Hoe de attendants worden omgezet

- Eerst speelt het welkomstbandje, daarna kiest de beller een toets, net als in Talk.
- Een toets met een eigen bandje speelt dat af en gaat daarna naar de gebruiker, groep,
  wachtrij of attendant waar hij naar verwijst.
- Een toets die een submenu opent wordt een stap **Keuzemenu** met eigen toetsen.
- "Druk 9 om het menu te herhalen", in Talk gebouwd met een extra attendant die terugstuurt,
  gaat in Kiestoon direct terug naar het hoofdmenu; die extra attendant wordt niet
  geïmporteerd.
- Wat er gebeurt als de beller niets kiest wordt de stap **Geen keuze**.
- De bandjes (MP3) worden in je browser omgezet naar telefoonkwaliteit (8 kHz) en opgeslagen
  als gesproken teksten, genoemd naar de attendant en de stap (bijvoorbeeld
  `Talk - IJRB V2 - Zwembad`).

## 3. Wat niet wordt geïmporteerd

- **Telefoons.** Kiestoon richt zijn eigen telefoons in; voeg ze toe op de pagina
  **Telefoons**. Telefoons met een SIP-account blijven werken zodra ze naar Kiestoon wijzen,
  met het nieuwe nummer en hetzelfde wachtwoord.
- **Meldingen die een wachtrij afspeelt** voordat bellers worden doorgestuurd (bijvoorbeeld
  "er is niemand beschikbaar"). Ze worden wel als gesproken tekst geïmporteerd, zodat je ze in
  een keuzemenu kunt gebruiken.
- Gesproken (tekst-naar-spraak) begroetingen van attendants zonder bandje: maak een gesproken
  tekst, bijvoorbeeld met [tekst naar spraak](../prompts.md).
- Voicemailberichten, gespreksgeschiedenis, sms, contacten en Talk-instellingen zoals
  telefoonontwerpen en omroepen.

Controleer na de import de keuzemenu's in de IVR-bouwer, doe een testgesprek (**Testgesprek**
in de bouwer) en zet de nummers bij je provider over naar de nieuwe trunk.
