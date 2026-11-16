# Installeren

Kiestoon draait op **Debian 13 "Trixie"** (amd64 of arm64). Kies een van twee manieren: het kant-en-klare
Proxmox LXC-template, of het installatiescript op een Debian 13-server of virtuele machine. Beide eindigen op
hetzelfde: een draaiend portaal en een eenmalige **setup-token** voor de eerste beheerder.

## Wat heb je nodig

| | Minimaal | Comfortabel voor 50+ gebruikers |
| --- | --- | --- |
| CPU | 1 core | 2 cores |
| Geheugen | 1 GB | 2 GB (4 GB met lokale spraak-naar-tekst) |
| Schijf | 8 GB | 20 GB of meer voor opnames en voicemail |
| Netwerk | Een vast IP-adres in het kantoornetwerk | Plus een publieke hostnaam voor externe toestellen en de softphone |

## Optie 1: Proxmox LXC-template

1. Download `kiestoon-<versie>-debian-13-amd64.tar.zst` bij de
   [releases](https://github.com/rhisijmuiden/Kiestoon/releases) op GitHub.
2. Open in Proxmox *je opslag › CT Templates › Upload* en upload het bestand.
3. Maak een container van het template. Unprivileged is prima. Begin met 1 CPU, 1 GB RAM en 8 GB schijf, en geef
   hem een vast IP-adres (of een DHCP-reservering).
4. Start de container en log op de console in als `root`. De console toont het adres van het portaal en de
   setup-token.

!!! note "Testkanaal"
    Release candidates (`vX.Y.Z-rc.N`) hebben ook een template. Een container van een rc-template volgt het
    **testkanaal**; een stabiel template volgt **productie**. Je kunt later wisselen, zie [Updates](updates.md).

## Optie 2: Debian 13-server of VM

Start het installatiescript als root op een schone Debian 13-machine:

```sh
curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/install.sh | sudo bash
```

Om het testkanaal (release candidates) te volgen:

```sh
curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/install.sh | sudo bash -s -- --channel testing
```

!!! note ""
    Tot de eerste stabiele release heeft alleen het testkanaal releases, dus gebruik voorlopig `--channel testing`.

Opties van het script:

| Optie | Wat het doet |
| --- | --- |
| `--channel production|testing` | Updatekanaal dat gevolgd wordt (standaard `production`) |
| `--version vX.Y.Z` | Precies deze versie installeren in plaats van de nieuwste |
| `--deb`, `--asterisk-deb`, `--sounds-deb`, `--speech-deb` | Lokale pakketten installeren (voor ontwikkeling) |

Het script installeert de pakketten `kiestoon` (portaal en API), `kiestoon-asterisk` (Asterisk 22 LTS, gebouwd
voor Kiestoon), `kiestoon-sounds-nl` (Nederlandse gesproken teksten) en de diensten `kiestoon-helper` (een kleine
root-helper voor firewall, back-ups en tunnels) en `kiestoon-updater`. Aan het eind toont het het adres van het
portaal en de setup-token.

## Het portaal openen

| Adres | Wanneer |
| --- | --- |
| `http://<server>:8080` | Altijd, in je eigen netwerk |
| `https://<server>:8443` | Hetzelfde portaal via HTTPS met een zelfondertekend certificaat. Nodig voor de browser-softphone (microfoon) |
| `https://<jouw domein>` | Nadat je [internettoegang](internet.md) aanzet (Let's Encrypt op poort 443) |

Setup-token kwijt? Toon hem opnieuw op de server:

```sh
sudo kiestoon setup-token
```

## Poorten

Zet deze open in een firewall vóór Kiestoon. De [firewallpagina](firewall.md) in het portaal opent precies deze op
de server zelf.

| Poort | Wat |
| --- | --- |
| 8080/tcp, 8443/tcp | Portaal (HTTP, HTTPS) en provisioning van telefoons (`/prov/`) |
| 80/tcp, 443/tcp | Alleen met internettoegang aan (Caddy, Let's Encrypt) |
| 5060/udp+tcp, 5061/tcp | SIP, SIP over TLS |
| 10000-20000/udp | Audio (RTP) |
| 51820/udp | WireGuard voor vestigingen en gekoppelde systemen (alleen als het aan staat) |
| 51821/udp | WireGuard tussen de twee nodes van een HA-paar (alleen als het aan staat) |
| 389/tcp, 636/tcp | LDAP-telefoonboek, alleen eigen netwerk (alleen als het aan staat) |

## Waar staat wat

| Pad | Wat |
| --- | --- |
| `/etc/kiestoon/kiestoon.env` | Instellingen: poort, datamap, standaardkanaal |
| `/var/lib/kiestoon/` | Alle gegevens: database, gegenereerde Asterisk-config, voicemail, opnames, faxen, mailwachtrij |
| `/etc/asterisk/` | Vaste Asterisk-config die de gegenereerde bestanden inleest. Niet met de hand aanpassen |
| `/var/lib/kiestoon-backups/` | Automatische back-ups vóór elke update (de nieuwste 5 blijven bewaard) |

!!! info "Eén regel achter alles"
    Alle gegevens staan in `/var/lib/kiestoon`, en elke gegenereerde configuratie (Asterisk, firewall, provisioning)
    kan opnieuw uit de database worden opgebouwd. Daardoor blijven back-ups, herstel en hoge beschikbaarheid simpel.

Volgende stap: [eerste inrichting](first-setup.md).
