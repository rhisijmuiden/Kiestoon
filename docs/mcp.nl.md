# MCP-server

Met de MCP-server kunnen AI-assistenten zoals Claude met je telefooncentrale werken. Ze zoeken dingen op, maken
rapportages en kunnen, als je dat toestaat, instellingen aanpassen of gesprekken starten. MCP (Model Context
Protocol) is de open standaard waarmee Claude, Claude Code, Cursor, n8n en veel andere clients met tools praten.

Open **MCP-server** in het menu Systeem. De server staat uit totdat je hem bovenaan met de schakelaar aanzet.

![MCP-server: status, beleid en verbindingen](shot:mcp)

## Wat een assistent kan

Je vraagt dingen in gewone taal, bijvoorbeeld:

- "Hoeveel gesprekken hebben we gisteren gemist en waarom?"
- "Maak een toestel aan voor Lisa, nummer 214, en koppel de Yealink T54W"
- "Welke trunk had deze week de slechtste gesprekskwaliteit?"
- "Zet de nachtstand aan tot morgen 8:00"
- "Bel 020 123 4567 vanaf mijn toestel"

De assistent kiest uit **tools**. Elke tool hoort bij een **recht**, en een verbinding ziet alleen de tools van
de rechten die ze heeft gekregen:

| Recht | Wat het geeft | Laagste rol |
| --- | --- | --- |
| **Centrale bekijken** | De configuratie lezen: toestellen, wachtrijen, keuzemenu's, routes, trunks, tijdcondities, status | Beheerder |
| **Gesprekken en statistieken** | Gespreksgeschiedenis, gespreksverloop, wachtrij- en wallboardcijfers, gesprekskwaliteit, terugbelverzoeken | Gebruiker (eigen gesprekken) |
| **Telefoons en provisioning** | Bureautelefoons, gevonden telefoons, firmware | Beheerder |
| **Voicemail en opnames** | Voicemail, opnames en hun AI-samenvattingen (alleen met **Voicemail en opnames delen** aan) | Gebruiker (eigen) |
| **Gesprekken starten en sturen** | Klik-en-bel, doorschakelen, in- en uitloggen bij wachtrijen, dag-/nachtstand, terugbelverzoeken | Gebruiker (eigen toestel) |
| **Configuratie wijzigen** | Configuratie aanmaken, wijzigen en verwijderen, bureautelefoons koppelen | Beheerder |
| **Systeem en onderhoud** | Back-ups (bekijken en starten), op updates controleren, een telefoon opnieuw laten ophalen, het auditlog lezen | Beheerder |

Naast tools geeft de server een korte uitleg over hoe Kiestoon is opgebouwd, en kant-en-klare **prompts** zoals
*Daily call report*, *Why were calls missed?*, *Set up a new employee*, *Health check of the phone system* en
*Design an IVR menu*. Claude toont ze in het promptmenu en antwoordt in jouw taal.

Sommige dingen kunnen nooit via MCP, welke rechten je ook geeft: gebruikers en tokens beheren, een back-up
terugzetten, updates installeren en wachtwoorden of API-sleutels uitlezen. Geheimen in de configuratie zie je als
`[redacted]` en blijven ongewijzigd als de assistent een object opslaat. Trunks zijn alleen-lezen.

![Tools & rechten: losse tools uitzetten](shot:mcp-tools)

Onder **Tools & rechten** zie je elke tool met zijn recht en soort (lezen, actie of wijziging). Zet je een tool uit,
dan kan geen enkele verbinding hem gebruiken.

## Claude koppelen (claude.ai en Claude Desktop)

claude.ai en Claude Desktop koppelen via OAuth: je logt zelf in bij Kiestoon en kiest wat Claude mag. Je hoeft geen
token te kopiëren.

1. Open in Claude **Instellingen → Connectors → Aangepaste connector toevoegen**.
2. Plak het adres van het tabblad **Koppelen**, bijvoorbeeld `https://pbx.example.com/mcp`.
3. Claude opent Kiestoon. Log in (met MFA of een passkey) en kies de rechten en hoe lang de verbinding geldig is.
4. Klaar. De verbinding verschijnt onder **Verbindingen**.

