# Gebruikers, rollen en inloggen

Iedereen die het portaal opent logt in met een eigen account. De **rol** van het account bepaalt wat iemand ziet.

## Rollen

| Rol | Wat diegene kan |
| --- | --- |
| **Superbeheerder** | Alles, ook andere superbeheerders beheren. |
| **Beheerder** | Beheert de telefooncentrale. Moet tweestapsverificatie gebruiken. |
| **Receptie** | Ziet live gesprekken en wachtrijen, meldt medewerkers aan en af, zet dag- en nachtstand om, gebruikt het schakelbord en wallboard. |
| **Gebruiker** | Regelt de eigen telefoon in het [gebruikersportaal](portal.md). |

Een account dat aan een **toestel** gekoppeld is krijgt ook het gebruikersportaal van dat toestel, welke rol het
ook heeft. Een beheerder met een toestel ziet beide menu's.

![Gebruikers & rollen](shot:users)

## Medewerkers toevoegen

Dat kan op twee manieren:

- **Met het toestel.** Zet in het formulier van het [toestel](extensions.md) **Portalaccount aanmaken en
  welkomstmail versturen** aan. De medewerker krijgt een e-mail met een link om een wachtwoord te kiezen (72 uur
  geldig), het adres van het portaal en de gegevens voor een softphone-app. Er wordt nooit een wachtwoord gemaild.
  **Welkomstmail versturen** stuurt later een nieuwe link.
- **Onder Gebruikers & rollen › Nieuwe gebruiker**, met gebruikersnaam, volledige naam, e-mail, rol en eventueel
  een toestel. Laat het wachtwoord leeg voor accounts die alleen single sign-on gebruiken.

Een uitgeschakeld account kan niet inloggen. **MFA resetten** zet de tweede stap van een account uit, bijvoorbeeld
na een verloren telefoon; diegene stelt hem bij de volgende keer inloggen opnieuw in.

## Tweestapsverificatie (MFA)

Beheerders en superbeheerders moeten een tweede stap gebruiken. Bij de eerste keer inloggen toont Kiestoon een
QR-code voor een authenticator-app (Microsoft Authenticator, Google Authenticator, 1Password, Bitwarden…). Na de
eerste code krijgen ze **herstelcodes**: elke code werkt één keer, voor als de telefoon kwijt is. Bewaar ze in een
wachtwoordmanager.

Een passkey, of single sign-on met **MFA van de provider vertrouwen**, telt ook als tweede stap.

Vanaf een [vertrouwd netwerk](firewall.md#vertrouwde-netwerken-mfa-overslaan), zoals het kantoornetwerk, kan de MFA-stap worden overgeslagen voor de rollen die je kiest.

## Passkeys

Onder **Mijn account › Passkeys** kan iedereen een passkey toevoegen: inloggen met vingerafdruk, gezicht of de
pincode van het apparaat in plaats van een wachtwoord. Passkeys werken alleen via **https** met een hostnaam
(bijvoorbeeld `https://pbx.example.com`), niet met een IP-adres. Zie [Internettoegang](internet.md).

![Mijn account](shot:account)

## Single sign-on

![Single sign-on](shot:sso)

Onder **Single sign-on › Nieuwe provider** koppel je een identiteitsprovider. De inlogpagina toont dan een knop
"Inloggen met …".

| Provider | Issuer-URL |
| --- | --- |
| **Microsoft Entra ID** | `https://login.microsoftonline.com/<tenant-id>/v2.0` |
| **Google Workspace** | Vast voor Google |
| **Keycloak** | Je Keycloak-adres met de naam van de realm |
| **authentik** | Eindigt op de slug van de applicatie |
| **Anders (OpenID Connect)** | Het adres dat `/.well-known/openid-configuration` levert |
| **SAML 2.0** | De metadata-URL van de identiteitsprovider |

### Microsoft Entra ID stap voor stap

1. Ga in het Entra-beheercentrum naar **App-registraties › Nieuwe registratie**.
2. Voeg de **Redirect-URI** toe die Kiestoon toont, met type **Web**.
3. Maak een **clientgeheim** onder *Certificaten en geheimen*.
4. Voeg onder *Tokenconfiguratie* een **groepenclaim** toe (beveiligingsgroepen).
5. Vul in Kiestoon de tenant-ID in de issuer-URL in, het **Client-ID (toepassings-ID)** en het **Clientgeheim**.
6. Koppel onder **Rolkoppeling** groepen aan rollen. Gebruik voor Entra ID de **object-ID** van de groep. Leden
   krijgen de rol van hun groep; de hoogste rol wint.
7. Kies de **Rol zonder passende groep**: een rol, of **Geen toegang** om iedereen buiten de groepen te weigeren.

Verder:

- **Accounts automatisch aanmaken**: bij de eerste keer inloggen. Uit: een beheerder maakt het account eerst aan,
  met hetzelfde e-mailadres.
- **MFA van de provider vertrouwen**: beheerders die met SSO inloggen hebben geen tweede Kiestoon-code nodig.
  Dwing MFA dan af bij de provider, bijvoorbeeld met Voorwaardelijke toegang.

Geef bij **SAML 2.0** de identiteitsprovider de **entity-ID / metadata** en de **reply-URL (ACS)** die Kiestoon
toont, en vul de metadata-URL in (of plak de XML). Groepen komen uit het **groepenattribuut**.

## Auditlog

Het **Auditlog** toont de laatste 200 gebeurtenissen, met tijd, gebruiker en IP-adres: elke keer inloggen en elke
mislukte poging, elke wijziging aan de configuratie, en gevoelige handelingen zoals een opname afspelen,
gespreksgeschiedenis exporteren, de back-upsleutel bekijken, herstellen, bijwerken of een fraudemelding.

![Auditlog](shot:audit)
