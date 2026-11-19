# Updates en kanalen

Kiestoon werkt zichzelf bij vanuit het portaal. Elke release staat op
[GitHub](https://github.com/rhisijmuiden/Kiestoon/releases) met de `.deb`-pakketten, LXC-templates, de
Windows-softphone en een bestand `SHA256SUMS`.

## Info & updates

De knop **Info & updates** onderaan het menu toont de versie, commit, builddatum, hostnaam, uptime en het
updatekanaal.

![Info & updates](shot:info)

1. Klik op **Controleren op updates**. Kiestoon vraagt GitHub naar de nieuwste release op je kanaal.
2. Is er een nieuwere versie, lees dan de **Release notes** en klik op **Nu bijwerken**.
3. Kiestoon maakt een back-up, installeert de nieuwe versie en start opnieuw. Het portaal is ongeveer een minuut
   onbereikbaar en komt vanzelf terug. Lopende gesprekken vallen weg, dus werk bij buiten kantoortijden.

Alleen beheerders kunnen het kanaal wijzigen of bijwerken.

## Kanalen

| Kanaal | Wat je krijgt |
| --- | --- |
| **Productie** | Stabiele releases (`v0.6.0`). Aangeraden. |
| **Test** | Release candidates (`v0.6.0-rc.12`), om nieuwe functies eerder dan iedereen te proberen. |

Je wisselt het kanaal in hetzelfde venster. Van Test terug naar Productie kan betekenen dat de nieuwste
productieversie ouder is dan wat je draait; Kiestoon meldt dat en laat je hem installeren.

Het menu toont het kanaal altijd met een gekleurde stip: groen voor productie, oranje voor test.

## Wat er gebeurt tijdens een update

De update draait als aparte systeemdienst (`kiestoon-updater`), zodat hij het herstarten van het portaal overleeft:

1. Hij downloadt de pakketten en `SHA256SUMS` van de GitHub-release en controleert de checksums.
2. Hij stopt het portaal en maakt een back-up van de datamap in `/var/lib/kiestoon-backups` (de nieuwste 5 blijven
   bewaard).
3. Hij installeert de pakketten, waarna het portaal opnieuw start en de nieuwe configuratie toepast.

Gaat er iets mis, dan toont het venster de fout en de back-up die gemaakt is. Updates vervangen je [back-ups naar een
externe locatie](backups.md) niet; laat die ook gewoon lopen.

## Met de hand bijwerken

Installeer op de opdrachtregel een gedownload pakket met `apt install ./kiestoon_<versie>_amd64.deb`. Voor
[edges van vestigingen](edge.md) is dat de manier om bij te werken.

## Een hoog-beschikbaar paar bijwerken

Werk eerst de standby bij, klik dan op **Omschakelen** en werk daarna de oude primaire bij. Zie
[Hoge beschikbaarheid](high-availability.md#omschakelen).