![Het toestemmingsscherm: jij kiest wat Claude mag](shot:mcp-consent)

!!! warning "Kiestoon moet vanaf internet bereikbaar zijn"
    claude.ai maakt verbinding vanuit de cloud van Anthropic, ook als je Claude Desktop gebruikt. Kiestoon moet via
    HTTPS met een eigen domein bereikbaar zijn (zie [Internettoegang en SSL](internet.md)). Met
    **OAuth alleen vanaf Anthropic-adressen** aan accepteert het MCP-adres alleen OAuth-verbindingen vanaf de
    gepubliceerde adressen van Anthropic (`160.79.104.0/21` en `2607:6bc0::/48`); je eigen netwerk blijft altijd werken.

De verbinding gebruikt kortlevende toegangstokens (één uur) die Claude zelf vernieuwt. Trek je hem in onder
**Verbindingen**, dan verliest Claude meteen de toegang.

## Claude Code en andere clients

Claude Code, Cursor, n8n en je eigen agents gebruiken een token. Maak er een aan onder **Verbindingen → Nieuwe
verbinding**: kies een naam, de rechten, hoe lang hij geldig is en eventueel de adressen waarvandaan hij gebruikt
mag worden. Het token zie je **één keer**, samen met het commando voor Claude Code:

```sh
claude mcp add --transport http kiestoon https://pbx.example.com/mcp \
  --header "Authorization: Bearer kt_mcp_…"
```

Voor andere clients staat op het tabblad **Koppelen** een JSON-configuratie om te kopiëren. Een token werkt ook op
je lokale netwerk, zonder internetmodus.

![Een nieuwe verbinding met haar rechten](shot:mcp-connection)

## Je eigen toestel koppelen

Staat **Gebruikers mogen hun eigen toestel koppelen** aan, dan vinden medewerkers zonder beheerrechten
**AI-assistenten** in hun werkplekmenu. Ze kunnen Claude aan hun eigen toestel koppelen: hun eigen gesprekken en
voicemail bekijken, bellen vanaf hun toestel, doorschakelen en in- of uitloggen bij hun wachtrijen. Een verbinding
krijgt nooit meer rechten dan degene die haar maakte, en beheerders zien deze verbindingen ook.

## Veiligheid

| Instelling | Wat het doet |
| --- | --- |
| **Nieuwe verbindingen standaard alleen lezen** | Wijzigen en acties moeten per verbinding aan |
| **Wijzigingen eerst als voorstel** | Een wijziging of actie komt eerst terug als voorstel met een samenvatting van wat er verandert. Pas als de assistent de tool, na jouw ja, opnieuw aanroept met de bevestigingscode (10 minuten geldig) wordt hij uitgevoerd |
| **Telefoonnummers maskeren** | Verbergt de middelste cijfers van nummers in gespreksgeschiedenis en voicemail, bijvoorbeeld `+3161•••••78` |
| **Voicemail en opnames delen** | Uit: de assistent kan geen voicemail, opnames of transcripties lezen |
| **OAuth alleen vanaf Anthropic-adressen** | Zie hierboven |
| **Gebruikers mogen hun eigen toestel koppelen** | Laat gebruikers zonder beheerrechten verbindingen maken voor hun eigen toestel |

Elke verbinding heeft daarnaast een optionele lijst met toegestane adressen en een limiet van 120 toolaanroepen per
minuut. Wijzigingen gaan door dezelfde controles als in het portaal en komen in het auditlog met de naam van de
verbinding.

## Activiteit

Het tabblad **Activiteit** toont elke toolaanroep van de afgelopen 90 dagen: wanneer, welke verbinding en
gebruiker, welke tool, een korte samenvatting en of hij gelukt is, een voorstel was of geweigerd werd. Filter op
lezen, acties of wijzigingen, zoek, en exporteer naar CSV.

![Activiteit van alle verbindingen](shot:mcp-activity)
