# Provider: Cheapconnect

[Cheapconnect](https://www.cheapconnect.net) is een Nederlandse VoIP-provider met SIP-accounts die werken met je
eigen centrale. Er is nog geen eigen sjabloon; het sjabloon **Generic (registration)** doet het werk.

!!! note "Controleer de gegevens in je Cheapconnect-account"
    De server en de opbouw van het account hieronder zijn wat Cheapconnect publiceert voor SIP-accounts. Gebruik
    altijd de exacte waarden uit je eigen Cheapconnect-dashboard: die gaan voor deze pagina.

## 1. Bij Cheapconnect

1. Log in op je Cheapconnect-account en open het SIP-account (het telefoonnummer) dat je wilt gebruiken.
2. Noteer de **SIP-gebruikersnaam** (meestal het account- of telefoonnummer), het **wachtwoord** en de
   **SIP-server**.
3. Optioneel, veiliger: beperk het account tot het publieke IP-adres van je Kiestoon-server en stel een
   bestedingslimiet in.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Trunk toevoegen**, provider **Generic (registration)**.
2. **Naam**: `Cheapconnect`.
3. **Host**: de SIP-server uit je account (`sip.cheapconnect.net`), **poort** `5060`, **transport** UDP.
4. **Gebruikersnaam** en **wachtwoord**: van het SIP-account.
5. **Gebeld nummer komt uit**: *To-header* (de standaard van het sjabloon Generic). Kies *Request-URI* als
   inkomende gesprekken op de verkeerde route terechtkomen.
6. **Codecs**: `alaw,ulaw`.
7. **Nummerweergave**: het Cheapconnect-nummer van dit account.
8. **T.38-fax**: aan, als je op dit nummer faxen ontvangt.
9. Opslaan. De status wordt **Aangemeld**.

Maak daarna een [inkomende route](../routes.md) voor het nummer en zet de trunk in je
[uitgaande route](../routes.md), bijvoorbeeld als tweede trunk zodat hij overneemt als je hoofdprovider uitvalt.

## Als het niet werkt

| Symptoom | Oplossing |
| --- | --- |
| Status **Geweigerd** | Verkeerde gebruikersnaam of wachtwoord; kopieer ze opnieuw uit Cheapconnect |
| Status blijft **Niet aangemeld** | Controleer de servernaam en of uitgaand UDP 5060 is toegestaan |
| Gesprek komt tot stand maar zonder geluid | NAT: vul het publieke adres in onder Instellingen en stuur de RTP-reeks door |
