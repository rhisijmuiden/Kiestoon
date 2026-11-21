# Vestigingen (Edge)

Kiestoon Edge is een kleine satelliet voor een vestiging. Hij verbindt de vestiging met de centrale Kiestoon
(bijvoorbeeld in een datacenter) via een WireGuard-tunnel en draait een SIP-proxy (Kamailio) en media-relay
(rtpengine) voor de telefoons op locatie.

```
Telefoons vestiging ──LAN── Kiestoon Edge ══ WireGuard (UDP 51820) ══ Kiestoon (datacenter)
  SIP 5060, RTP              Kamailio, rtpengine,                       Asterisk, portaal,
  provisioning :80           provisioning-proxy                         alle instellingen
```

Alleen de centrale Kiestoon heeft een bereikbare UDP-poort nodig. De edge verbindt naar buiten, dus de vestiging
heeft **geen port forwarding** nodig en NAT-problemen tussen vestiging en datacenter zijn verdwenen: alleen de tunnel
gaat door de NAT. Toestellen, voicemail en alle instellingen blijven op de centrale Kiestoon.

![Vestigingen](shot:sites)

## Installeren

Kies er één:

| Waar | Wat je uit de release downloadt |
| --- | --- |
| Raspberry Pi 3, 4, 5 of Zero 2 W | `kiestoon-edge-<versie>-raspberry-pi-arm64.img.xz` (Raspberry Pi OS Lite 64-bit met Kiestoon Edge) |
| Proxmox | `kiestoon-edge-<versie>-debian-13-amd64.tar.zst` (LXC-template) |
| Elke Debian 13-machine | `kiestoon-edge_<versie>_amd64.deb` of `_arm64.deb`: `apt install ./kiestoon-edge_*.deb` |

**Raspberry Pi:** schrijf het image met Raspberry Pi Imager (*Use custom*). De instellingen van de Imager
(gebruiker, SSH, wifi, hostnaam) werken zoals altijd. Een bekabelde verbinding is aan te raden voor telefoons.

**Proxmox in één commando:** voer `bash -c "$(curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/proxmox/kiestoon-lxc.sh)" -- --edge` uit in de shell van de Proxmox-host.
Het downloadt het Edge-template en maakt de container (512 MB RAM, 4 GB schijf). Zie [Installatie](install.md).

**LXC:** de WireGuard-module moet beschikbaar zijn in de kernel van de host (Proxmox heeft hem). Unprivileged
containers werken. Zonder kernelmodule laat `apt install wireguard-go` `wg-quick` de (tragere) userspace-versie
gebruiken.

## Koppelen

1. Open op de centrale Kiestoon **Vestigingen**, zet **Vestigingen en gekoppelde systemen toestaan** aan en vul
   het publieke adres van de Kiestoon in. Stuur UDP-poort 51820 door als hij achter een router staat.
2. Klik op **Vestiging toevoegen**. Kiestoon toont een koppelcode (24 uur geldig, werkt één keer).
3. Open de pagina van de edge op de vestiging: `http://<adres van de edge>/` en plak de code.
    - Zonder scherm: zet de code in een bestand `kiestoon-edge.txt` op de opstartpartitie van de SD-kaart (de schijf
      `bootfs` op je computer). De edge leest hem bij het opstarten, koppelt en verwijdert het bestand.
    - Op de opdrachtregel: `kiestoon-edge pair <code>`.

De code bevat het adres van het portaal en de SHA-256-vingerafdruk van het certificaat. De edge praat alleen met een
portaal met precies dat certificaat, dus een zelfondertekend certificaat is veilig. De edge maakt zijn eigen
WireGuard-sleutel; de privésleutel verlaat de edge nooit.

## Telefoons

Laat de telefoons op de vestiging naar de edge wijzen:

- SIP-server / registrar: `<adres van de edge>`, poort 5060 (UDP of TCP)
- Provisioning-URL: `http://<adres van de edge>/prov/` (bijvoorbeeld in DHCP-optie 66)

De edge stuurt provisioning door naar de centrale Kiestoon, die de edge als SIP-server uitdeelt. Telefoons
registreren via de edge (de edge voegt een `Path`-header toe); de centrale Asterisk stuurt gesprekken via dezelfde
edge terug. Media gaat telefoon → edge → tunnel → Asterisk; rtpengine verankert die aan beide kanten.

## Status en onderhoud

- De pagina van de edge toont de tunnel, het laatste contact en de diensten. Het centrale portaal toont elke edge
  met het netwerk van de vestiging, de versie en de diensten.
- De edge stuurt elke 30 seconden een hartslag door de tunnel. Een nieuw adres op de vestiging (DHCP) pikt hij zelf
  op.
- Werk de edge bij zoals elk Debian-pakket (`apt install ./kiestoon-edge_<nieuw>.deb`).
- Een edge naar een andere Kiestoon verhuizen: `kiestoon-edge reset` op de edge, daarna opnieuw koppelen. Een nieuwe
  koppelcode maken in het portaal verbreekt de oude edge.
- `kiestoon-edge status` geeft de status als JSON. Logs: `journalctl -u kiestoon-edge -u kamailio -u rtpengine-daemon`.

## Beveiliging

- Koppelen vanaf de lokale pagina kan alleen zolang de edge niet gekoppeld is, dus niemand op de vestiging kan een
  werkende edge naar een ander systeem verhuizen. Resetten vraagt een shell op de edge.
- De edge stuurt verzoeken van de vestiging alleen door naar de centrale Kiestoon, met floodbescherming (Kamailio
  pike). Alle telefoons van een vestiging delen het tunneladres van de edge, dus fail2ban op het centrale systeem
  blokkeert de hele vestiging als één telefoon steeds verkeerd inlogt.
- De edge bewaart geen wachtwoorden van toestellen; de authenticatie gebeurt op de centrale Asterisk.

## Poorten

| Poort | Waar | Wat |
| --- | --- | --- |
| UDP 51820 | centrale Kiestoon, vanaf internet | WireGuard (aan te passen) |
| UDP/TCP 5060 | edge, netwerk vestiging | SIP voor de telefoons |
| UDP 30000–39999 | edge, netwerk vestiging | RTP (media) |
| TCP 80 | edge, netwerk vestiging | statuspagina en provisioning |
