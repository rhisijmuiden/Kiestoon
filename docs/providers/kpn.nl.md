# Provider: KPN

KPN levert zakelijke SIP-trunks (KPN EEN Vast / VoIP Connect) als **IP-trunk**: er is geen registratie,
KPN herkent je centrale aan het vaste publieke IP-adres. Kiestoon heeft een kant-en-klaar sjabloon **KPN SIP trunk**.

## Wat het sjabloon instelt

| Instelling | Waarde | Waarom |
|---|---|---|
| Authenticatie | IP-adres | KPN accepteert alleen SIP van de centrale-adressen uit je order |
| Host / poort | `213.162.171.164`, `5060` | De standaard SBC van KPN |
| IP-adressen provider | `213.162.171.164`, `213.162.171.180` | De tweede SBC wordt gebruikt bij Verhoogde Beschikbaarheid, met load sharing |
| Transport | TCP | KPN ondersteunt UDP en TCP en heeft TCP liever |
| Codecs | `alaw` | G.711 A-law (20 ms) is verplicht naar het openbare net; G.722/G.729 alleen on-net |
| DTMF | RFC 2833 / 4733 | SIP INFO en KPML worden niet ondersteund |
| Gebeld nummer komt uit | Request-URI | KPN stuurt nummers standaard in internationale notatie (E.164) |

!!! warning "Je KPN-order gaat voor"
    KPN geeft de SBC-adressen en het SIP-domein per order uit en kan afwijken van de standaardadressen. De waarden
    hier komen uit de configuratierichtlijnen van KPN (2017). Controleer ze altijd met je order.

## 1. Bij KPN

1. Geef KPN bij de bestelling het **vaste publieke IP-adres** van je Kiestoon-server (KTOF-formulier).
2. Noteer de SBC-adressen en het SIP-domein uit je order.
3. Houd het standaardprofiel met nummers in internationale notatie (`+31...`).

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Trunk toevoegen**, provider **KPN SIP trunk**. Pas host en IP-adressen aan als je
   order andere SBC's noemt.
2. **Nummerweergave**: een nummer uit je KPN-blok. Andere nummers staat KPN niet toe.
3. **T.38-fax** (Geavanceerde instellingen): aan als je faxen ontvangt. KPN biedt eerst T.38 aan en valt terug op G.711.
4. Opslaan, en maak [inkomende routes](../routes.md) voor je nummers en een [uitgaande route](../routes.md).
5. Noodnummer: KPN herkent `112`.

## Als het niet werkt

| Symptoom | Oplossing |
| --- | --- |
| Helemaal geen gesprekken | Controleer of KPN het juiste publieke IP van deze server heeft en of TCP 5060 in beide richtingen open is |
| Gesprekken naar KPN mobiel mislukken | Zorg dat alleen `alaw` wordt aangeboden |
| Verkeerd nummer zichtbaar | De nummerweergave moet een nummer van je KPN-trunk zijn; KPN verwijdert P-Asserted-Identity |
| Geluid valt weg bij grote pakketten | KPN staat geen IP-fragmentatie toe; controleer de MTU op je router |

Bronnen: [KPN EEN dienst Vast (VoIP Connect) configuratierichtlijnen](https://wiki.innovaphone.com/images/4/4b/KPN_EEN_dienst_Vast_%28VoIP_Connect%29_Configuration_Guidelines_innovaphone_PBX_12r2.pdf),
[innovaphone: KPN VoIP Connect SIP-provider](https://wiki.innovaphone.com/?i=9983).
