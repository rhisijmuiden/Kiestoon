# Voicemail

Elk toestel kan een **voicemailbox** hebben. Bellers spreken een bericht in als niemand opneemt, als de lijn bezet
is, of als een route of keuzemenu ze direct naar de voicemail stuurt.

## Aanzetten

In het formulier van het toestel (zie [Toestellen](extensions.md)):

- **Voicemail** aan, met een **pincode** van 4-8 cijfers (wordt gemaakt als je hem leeg laat).
- **Voicemail naar e-mail sturen**: het bericht komt als audiobijlage, met beller, tijd en lengte. Vereist een
  e-mailadres bij het toestel en een [mailserver](mail.md).
- **Verwijderen na versturen**: het bericht staat dan alleen nog in de e-mail.

Met [spraak-naar-tekst](prompts.md#spraak) aan tonen de e-mail en het portaal ook een **uitgeschreven tekst** van het
bericht.

## Beluisteren

| Waar | Hoe |
| --- | --- |
| **Gebruikersportaal › Voicemail** | Berichten afspelen in de browser, markeren als beluisterd, verwijderen |
| **Telefoon** | Kies `*97` vanaf je eigen telefoon, of de voicemailtoets |
| **Elke telefoon** | Kies `*98`, vul het boxnummer en de pincode in |
| **E-mail** | De audiobijlage |

![Voicemail in het gebruikersportaal](shot:voicemail)

## Begroetingen

In **Gebruikersportaal › Voicemail › Begroetingen** nemen medewerkers hun eigen begroetingen op in de browser:

| Begroeting | Speelt |
| --- | --- |
| **Niet bereikbaar** | Als je niet opneemt |
| **In gesprek** | Als je al aan het bellen bent |
| **Naam** | Je naam, gebruikt in standaardberichten |
| **Tijdelijk** | Gaat voor de andere zolang hij bestaat, bijvoorbeeld tijdens een vakantie |

Zonder eigen begroeting horen bellers een standaardbericht met het toestelnummer, in de taal uit
[Instellingen](settings.md). De pincode wijzig je op dezelfde pagina.
