# Kiestoon REST API

Everything the web portal does goes through the same JSON API under `/api/`. Other programs (a CRM, a
helpdesk, scripts) use it with an **API token**. Webhooks tell those programs when something happens.

## API tokens

Create a token under **System → API & webhooks → API tokens**. The token is shown once; Kiestoon stores only
a hash of it.

```sh
curl -H "Authorization: Bearer kt_…" https://pbx.example.com/api/pbx/extensions
```

- A token has a role (`admin`, `reception` or `user`) and never more rights than the user who created it has
  at that moment. A token of a disabled user stops working.
- A **read-only** token can only use `GET`.
- Tokens cannot reach `/api/tokens`, `/api/webhooks`, `/api/users`, `/api/account`, `/api/auth`,
  `/api/system/updates` or `/api/backups/restore`.
- Write requests send JSON with `Content-Type: application/json`.
- Errors come back as `{"error": "<code>", "field": "<field>"}` with a 4xx or 5xx status.

### Useful endpoints

| Method | Path | Role | What |
| --- | --- | --- | --- |
| GET | `/api/pbx/extensions` | admin | All extensions |
| GET | `/api/live` | reception | Calls in progress, trunk status |
| GET | `/api/reception/queues` | reception | Queues with waiting callers and agents |
| GET | `/api/cdr` | reception | Call history (call detail records); `/api/cdr/export` gives CSV |
| GET | `/api/phonebook` | user | The central phone book |
| POST | `/api/phonebook` | reception | Add a contact `{name, company, work, mobile, other, email, notes}` |
| POST | `/api/calls` | reception | Click-to-call `{extension: "201", number: "0201234567"}`: the extension rings first |
| GET | `/api/faxes` | reception | Received faxes; `GET /api/faxes/{id}/pdf` downloads one |
| POST | `/api/fax/send` | reception | Send a fax `{boxId, number, name, pdf}`; `pdf` is the PDF in base64 (at most about 11 MB) |
| GET | `/api/fax/jobs` | reception | Outgoing faxes with their status (`queued`, `sending`, `success`, `failed`) |

## Webhooks

Add a webhook under **System → API & webhooks → Webhooks** and choose the events. Kiestoon sends a `POST` with
JSON to your URL:

```json
{
  "id": "4f1c2a…",
  "event": "call.ringing",
  "timestamp": "2026-11-15T09:30:12Z",
  "data": { "callId": "1731663012.42", "from": "0612345678", "fromName": "", "did": "0201234567", "extension": "201", "direction": "inbound" }
}
```

| Event | When | Data |
| --- | --- | --- |
| `call.ringing` | An incoming call rings on an extension | `callId, from, fromName, did, extension, direction` |
| `call.answered` | An incoming call is answered | same as `call.ringing` |
| `call.ended` | A call ended | the call detail record |
| `voicemail.updated` | The message count of a mailbox changed | `mailbox, new, old` |
| `fax.received` | A fax arrived | `id, box, status, pages, caller, did` |
| `queue.abandoned` | A caller hung up while waiting in a queue | `queue, from, fromName, position, holdTime, callId` |

### Headers and signature

| Header | Value |
| --- | --- |
| `X-Kiestoon-Event` | the event name |
| `X-Kiestoon-Delivery` | unique id of this delivery |
| `X-Kiestoon-Timestamp` | Unix time in seconds |
| `X-Kiestoon-Signature` | `sha256=` + hex HMAC-SHA256 of `<timestamp>.<body>` with the webhook secret |

Check the signature and reject old timestamps (for example older than 5 minutes):

```python
import hmac, hashlib, time

def valid(secret: str, timestamp: str, body: bytes, signature: str) -> bool:
    if abs(time.time() - int(timestamp)) > 300:
        return False
    mac = hmac.new(secret.encode(), timestamp.encode() + b"." + body, hashlib.sha256).hexdigest()
    return hmac.compare_digest("sha256=" + mac, signature)
```

Answer with a 2xx status within 10 seconds. Other answers are retried after 10 seconds and after 1 minute.
Redirects are not followed, and webhooks to this server itself (localhost) are refused; addresses in your own
network are allowed. The last 200 deliveries of each webhook are shown in the portal.

## Phone book for phones

- XML phone books: `/prov/phonebook/{yealink|snom|fanvil}.xml` and `/prov/phonebook/grandstream/phonebook.xml`,
  behind the provisioning user name and password. Provisioned phones get the address automatically.
- LDAP: base DN `ou=phonebook,dc=kiestoon`, attributes `cn`, `sn`, `givenName`, `o`, `telephoneNumber`,
  `mobile`, `homePhone`, `mail`. Bind as `cn=<provisioning user>,ou=phonebook,dc=kiestoon` with the provisioning
  password when "Require a password" is on. Only reachable from the networks that may use SIP.

## Prometheus metrics

Turn on **System → Internet access → Prometheus metrics**. Kiestoon then serves `GET /metrics` (outside
`/api/`) in the Prometheus text format. Prometheus sends the token as a bearer token; addresses in the
"allowed without a token" list may scrape without one.

```yaml
- job_name: kiestoon
  scheme: https
  authorization:
    credentials: <token>
  static_configs:
    - targets: ['pbx.example.com']
```

| Metric | What |
| --- | --- |
| `kiestoon_info{version}` | Always 1, with the running version |
| `kiestoon_uptime_seconds` | Seconds since the portal started |
| `kiestoon_asterisk_connected` | 1 when the portal talks to Asterisk |
| `kiestoon_active_calls`, `kiestoon_active_calls_by_direction{direction}` | Calls in progress |
| `kiestoon_calls_today` | Calls since midnight |
| `kiestoon_extensions`, `kiestoon_registered_endpoints` | Extensions and registered phones |
| `kiestoon_trunk_up{trunk}` | 1 when a trunk is registered or reachable |
| `kiestoon_queue_waiting_callers{queue}`, `kiestoon_queue_longest_wait_seconds{queue}`, `kiestoon_queue_available_agents{queue}` | Queue state |
| `kiestoon_queue_completed_total{queue}`, `kiestoon_queue_abandoned_total{queue}` | Queue counters since the last Asterisk restart |
| `kiestoon_fax_jobs{status}` | Outgoing faxes per status |
| `kiestoon_mail_queue_length` | Mails waiting to be sent |
| `kiestoon_backup_last_success_timestamp_seconds{target}` | Last successful backup per location |
