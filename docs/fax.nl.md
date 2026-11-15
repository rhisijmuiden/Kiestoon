# Fax

Kiestoon ontvangt faxen en mailt ze als pdf (**fax-naar-mail**), en verstuurt pdf's als fax vanuit het portaal of
per e-mail (**mail-naar-fax**). Geen faxapparaat nodig.

![Ontvangen faxen](shot:fax)

## Faxen ontvangen

1. Open **Fax › Faxboxen** en klik **Faxbox toevoegen**.
2. Vul de **e-mailadressen** in die de faxen krijgen (maximaal 10, gescheiden door komma's), hoe lang ze **bewaard**
   worden (0: tot je ze verwijdert) en laat **Foutcorrectie (ECM)** aan staan.
3. Maak een [inkomende route](routes.md) voor je faxnummer met de faxbox als bestemming.
4. Zet **T.38-fax** aan op de trunk als je provider het ondersteunt: dat is betrouwbaarder dan fax over G.711.

Elke ontvangen fax wordt omgezet naar pdf en gemaild. Het tabblad **Ontvangen** toont alle faxen met afzender,
nummer, pagina's en status; daar download je ook de pdf.

## Faxen versturen vanuit het portaal

1. Zet bij de faxbox **Verzenden vanuit deze box toestaan** aan en vul het **faxnummer** in (de ontvanger ziet het
   als afzender en in de faxkop).
2. Gebruikers met toegang openen **Fax › Versturen**, kiezen de box, het faxnummer en een pdf (max 11 MB) en klikken
   **Fax versturen**.
3. Kiestoon zet de pdf om, belt, en probeert het tot drie keer opnieuw als de lijn bezet is. Er gaan maximaal twee
   faxen tegelijk. De afzender krijgt een e-mail met het resultaat.

## Mail-naar-fax

1. Gebruik een aparte mailbox, bijvoorbeeld `fax@jouwbedrijf.nl`.
2. Vul op **Fax › E-mail naar fax** de IMAP-server in en log in. Kiestoon kijkt elke minuut.
3. Zet de **toegestane e-mailafzenders** bij een faxbox die mag versturen, en eventueel een **pincode in het onderwerp**.
4. Stuur een e-mail met het faxnummer in het onderwerp (of naar `fax+0201234567@jouwbedrijf.nl` als je mailserver
   plus-adressen ondersteunt) en een of meer pdf's als bijlage.

!!! warning "Veiligheid"
    Mail die niet door de SPF- of DMARC-controle van je mailserver komt, wordt genegeerd, zodat een vervalste
    afzender geen faxen kan versturen. Versturen hangt af van T.38-ondersteuning bij je SIP-provider.
