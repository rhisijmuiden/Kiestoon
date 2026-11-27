package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Resources give the assistant background it can read without calling a tool; prompts are ready-made
// tasks a user can pick in the client.

const mcpGuide = `# How Kiestoon is organised

Kiestoon is a single-tenant phone system (PBX) on Asterisk, administered in a web portal.

- Extensions: the internal numbers of users and devices. An extension has a SIP account, voicemail, call
  features (do not disturb, forwarding, follow-me) and can be linked to a portal user, desk phones, the
  softphone in the browser, the Windows softphone and the mobile app.
- Ring groups ring several extensions together or in turn. Queues hold callers until an agent answers, with
  members (static or logged in), strategy, timeouts, callback offers and service level (SLA) targets.
- IVR menus are visual call flows: greetings, key choices, speech intents, time checks and destinations.
- Time conditions are opening-hours schedules. Their mode can be forced to open (day) or closed (night).
- Inbound routes send a phone number (DID) to a destination; outbound routes choose the trunk for dialled numbers.
- Trunks are the SIP accounts at the telecom provider.
- Desk phones are provisioned automatically (Yealink, Fanvil, Snom, Grandstream, Gigaset, Poly, …).
- Call history (CDR) has one record per call with a linkedId; the call journey shows every step of a call.
- Wallboard figures and call quality (MOS) come from live data; reports are mailed on a schedule.
- Backups go to FTP, FTPS, SCP, SFTP or OneDrive, full or incremental; restoring is only possible in the portal.

Destinations used in routes, IVRs and queues are written as objects like {"type": "extension", "id": 12}.
Look at an existing object with get_config before creating a similar one.`

func (c *mcpCall) resources() []map[string]any {
	out := []map[string]any{
		{"uri": "kiestoon://guide", "name": "guide", "title": "How Kiestoon is organised", "mimeType": "text/markdown",
			"description": "Concepts of the phone system: extensions, queues, IVR, routes, trunks, phones."},
	}
	if c.hasScope("pbx:read") && c.role() == "admin" {
		out = append(out,
			map[string]any{"uri": "kiestoon://system/status", "name": "system-status", "title": "System status", "mimeType": "application/json"},
			map[string]any{"uri": "kiestoon://feature-codes", "name": "feature-codes", "title": "Feature codes", "mimeType": "application/json"})
	}
	if c.hasScope("calls:read") && hasExtension(c) {
		out = append(out, map[string]any{"uri": "kiestoon://me", "name": "my-phone", "title": "My phone", "mimeType": "application/json"})
	}
	return out
}

func (c *mcpCall) resourceTemplates() []map[string]any {
	if !c.hasScope("pbx:read") || c.role() != "admin" {
		return []map[string]any{}
	}
	return []map[string]any{
		{"uriTemplate": "kiestoon://config/{kind}", "name": "config-list", "title": "Configuration list", "mimeType": "application/json",
			"description": "All objects of a kind: " + strings.Join(kindNames(false), ", ")},
		{"uriTemplate": "kiestoon://config/{kind}/{id}", "name": "config-item", "title": "Configuration object", "mimeType": "application/json"},
	}
}

func (c *mcpCall) readResource(uri string) (any, *rpcError) {
	text := func(mime, s string) (any, *rpcError) {
		return map[string]any{"contents": []map[string]any{{"uri": uri, "mimeType": mime, "text": s}}}, nil
	}
	asJSON := func(v any, err error) (any, *rpcError) {
		if err != nil {
			return nil, &rpcError{-32002, err.Error()}
		}
		b, _ := json.MarshalIndent(redact(v), "", " ")
		return text("application/json", string(b))
	}
	admin := c.hasScope("pbx:read") && c.role() == "admin"
	switch {
	case uri == "kiestoon://guide":
		return text("text/markdown", mcpGuide)
	case uri == "kiestoon://system/status" && admin:
		t, _ := mcpToolByName("system_status")
		return asJSON(t.Run(c, mcpArgs{}))
	case uri == "kiestoon://feature-codes" && admin:
		return asJSON(c.get("/api/pbx/featurecodes", nil))
	case uri == "kiestoon://me" && c.hasScope("calls:read"):
		return asJSON(c.get("/api/portal/me", nil))
	case strings.HasPrefix(uri, "kiestoon://config/") && admin:
		parts := strings.Split(strings.TrimPrefix(uri, "kiestoon://config/"), "/")
		ck, ok := configKindByKey(parts[0])
		if !ok {
			return nil, &rpcError{-32002, "resource not found"}
		}
		if len(parts) == 2 {
			var id int64
			fmt.Sscan(parts[1], &id)
			return asJSON(c.item(ck, id))
		}
		return asJSON(c.get(ck.Path, nil))
	}
	return nil, &rpcError{-32002, "resource not found"}
}

