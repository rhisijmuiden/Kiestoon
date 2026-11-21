# API, webhooks en metrics

Alles wat het webportaal doet, gaat via dezelfde JSON-API onder `/api/`. Andere programma's (een CRM, een
helpdesk, scripts) gebruiken die met een **API-token**. Webhooks laten die programma's weten wanneer er iets gebeurt.

![Integraties](shot:integrations)

## API-tokens

Maak een token aan onder **Integraties › API-tokens › Nieuw token**. Het token wordt één keer getoond; Kiestoon
bewaart alleen een hash ervan.

```sh
curl -H "Authorization: Bearer kt_…" https://pbx.example.com/api/pbx/extensions
```

- Een token heeft een rol (`admin`, `reception` of `user`) en nooit meer rechten dan de gebruiker die het maakte op
  dat moment heeft. Een token van een uitgeschakelde gebruiker werkt niet meer.
- Een **alleen-lezen**-token kan alleen `GET` gebruiken.
- Tokens kunnen niet bij `/api/tokens`, `/api/webhooks`, `/api/users`, `/api/account`, `/api/auth`,
  `/api/system/updates` of `/api/backups/restore`.
- Schrijfverzoeken sturen JSON met `Content-Type: application/json`.
- Fouten komen terug als `{"error": "<code>", "field": "<veld>"}` met een 4xx- of 5xx-status.

### Handige endpoints

| Methode | Pad | Rol | Wat |
| --- | --- | --- | --- |
| GET | `/api/pbx/extensions` | admin | Alle toestellen |
| GET | `/api/live` | reception | Lopende gesprekken, status van trunks |
| GET | `/api/reception/queues` | reception | Wachtrijen met wachtende bellers en medewerkers |
| GET | `/api/cdr` | reception | Gespreksgeschiedenis (call detail records); `/api/cdr/export` geeft CSV |
| GET | `/api/phonebook` | user | Het centrale telefoonboek |
| POST | `/api/phonebook` | reception | Contact toevoegen `{name, company, work, mobile, other, email, notes}` |
| POST | `/api/calls` | reception | Click-to-call `{extension: "201", number: "0201234567"}`: eerst gaat het toestel over |
| GET | `/api/faxes` | reception | Ontvangen faxen; `GET /api/faxes/{id}/pdf` downloadt er een |
| POST | `/api/fax/send` | reception | Fax versturen `{boxId, number, name, pdf}`; `pdf` is de PDF in base64 (maximaal ongeveer 11 MB) |
| GET | `/api/fax/jobs` | reception | Uitgaande faxen met hun status (`queued`, `sending`, `success`, `failed`) |

## Webhooks

Voeg een webhook toe onder **Integrations › Webhooks › Webhook toevoegen** en kies de gebeurtenissen. Kiestoon
stuurt een `POST` met JSON naar je URL:

```json
{
  "id": "4f1c2a…",
  "event": "call.ringing",
  "timestamp": "2026-11-15T09:30:12Z",
  "data": { "callId": "1731663012.42", "from": "0612345678", "fromName": "", "did": "0201234567", "extension": "201", "direction": "inbound" }
}
```

| Gebeurtenis | Wanneer | Data |
| --- | --- | --- |
| `call.ringing` | Een inkomend gesprek gaat over op een toestel | `callId, from, fromName, did, extension, direction` |
| `call.answered` | Een inkomend gesprek wordt aangenomen | hetzelfde als `call.ringing` |
| `call.ended` | Een gesprek is afgelopen | het call detail record |
| `voicemail.updated` | Het aantal berichten van een voicemailbox veranderde | `mailbox, new, old` |
| `fax.received` | Er kwam een fax binnen | `id, box, status, pages, caller, did` |
| `queue.abandoned` | Een beller hing op terwijl hij in een wachtrij wachtte | `queue, from, fromName, position, holdTime, callId` |

### Headers en handtekening

