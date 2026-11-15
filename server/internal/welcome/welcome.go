// Package welcome renders the welcome e-mail a new portal account gets: a link to set a password,
// the portal addresses and the details to connect a phone or softphone app. English and Dutch.
package welcome

import (
	"bytes"
	htmltemplate "html/template"
	"strconv"
	"strings"
	texttemplate "text/template"
)

// Link is a portal address with what it is for.
type Link struct {
	Label string
	URL   string
}

// Data fills the templates.
type Data struct {
	Name           string
	Username       string
	SetPasswordURL string
	ExpiresHours   int
	Portal         []Link
	// DevicesURL is the "My devices" page with the SIP password and the QR code.
	DevicesURL string
	// Extension is empty for an account without a phone.
	Extension string
	SIPServer string
	SIPPort   int
	Transport string
	Voicemail bool
	WebRTC    bool
	// SSO names the identity providers the account can also sign in with.
	SSO []string
}

type strs struct {
	Subject, Hello, Intro, Button, ButtonHint, NoButton, Account, Username, Extension, PortalTitle, PortalHTTPSHint,
	PhoneTitle, PhoneText, Server, Port, Transport, SIPUser, SIPPassword, SIPPasswordValue, Voicemail, VoicemailText,
	Softphone, SSOTitle, SSOText, Footer string
}

var texts = map[string]strs{
	"en": {
		Subject:          "Welcome to Kiestoon: your phone account",
		Hello:            "Hello",
		Intro:            "An account has been created for you on the company phone system. Set a password to sign in to your personal portal, where you manage your phone, voicemail, call forwarding and devices.",
		Button:           "Set your password",
		ButtonHint:       "This link works once and expires in %d hours. Ask your administrator for a new one if it has expired.",
		Account:          "Your account",
		Username:         "Username",
		Extension:        "Extension",
		PortalTitle:      "Portal",
		PortalHTTPSHint:  "Use the https address for the browser softphone: the browser only allows the microphone on a secure connection.",
		PhoneTitle:       "Connect a softphone app on your mobile",
		PhoneText:        "Use these details in a SIP app such as Linphone, Zoiper or Groundwire. The page My devices in the portal shows your SIP password and a QR code.",
		Server:           "Server",
		Port:             "Port",
		Transport:        "Transport",
		SIPUser:          "SIP username",
		SIPPassword:      "SIP password",
		SIPPasswordValue: "in the portal under My devices",
		Voicemail:        "Voicemail",
		VoicemailText:    "Dial *97 from your phone to listen to your voicemail. Choose your own voicemail PIN in the portal under Voicemail.",
		Softphone:        "You can also call from your browser: open the portal over https and switch on the softphone.",
		SSOTitle:         "Single sign-on",
		SSOText:          "You can also sign in with your work account:",
		Footer:           "You get this e-mail because an administrator created an account for you. Not expecting it? Then you can ignore it.",
	},
	"nl": {
		Subject:          "Welkom bij Kiestoon: je telefoonaccount",
		Hello:            "Hallo",
		Intro:            "Er is een account voor je aangemaakt op de telefooncentrale. Stel een wachtwoord in om in te loggen op je persoonlijke portal, waar je je toestel, voicemail, doorschakelingen en apparaten beheert.",
		Button:           "Wachtwoord instellen",
		ButtonHint:       "Deze link werkt één keer en verloopt over %d uur. Is hij verlopen? Vraag je beheerder dan om een nieuwe.",
		Account:          "Je account",
		Username:         "Gebruikersnaam",
		Extension:        "Toestelnummer",
		PortalTitle:      "Portal",
		PortalHTTPSHint:  "Gebruik het https-adres voor de softphone in de browser: de browser staat de microfoon alleen toe via een beveiligde verbinding.",
		PhoneTitle:       "Een softphone-app op je mobiel koppelen",
		PhoneText:        "Gebruik deze gegevens in een SIP-app zoals Linphone, Zoiper of Groundwire. Op de pagina Mijn apparaten in de portal staan je SIP-wachtwoord en een QR-code.",
		Server:           "Server",
		Port:             "Poort",
		Transport:        "Transport",
		SIPUser:          "SIP-gebruikersnaam",
		SIPPassword:      "SIP-wachtwoord",
		SIPPasswordValue: "in de portal onder Mijn apparaten",
		Voicemail:        "Voicemail",
		VoicemailText:    "Bel *97 vanaf je toestel om je voicemail te beluisteren. Kies je eigen voicemail-pincode in de portal onder Voicemail.",
		Softphone:        "Je kunt ook bellen vanuit je browser: open de portal via https en zet de softphone aan.",
		SSOTitle:         "Single sign-on",
		SSOText:          "Je kunt ook inloggen met je werkaccount:",
		Footer:           "Je krijgt deze e-mail omdat een beheerder een account voor je heeft aangemaakt. Verwachtte je hem niet? Dan kun je hem negeren.",
	},
}

