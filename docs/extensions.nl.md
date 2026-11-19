# Toestellen (extensies)

Een **toestel** (extensie) is een intern nummer voor één persoon of één telefoon, met een SIP-account, een
voicemailbox en persoonlijke instellingen. Elke bureautelefoon, softphone en browsertab van die persoon gaat erop
over.

![Toestellen met status, voicemail en transport](shot:extensions)

## Een toestel toevoegen

1. Open **Toestellen** en klik **Toestel toevoegen**. Kiestoon vult het volgende vrije nummer uit het
   [nummerplan](concepts.md#het-nummerplan) in.
2. Vul de **naam** en het **e-mailadres** in.
3. Laat **Portalaccount aanmaken en welkomstmail versturen** aan staan. De medewerker krijgt (in het Nederlands of
   Engels):
    - een eenmalige link om een wachtwoord in te stellen (72 uur geldig; er gaat nooit een wachtwoord per e-mail),
    - de adressen van het portaal, de gebruikersnaam en het toestelnummer,
    - de SIP-gegevens voor een softphone-app op de mobiel, en hoe voicemail werkt,
    - een opmerking als single sign-on aan staat.
4. Klik **Opslaan**. Het SIP-wachtwoord wordt gemaakt (24 tekens) en de configuratie wordt opnieuw opgebouwd.

Zonder mailserver wordt het account toch aangemaakt en krijg je de link om zelf door te geven. Met de knop **Welkomstmail versturen**
op de regel stuur je de welkomstmail later opnieuw.

![Een toestel bewerken](shot:extension-edit)

## Instellingen per toestel

| Instelling | Wat het doet |
| --- | --- |
| **Transport** | UDP (standaard), TCP of TLS. Met TLS kun je ook **audio versleutelen (SRTP)** |
| **Apparaten tegelijk** | Hoeveel apparaten tegelijk mogen registreren, bijvoorbeeld bureautoestel, softphone en mobiele app (standaard 3) |
| **Uitgaand nummer** | Het nummer dat anderen zien. Leeg: de uitgaande route of trunk bepaalt het |
| **Belduur** | Seconden voordat het gesprek naar voicemail of de doorschakeling bij geen gehoor gaat (standaard 25) |
| **Voicemail** | Aan of uit, met een **pincode** (4-8 cijfers, wordt gemaakt als je hem leeg laat) |
| **Voicemail naar e-mail sturen** | Het bericht komt als audiobijlage; eventueel **verwijderen na versturen** |
| **Uitgaand bellen** | Alleen intern, nationaal of internationaal |
| **Betaalnummers toestaan** | 0900, 0906 en dergelijke. Uit is veiliger |
| **Opnemen** | Nooit, op verzoek (`*1` tijdens een gesprek) of altijd. Zie [opnames](history.md) |
| **Opnamegroep** | `*8` neemt gesprekken op die in dezelfde groep overgaan |
| **Browsersoftphone** | De gebruiker kan met een headset bellen in het [gebruikersportaal](portal.md) |

## SIP-gegevens en telefoons

De knop **SIP-gegevens** op een regel toont de SIP-gegevens (server, poort, gebruikersnaam, wachtwoord, voicemailnummer)
voor een softphone of een telefoon die je met de hand instelt. **Nieuw wachtwoord** maakt een nieuw
SIP-wachtwoord; telefoons met het oude werken pas weer als ze het nieuwe krijgen. Geprovisionde telefoons krijgen
het automatisch.

De knop **Telefoons van deze extensie** op een regel toont de bureautelefoons die aan dit toestel gekoppeld zijn, en laat je daar meteen
een wachtende telefoon koppelen. Zie [Telefoons en provisioning](phones.md).

## Status

De kolom **Status** toont *Online* als minstens één apparaat geregistreerd is. Het dashboard toont dezelfde
informatie als "Toestellen online".

## Doorschakelen, niet storen en follow-me

Dit zijn persoonlijke instellingen. De medewerker zet ze in het [gebruikersportaal](portal.md) (Mijn toestel) of
met [sneltoetscodes](feature-codes.md) op de telefoon: `*72<nummer>` schakelt alles door, `*73` stopt, `*76` zet
niet storen aan of uit. Beheerders zien en wijzigen dezelfde instellingen in het formulier van het toestel.

## Een toestel verwijderen

De prullenbak verwijdert het toestel en de voicemailbox. Gekoppelde telefoons werken zonder account door tot je ze
aan een ander toestel koppelt.