| Header | Waarde |
| --- | --- |
| `X-Kiestoon-Event` | de naam van de gebeurtenis |
| `X-Kiestoon-Delivery` | unieke id van deze aflevering |
| `X-Kiestoon-Timestamp` | Unix-tijd in seconden |
| `X-Kiestoon-Signature` | `sha256=` + hex HMAC-SHA256 van `<timestamp>.<body>` met het geheim van de webhook |

Controleer de handtekening en weiger oude tijdstempels (bijvoorbeeld ouder dan 5 minuten):

```python
import hmac, hashlib, time

def valid(secret: str, timestamp: str, body: bytes, signature: str) -> bool:
    if abs(time.time() - int(timestamp)) > 300:
        return False
    mac = hmac.new(secret.encode(), timestamp.encode() + b"." + body, hashlib.sha256).hexdigest()
    return hmac.compare_digest("sha256=" + mac, signature)
```

Antwoord binnen 10 seconden met een 2xx-status. Andere antwoorden worden na 10 seconden en na 1 minuut opnieuw
geprobeerd. Redirects worden niet gevolgd, en webhooks naar deze server zelf (localhost) worden geweigerd; adressen
in je eigen netwerk zijn toegestaan. De laatste 200 afleveringen van elke webhook staan in het portaal.

## Telefoonboek voor telefoons

- XML-telefoonboeken: `/prov/phonebook/{yealink|snom|fanvil}.xml` en `/prov/phonebook/grandstream/phonebook.xml`,
  achter de gebruikersnaam en het wachtwoord van provisioning. Geprovisionede telefoons krijgen het adres
  automatisch.
- LDAP: base DN `ou=phonebook,dc=kiestoon`, attributen `cn`, `sn`, `givenName`, `o`, `telephoneNumber`, `mobile`,
  `homePhone`, `mail`. Bind als `cn=<provisioning-gebruiker>,ou=phonebook,dc=kiestoon` met het
  provisioning-wachtwoord als "Wachtwoord verplicht" aan staat. Alleen bereikbaar vanaf de netwerken die SIP mogen
  gebruiken.

## Prometheus-metrics

Zet **Internettoegang › Prometheus-metrics › Metrics publiceren** aan. Kiestoon levert dan `GET /metrics` (buiten
`/api/`) in het tekstformaat van Prometheus. Prometheus stuurt het token als bearer-token; adressen in de lijst
"Toegestaan zonder token" mogen zonder token ophalen.

```yaml
- job_name: kiestoon
  scheme: https
  authorization:
    credentials: <token>
  static_configs:
    - targets: ['pbx.example.com']
```

| Metric | Wat |
| --- | --- |
| `kiestoon_info{version}` | Altijd 1, met de draaiende versie |
| `kiestoon_uptime_seconds` | Seconden sinds het portaal startte |
| `kiestoon_asterisk_connected` | 1 als het portaal met Asterisk praat |
| `kiestoon_active_calls`, `kiestoon_active_calls_by_direction{direction}` | Lopende gesprekken |
| `kiestoon_calls_today` | Gesprekken sinds middernacht |
| `kiestoon_extensions`, `kiestoon_registered_endpoints` | Toestellen en geregistreerde telefoons |
| `kiestoon_trunk_up{trunk}` | 1 als een trunk geregistreerd of bereikbaar is |
| `kiestoon_queue_waiting_callers{queue}`, `kiestoon_queue_longest_wait_seconds{queue}`, `kiestoon_queue_available_agents{queue}` | Status van wachtrijen |
| `kiestoon_queue_completed_total{queue}`, `kiestoon_queue_abandoned_total{queue}` | Tellers van wachtrijen sinds de laatste herstart van Asterisk |
| `kiestoon_fax_jobs{status}` | Uitgaande faxen per status |
| `kiestoon_mail_queue_length` | Mails die wachten op verzending |
| `kiestoon_backup_last_success_timestamp_seconds{target}` | Laatste geslaagde back-up per locatie |
