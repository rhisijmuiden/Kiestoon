# CRM integrations and call pop-up

Kiestoon links to **HubSpot**, **Salesforce**, **Pipedrive** and **Exact Online**. While the phone rings, a
pop-up shows who is calling according to the CRM, and every call is written back to the customer in the CRM
automatically. Nothing changes for installations without a CRM.

![System › Integrations › CRM](shot:crm)

## What it does

- **Recognising callers.** As soon as a call from outside comes in, Kiestoon looks the number up in the CRMs in
  the order you set. Numbers are compared on their last 9 digits, so `06…`, `+316…` and `00316…` all match the
  same contact. Results are kept for 10 minutes.
- **Pop-up.** In the web portal (on every page), the Windows softphone and the switchboard. It shows the contact,
  the company, open deals and tickets, the last contact and the owner, with a button to open the record in the
  CRM. When the browser phone rings, it also has *Answer* and *Decline*. An unknown number gets a button to
  create the contact.
- **Name on desk phones.** The caller name on the phone display becomes `Jan de Vries - Bakkerij De Vries`. The
  call waits at most 1.5 seconds for the CRM; when it is slower the phone rings with the number only.
- **Logging calls.** Every call with a customer becomes an activity on the contact (and company), under the
  colleague who took or made it: direction, duration, outcome and the number called. Missed calls can become a
  *Call back* task.
- **After the call** the pop-up asks for a note, the outcome and an optional follow-up task. The note is added to
  the activity that was logged automatically.
- **Recordings never leave the server.** The CRM gets a link that needs a Kiestoon login. With AI summaries of
  calls switched on ([AI](ai.md)), the summary is added to the activity a few minutes later.

Only the caller's phone number goes to the CRM to look it up. Tokens and secrets are stored on the server and
never shown again.

## Connecting a CRM

Open **System › Integrations › CRM** and click **Connect** on the CRM.

![Connecting a CRM](shot:crm-connect)

### HubSpot

1. In HubSpot: **Settings › Integrations › Private apps › Create a private app**.
2. Scopes: `crm.objects.contacts.read` and `.write`, `crm.objects.companies.read`, `crm.objects.deals.read`,
   `tickets`, `crm.objects.owners.read`.
3. Paste the access token in Kiestoon and save. Kiestoon checks the token at once.

### Salesforce

1. In Setup: **External Client App Manager › New**. Enable OAuth with the scope *Manage user data via APIs (api)*.
2. Turn on the **client credentials flow** and pick a *run as* user that may read contacts and accounts and
   create tasks.
3. Fill in your **My Domain URL** (`https://acme.my.salesforce.com`), the **consumer key** and the **consumer
   secret**.

### Pipedrive

1. In Pipedrive: your name › **Personal preferences › API**, copy the token. Use an account that sees all persons.
2. Fill in the **company domain** (the part before `.pipedrive.com`) and the token.

Pipedrive organisations have no phone field, so Kiestoon looks up persons only.

### Exact Online

1. In the [Exact App Center](https://apps.exactonline.com): **Manage apps › Register a test or production app**.
   Use the **redirect URI** that Kiestoon shows in the form (`https://<your PBX>/api/crm/exact/callback`).
2. Fill in the country, the **client ID** and the **client secret**, then click **Sign in with Exact** and sign in
   with an Exact user that may see relations.
3. Kiestoon refreshes the sign-in itself. Exact has no phone search, so Kiestoon keeps a list of the phone numbers
   of all contacts and accounts and refreshes it every hour.

Exact has no call activity: calls are logged as a *communication note* on the relation.

!!! note "The PBX address"
    The redirect URI and the links to recordings use the PBX address from [Separate host names](hostnames.md)
    or [Internet access](internet.md). Set it before you register the Exact app.

## Settings per CRM

| Setting | What |
| --- | --- |
| Look up contacts / companies | Which records are searched for the caller's number |
| Show in the pop-up | Open deals, open tickets, last activity, owner, lifecycle stage |
| Log every call automatically | Inbound and outbound, only inbound, or only answered calls |
| Missed calls become a task | A *Call back* task for the owner of the contact |
| Add a link to the recording | When the call was recorded |
| Add the AI summary | When AI summaries of calls are on |
| Create a contact for unknown numbers | Off by default: colleagues create one from the pop-up |
| Users ↔ owners | Matched on e-mail address; pick an owner by hand where it differs |

## For all CRMs

- **Look-up order**: the first connected CRM that knows the number wins. Unknown numbers fall back to the
  [phone book](phonebook.md).
- **Show the CRM name on desk phones** (on by default).
- **Pop-up for everyone whose phone rings**: also for colleagues in a queue or ring group. Off: only the
  colleague who answers sees it.

## Sync log and call history

The **CRM sync log** lists the look-ups and writes of the last 30 days. When a CRM cannot be reached, Kiestoon
tries again after 1, 5 and 30 minutes and then every hour, for 24 hours; then the admins get a mail and you can
**Retry now**. When a CRM refuses the stored sign-in, its card says **Sign in again** and look-ups skip it.

The [call history](history.md) gets a **CRM** column: *Logged*, *Task created*, *Retrying* or *Not a customer*.
Click it to open the record in the CRM.

To test a number without calling, use **Test a number** on the CRM tab.

## Other software

For a CRM that is not in the list, use the [API and webhooks](api.md): the `call.ringing` webhook for a pop-up in
your own software and `POST /api/calls` for click-to-call.
