# Privacybeleid van de Kiestoon-app

_Geldt voor de Kiestoon-app voor Android en iPhone, uitgegeven door RHIS. Laatst bijgewerkt: 27 november 2026._

De Kiestoon-app is de mobiele client van een **Kiestoon-telefooncentrale**. Die centrale wordt beheerd door je
werkgever of hun IT-partner (de "organisatie"), niet door RHIS. De app maakt rechtstreeks verbinding met de
centrale van je organisatie; RHIS beheert die centrale niet en kan je gesprekken, contacten, gespreksgeschiedenis
en voicemail niet zien.

## Wat de app verwerkt, en waar het naartoe gaat

| Gegevens | Waarom | Waar het naartoe gaat |
| --- | --- | --- |
| **Adres van de telefooncentrale** en de **inlogsessie** | Om verbinding te maken met je centrale | Alleen opgeslagen op je telefoon |
| **Microfoongeluid** tijdens een gesprek | Om te kunnen bellen | Rechtstreeks naar de centrale van je organisatie, versleuteld (DTLS-SRTP). Nooit naar RHIS. De microfoon wordt alleen tijdens een gesprek gebruikt |
| **Pushtoken** van je telefoon (van Google Firebase Cloud Messaging of Apple) en de **naam van de telefoon** (bijvoorbeeld "Pixel 9") | Om de app te laten overgaan als hij dicht is, en om de telefoon te tonen onder Mijn apparaten | Naar de centrale van je organisatie. Het pushtoken wordt verzegeld door de pushdienst van RHIS, zie hieronder |
| Je **collega's, telefoonboek, gespreksgeschiedenis, wachtrijen en voicemail** | Om ze in de app te tonen | Opgehaald van de centrale van je organisatie en getoond in de app. Niet opgeslagen door RHIS |

De centrale van je organisatie houdt eigen gegevens bij (bijvoorbeeld de gespreksgeschiedenis). Daarvoor is je
organisatie verantwoordelijk; vraag hen om hun privacybeleid.

## De pushdienst van RHIS

Google en Apple bezorgen meldingen voor een app alleen namens de uitgever, dus de centrale van je organisatie
stuurt wekmeldingen via de pushdienst van RHIS (`push.rhis.nl`):

- Hij **bewaart niets**: het pushtoken wordt omgezet in een verzegelde sleutel die alleen de pushdienst kan
  openen, en die sleutel wordt bewaard door de centrale van je organisatie, niet door RHIS.
- Hij **kan de meldingen niet lezen**: wie er belt en de samenvatting van de voicemail zijn versleuteld met een
  sleutel die alleen de centrale en je telefoon kennen.
- Hij **logt geen telefoonnummers, tokens of inhoud**.

De melding zelf wordt bezorgd door Google (Firebase Cloud Messaging) of Apple (Apple Push Notification service).

## Wat de app niet doet

- Geen advertenties, geen tracking, geen analytics en geen crashrapportage.
- Geen toegang tot de contacten, foto's, locatie of bestanden op je telefoon.
- Geen verkoop of delen van gegevens met derden.

## Toestemmingen

| Toestemming | Waarom |
| --- | --- |
| Microfoon | Zodat de ander je hoort tijdens een gesprek |
| Meldingen | Zodat de app kan overgaan bij inkomende gesprekken en nieuwe voicemail kan tonen |
| Melding op volledig scherm (Android) | Om een inkomend gesprek over het vergrendelscherm te tonen, zoals het eigen belscherm van de telefoon |
| Voorgrondservice voor telefoongesprekken en de microfoon (Android) | Om een gesprek door te laten lopen als je van app wisselt of het scherm vergrendelt |

## Jouw keuzes

- **Ontkoppel** de telefoon onder Mijn apparaten in het Kiestoon-portaal (of vraag het je beheerder). De centrale
  vergeet het pushtoken dan meteen.
- **Verwijder de app** van je telefoon: alles wat de app op de telefoon bewaarde, verdwijnt mee.
- Voor gegevens die de centrale van je organisatie bewaart, neem je contact op met je organisatie.

Er zijn geen Kiestoon-accounts in de app: je account wordt aangemaakt en verwijderd door je organisatie.

## Beveiliging

Alle verbindingen gebruiken HTTPS en versleuteld geluid. De software van de app en de pushdienst is open source:
[github.com/rhisijmuiden/Kiestoon](https://github.com/rhisijmuiden/Kiestoon).

## Contact

Vragen over dit beleid: gebruik de contactgegevens van RHIS op de Google Play- of App Store-pagina van de app, of
open een issue op [GitHub](https://github.com/rhisijmuiden/Kiestoon/issues). Wijzigingen in dit beleid worden op
deze pagina gepubliceerd.