type view struct {
	Data
	T          strs
	ButtonHint string
}

// Render returns the subject, the plain text and the HTML version in the language ("en" or "nl").
func Render(lang string, d Data) (subject, text, html string, err error) {
	t, ok := texts[lang]
	if !ok {
		t = texts["en"]
	}
	v := view{Data: d, T: t, ButtonHint: strings.Replace(t.ButtonHint, "%d", strconv.Itoa(d.ExpiresHours), 1)}
	var tb, hb bytes.Buffer
	if err := textTpl.Execute(&tb, v); err != nil {
		return "", "", "", err
	}
	if err := htmlTpl.Execute(&hb, v); err != nil {
		return "", "", "", err
	}
	return t.Subject, tb.String(), hb.String(), nil
}

var textTpl = texttemplate.Must(texttemplate.New("t").Parse(`{{.T.Hello}} {{.Name}},

{{.T.Intro}}

{{.T.Button}}:
{{.SetPasswordURL}}
{{.ButtonHint}}

{{.T.Account}}
  {{.T.Username}}: {{.Username}}
{{- if .Extension}}
  {{.T.Extension}}: {{.Extension}}{{end}}

{{.T.PortalTitle}}
{{- range .Portal}}
  {{.Label}}: {{.URL}}{{end}}
{{- if gt (len .Portal) 1}}
{{.T.PortalHTTPSHint}}{{end}}
{{- if .SSO}}

{{.T.SSOTitle}}
{{.T.SSOText}} {{range $i, $n := .SSO}}{{if $i}}, {{end}}{{$n}}{{end}}{{end}}
{{- if .Extension}}

{{.T.PhoneTitle}}
{{.T.PhoneText}}
  {{.T.Server}}: {{.SIPServer}}
  {{.T.Port}}: {{.SIPPort}}
  {{.T.Transport}}: {{.Transport}}
  {{.T.SIPUser}}: {{.Extension}}
  {{.T.SIPPassword}}: {{.T.SIPPasswordValue}}
  {{.DevicesURL}}
{{- if .WebRTC}}
{{.T.Softphone}}{{end}}
{{- if .Voicemail}}

{{.T.Voicemail}}
{{.T.VoicemailText}}{{end}}{{end}}

--
{{.T.Footer}}
Kiestoon · Powered by RHIS
`))

