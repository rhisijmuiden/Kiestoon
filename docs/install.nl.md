# Installeren

Kiestoon draait op **Debian 13 "Trixie"** (amd64 of arm64). Kies een van twee manieren: het kant-en-klare
Proxmox LXC-template, of het installatiescript op een Debian 13-server of virtuele machine. Beide eindigen op
hetzelfde: een draaiend portaal, en het adres, de gebruikersnaam en het wachtwoord om mee in te loggen. De rest richt je in het portaal in.

## Wat heb je nodig

| | Minimaal | Comfortabel voor 50+ gebruikers |
| --- | --- | --- |
| CPU | 1 core | 2 cores |
| Geheugen | 1 GB | 2 GB (4 GB met lokale spraak-naar-tekst) |
| Schijf | 8 GB | 20 GB of meer voor opnames en voicemail |
| Netwerk | Een vast IP-adres in het kantoornetwerk | Plus een publieke hostnaam voor externe toestellen en de softphone |

## Optie 1: Proxmox LXC-template

### Downloaden via URL in Proxmox

De eigen templatelijst van Proxmox (*CT Templates › Templates*) toont alleen de officiële Proxmox- en
TurnKey-appliances, en je kunt daar geen andere bronnen aan toevoegen. Proxmox kan wel elk template van een URL
downloaden:

1. Open in Proxmox *je opslag (bijvoorbeeld `local`) › CT Templates › **Download from URL***.
2. Plak deze URL, klik op **Query URL** en daarna op **Download**:

    ```text
    https://github.com/rhisijmuiden/Kiestoon/releases/latest/download/kiestoon-debian-13-amd64.tar.zst
    ```

    !!! note "Tot de eerste stabiele release"
        `latest` wijst alleen naar stabiele releases. Zolang die er nog niet is, gebruik je de URL van de nieuwste
        release candidate, bijvoorbeeld `https://github.com/rhisijmuiden/Kiestoon/releases/download/v0.7.0-rc.28/kiestoon-debian-13-amd64.tar.zst` (kopieer op de pagina
        [releases](https://github.com/rhisijmuiden/Kiestoon/releases) de link van `kiestoon-debian-13-amd64.tar.zst`).
        Kiestoon Edge: `kiestoon-edge-debian-13-amd64.tar.zst`.

3. Klik op **Create CT**. Kies het template, laat *Unprivileged container* en *Nesting* aan, en begin met 1 CPU,
   1 GB RAM en 8 GB schijf. Geef hem een vast IP-adres (of een DHCP-reservering).
4. Start de container en open de **Console**. Boven de loginprompt staat alleen wat je nodig hebt:

    ```text
      Kiestoon portal: http://192.168.1.20:8080
      Username:        admin
      Password:        k7Rm-Xq4T-9fwP-Lh2c
    ```

5. Open de URL, meld je aan en [ga verder in het portaal](first-setup.md).

### Met één commando op de Proxmox-host

Open een shell op de Proxmox-host (*node › Shell*) en voer uit:

```sh
bash -c "$(curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/proxmox/kiestoon-lxc.sh)"
```

Het script zoekt de nieuwste release, downloadt het LXC-template naar de template-opslag (standaard `local`) en
controleert de SHA-256-checksum. Daarna toont het een overzicht, maakt een unprivileged container (1 CPU, 1 GB RAM,
8 GB schijf, DHCP op `vmbr0`), start hem en toont de URL, gebruikersnaam en het wachtwoord.

Handige opties:

| Optie | Wat het doet |
|---|---|
| `--download-only` | Alleen het template in *CT Templates* zetten; de container maak je zelf |
| `--edge` | Het [Kiestoon Edge](edge.md)-template gebruiken |
| `--ip 192.168.1.20/24 --gateway 192.168.1.1` | Vast IP-adres in plaats van DHCP |
| `--id 120 --hostname pbx` | Container-ID en hostnaam |
| `--storage local-zfs --disk 16` | Opslag en grootte van de rootschijf |
| `--cores 2 --memory 2048` | CPU en geheugen |
| `--bridge vmbr1 --vlan 20` | Netwerkbridge en VLAN-tag |
| `--channel testing` / `--version v0.7.0-rc.26` | Releasekanaal of een vaste versie |
| `--ssh-key ~/.ssh/id_ed25519.pub` | Een SSH-sleutel op de container zetten |
| `--yes` | Niet om bevestiging vragen |

Geef opties mee na `--`, bijvoorbeeld:

```sh
bash -c "$(curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/proxmox/kiestoon-lxc.sh)" -- --ip 192.168.1.20/24 --gateway 192.168.1.1
```

### Met de hand

1. Download `kiestoon-<versie>-debian-13-amd64.tar.zst` bij de
   [releases](https://github.com/rhisijmuiden/Kiestoon/releases) op GitHub.
2. Open in Proxmox *je opslag › CT Templates › Upload* en upload het bestand.
3. Maak een container van het template. Unprivileged is prima. Begin met 1 CPU, 1 GB RAM en 8 GB schijf, en geef
   hem een vast IP-adres (of een DHCP-reservering).
4. Start de container. De console toont de URL, gebruikersnaam en het wachtwoord.

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
| `--dry-run` | Alleen tonen welke release geïnstalleerd zou worden, niets wijzigen |

Het script installeert de pakketten `kiestoon` (portaal en API), `kiestoon-asterisk` (Asterisk 22 LTS, gebouwd
voor Kiestoon), `kiestoon-sounds-nl` (Nederlandse gesproken teksten) en de diensten `kiestoon-helper` (een kleine
root-helper voor firewall, back-ups en tunnels) en `kiestoon-updater`. Het werkt hetzelfde op hardware, in een
virtuele machine of in een container. Aan het eind toont het alleen wat je nodig hebt:

```text
Kiestoon v0.7.0 is installed.
  URL:      http://192.168.1.20:8080
  Username: admin
  Password: k7Rm-Xq4T-9fwP-Lh2c
```

## Het portaal openen

| Adres | Wanneer |
| --- | --- |
| `http://<server>:8080` | Altijd, in je eigen netwerk |
| `https://<server>:8443` | Hetzelfde portaal via HTTPS met een zelfondertekend certificaat. Nodig voor de browser-softphone (microfoon) |
| `https://<jouw domein>` | Nadat je [internettoegang](internet.md) aanzet (Let's Encrypt op poort 443) |

Het wachtwoord wordt per installatie aangemaakt. Kwijt voor je de eerste keer inlogt? Toon het opnieuw op de server:

```sh
sudo kiestoon initial-admin
```

Bij de eerste aanmelding kies je je eigen wachtwoord; het aangemaakte wachtwoord werkt daarna niet meer en wordt niet
meer getoond.

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
