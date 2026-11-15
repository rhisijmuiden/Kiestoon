# Linking Kiestoon systems

Two or more Kiestoon systems (for example an office in the Netherlands and one in the US) can be linked. They keep
their own extensions, trunks and settings, and get:

- one phone book: the numbers of the other site appear with a **site prefix** (for example `8` + `201`);
- free internal calls through an encrypted WireGuard tunnel, with the caller shown as prefix + number so call back works;
- busy lamp (BLF) keys for colleagues on the other site (key value: prefix + number);
- queues with agents on both sites;
- **follow the sun**: a number goes to the site that is open now, each in its own time zone.

## Link two systems

1. On system A: **Sites → Branch offices**, switch on the tunnel hub and enter its public address. Forward UDP 51820
   (and TCP 8443 for pairing) to it.
2. On system A: **Sites → Linked systems → Invite a system**. Enter the name and prefix the numbers of B get on A.
3. On system B: **Linked systems → Join with a code**. Paste the code and choose the prefix the numbers of A get on B.

B connects to the hub of A; only A needs open ports. A star works too: several systems join one hub. The code pins the
certificate of A, like an edge pairing code (see [edge.md](edge.md)).

A prefix must not overlap with the numbers on the system (extensions, ring groups, queues, conference rooms, parking
70–79) or with another link. Prefixes start with 1–9; 0 stays the outside line.

## How it works

- Each link is a PJSIP endpoint `fed-<id>` over TCP to the tunnel address of the other system, identified by that
  address. Dialling `<prefix><number>` goes to the other system, which only routes it to its own internal numbers
  (never to its trunks), so a link cannot be used to make outside calls on the other side.
- Every minute each side fetches the phone book of the other (`/api/fed/v1/directory` through the tunnel, with a
  per-link token). Changes in busy state are pushed straight away and fully every minute.
- Busy lamps of remote numbers are custom device states (`Custom:ktfed<id>-<number>`), updated by the portal.
- Remote queue agents are `Local/<number>@kiestoon-fedout-<id>` members with that custom device state, so the queue
  knows when they are in a call.
- **Dial without prefix when unique** (per link): numbers of that site that do not exist locally can be dialled
  without the prefix.
- Routing destinations: *Linked system* (a number on the other site) and *Follow the sun* can be chosen everywhere a
  destination is chosen (inbound routes, IVR, time conditions, queue overflow).

## Follow the sun

**Sites → Follow the sun**: a list of rules, each with a time zone, days and hours and a destination. The first rule
that matches the current time in its own time zone gets the call; otherwise the fallback destination does. Example:
Amsterdam 08:00–17:00 (Europe/Amsterdam) to the Dutch support queue, New York 09:00–17:00 (America/New_York) to the
American support queue on the linked system, else voicemail.

## Unlinking

Removing a link on one side tells the other side, removes the endpoint and clears destinations that pointed to it.

## Ports

| Port | Where | What |
| --- | --- | --- |
| UDP 51820 | hub, from the other systems | WireGuard |
| TCP 8443 | hub, once while pairing | portal (HTTPS) |
