# Research: multi-tenant Kiestoon

Status: research only (phase 4). Nothing in this document is built. It answers one question: what would it
take for one Kiestoon installation to serve several organisations (tenants), for example a reseller with
twenty small offices?

## Summary

Kiestoon is single-tenant by design: one organisation, one Asterisk, one SQLite database, all state in
`/var/lib/kiestoon`. That keeps installation, backups and updates simple, and it is what most customers
need.

There are three ways to serve several organisations. The recommendation is **option B: one Kiestoon
container per tenant, with a shared SIP edge (Kamailio) and a small management portal in front**. It reuses
everything Kiestoon already does, keeps tenants fully separated, and the extra work is mostly around the
edges instead of inside every feature.

| | A. Tenants inside one Kiestoon | B. One container per tenant + SIP edge | C. Separate servers, no sharing |
| --- | --- | --- | --- |
| Separation between tenants | Logical (a `tenant_id` everywhere) | Process, file system and database | Complete |
| Work in Kiestoon itself | Very large: every table, dialplan context, API call and UI page | Small: a few settings for running behind an edge | None |
| New components | None | Kamailio + RTPengine, management portal | None |
| Public IP addresses needed | 1 | 1 | 1 per tenant |
| Resources per tenant | Lowest | Roughly 150 MB RAM idle per container (estimate) | A whole VM |
| Backups and restore per tenant | Hard (one database) | Already works (per container) | Already works |
| Updates per tenant | All at once | Per tenant, staged | Per tenant |
| Risk of a bug leaking data between tenants | Real | Very low | None |

## Option A: tenants inside one installation

Every row in the database gets a `tenant_id`, every Asterisk object gets a tenant prefix (`t12-201`), every
dialplan context is duplicated per tenant, and every API handler and UI page filters on the tenant of the
signed-in user. Asterisk itself has no tenant concept, so separation is only as good as the generated
configuration.

Problems found:

- About 40 tables and roughly 300 API routes would need tenant filtering. One missed `WHERE tenant_id = ?`
  shows one customer's call history to another.
- Feature codes, parking lots, pickup groups, conference numbers, voicemail contexts and music on hold all
  share one Asterisk namespace and must be prefixed and tested per tenant.
- Restoring one tenant from a backup means extracting its rows from a shared database and a shared spool.
- Reloading Asterisk for one tenant's change reloads it for everyone.
- Phone provisioning uses one set of credentials; it would need one per tenant.

This is how FreePBX-style multi-tenant forks and FusionPBX (on FreeSWITCH, which has domains built in) work.
For Kiestoon it would touch almost every file and would delay v1.0 by months. **Not recommended.**

## Option B: one container per tenant behind a shared SIP edge (recommended)

```
                 internet
                    |
     +--------------+---------------+
     |  Edge (one public IP)         |
     |  - Caddy: https://<tenant>.pbx.example.com -> container
     |  - Kamailio: SIP 5060/5061, routes on domain / DID
     |  - RTPengine: audio relay, one port range
     +--+-----------+-----------+----+
        |           |           |
   +----+---+  +----+---+  +----+---+
   | tenant | | tenant |  | tenant |   unchanged Kiestoon LXC containers,
   |   a    | |   b    |  |   c    |   private addresses (10.10.0.x)
   +--------+  +--------+  +--------+
```

### How the pieces fit

- **Kiestoon containers.** The LXC image Kiestoon already builds runs per tenant on a private network. Each
  keeps its own database, Asterisk, backups, update channel and admin accounts. Nothing inside changes.
- **Web.** Caddy (or the reverse proxy support built in phase 4) routes `https://<tenant>.pbx.example.com` to
  the right container. Each container trusts the edge as its reverse proxy and uses its own public URL.
- **SIP signalling.** Kamailio listens on the one public IP. Phones register as `201@a.pbx.example.com`;
  Kamailio picks the container from the domain and forwards (it does not need to know the passwords; the
  container still authenticates). Kamailio adds `Path` headers so calls to the phone go back through the edge.
- **Trunks.** Incoming calls from a provider are routed on the dialled number (DID) with a table in Kamailio
  (`dispatcher` or `dialplan` module, filled from the management portal). Each tenant can also keep its own
  trunk registration, which then goes out through the edge.
- **Audio.** RTPengine relays the RTP between the internet and the containers, so only one public port range
  is needed and phones behind NAT keep working. It also bridges WebRTC (DTLS-SRTP) to plain RTP if the
  browser softphone should go through the edge instead of Caddy.
- **Provisioning and zero-touch.** Phones get `https://<tenant>.pbx.example.com/prov/` as their address, so
  the redirection services (Yealink RPS, Snom SRAPS and the others) already work per tenant.
- **Management portal.** A small extra service creates and deletes tenant containers (`lxc-copy` of the
  image, or `incus`), fills the Kamailio routing tables, shows the status of every tenant and gives the
  reseller one sign-in that opens each tenant's Kiestoon with single sign-on (OIDC, already supported).

### Changes needed in Kiestoon

Small and contained:

1. A setting "SIP edge": the address of the Kamailio host, trusted for `Path`/`Record-Route`, and the
   external address to advertise. Asterisk then uses the edge as outbound proxy for trunks and phones.
2. Generating `rtp.conf` and PJSIP `external_media_address` for the edge case (RTP to RTPengine instead of
   the internet).
3. A management API token per container so the portal can read status (the API tokens of phase 3 already
   cover this) and an endpoint that reports health and version (exists: `/api/system/info`, `/metrics`).
4. Headless setup: create the first admin and SSO provider through the API when the container is created, so
   no setup token has to be copied by hand.

### Costs and limits

- Memory: an idle Kiestoon container is expected to use roughly 150 MB (to be measured before building). A server
  with 8 GB RAM and 4 cores comfortably runs 25 to 40 small tenants; concurrent calls, transcoding and
  speech-to-text set the real limit.
- Kamailio and RTPengine are mature (used by most large carriers) but are a new skill set to operate.
  RTPengine needs a kernel module for best performance; the user-space mode is fine for a few hundred calls.
- Updates still happen per container. The management portal can roll them out in waves.
- Fax with T.38 through RTPengine works but needs testing with each provider.

### Rough effort

| Part | Estimate |
| --- | --- |
| Edge: Kamailio + RTPengine configuration, packaging, tests | 3–4 weeks |
| Kiestoon changes (edge setting, headless setup) | 1 week |
| Management portal (tenants, routing tables, status, SSO, updates) | 3–4 weeks |
| Load and failure testing | 1–2 weeks |

## Option C: separate servers

Every customer gets their own Kiestoon VM or LXC container with its own public IP. This works today without
any changes and is the right answer for a handful of customers. It stops scaling when IPv4 addresses, router
port forwards and per-customer certificates become the bottleneck, which is exactly what option B solves.

## Recommendation

1. Ship v1.0 single-tenant as planned.
2. For resellers with a few customers, use option C now (one LXC per customer).
3. When there is demand for more, build option B as a separate product (working name "Kiestoon Host") in
   its own repository, with the small Kiestoon changes listed above in a minor release. The Kamailio +
   RTPengine work overlaps with Kiestoon Edge in phase 5, so build both on the same SIP edge component.
4. Do not build option A.

## Open questions for Reinder

- Is the target a reseller hosting many customers, or one organisation with several departments? For
  departments, Kiestoon's existing features (separate ring groups, time conditions and trunks per number) may
  already be enough.
- Should tenants be able to bring their own trunks, or does the reseller provide all numbers?
- Is a separate repository and product name for the edge acceptable?
