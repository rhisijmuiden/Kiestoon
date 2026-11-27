# Kiestoon-app in Google Play

Alles om de Android-app `nl.rhis.kiestoon` in Google Play te publiceren. English: [README.md](README.md).

## Stappen die je zelf doet (eenmalig)

Deze stappen kunnen alleen met jouw accounts. De volgorde is belangrijk.

1. **Firebase (push).** Ga naar console.firebase.google.com, maak een project "Kiestoon" (Analytics uit), voeg een
   **Android-app** toe met pakketnaam `nl.rhis.kiestoon` en download `google-services.json`. Zet de inhoud in GitHub
   als secret `GOOGLE_SERVICES_JSON`. Zonder dit bestand gaat de app alleen over als hij open staat.
2. **Uploadsleutel.** Draai op je eigen computer `./mobile/store/make-upload-key.sh ~/kiestoon-keys` (buiten de
   repo). Zet de twee waarden die het script toont in GitHub als secrets `ANDROID_UPLOAD_KEYSTORE` en
   `ANDROID_UPLOAD_STORE_PASSWORD`. Bewaar het `.p12`-bestand en het wachtwoord in je wachtwoordmanager.
3. **Eerste build.** GitHub > Actions > **Mobile app (Google Play)** > Run workflow (track `internal`). Zonder het
   Play-secret van stap 6 maakt hij alleen de ondertekende `.aab` als artifact: download die.
4. **App aanmaken in Play Console.** Alle apps > App maken: naam "Kiestoon zakelijke telefoon" / "Kiestoon
   Business Phone", standaardtaal Engels (en-US), App, Gratis. Laat **Play App Signing** aan staan.
5. **Eerste upload met de hand.** Testen > Interne tests > Nieuwe release, upload de `.aab` uit stap 3 en voeg
   jezelf toe als tester. Google accepteert de allereerste upload alleen via de website.
6. **Serviceaccount voor automatisch uploaden.** In console.cloud.google.com (zelfde project als Firebase mag):
   IAM > Serviceaccounts > maken ("play-upload"), tabblad Sleutels > JSON-sleutel maken. Zet de inhoud in GitHub
   als secret `PLAY_SERVICE_ACCOUNT_JSON`. Nodig in Play Console onder **Gebruikers en rechten** het e-mailadres van
   het serviceaccount uit met voor deze app: releases beheren voor testtracks (en productie als je dat wilt).
   Daarna gaat elke release vanzelf naar de interne testtrack.
7. **Winkelvermelding invullen** met de teksten en afbeeldingen uit `android/en-US` en `android/nl-NL` (zie
   hieronder), en de vragenlijsten (Gegevensveiligheid, App-toegang, Inhoudsclassificatie enz.) met de antwoorden
   onderaan deze pagina.
8. **Demo voor de reviewer.** Google test de app zelf. Zet een Kiestoon klaar die vanaf internet bereikbaar is, met
   een demogebruiker met toestel en zonder MFA, en vul die in bij App-toegang.
9. **Video voor de voorgrondservice.** Neem een korte schermopname (YouTube, niet-openbaar) van een gesprek in de
   app dat doorloopt als je het scherm vergrendelt. Play vraagt die bij de verklaring voor
   voorgrondservices.
10. **Pushdienst.** De Firebase-serviceaccountsleutel (Firebase > Projectinstellingen > Serviceaccounts > Nieuwe
    privésleutel) hoort op de server van `push.rhis.nl` in `/etc/kiestoon-push/fcm.json`, zie
    `docs/mobile-app.md` (Eigen pushdienst). Dat is een andere sleutel dan die van stap 6.

**Let op bij een persoonlijk ontwikkelaarsaccount** (aangemaakt na november 2023): Google eist dan eerst een
gesloten test met minimaal 12 testers gedurende 14 dagen voordat je naar productie mag. Een
organisatie-account (RHIS, met D-U-N-S-nummer) heeft die eis niet.

## Bestanden

