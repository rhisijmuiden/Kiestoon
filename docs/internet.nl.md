# Internettoegang en SSL

Op je eigen netwerk is het portaal altijd bereikbaar op `http://<server>:8080` (en `https://<server>:8443` met een
zelfondertekend certificaat). Wil je het portaal, de softphone en passkeys overal vandaan gebruiken met een
vertrouwd certificaat, zet dan **Internettoegang** aan. Kiestoon start dan **Caddy** op poort 80 en 443, haalt een
gratis certificaat bij **Let's Encrypt** voor je domeinnaam en vernieuwt het zelf.

![Internettoegang](shot:internet)

## Stap voor stap

1. **DNS**: maak een `A`-record (en `AAAA` voor IPv6) aan voor de domeinnaam, bijvoorbeeld `pbx.example.com`, dat
   naar je publieke IP-adres wijst. Kiestoon laat zien waar de naam nu naar wijst, en waarschuwt als hij nog
   nergens naar verwijst.
2. **Router**: stuur poort **80** en **443** (TCP) door naar de Kiestoon-server.
3. Ga in Kiestoon naar **Internettoegang**, zet **Directe internettoegang** aan, vul de **Domeinnaam** en een
   **E-mail voor Let's Encrypt** in (die krijgt bericht als vernieuwen mislukt) en sla op.
4. Caddy haalt het certificaat binnen ongeveer een minuut op. Het blok **Certificaat** toont voor wie het is, wie
   het uitgaf en tot wanneer het geldig is. Is er nog geen certificaat, dan probeert Caddy het elke paar minuten
   opnieuw.

De firewall zet poort 80 en 443 automatisch open.

| Optie | Wat het doet |
| --- | --- |
| **Certificaat ook voor SIP over TLS gebruiken** | Toestellen en softphones vertrouwen de versleutelde SIP-verbinding (poort 5061) zonder dat je een certificaat installeert. |
| **Testcertificaten (Let's Encrypt staging)** | Om uit te proberen zonder tegen de limieten van Let's Encrypt aan te lopen. Browsers vertrouwen deze certificaten niet. |

## Wat werkt via het internetadres

- Het beheerportaal en het [gebruikersportaal](portal.md), met een geldig slotje.
- De [softphone in de browser](portal.md#softphone-toestellen) en de [Windows-softphone](softphone.md) (WebRTC via
  beveiligde WebSockets).
- [Passkeys](users.md#passkeys), die https en een hostnaam nodig hebben.
- Links in welkomstmails, voicemailmails en zero-touch-[provisioning](phones.md).

!!! tip "Heb je al een reverse proxy?"
    Staat er al nginx, Traefik, HAProxy of een firewall met webproxy voor? Laat internettoegang dan uit en volg
    [Eigen reverse proxy](reverse-proxy.md).

!!! warning "Telefoons van buiten"
    Internettoegang is voor het webportaal en de softphones. Bureautoestellen en SIP-apps buiten kantoor hebben nog
    steeds SIP nodig in de [firewall](firewall.md). Gebruik voor vestigingen een [Edge](edge.md): die verbindt via
    een versleutelde tunnel, zonder SIP open te zetten naar internet.
