# MCP server

With the MCP server, AI assistants such as Claude can work with your phone system. They can look things up,
make reports and, if you allow it, change settings or start calls. MCP (Model Context Protocol) is the open
standard that Claude, Claude Code, Cursor, n8n and many other clients use to talk to tools.

Open **MCP server** in the System menu. The server is off until you turn it on with the switch at the top.

![MCP server: status, policy and connections](shot:mcp)

## What an assistant can do

You can ask things in plain language, for example:

- "How many calls did we miss yesterday and why?"
- "Create an extension for Lisa, number 214, and link the Yealink T54W"
- "Which trunk had the worst call quality this week?"
- "Turn on night mode until 8:00 tomorrow"
- "Call 020 123 4567 from my phone"

The assistant chooses from **tools**. Each tool belongs to a **scope**, and a connection only sees the tools
of the scopes it was given:

| Scope | What it gives | Lowest role |
| --- | --- | --- |
| **View PBX** | Read the configuration: extensions, queues, IVRs, routes, trunks, time conditions, status | Admin |
| **Calls and statistics** | Call history, call journeys, queue and wallboard figures, call quality, callbacks | User (own calls) |
| **Phones and provisioning** | Desk phones, discovered phones, firmware | Admin |
| **Voicemail and recordings** | Voicemail, recordings and their AI summaries (only with **Share voicemail and recordings** on) | User (own) |
| **Start and steer calls** | Click to call, call forwarding, log in and out of queues, day/night mode, callbacks | User (own phone) |
| **Change configuration** | Create, change and delete configuration objects, link desk phones | Admin |
| **System and maintenance** | Backups (list and start), check for updates, resync a phone, read the audit log | Admin |

Besides tools the server offers a short guide to how Kiestoon is organised, and ready-made **prompts** such as
*Daily call report*, *Why were calls missed?*, *Set up a new employee*, *Health check of the phone system* and
*Design an IVR menu*. Claude shows them in its prompt menu.

Some things are never possible through MCP, whatever the scopes: managing users and tokens, restoring a backup,
installing updates and reading passwords or API keys. Secrets in the configuration are shown as `[redacted]`
and stay unchanged when the assistant saves an object. Trunks are read-only.

![Tools & scopes: turn single tools off](shot:mcp-tools)

Under **Tools & scopes** you see every tool with its scope and kind (read, action or change). Turn a tool off and
no connection can use it.

## Connect Claude (claude.ai and Claude Desktop)

claude.ai and Claude Desktop connect with OAuth: you sign in to Kiestoon yourself and choose what Claude may do.
No token has to be copied.

1. In Claude, open **Settings → Connectors → Add custom connector**.
2. Paste the address from the **Connect** tab, for example `https://pbx.example.com/mcp`.
3. Claude opens Kiestoon. Sign in (with MFA or a passkey) and choose the scopes and how long the connection is valid.
4. Done. The connection appears under **Connections**.

![The consent page: you choose what Claude may do](shot:mcp-consent)

!!! warning "Kiestoon must be reachable from the internet"
    claude.ai connects from Anthropic's cloud, also when you use Claude Desktop. Kiestoon must be reachable via
    HTTPS with its own domain (see [Internet access and SSL](internet.md)). With
    **OAuth only from Anthropic addresses** on, the MCP endpoint only accepts OAuth connections from Anthropic's
    published addresses (`160.79.104.0/21` and `2607:6bc0::/48`); your own network always keeps working.

The connection uses short-lived access tokens (one hour) that Claude renews itself. Revoke it under
**Connections** and Claude loses access straight away.

## Claude Code and other clients

Claude Code, Cursor, n8n and your own agents use a token. Create one under **Connections → New connection**:
choose a name, the rights, the scopes, how long it is valid and, optionally, the addresses it may be used from.
The token is shown **once**, together with the command for Claude Code:

```sh
claude mcp add --transport http kiestoon https://pbx.example.com/mcp \
  --header "Authorization: Bearer kt_mcp_…"
```

For other clients the **Connect** tab has a JSON configuration to copy. A token also works on your local network,
without internet mode.

![A new connection with its scopes](shot:mcp-connection)

## Connect your own phone

When **Users may connect their own phone** is on, employees without admin rights find **AI assistants** in their
workspace menu. They can connect Claude to their own extension: see their own calls and voicemail, call from their
phone, set call forwarding and log in or out of their queues. A connection never gets more rights than the person
who made it, and administrators see these connections too.

## Safety

| Setting | What it does |
| --- | --- |
| **New connections read-only by default** | Changes and actions must be switched on per connection |
| **Changes as a proposal first** | A change or action is first returned as a proposal with a summary of what will change. Only when the assistant calls the tool again with the confirmation code (valid for 10 minutes), after you said yes, is it carried out |
| **Mask phone numbers** | Hides the middle digits of phone numbers in call history and voicemail, for example `+3161•••••78` |
| **Share voicemail and recordings** | Off: the assistant cannot read voicemail, recordings or their transcripts |
| **OAuth only from Anthropic addresses** | See above |
| **Users may connect their own phone** | Lets users without admin rights make connections for their own extension |

Every connection also has an optional list of allowed addresses and a limit of 120 tool calls per minute.
Changes go through the same checks as in the portal and appear in the audit log with the name of the connection.

## Activity

The **Activity** tab shows every tool call of the last 90 days: when, which connection and user, which tool, a
short summary and whether it succeeded, was a proposal or was refused. Filter on reads, actions or changes,
search, and export to CSV.

![Activity of all connections](shot:mcp-activity)