| Pad | Wat |
| --- | --- |
| `android/<taal>/title.txt`, `short_description.txt`, `full_description.txt` | Winkelvermelding, Engels (`en-US`) en Nederlands (`nl-NL`) |
| `android/<taal>/changelogs/default.txt` | Tekst "Wat is er nieuw" bij elke upload |
| `android/<taal>/images/` | Pictogram 512 x 512, feature graphic 1024 x 500, vijf schermafbeeldingen 1080 x 1920 |
| `make-upload-key.sh` | Maakt de uploadsleutel en toont de GitHub-secrets |
| `../../.github/workflows/mobile-release.yml` | Bouwt de ondertekende `.aab` en uploadt naar Google Play |
| `../../docs/app-privacy.nl.md` | Privacybeleid: https://rhisijmuiden.github.io/Kiestoon/nl/app-privacy/ (Engels: `/app-privacy/`) |

## Hoe een release in Google Play komt

Na elke geslaagde release (workflow **Release**) bouwt **Mobile app (Google Play)** de App Bundle, ondertekent die
met de uploadsleutel en zet hem op de **interne testtrack**. Het versienummer voor Play (versionCode) komt uit
`VERSION`: 0.7.0-rc.35 wordt 700035, 0.7.0 wordt 700999. Doorzetten naar gesloten test of productie doe je in Play
Console, of je start de workflow met de hand en kiest een track.

De echte ondertekeningssleutel van de app beheert Google (Play App Signing). De repository ziet alleen de
**uploadsleutel**, en alleen als GitHub-secret; hij staat nooit in git.

## Antwoorden voor Play Console

### Gegevensveiligheid

| Vraag | Antwoord |
| --- | --- |
| Verzamelt of deelt je app gegevens van de vereiste typen? | Ja |
| Worden alle gegevens versleuteld tijdens verzending? | Ja |
| Kunnen gebruikers verzoeken hun gegevens te verwijderen? | Ja (telefoon ontkoppelen onder Mijn apparaten, of via de organisatie; zie privacybeleid) |

Gegevenstypen (allemaal: **verzameld, niet gedeeld**, **vereist**, doel **App-functionaliteit**):

| Type | Waarom | Tijdelijk verwerkt |
| --- | --- | --- |
| Persoonlijke info > **Gebruikers-ID's** | Inlognaam, naar de centrale van de organisatie | Nee |
| Audio > **Spraak- of geluidsopnamen** | Geluid van gesprekken, naar de centrale (die gesprekken kan opnemen als de organisatie dat aanzet) | Nee |
| App-activiteit > **App-interacties** | Gesprekken, aanmelden bij wachtrijen, niet storen | Nee |
| Apparaat- of andere ID's | Pushtoken en telefoonnaam, om de app te laten overgaan | Nee |

"Niet gedeeld": de gegevens gaan alleen naar de centrale waar de gebruiker de app zelf aan koppelt en naar Google
Firebase Cloud Messaging als dienstverlener. Geen advertenties, analytics of crashrapportage.

### Overige verklaringen

| Onderdeel | Antwoord |
| --- | --- |
| App-toegang | Sommige functies zijn beperkt. Geef het adres van de demo-Kiestoon, de demogebruiker en: "Open the app, enter the server address, leave the pairing code empty, tap Link, sign in with the user below." |
| Advertenties | Geen advertenties |
| Inhoudsclassificatie | Categorie: Hulpprogramma, productiviteit, communicatie of overig. Gebruikers kunnen met elkaar communiceren (spraak): Ja. Al het andere: Nee |
| Doelgroep | 18 jaar en ouder |
| Nieuws-app, Overheids-app, Financiële functies, Gezondheid | Nee |
| Rechten voor voorgrondservices | `phoneCall` en `microphone`: "Keeps an ongoing VoIP call (audio) running when the user leaves the app or locks the screen." Plus de video van stap 9 |
| Intent voor volledig scherm | De app is een bel-app: inkomende gesprekken worden op volledig scherm getoond |
| App-categorie | Zakelijk. Tags: Zakelijk, Communicatie |
| Privacybeleid | https://rhisijmuiden.github.io/Kiestoon/app-privacy/ |
| Contactgegevens | E-mailadres en website van RHIS (verplicht, zichtbaar in de winkel) |
