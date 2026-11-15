# Firewall en fraudebescherming

Een telefooncentrale op internet wordt binnen enkele minuten aangevallen: bots proberen dag en nacht
SIP-wachtwoorden, en een gehackt toestel kan dure nummers in het buitenland bellen. Kiestoon beschermt je in lagen:
een firewall, automatisch blokkeren, geo-blokkering en fraudebescherming op uitgaande gesprekken.

## Firewall

![Firewall](shot:firewall)

Kiestoon beheert een **nftables**-firewall op de server.

| Instelling | Wat het doet |
| --- | --- |
| **Firewall aan** | Blokkeert alles behalve het portaal, SIP en de audiopoorten. |
| **SIP open voor iedereen** | Voor toestellen die overal vandaan verbinden. Niet aangeraden: gebruik de whitelist of [Edge](edge.md). |
| **SIP vanaf lokale netwerken toestaan** | Toestellen in het kantoornetwerk kunnen zich altijd aanmelden. |
| **SIP-whitelist** | IP-adressen of netwerken die SIP mogen gebruiken, één per regel. De adressen van je [trunks](trunks.md) worden automatisch toegevoegd. |
| **Toegang tot het portaal** | Netwerken die het portaal mogen openen. Leeg: iedereen. |

**Gegenereerde regels tonen** laat de precieze nftables-regels zien.

!!! warning "Veilige modus"
    Elke wijziging wordt in **veilige modus** toegepast: Kiestoon controleert of je het portaal nog kunt bereiken,
    en draait de wijziging binnen 90 seconden automatisch terug als dat niet zo is. Kiestoon waarschuwt ook voordat
    je een portaaltoegangslijst opslaat zonder je eigen IP-adres.

## fail2ban

Met **fail2ban aan** wordt een adres dat te vaak mislukt inlogt op SIP of het portaal geblokkeerd.

- **Pogingen**, **Binnen (minuten)** en **Blokkeren voor (minuten)** bepalen hoe streng het is.
- **Nooit blokkeren**: vertrouwde adressen, zoals het IP-adres van kantoor.
- **Geblokkeerde adressen** toont de huidige blokkades; **Deblokkeren** heft er een op.

## Geo-blokkering

Laat telefonie (SIP), en als je wilt het portaal, alleen binnen vanuit bepaalde landen.

- **Alleen deze landen toestaan**, bijvoorbeeld met **+ Nederland & België** of **+ Europa**, of
  **Deze landen blokkeren**.
- **Ook voor het webportaal**: poort 80 blijft open voor Let's Encrypt.
- Toestellen in je eigen netwerk, de trunkproviders en de whitelists komen er altijd in.
- Collega's die naar het buitenland reizen kunnen de centrale niet bereiken vanuit landen buiten de lijst.

De landendata is de gratis database "IP to Country Lite" van DB-IP (CC BY 4.0). Kiestoon downloadt die één keer
per maand; **Landendata bijwerken** haalt hem direct op.

## Fraudebescherming

![Belfuncties & fraude](shot:features)

Onder **Belfuncties & fraude › Bescherming tegen belfraude** beperk je wat er gebeld kan worden, en slaat Kiestoon
alarm bij ongewoon verkeer, zodat een gestolen wachtwoord je geen fortuin kost.

| Instelling | Wat het doet |
| --- | --- |
| **Betaalnummer-voorvoegsels** | Geblokkeerd voor toestellen zonder **Betaalnummers toestaan**, zoals 0900 en 0906. |
| **Geblokkeerde landcodes** | Nooit te bellen, bijvoorbeeld 7, 234 of 882. |
| **Maximum gelijktijdige uitgaande gesprekken** | Een harde grens voor het hele systeem. 0 is onbeperkt. |
| **Alarm boven uitgaande gesprekken** / **Alarm boven internationale gesprekken** | Per uur. 0 zet het alarm uit. |
| **E-mail voor alarm** | Wie het alarm krijgt. Leeg: het meldingsadres onder [Mailserver](mail.md). |
| **Alle uitgaande gesprekken blokkeren bij alarm** | Stopt alle uitgaande gesprekken tot een beheerder op **Deblokkeren** klikt. Alarmnummers blijven werken. |

Per toestel bepaalt **Uitgaand bellen** wat het mag bellen: **Alleen intern**, **Nationaal** of **Internationaal**.

!!! tip "Alarmnummers werken altijd"
    112 en 911 werken vanaf elk toestel, zonder route, tijdens een fraudeblokkade en zelfs als een trunklimiet
    bereikt is. Zie [Instellingen](settings.md#alarmnummers).

## Op dezelfde pagina: parkeren en opnemen

- **Gesprek parkeren**: kies `70` om een gesprek te parkeren; het komt op plek 71 en verder. Een geparkeerd gesprek
  dat niemand oppakt, belt terug naar wie het parkeerde.
- **Opnemen**: hoe lang [opnames](history.md#opnames) bewaard worden (let op de AVG), en een piep elke 15 seconden
  zodat bellers weten dat het gesprek wordt opgenomen.
