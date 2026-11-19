# Provider: Voys

Kiestoon heeft een kant-en-klaar sjabloon voor [Voys](https://www.voys.nl). Voys draait op het VoIPGRID-platform en
noemt een SIP-trunk naar je eigen centrale een **VoIP-trunk**.

## Wat het sjabloon instelt

| Instelling | Waarde | Waarom |
|---|---|---|
| Authenticatie | Registratie | Voys geeft per VoIP-trunk een Account ID en wachtwoord |
| Host | `ha.voys.nl` | Het hoog-beschikbare SIP-domein van Voys |
| Poort | `0` (DNS SRV) | `ha.voys.nl` publiceert SRV-records: eerst UDP 6060, daarna 5060, op twee SIP-proxy's (Amsterdam en Groningen). Met poort 0 volgt Asterisk die, dus een uitgevallen proxy wordt overgeslagen |
| Transport | UDP | Voys ondersteunt UDP en TCP; geen TLS/SRTP voor VoIP-trunks |
| Codecs | `alaw,g722` | Voorkeurscodecs van Voys: G.711 A-law en G.722 |
| IP-adressen van de provider | `195.35.114.0/23`, `185.103.76.0/22` | Voys stuurt SIP en audio vanaf deze reeksen. Kiestoon accepteert gesprekken daarvandaan en zet de firewall ervoor open |
| Gebeld nummer uit | Request URI | Voys: "Bestemmings-URL gebruiken uit: Request line"; nummers komen binnen als `+31201234567` |
| Nummerweergave | `P-Asserted-Identity` en `Remote-Party-ID` | Kiestoon stuurt beide; zet de CLI-header in Freedom op Remote-Party-ID |
| DTMF | RFC 4733 | Standaard |

Inkomende INVITEs van Voys krijgen geen wachtwoordvraag (de "insecure=invite" uit oude `sip.conf`-handleidingen);
ze worden herkend aan het bronadres.

## 1. In Voys Freedom

1. Log in op [freedom.voys.nl](https://freedom.voys.nl) en open **Beheer → VoIP-trunks**
   (direct: `freedom.voys.nl/redirect/TrunkAccount`). Klik **Toevoegen**.
2. **Beschrijving**: bijvoorbeeld `Kiestoon`. Voys gebruikt dit ook als uitgaande naam.
3. **Aantal lijnen**: het maximale aantal gelijktijdige gesprekken.
4. **Intern nummer**: een vrij nummer van drie cijfers, bijvoorbeeld `900`.
5. **Belbevoegdheden**: sta alleen de bestemmingen toe die je nodig hebt (bijvoorbeeld Nederland en Europa). Dat
   beperkt de schade als het systeem ooit misbruikt wordt.
6. Zet **Caller ID van de trunk gebruiken** aan, zodat het nummer dat Kiestoon meestuurt getoond wordt. Voys
   accepteert alleen nummers die van jou zijn. Met deze optie aan gaat ze voor "Uitgaand nummer onderdrukken".
7. **Opslaan**. Voys maakt het **Account ID** en het **wachtwoord**. Je hebt beide nodig in Kiestoon.
8. Open **Geavanceerde (SIP) instellingen** en zet de **CLI-header** op `Remote-Party-ID`. Laat de vaste hostnaam
   leeg: Kiestoon registreert zichzelf.
9. Optioneel, veiliger: sta onder **Extra beveiliging** alleen het publieke IP-adres van je Kiestoon-server toe.
10. Open het belplan van elk Voys-nummer en stuur inkomende gesprekken naar deze VoIP-trunk.

## 2. In Kiestoon

1. Ga naar **Trunks & routes → Trunks → Trunk toevoegen** en kies provider **Voys**. De stap-voor-stap-uitleg staat
   ook in het formulier.
2. **Gebruikersnaam**: het Account ID. **Wachtwoord**: het trunkwachtwoord. Laat de authenticatiegebruikersnaam
   leeg.
3. **Nummerweergave**: een van je Voys-nummers, `0201234567` of `+31201234567`.
4. **Maximaal aantal gelijktijdige gesprekken**: het aantal lijnen uit Freedom.
5. Opslaan. Binnen een minuut wordt de status **Aangemeld**.
6. **Inkomende routes**: een route per Voys-nummer. Voys stuurt `+31201234567`; Kiestoon herkent ook `0201234567`
   en `31201234567`.
7. **Uitgaande routes**: een route met het voorbeeld Nederlandse nummers en de Voys-trunk.
8. Achter NAT? Vul het publieke adres in onder **Instellingen → Netwerk en NAT** en stuur de SIP-poort en de
   RTP-reeks door naar de server. De Voys-reeksen worden automatisch toegestaan in de Kiestoon-firewall.

Anoniem bellen: Voys verbergt je nummer als je er `31*` voor kiest. Voeg een uitgaand patroon met dat voorvoegsel
toe als je dat wilt.

## Problemen oplossen

| Symptoom | Oplossing |
|---|---|
| Status **Geweigerd** | Verkeerd Account ID of wachtwoord. Kopieer het wachtwoord opnieuw uit Freedom |
| Status blijft **Niet aangemeld** | Je netwerk blokkeert misschien DNS SRV of poort 6060. Zet host `sipproxy.voipgrid.nl` en poort `5060` |
| Inkomende gesprekken komen op de verkeerde route | Zet **Gebeld nummer komt uit** op **To-header** |
| Eenzijdig of geen geluid | NAT: controleer het publieke adres en het doorsturen van de RTP-poorten |
| Verkeerd nummer zichtbaar | Zet "Caller ID van de trunk gebruiken" aan in Freedom; het nummer moet van jou zijn |

## Voorwaarden van Voys voor een eigen centrale

- Voys ondersteunt het koppelen van een centrale via een VoIP-trunk, maar **niet het instellen van de centrale
  zelf**.
- Een fraudedetectiesysteem zet een trunk uit bij verdacht verkeer. Houd de belbevoegdheden krap en gebruik de
  fraudelimieten van Kiestoon.
- Voys ondersteunt elke SIP-compatibele centrale.

Bronnen: [VoIP-trunk: Mijn eigen PBX instellen](https://help.voys.nl/voip-trunk-configuratie),
[Asterisk](https://help.voys.nl/asterisk) (helpcentrum Voys), en de DNS SRV-records van `ha.voys.nl` (gecontroleerd
november 2026).
