# Eigen hostnamen

Kiestoon kan twee adressen gebruiken op dezelfde server:

| Adres | Voorbeeld | Gebruikt voor |
| --- | --- | --- |
| **Adres centrale** | `https://sip.mijndomein.nl` of `https://voip.mijndomein.nl` | Beheer, provisioning van toestellen, firmware, telefoonboek, SIP-registratie, SAML |
| **Adres medewerkersportaal** | `https://voipcentrale.mijndomein.nl` | Het [gebruikersportaal](portal.md), welkomstmails, links om een wachtwoord in te stellen, de [Windows-softphone](softphone.md) |

Beide adressen komen bij dezelfde Kiestoon uit. Zonder adres voor het medewerkersportaal gebruikt iedereen het
adres van de centrale, zoals voorheen.

![Hostnamen](shot:internet)

## Met de ingebouwde internettoegang (Caddy)

1. Maak DNS-records aan voor **beide** namen, die naar je publieke IP-adres wijzen.
2. Ga naar **Internettoegang**, vul de **Domeinnaam** in (het adres van de centrale, bijv. `sip.mijndomein.nl`)
   en de **Hostnaam medewerkersportaal** (bijv. `voipcentrale.mijndomein.nl`), en sla op.
3. Caddy haalt voor elke naam een eigen Let's Encrypt-certificaat. Het blok **Certificaat** toont ze allebei.

Staat **Certificaat ook voor SIP over TLS gebruiken** aan, dan gebruikt SIP het certificaat van het
centrale-adres.

## Achter je eigen reverse proxy

1. Ga naar **Internettoegang › Eigen reverse proxy**, vul het **Publiek adres** (centrale) en het
   **Adres medewerkersportaal** in.
2. Laat in je proxy beide hostnamen naar dezelfde Kiestoon wijzen. Geef de `Host`-header ongewijzigd door (of
   zet `X-Forwarded-Host`; Kiestoon leest die alleen van een vertrouwde proxy).

Voorbeeld voor nginx:

```nginx
server {
    listen 443 ssl http2;
    server_name sip.mijndomein.nl voipcentrale.mijndomein.nl;
    ssl_certificate     /etc/ssl/mijndomein.nl/fullchain.pem;   # certificaat voor beide namen
    ssl_certificate_key /etc/ssl/mijndomein.nl/privkey.pem;
    location / {
        proxy_pass http://10.0.0.20:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;
    }
}
```

## Beheer blokkeren op het adres van het medewerkersportaal

Zet in het blok **Hostnamen** de optie **Beheer blokkeren op het adres van het medewerkersportaal** aan. Op dat
adres:

- krijgen beheerders het gebruikersportaal en het schakelbord, nooit de beheerpagina's of de beheer-API;
- toont het menu **Beheer is op dit adres gesloten** met een link naar het adres van de centrale;
- werken medewerkers en receptie gewoon door.

Je kunt dit alleen aanzetten vanaf het adres van de centrale, zodat je jezelf niet buitensluit.

!!! warning "Beheer helemaal van internet houden"
    Kiestoon herkent de adressen aan de hostnaam. Wie het centrale-adres kent en de server kan bereiken, kan het
    nog steeds proberen. Wil je volledige scheiding, publiceer dan alleen het portaaladres in je proxy of
    publieke DNS, en bereik het centrale-adres via je eigen netwerk of een VPN. Combineer dit met regels in de
    [Firewall](firewall.md) voor de beheernetwerken.

## Wat gaat waarheen

| Link of instelling | Adres |
| --- | --- |
| Welkomstmail, link om wachtwoord in te stellen | Medewerkersportaal |
| Koppelen van de Windows-softphone | Medewerkersportaal |
| Softphone in de browser (WebRTC) | Het adres waarop de pagina geopend is |
| SIP-server in het portaal en de welkomstmail | Hostnaam centrale |
| Provisioning, firmware, telefoonboek-XML, zero-touch | Adres centrale |
| Inloggen met OpenID Connect | Het adres waarop je inlogt (registreer beide terugkeeradressen, zie **Hostnamen**) |
| Inloggen met SAML | Adres centrale |
| Passkeys | Per adres: een passkey van het ene adres werkt niet op het andere |

SIP (5060/5061) en audio (RTP) gaan nooit via een webproxy: toestellen registreren rechtstreeks op de hostnaam
van de centrale, dus dat DNS-record moet wijzen naar het adres waar die poorten doorgestuurd worden.
