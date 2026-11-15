# Inbound and outbound routes

Routes decide where calls go: **inbound routes** send each of your phone numbers to a destination, **outbound
routes** pick the trunk for numbers your users dial. Both are on **Trunks & routes**.

## Inbound routes

1. Open **Trunks & routes › Inbound routes** and click **Add inbound route**.
2. **Phone number**: your number as the provider sends it. `0201234567`, `31201234567` and `+31201234567` are
   treated as the same number. Leave it empty for a catch-all route: every number that no other route matches.
3. **Send to**: any [destination](concepts.md#destinations), for example a time condition for your main number, a
   queue for a support number, an extension for a direct number or a fax box for your fax number.
4. **Caller name prefix** (optional): shown before the caller's name on the phone display, for example `Support: `,
   so the person answering knows which number was called.

!!! tip "A typical setup"
    | Number | Send to |
    | --- | --- |
    | Main number | Time condition *Office hours* › IVR menu › ring group / queue |
    | Support number | Call queue *Support* |
    | Direct numbers | The employee's extension |
    | Fax number | Fax box |
    | Empty (catch-all) | Reception ring group |

Calls from someone in the [phone book](phonebook.md) show the contact's name.

## Outbound routes

Outbound routes are tried **from top to bottom**; the first route with a matching dial pattern is used. Use the
arrows to change the order.

1. Open **Outbound routes** and click **Add outbound route**.
2. Click a preset: **Dutch numbers** (`0[1-9]XXXXXXXX`) or **International** (`00X.`), or write your own patterns.
3. Add one or more **trunks**. They are tried in this order; the next trunk is used when one fails or is busy.
4. **Caller ID** (optional) overrides the trunk's caller ID for this route. An extension's own outbound caller ID
   always wins.

### Dial patterns

| Symbol | Means |
| --- | --- |
| `X` | Any digit 0-9 |
| `Z` | 1-9 |
| `N` | 2-9 |
| `[1-5]` | A digit in the range |
| `.` | One or more digits |

Each pattern has three parts: **prefix** (removed before dialing), **pattern** and **prepend** (added before
dialing). Example: prefix `9`, pattern `0XXXXXXXXX` lets users dial `9` for an outside line.

### Who may call what

Routes say how a number is dialed; the extension says whether it may be dialed. Each extension has an **outbound
class** (internal only, national, international) and a switch for premium-rate numbers. On top of that the
[fraud protection](firewall.md#fraud-protection) limits concurrent outbound calls and sends alerts.

## Emergency calls

**112 and 911 always work**, from every extension, without a route, and even when a trunk limit or the fraud lock
is active. Under **Settings › Emergency calls** you set the caller ID for emergency calls (a number tied to your
address) and the order in which trunks are tried.