// The HTML uses tables and inline styles: most mail clients ignore style sheets. Colours are the
// Kiestoon tokens (branding/tokens.json), dark header with the light theme body for readability.
var htmlTpl = htmltemplate.Must(htmltemplate.New("h").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><title>{{.T.Subject}}</title></head>
<body style="margin:0;padding:0;background:#F4F7FB;font-family:Inter,Segoe UI,Helvetica,Arial,sans-serif;color:#0F172A">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F4F7FB;padding:24px 12px"><tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:600px;background:#FFFFFF;border-radius:16px;overflow:hidden;border:1px solid rgba(15,23,42,0.12)">
<tr><td style="background:#070B14;padding:24px 32px">
  <span style="display:inline-block;width:12px;height:12px;border-radius:50%;background:#22D3EE;box-shadow:0 0 12px #22D3EE;vertical-align:middle"></span>
  <span style="font-family:'Space Grotesk',Inter,Arial,sans-serif;font-size:22px;font-weight:600;color:#E2E8F0;letter-spacing:-0.3px;vertical-align:middle;margin-left:8px">Kiestoon</span>
</td></tr>
<tr><td style="padding:32px">
  <h1 style="margin:0 0 12px;font-family:'Space Grotesk',Inter,Arial,sans-serif;font-size:24px;font-weight:600;color:#0F172A">{{.T.Hello}} {{.Name}},</h1>
  <p style="margin:0 0 24px;font-size:15px;line-height:1.6;color:#526072">{{.T.Intro}}</p>
  <table role="presentation" cellpadding="0" cellspacing="0"><tr><td style="border-radius:10px;background:#0891B2">
    <a href="{{.SetPasswordURL}}" style="display:inline-block;padding:13px 26px;font-size:15px;font-weight:600;color:#FFFFFF;text-decoration:none;border-radius:10px">{{.T.Button}}</a>
  </td></tr></table>
  <p style="margin:12px 0 0;font-size:13px;line-height:1.5;color:#64748B">{{.ButtonHint}}</p>
  <p style="margin:8px 0 0;font-size:12px;line-height:1.5;color:#64748B;word-break:break-all"><a href="{{.SetPasswordURL}}" style="color:#0891B2">{{.SetPasswordURL}}</a></p>

  <h2 style="margin:32px 0 12px;font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#0891B2">{{.T.Account}}</h2>
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid rgba(15,23,42,0.12);border-radius:10px">
    <tr><td style="padding:10px 14px;font-size:14px;color:#526072;width:45%">{{.T.Username}}</td><td style="padding:10px 14px;font-family:'JetBrains Mono',Consolas,monospace;font-size:14px;color:#0F172A">{{.Username}}</td></tr>
    {{- if .Extension}}<tr><td style="padding:10px 14px;font-size:14px;color:#526072;border-top:1px solid rgba(15,23,42,0.08)">{{.T.Extension}}</td><td style="padding:10px 14px;font-family:'JetBrains Mono',Consolas,monospace;font-size:14px;color:#0F172A;border-top:1px solid rgba(15,23,42,0.08)">{{.Extension}}</td></tr>{{end}}
  </table>

  <h2 style="margin:28px 0 12px;font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#0891B2">{{.T.PortalTitle}}</h2>
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid rgba(15,23,42,0.12);border-radius:10px">
    {{- range $i, $l := .Portal}}<tr><td style="padding:10px 14px;font-size:14px;color:#526072;width:45%{{if $i}};border-top:1px solid rgba(15,23,42,0.08){{end}}">{{$l.Label}}</td><td style="padding:10px 14px;font-size:14px{{if $i}};border-top:1px solid rgba(15,23,42,0.08){{end}}"><a href="{{$l.URL}}" style="color:#0891B2;font-family:'JetBrains Mono',Consolas,monospace">{{$l.URL}}</a></td></tr>{{end}}
  </table>
  {{- if gt (len .Portal) 1}}<p style="margin:8px 0 0;font-size:13px;line-height:1.5;color:#64748B">{{.T.PortalHTTPSHint}}</p>{{end}}

  {{- if .SSO}}
  <h2 style="margin:28px 0 12px;font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#0891B2">{{.T.SSOTitle}}</h2>
  <p style="margin:0;font-size:14px;line-height:1.6;color:#526072">{{.T.SSOText}} <strong style="color:#0F172A">{{range $i, $n := .SSO}}{{if $i}}, {{end}}{{$n}}{{end}}</strong></p>
  {{- end}}

  {{- if .Extension}}
  <h2 style="margin:28px 0 12px;font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#0891B2">{{.T.PhoneTitle}}</h2>
  <p style="margin:0 0 12px;font-size:14px;line-height:1.6;color:#526072">{{.T.PhoneText}}</p>
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid rgba(15,23,42,0.12);border-radius:10px;font-size:14px">
    <tr><td style="padding:10px 14px;color:#526072;width:45%">{{.T.Server}}</td><td style="padding:10px 14px;font-family:'JetBrains Mono',Consolas,monospace;color:#0F172A">{{.SIPServer}}</td></tr>
    <tr><td style="padding:10px 14px;color:#526072;border-top:1px solid rgba(15,23,42,0.08)">{{.T.Port}}</td><td style="padding:10px 14px;font-family:'JetBrains Mono',Consolas,monospace;color:#0F172A;border-top:1px solid rgba(15,23,42,0.08)">{{.SIPPort}}</td></tr>
    <tr><td style="padding:10px 14px;color:#526072;border-top:1px solid rgba(15,23,42,0.08)">{{.T.Transport}}</td><td style="padding:10px 14px;font-family:'JetBrains Mono',Consolas,monospace;color:#0F172A;border-top:1px solid rgba(15,23,42,0.08)">{{.Transport}}</td></tr>
    <tr><td style="padding:10px 14px;color:#526072;border-top:1px solid rgba(15,23,42,0.08)">{{.T.SIPUser}}</td><td style="padding:10px 14px;font-family:'JetBrains Mono',Consolas,monospace;color:#0F172A;border-top:1px solid rgba(15,23,42,0.08)">{{.Extension}}</td></tr>
    <tr><td style="padding:10px 14px;color:#526072;border-top:1px solid rgba(15,23,42,0.08)">{{.T.SIPPassword}}</td><td style="padding:10px 14px;border-top:1px solid rgba(15,23,42,0.08)"><a href="{{.DevicesURL}}" style="color:#0891B2">{{.T.SIPPasswordValue}}</a></td></tr>
  </table>
  {{- if .WebRTC}}<p style="margin:12px 0 0;font-size:14px;line-height:1.6;color:#526072">{{.T.Softphone}}</p>{{end}}
  {{- if .Voicemail}}
  <h2 style="margin:28px 0 12px;font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#0891B2">{{.T.Voicemail}}</h2>
  <p style="margin:0;font-size:14px;line-height:1.6;color:#526072">{{.T.VoicemailText}}</p>
  {{- end}}
  {{- end}}
</td></tr>
<tr><td style="padding:20px 32px;border-top:1px solid rgba(15,23,42,0.08);font-size:12px;line-height:1.5;color:#64748B">
  {{.T.Footer}}
  <div style="margin-top:10px;text-align:right;color:#64748B">Powered by <strong style="color:#0F172A">RHIS</strong></div>
</td></tr>
</table>
</td></tr></table>
</body></html>
`))
