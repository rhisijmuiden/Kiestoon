# Telefoons en provisioning

Kiestoon stelt bureautelefoons voor je in: accounts, toetsen, taal, tijdserver, telefoonboek en firmware. Een
telefoon hoeft alleen te weten waar Kiestoon is. Daarna koppel je hem in het portaal aan een toestel.

![Toestellen & provisioning](shot:phones)

## Ondersteunde merken

| Merk | Modellen | Status |
| --- | --- | --- |
| **Yealink** | T3x, T4xU, T5xW, met EXP40-, EXP43- en EXP50-modules | Getest |
| **Snom** | D7xx, D8xx, met D7-module | Getest |
| **Grandstream** | GRP26xx, GXP2170, met GBX20-module | Gebouwd volgens de documentatie van de fabrikant |
| **Fanvil** | X3U, X4U, X5U en meer | Gebouwd volgens de documentatie van de fabrikant |
| **Gigaset** | N670 / N870 IP PRO DECT, met meerdere handsets | Gebouwd volgens de documentatie van de fabrikant |
| **Poly** | VVX, Edge E | Gebouwd volgens de documentatie van de fabrikant |
| **Htek**, **Akuvox**, **Cisco 8800** (3PCC-firmware) | | Gebouwd volgens de documentatie van de fabrikant |

Merken die "volgens de documentatie" gebouwd zijn, zijn nog niet op een echt toestel getest. Laat het weten als iets
niet werkt.

## 1. Provisioning aanzetten

Open **Toestellen & provisioning** en klik **Provisioning-instellingen**:

| Veld | Wat |
| --- | --- |
| **Provisioning aan** | Telefoons mogen hun configuratie ophalen |
| **Serveradres voor toestellen** | Hostnaam of IP-adres waarmee telefoons Kiestoon bereiken |
| **Gebruikersnaam / Wachtwoord** | Beveiligen het provisioning-adres. Worden gemaakt; houd ze geheim |
| **Beheerwachtwoord toestel** | Wordt op elke telefoon gezet, voor de eigen webinterface |
| **Tijdserver** | NTP-server voor de telefoons, bijvoorbeeld `nl.pool.ntp.org` |
| **Taal** | Taal op het display van de telefoon |

De kaart toont daarna de **provisioning-URL's** om te kopiëren:

- `http://gebruiker:wachtwoord@<server>:8080/prov/` voor Yealink, Grandstream, Fanvil, Poly, Htek, Akuvox en Cisco,
- hetzelfde adres met `snom{model}.htm` voor Snom,
- hetzelfde adres met `gigaset/` voor het Gigaset-basisstation.

## 2. Telefoons naar Kiestoon laten wijzen

Kies wat bij je netwerk past:

- **DHCP-optie 66** op je router of DHCP-server, met de URL hierboven. Nieuwe telefoons vinden Kiestoon dan zelf.
- **Met de hand** in de webinterface van de telefoon (*Auto provisioning* / *Setting server*).
- **Zero-touch** via de redirectiedienst van de fabrikant, ook voor telefoons buiten kantoor. Zie hieronder.

## 3. Een telefoon aan een toestel koppelen

Een telefoon die zijn configuratie opvraagt, maar waarvan het MAC-adres onbekend is, verschijnt onder **Wachten op
een extensie**, met merk en model herkend uit wat hij meestuurde. Kies een toestel en klik **Koppelen**.

Kiestoon schrijft de configuratie en stuurt de SIP NOTIFY van de fabrikant (check-sync; resync voor Cisco), zodat
een geregistreerde telefoon binnen enkele seconden zijn nieuwe configuratie ophaalt. Een telefoon die nog niet
geregistreerd is, haalt hem op bij de volgende provisioningcontrole of direct na een herstart.

Je kunt een telefoon ook vooraf toevoegen met **Toestel toevoegen** (MAC-adres van het label achterop, model,
toestel en label), een telefoon naar een ander toestel verplaatsen of ontkoppelen. **Configuratie nu ophalen** laat
een telefoon opnieuw laden. **Configuratie bekijken** toont precies het bestand dat de telefoon krijgt.

## DECT met meerdere handsets (Gigaset)

Een Gigaset N670- of N870-basisstation heeft meerdere handsets. Handset 1 gebruikt het toestel van de telefoon; geef
elke volgende handset een eigen toestel onder **Handsets**.

## Firmware

Upload op het tabblad **Firmware** de firmware één keer (of download hem via een link naar de site van de
fabrikant) en kies voor welke modellen hij is. Een `.zip` wordt uitgepakt. Klik **Uitrollen naar toestellen**:
telefoons van die modellen installeren hem bij hun volgende controle, meestal 's nachts of na een herstart. Ze halen
hem bij Kiestoon op (`/prov/fw/<willekeurige code>/`), dus ze hebben geen internet nodig.

!!! warning
    Kies alleen modellen waarvoor een bestand bedoeld is. Een verkeerd bestand kan een telefoon onbruikbaar maken.

## Zero-touch provisioning

Op het tabblad **Zero-touch** toont Kiestoon het adres om bij de redirectiedienst van de fabrikant te registreren.
Een nieuwe telefoon vraagt die dienst dan waar zijn instellingen staan en stelt zichzelf in, ook thuis.

- **Yealink RPS** gaat automatisch: vul een AccessKey ID en secret in uit het Yealink Device Management-portaal
  (*API*). Telefoons worden aangemeld en afgemeld als je ze toevoegt of verwijdert.
- **Snom SRAPS, Grandstream GDMS, Fanvil FDPS, Htek, Poly, Gigaset en Cisco** hebben geen openbare API. Log in op
  hun portaal, vul het getoonde serveradres in en importeer de MAC-adressen met de export **MAC-lijst (CSV)**.

Telefoons buiten je netwerk hebben HTTPS nodig: zet [internettoegang](internet.md) aan of stel een publiek adres in.

## Toetsen en uitbreidingsmodules

Elke telefoon heeft een visuele toetseneditor met extra lijnen, BLF, snelkeuzes, parkeren, intercom en
uitbreidingsmodules. Dat heeft een eigen hoofdstuk: [Toetsen en uitbreidingsmodules](phone-keys.md).