type mcpPrompt struct {
	Name, Title, Desc string
	Args              []map[string]any
	Scope             string
	Text              func(args map[string]string) string
}

var mcpPrompts = []mcpPrompt{
	{Name: "daily_call_report", Title: "Daily call report", Scope: "calls:read",
		Desc: "Summary of yesterday's (or a given day's) calls: volumes, missed calls, queues and notable calls.",
		Args: []map[string]any{{"name": "date", "description": "Day as YYYY-MM-DD (default yesterday)", "required": false}},
		Text: func(a map[string]string) string {
			d := a["date"]
			if d == "" {
				d = "yesterday"
			}
			return "Make a short call report for " + d + " of our Kiestoon phone system. Use search_calls (and wallboard_figures or queue figures when available): total calls inbound/outbound/internal, answered vs missed, the busiest hours, queues and their service level, and up to five calls that need attention (missed callers who did not get a callback, long waits). End with two or three concrete suggestions. Answer in the user's language."
		}},
	{Name: "missed_call_analysis", Title: "Why were calls missed?", Scope: "calls:read",
		Desc: "Analyse missed and abandoned calls of a period with their call journeys.",
		Args: []map[string]any{{"name": "from", "description": "First day YYYY-MM-DD", "required": false}, {"name": "to", "description": "Last day YYYY-MM-DD", "required": false}},
		Text: func(a map[string]string) string {
			return fmt.Sprintf("Find the missed and abandoned inbound calls between %s and %s with search_calls. For a sample of them, look at get_call_journey to see where the caller ended up (IVR, queue, voicemail, hung up while waiting). Group the causes, say when they happen most and suggest changes (queue timeouts, members, callback, opening hours). Do not change anything without asking.", orDefault(a["from"], "7 days ago"), orDefault(a["to"], "today"))
		}},
	{Name: "onboard_employee", Title: "Set up a new employee", Scope: "pbx:write",
		Desc: "Create an extension with voicemail for a new colleague, link a desk phone and add them to groups.",
		Args: []map[string]any{{"name": "name", "description": "Full name", "required": true}, {"name": "email", "description": "E-mail address", "required": false}, {"name": "details", "description": "Phone, department, queues …", "required": false}},
		Text: func(a map[string]string) string {
			return "Set up a new employee in Kiestoon: " + a["name"] + " " + a["email"] + ". " + a["details"] + "\nSteps: get the next free number with next_free_extension, look at an existing extension with get_config for the fields, then propose create_config for the extension (with voicemail). If a desk phone is mentioned, find it with list_phones or list_discovered_phones and propose assign_phone. Propose adding them to the right ring groups or queues with update_config. Every change is a proposal: show it and wait for my yes."
		}},
	{Name: "health_check", Title: "Health check of the phone system", Scope: "pbx:read",
		Desc: "Check trunks, phones, call quality, backups and updates and list what needs attention.",
		Text: func(a map[string]string) string {
			return "Do a health check of our Kiestoon phone system: system_status, every trunk with diagnose_trunk, call_quality for the last 7 days, desk phones that were not seen recently or have old firmware (list_phones, firmware_status), the latest backups (list_backups) and check_updates. Use only the tools you have. Report as a short list: OK / attention / action needed."
		}},
	{Name: "design_ivr", Title: "Design an IVR menu", Scope: "pbx:read",
		Desc: "Design a call flow for opening hours, menu choices and queues, based on what exists now.",
		Args: []map[string]any{{"name": "wish", "description": "What callers should be able to do", "required": true}},
		Text: func(a map[string]string) string {
			return "Help me design an IVR menu for Kiestoon: " + a["wish"] + "\nFirst look at the existing IVRs, queues, ring groups and time conditions (list_config / get_config). Propose the flow in plain language with the greeting texts. Only when I agree, and when you have the pbx:write scope, propose create_config or update_config for the IVR."
		}},
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func (c *mcpCall) prompts() []map[string]any {
	out := []map[string]any{}
	for _, p := range mcpPrompts {
		if !c.hasScope(p.Scope) {
			continue
		}
		args := p.Args
		if args == nil {
			args = []map[string]any{}
		}
		out = append(out, map[string]any{"name": p.Name, "title": p.Title, "description": p.Desc, "arguments": args})
	}
	return out
}

func (c *mcpCall) getPrompt(name string, args map[string]string) (any, *rpcError) {
	for _, p := range mcpPrompts {
		if p.Name == name && c.hasScope(p.Scope) {
			if args == nil {
				args = map[string]string{}
			}
			return map[string]any{"description": p.Desc, "messages": []map[string]any{
				{"role": "user", "content": map[string]any{"type": "text", "text": p.Text(args)}},
			}}, nil
		}
	}
	return nil, &rpcError{-32602, "unknown prompt"}
}
