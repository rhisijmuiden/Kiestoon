# Hoge beschikbaarheid

Twee Kiestoon-systemen kunnen draaien als **actief/passief-paar**. De primaire node draait de telefooncentrale; de
standby-node kopieert continu de database en bestanden en neemt het over als de primaire uitvalt.

![Hoge beschikbaarheid](shot:cluster)

## Instellen

1. Installeer Kiestoon twee keer, met **dezelfde versie** (twee LXC-containers op verschillende Proxmox-hosts, of
   twee machines).
2. Op het eerste systeem (rond de eerste aanmelding af): **Hoge beschikbaarheid**, kies de failovermodus en klik op
   **Hoge beschikbaarheid aanzetten**. Kiestoon toont een aansluitcode.
3. Open het portaal van het tweede systeem. Meld je aan met de gebruikersnaam en het wachtwoord van het tweede
   systeem, kies **Aansluiten bij een bestaande Kiestoon als standby-node** en plak de code.

Het tweede systeem start opnieuw als standby. Het portaal toont de replicatiestatus en een formulier om te
**promoveren**; al het andere beheer je op de primaire node.

## Failovermodi

**Virtueel IP (zelfde netwerk).** Beide nodes staan in één netwerk (laag 2). keepalived (VRRP, unicast tussen de
nodes) houdt een virtueel IP op de primaire node; telefoons en trunks gebruiken dat adres. Valt de primaire uit, dan
neemt de standby binnen een paar seconden het virtuele IP over en promoveert zichzelf. Beide nodes draaien keepalived
in BACKUP-modus met *nopreempt*, dus een herstelde node neemt het adres niet vanzelf terug.

**Handmatig of via DNS.** De nodes staan in verschillende netwerken of datacenters. Promoveer de standby vanuit zijn
eigen portaal (log in met een beheerdersaccount van de primaire) en laat DNS (bijvoorbeeld SRV-records met een korte
TTL) of de telefoons ernaar wijzen.

## Wat er gerepliceerd wordt

- De database: de primaire maakt binnen enkele seconden na elke wijziging een consistente snapshot (`VACUUM INTO`);
  de standby haalt die op door de tunnel.
- Bestanden: de Asterisk-spool (voicemail, opnames), gesproken teksten, fax, spraakmodellen, firmware en
  TLS-certificaten, elke paar seconden vergeleken op grootte en checksum.
- Gegenereerde configuratie (Asterisk, firewall, provisioning) wordt niet gekopieerd: die wordt opnieuw opgebouwd uit
  de database.

Alle replicatie loopt door een eigen WireGuard-tunnel (`kt-ha`, UDP 51821) met een gedeeld geheim, dus het werkt ook
via internet.

## Omschakelen

**Omschakelen** op de primaire draagt netjes over: die maakt een laatste snapshot, wacht tot de standby hem heeft en
treedt dan terug. De standby promoveert zichzelf; de oude primaire wordt de nieuwe standby. Gebruik dit voor
onderhoud en updates: werk de standby bij, schakel om, werk de oude primaire bij.

## Split brain en dataverlies

- Lopende gesprekken vallen weg als de andere node het overneemt. Registraties komen terug binnen het
  herregistratie-interval van de telefoons.
- Wijzigingen van de laatste seconden voor een crash kunnen verloren gaan.
- Wordt de standby gepromoveerd terwijl de primaire nog draait (netwerksplitsing), dan kunnen beide nodes de centrale
  draaien. Handmatig promoveren weigert dat zolang de primaire nog antwoordt, tenzij *forceren* aangevinkt is.
- Modus virtueel IP: zien de nodes elkaar weer, dan houdt keepalived één master en treedt de andere node terug als
  standby die van de master kopieert. Wijzigingen op de teruggetreden node tijdens de splitsing gaan verloren.
- Handmatig of via DNS: een oude primaire die terugkomt denkt nog dat hij primair is. Houd hem van het netwerk (of
  stop `kiestoon` en `asterisk` erop), installeer hem daarna opnieuw en sluit hem aan als nieuwe standby.
- Een gepromoveerde node bewaart zijn eigen vorige database in `/var/lib/kiestoon/cluster/previous`, om met de hand
  te herstellen.

## Uitzetten

**Uitzetten** op de primaire heft het paar op. De standby hoort dat, vergeet de koppeling en wordt weer een leeg
systeem (installatiewizard).

## Poorten

| Poort | Tussen de nodes | Wat |
| --- | --- | --- |
| UDP 51821 | beide kanten op | WireGuard-tunnel voor replicatie |
| TCP 8443 | standby → primaire, één keer | aansluiten |
| VRRP (protocol 112) | beide kanten op, alleen modus virtueel IP | keepalived |
