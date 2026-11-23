# Trunks

A **trunk** connects Kiestoon to your telecom provider, so you can call outside and receive calls on your phone
numbers. You can have several trunks, for example a main provider and a backup.

![Trunks & routes](shot:trunks)

## Add a trunk

1. Open **Trunks & routes › Trunks** and click **Add trunk**.
2. Choose the **provider**. A template fills in the server, port, transport, codecs and the provider's IP ranges,
   and shows a step-by-step guide in the form when there is one.
3. Fill in the account details from your provider and save. Within a minute the status turns **Registered**
   (registration) or **Reachable** (IP authentication).

![Editing the Voys trunk, with the built-in guide](shot:trunk-edit)

### Templates

| Template | Authentication | Notes |
| --- | --- | --- |
| **Voys** | Registration | `ha.voys.nl` with DNS SRV failover. [Full guide](providers/voys.md) |
| **VoIPGRID platform** (Spitsgroep, partners) | Registration | Create a VoIP trunk in their portal and use the server they give you |
| **KPN SIP trunk** | IP address | KPN SBC `213.162.171.164` (+ `.180`), TCP. [Full guide](providers/kpn.md) |
| **Cheapconnect** | Registration | `sip.cheapconnect.net`. [Full guide](providers/cheapconnect.md) |
| **Speakup** | Registration | `sip.speakup.nl`. [Full guide](providers/speakup.md) |
| **Vodafone Ziggo Zakelijk** | Registration | `trunk.sipnl.net`. [Full guide](providers/ziggo.md) |
| **Telnyx** | Registration | Credential connection; TLS on port 5061 recommended |
| **Twilio Elastic SIP** | IP address | Add the public IP of this server to the trunk's access control list |
| **Generic (registration)** | Registration | Any provider that gives a username and password |
| **Generic (IP authentication)** | IP address | Ask the provider which IP addresses they send calls from |

## Fields

| Field | What |
| --- | --- |
| **Authentication** | *Registration*: Kiestoon signs in with a username and password. *IP address*: the provider recognises your server by its public IP |
| **Host / Port** | The provider's SIP server. Port `0` follows the DNS SRV records of the host (recommended for Voys) |
| **Transport** | UDP, TCP or TLS |
| **Username / Password** | From your provider. **Authentication username** only when the provider gives a separate one |
| **From domain / From user** | Only when the provider asks for them |
| **Provider IP addresses** | Calls from these addresses are accepted as calls from this trunk, and the [firewall](firewall.md) lets them in |
| **Called number comes from** | *Request URI* or *To header*: where the provider puts your number on incoming calls |
| **Codecs** | In order of preference, for example `alaw,g722` |
| **Maximum simultaneous calls** | 0 means no limit |
| **Caller ID** | Default number shown to people you call. Must be one of your own numbers |
| **Encrypt audio (SRTP)** | When the provider supports it, with TLS |
| **T.38 fax** | Receive and send faxes with T.38 when the provider offers it; otherwise fax goes over G.711 |

## Status

The trunk list and the dashboard show the status of every trunk: *Registered*, *Reachable*, *Not registered*,
*Rejected* (wrong username or password) or *Unreachable*. A trunk that is offline shows a warning at the top of the
dashboard.

When a trunk is not registered or not reachable, the reason from the phone system log is shown under the status,
for example *Sign-in refused (401): the user name or password is wrong* or *The provider does not answer*.
**Show log lines** opens what to check and the log lines about the trunk. See [Logs](logs.md).

![Why a trunk does not work](shot:trunk-why)

### Refresh registration

A trunk that signs in with a username and password has a **Refresh registration** button (the round arrows) in the
trunk list. It makes Kiestoon sign in at the provider again right away, instead of waiting for the next retry. Use
it after you changed something at the provider, after an outage, or when the status stays *Rejected* or *Not
registered*. Kiestoon waits a few seconds for the provider's answer and shows it under the status:

| Message | Meaning |
| --- | --- |
| *Registered again just now* | The provider accepted the registration |
| *The provider rejected the registration* | Check the username and password with your provider |
| *Sent, the provider has not answered yet* | The status in the list updates by itself as soon as the answer comes |

![Refresh registration](shot:trunk-refresh)

The button is not there for a trunk with IP authentication (there is nothing to sign in) or a trunk that is turned off.

## Phone numbers

Under **Phone numbers** in the trunk form you note which numbers the provider delivers on this trunk. You can add:

- **A number block**: fill in the first number and the last number (`050 700 1200` to `050 700 1259`) or the first
  number and a count (`050 700 1200` and `60`). The form shows how many numbers that makes before you add them.
- **One or more numbers**: paste a list, one per line or separated by commas.

Numbers keep their leading zero or `+`. A number can belong to one trunk only, and a trunk holds at most 10,000
numbers. Remove a single number with its cross, or all of them with **Remove all**.

![Phone numbers of a trunk](shot:trunk-numbers)

The numbers are then offered wherever you choose a number:

- under **Inbound routes**, where numbers without a route are listed and can be sent to an extension, IVR menu,
  queue, ring group or any other [destination](concepts.md#destinations) in one go ([inbound routes](routes.md#inbound-routes));
- as suggestions for the **caller ID** of a trunk, an outbound route and an extension.

## Behind NAT

When Kiestoon has a private IP address, fill in the **public address** and the **local networks** under
[Settings › Network and NAT](settings.md), and forward the SIP port and the RTP range (10000-20000/udp) to the
server. Without that, calls connect but you hear nothing or only one side.

## Next

Calls on your numbers need an [inbound route](routes.md); calling out needs an [outbound route](routes.md).
