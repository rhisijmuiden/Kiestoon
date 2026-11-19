# Telefoonboek

Het **telefoonboek** bevat gedeelde contacten voor iedereen: in het portaal, op de bureautelefoons (XML), via LDAP,
en voor de namen van inkomende bellers.

![Het telefoonboek](shot:phonebook)

## Contacten

- **Contact toevoegen**: naam, bedrijf, werk-, mobiel en ander nummer, e-mail en notities.
- **Zoeken** op naam, bedrijf of nummer. Klik op een nummer om te bellen: je eigen telefoon gaat eerst over, daarna
  wordt het nummer gebeld.
- **Importeren** van een CSV-bestand met een kopregel: `name, company, work, mobile, other, email, notes`
  (Nederlandse kolomnamen en `;` werken ook). Kies **Het hele telefoonboek vervangen** of voeg toe en sla dubbele
  over.
- **Exporteren** van het telefoonboek als CSV.

## Instellingen

| Instelling | Effect |
| --- | --- |
| **Toestellen meenemen** | Collega's staan in het telefoonboek met hun toestelnummer |
| **Namen van inkomende bellers tonen** | Staat het nummer van een beller in het telefoonboek, dan toont de telefoon de naam |
| **Telefoonboek op de toestellen (XML)** | Yealink, Snom, Grandstream en Fanvil krijgen het telefoonboek via provisioning |
| **LDAP-directory** | Voor telefoons en programma's die namen opzoeken via LDAP (389, LDAPS 636). Alleen bereikbaar vanuit je eigen netwerk; eventueel met wachtwoord (de provisioninggebruiker) |

De instellingenkaart toont de server, base DN, bind DN en het zoekfilter om in te vullen op een telefoon of
programma dat LDAP gebruikt.

Met [gekoppelde centrales](multi-site.md) staan de toestellen van de andere locaties ook in het telefoonboek.
