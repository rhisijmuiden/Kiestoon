# Provider: Vodafone Ziggo

Vodafone Ziggo Zakelijk levert SIP-trunks met per trunk een gebruikersnaam en wachtwoord. Kiestoon heeft een
kant-en-klaar sjabloon **Vodafone Ziggo Zakelijk**.

## Wat het sjabloon instelt

| Instelling | Waarde | Waarom |
|---|---|---|
| Authenticatie | Registratie | Ziggo levert een gebruikersnaam en wachtwoord; registratie elk uur |
| Host / poort | `trunk.sipnl.net`, `5060` | De SIP-trunkserver van Ziggo |
| Transport | UDP | |
| Codecs | `alaw,ulaw` | G.711; vraag Ziggo als je andere codecs wilt |
| Gebeld nummer komt uit | To-header | Nummers komen in formaat `31...`; Kiestoon herkent ook `0...` en `+31...` |
| Fax | T.38 | Ziggo raadt T.38 aan voor fax; zet het aan onder Geavanceerde instellingen |

!!! warning "Vodafone Ziggo publiceert geen handleiding voor een eigen centrale"
    Deze waarden komen uit de Ziggo SIP-trunkhandleiding van Panasonic (2017). Gebruik altijd de gegevens die Ziggo
    je voor je trunk geeft: die gaan voor deze pagina.

## 1. Bij Vodafone Ziggo

1. Noteer de **gebruikersnaam** (bijvoorbeeld `31123450000`, zonder `@trunk.sipnl.net`) en het **wachtwoord**.
2. Noteer de nummers van je nummerblok, ook in formaat `31...`.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Trunk toevoegen**, provider **Vodafone Ziggo Zakelijk**.
2. **Gebruikersnaam** en **authenticatie-gebruikersnaam**: de Ziggo-gebruikersnaam. **Wachtwoord**: het Ziggo-wachtwoord.
3. **Nummerweergave**: een nummer uit je blok. Ziggo zet bij inkomende gesprekken altijd een P-Asserted-Identity-header.
4. **T.38-fax** (Geavanceerde instellingen): aan als je faxt.
5. Opslaan. De status wordt **Aangemeld**. Maak [inkomende routes](../routes.md) en een [uitgaande route](../routes.md).
6. Achter NAT? Vul het publieke adres in bij **Instellingen → Netwerk en NAT**, stuur UDP 5060 en de RTP-poorten door
   en beperk de forward tot de bronadressen van Ziggo.

## Als het niet werkt

| Symptoom | Oplossing |
| --- | --- |
| Status **Geweigerd** | Controleer de gebruikersnaam (zonder `@trunk.sipnl.net`) en het wachtwoord |
| Inkomende gesprekken komen op de verkeerde route | Zet **Gebeld nummer komt uit** op **Request-URI** |
| Gesprekken vallen weg achter NAT | Controleer de NAT-instellingen en de portforward |

Bron: [Panasonic: KX-NS-handleiding, Ziggo SIP-trunk](https://www.psn-web.net/cs-im/Global/PBX/pbx/ns_nsx/manual/SIP_Trunk/Netherlands/NS%20Series%20Setup%20Guidance%20Ver_1-2%20Ziggo.pdf).
