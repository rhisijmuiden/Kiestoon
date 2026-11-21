# CRM-koppelingen en belpop-up

Kiestoon koppelt met **HubSpot**, **Salesforce**, **Pipedrive** en **Exact Online**. Terwijl het toestel overgaat,
toont een pop-up wie er belt volgens het CRM, en elk gesprek wordt automatisch bij de klant in het CRM gelogd.
Zonder CRM verandert er niets.

![Systeem › Integraties › CRM](shot:crm)

## Wat het doet

- **Bellers herkennen.** Zodra een gesprek van buiten binnenkomt, zoekt Kiestoon het nummer op in de CRM’s, in de
  volgorde die je instelt. Nummers worden vergeleken op de laatste 9 cijfers, zodat `06…`, `+316…` en `00316…`
  allemaal hetzelfde contact vinden. Resultaten worden 10 minuten bewaard.
- **Pop-up.** In het webportaal (op elke pagina), de Windows-softphone en op het schakelbord. Je ziet het contact,
  het bedrijf, open deals en tickets, het laatste contact en de eigenaar, met een knop om het record in het CRM
  te openen. Gaat de browsertelefoon over, dan staan er ook *Opnemen* en *Weigeren* in. Bij een onbekend nummer
  maak je met één knop het contact aan.
- **Naam op bureautoestellen.** De naam op het display wordt `Jan de Vries - Bakkerij De Vries`. Het gesprek wacht
  maximaal 1,5 seconde op het CRM; is dat trager, dan gaat het toestel over met alleen het nummer.
- **Gesprekken loggen.** Elk gesprek met een klant wordt een activiteit bij het contact (en bedrijf), onder de
  collega die het voerde: richting, duur, uitkomst en het gebelde nummer. Gemiste gesprekken kunnen een taak
  *Terugbellen* worden.
- **Na het gesprek** vraagt de pop-up om een notitie, de uitkomst en eventueel een vervolgtaak. De notitie komt bij
  de activiteit die automatisch is gelogd.
- **Opnames verlaten de server nooit.** Het CRM krijgt een link waarvoor je moet inloggen bij Kiestoon. Staan
  AI-samenvattingen van gesprekken aan ([AI](ai.md)), dan komt de samenvatting een paar minuten later bij de
  activiteit.

Alleen het telefoonnummer van de beller gaat naar het CRM om het op te zoeken. Tokens en geheimen worden op de
server bewaard en daarna nooit meer getoond.

## Een CRM koppelen

Open **Systeem › Integraties › CRM** en klik bij het CRM op **Koppelen**.

![Een CRM koppelen](shot:crm-connect)

### HubSpot

1. In HubSpot: **Instellingen › Integraties › Private apps › Private app aanmaken**.
2. Rechten: `crm.objects.contacts.read` en `.write`, `crm.objects.companies.read`, `crm.objects.deals.read`,
   `tickets`, `crm.objects.owners.read`.
3. Plak het toegangstoken in Kiestoon en sla op. Kiestoon controleert het token meteen.

### Salesforce

1. In Setup: **External Client App Manager › New**. Zet OAuth aan met de scope *Manage user data via APIs (api)*.
2. Schakel de **client credentials flow** in en kies een *run as*-gebruiker die contacten en accounts mag lezen en
   taken mag aanmaken.
3. Vul je **My Domain-URL** in (`https://acme.my.salesforce.com`), de **consumer key** en het **consumer secret**.

### Pipedrive

1. In Pipedrive: je naam › **Persoonlijke voorkeuren › API**, kopieer het token. Gebruik een account dat alle
   personen ziet.
2. Vul het **bedrijfsdomein** in (het deel vóór `.pipedrive.com`) en het token.

Pipedrive-organisaties hebben geen telefoonveld, dus Kiestoon zoekt alleen personen op.

### Exact Online

1. In het [Exact App Center](https://apps.exactonline.com): **Apps beheren › Test- of productie-app registreren**.
   Gebruik de **redirect-URI** die Kiestoon in het formulier toont (`https://<jouw centrale>/api/crm/exact/callback`).
2. Vul het land, de **client ID** en het **client secret** in, klik op **Aanmelden bij Exact** en meld je aan met
   een Exact-gebruiker die relaties mag zien.
3. Kiestoon vernieuwt de aanmelding zelf. Exact kan niet op telefoonnummer zoeken, dus Kiestoon houdt een lijst
   bij van de nummers van alle contacten en accounts en ververst die elk uur.

Exact kent geen gespreksactiviteit: gesprekken worden als *communicatienotitie* bij de relatie gelogd.

!!! note "Het adres van de centrale"
    De redirect-URI en de links naar opnames gebruiken het adres van de centrale uit [Eigen hostnamen](hostnames.md)
    of [Internettoegang](internet.md). Stel dat in voordat je de Exact-app registreert.

## Instellingen per CRM

| Instelling | Wat |
| --- | --- |
| Contactpersonen / bedrijven opzoeken | Welke records worden doorzocht op het nummer van de beller |
| Tonen in de pop-up | Open deals, open tickets, laatste activiteit, eigenaar, levenscyclusfase |
| Elk gesprek automatisch loggen | Inkomend en uitgaand, alleen inkomend, of alleen beantwoorde gesprekken |
| Gemiste gesprekken worden een taak | Een taak *Terugbellen* voor de eigenaar van het contact |
| Link naar de opname toevoegen | Als het gesprek is opgenomen |
| AI-samenvatting toevoegen | Als AI-samenvattingen van gesprekken aan staan |
| Contact aanmaken voor onbekende nummers | Standaard uit: collega’s maken er een aan vanuit de pop-up |
| Gebruikers ↔ eigenaren | Gekoppeld op e-mailadres; kies met de hand een eigenaar als dat afwijkt |

## Voor alle CRM’s

- **Volgorde van opzoeken**: het eerste gekoppelde CRM dat het nummer kent, wint. Onbekende nummers vallen terug op
  het [telefoonboek](phonebook.md).
- **CRM-naam tonen op bureautoestellen** (standaard aan).
- **Pop-up voor iedereen bij wie het toestel overgaat**: ook voor collega’s in een wachtrij of belgroep. Uit: alleen
  wie opneemt, ziet hem.

## Synchronisatielog en gespreksgeschiedenis

Het **CRM-synchronisatielog** toont de opzoekingen en schrijfacties van de laatste 30 dagen. Is een CRM niet
bereikbaar, dan probeert Kiestoon het na 1, 5 en 30 minuten opnieuw en daarna elk uur, 24 uur lang; daarna krijgen
de beheerders een mail en kun je op **Nu opnieuw** klikken. Weigert een CRM de opgeslagen aanmelding, dan staat op
de kaart **Opnieuw aanmelden** en slaat het opzoeken dat CRM over.

De [gespreksgeschiedenis](history.md) krijgt een kolom **CRM**: *Gelogd*, *Taak aangemaakt*, *Opnieuw proberen* of
*Geen klant*. Klik erop om het record in het CRM te openen.

Een nummer testen zonder te bellen doe je met **Nummer testen** op het CRM-tabblad.

## Andere software

Voor een CRM dat niet in de lijst staat, gebruik je de [API en webhooks](api.md): de webhook `call.ringing` voor
een pop-up in je eigen software en `POST /api/calls` voor klikken-om-te-bellen.
