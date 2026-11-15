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
| **KPN SIP trunk** | IP address | Fill in the IP addresses from your KPN order |
| **Telnyx** | Registration | Credential connection; TLS on port 5061 recommended |
| **Twilio Elastic SIP** | IP address | Add the public IP of this server to the trunk's access control list |
| **Generic (registration)** | Registration | Any provider that gives a username and password. Example: [Cheapconnect](providers/cheapconnect.md) |
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

## Behind NAT

When Kiestoon has a private IP address, fill in the **public address** and the **local networks** under
[Settings › Network and NAT](settings.md), and forward the SIP port and the RTP range (10000-20000/udp) to the
server. Without that, calls connect but you hear nothing or only one side.

## Next

Calls on your numbers need an [inbound route](routes.md); calling out needs an [outbound route](routes.md).
