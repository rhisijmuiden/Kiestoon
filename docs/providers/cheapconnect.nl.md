# Provider: Cheapconnect

[Cheapconnect](https://www.cheapconnect.nl) is een Nederlandse VoIP-provider met SIP-accounts die werken met je
eigen centrale. Kiestoon heeft een kant-en-klaar sjabloon **Cheapconnect**.

## Wat het sjabloon instelt

| Instelling | Waarde | Waarom |
|---|---|---|
| Authenticatie | Registratie | Cheapconnect stuurt inkomende gesprekken alleen naar een aangemelde centrale |
| Host / poort | `sip.cheapconnect.net`, `5060` | De SIP-server van Cheapconnect |
| Transport | UDP | Zoals in het Asterisk-voorbeeld van Cheapconnect |
| Codecs | `alaw,ulaw,g722` | Cheapconnect biedt G.711 (A-law, µ-law), G.722 en G.729 |
| Gebeld nummer komt uit | To-header | Werkt met de registratie van Kiestoon; kies Request-URI als gesprekken op de verkeerde route komen |
| User agent | `Kiestoon PBX` | Cheapconnect reserveert de user agent `asterisk`; Kiestoon stuurt al een eigen naam |

!!! note "Controleer de gegevens in je Cheapconnect-account"
    Gebruik altijd de waarden uit je eigen Cheapconnect-dashboard: die gaan voor deze pagina.

## 1. Bij Cheapconnect

1. Log in op je Cheapconnect-account en open het SIP-account (het telefoonnummer) dat je wilt gebruiken.
2. Noteer de **SIP-gebruikersnaam**: het nummer in internationaal formaat zonder `+`, bijvoorbeeld `31851234567`.
3. Noteer het **SIP-wachtwoord**.
4. Optioneel, veiliger: beperk het account tot het publieke IP-adres van je Kiestoon-server en stel een
   bestedingslimiet in.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Trunk toevoegen**, provider **Cheapconnect**. De uitleg staat ook in het formulier.
2. **Gebruikersnaam**: het nummer, bijvoorbeeld `31851234567`. **Wachtwoord**: het SIP-wachtwoord.
3. **Nummerweergave**: hetzelfde Cheapconnect-nummer.
4. **T.38-fax** (Geavanceerde instellingen): aan, als je op dit nummer faxen ontvangt.
5. Opslaan. De status wordt **Aangemeld**.
6. Maak een [inkomende route](../routes.md) voor het nummer en zet de trunk in een [uitgaande route](../routes.md),
   bijvoorbeeld als tweede trunk zodat hij overneemt als je hoofdprovider uitvalt.
7. Achter NAT? Vul het publieke adres in bij **Instellingen → Netwerk en NAT** en stuur UDP 5060 en de
   RTP-poorten door.

## Als het niet werkt

| Symptoom | Oplossing |
| --- | --- |
| Status **Geweigerd** | Verkeerde gebruikersnaam of wachtwoord; kopieer ze opnieuw uit Cheapconnect |
| Status blijft **Niet aangemeld** | Controleer of uitgaand UDP 5060 is toegestaan |
| Inkomende gesprekken komen op de verkeerde route | Zet **Gebeld nummer komt uit** op **Request-URI** |
| Gesprek komt tot stand maar zonder geluid | NAT: vul het publieke adres in onder Instellingen en stuur de RTP-reeks door |

Bron: [Cheapconnect: SIP Trunk Setup, Asterisk & FreePBX](https://www.cheapconnect.nl/en/business-voip/sip-trunk-setup/).
