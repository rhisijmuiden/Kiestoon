package pbx

import (
	"context"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Snapshot is everything the Asterisk configuration is generated from.
type Snapshot struct {
	Settings       Settings
	Features       FeatureSettings
	Holidays       Holidays
	Extensions     []Extension
	Trunks         []Trunk
	Inbound        []InboundRoute
	Outbound       []OutboundRoute
	ExtGroups      []ExtensionGroup
	RingGroups     []RingGroup
	Queues         []Queue
	IVRs           []IVR
	TimeConditions []TimeCondition
	Conferences    []Conference
	Prompts        []Prompt
	FaxBoxes       []FaxBox
	Contacts       []Contact
	Phonebook      PhonebookSettings
	// Multi-site (phase 5)
	Hub       HubSettings
	Peers     []FedPeer
	SunRoutes []SunRoute
	// Teams is Microsoft Teams Direct Routing.
	Teams TeamsSettings
	// Supervisors are the extension numbers of enabled users with the role reception or higher:
	// they may listen, whisper and barge on calls of queues with supervision on.
	Supervisors []string
	// FraudLocked blocks international and premium calls after a fraud alert.
	FraudLocked bool
	// CRMDeskName asks the CRM for the caller's name on calls from trunks (desk phone display).
	CRMDeskName bool
	// OnCall is who every on-call team rings right now (regenerated when that changes).
	OnCall []OnCallLive
	// ForwardTargets are the numbers of the forward keys on phones (*74<ext>*<number> toggles).
	ForwardTargets []string
	// Now decides which year's holidays go into the dialplan (it is regenerated every night).
	Now time.Time
}

// Snapshot reads the current phone system data.
func (r *Repo) Snapshot(ctx context.Context) (*Snapshot, error) {
	var s Snapshot
	var err error
	if s.Settings, err = LoadSettings(ctx, r.DB); err != nil {
		return nil, err
	}
	if s.Extensions, err = r.Extensions(ctx); err != nil {
		return nil, err
	}
	if s.Trunks, err = r.Trunks(ctx); err != nil {
		return nil, err
	}
	if s.Inbound, err = r.InboundRoutes(ctx); err != nil {
		return nil, err
	}
	if s.Outbound, err = r.OutboundRoutes(ctx); err != nil {
		return nil, err
	}
	if s.ExtGroups, err = r.ExtensionGroups(ctx); err != nil {
		return nil, err
	}
	if s.Features, err = LoadFeatureSettings(ctx, r.DB); err != nil {
		return nil, err
	}
	if s.Holidays, err = LoadHolidays(ctx, r.DB); err != nil {
		return nil, err
	}
	if s.FaxBoxes, err = r.FaxBoxes(ctx); err != nil {
		return nil, err
	}
	if s.Contacts, err = r.Contacts(ctx); err != nil {
		return nil, err
	}
	if s.Phonebook, err = LoadPhonebookSettings(ctx, r.DB); err != nil {
		return nil, err
	}
	var deskName string
	_ = r.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'crm_desk_name'`).Scan(&deskName)
	s.CRMDeskName = deskName == "1"

	phones, err := r.Phones(ctx)
	if err != nil {
		return nil, err
	}
	s.ForwardTargets = ForwardTargets(phones)

	if s.RingGroups, err = r.RingGroups(ctx); err != nil {
		return nil, err
	}
	if s.Queues, err = r.Queues(ctx); err != nil {
		return nil, err
	}
	if s.IVRs, err = r.IVRs(ctx); err != nil {
		return nil, err
	}
	if s.TimeConditions, err = r.TimeConditions(ctx); err != nil {
		return nil, err
	}
	if s.Conferences, err = r.Conferences(ctx); err != nil {
		return nil, err
	}
	if s.Prompts, err = r.Prompts(ctx); err != nil {
		return nil, err
	}
	if s.Hub, err = LoadHub(ctx, r.DB); err != nil {
		return nil, err
	}
	if s.Peers, err = r.FedPeers(ctx); err != nil {
		return nil, err
	}
	if s.SunRoutes, err = r.SunRoutes(ctx); err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT DISTINCT e.number FROM users u JOIN extensions e ON e.id = u.extension_id
		WHERE u.enabled = 1 AND u.role IN ('reception','admin','superadmin') ORDER BY e.number`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		s.Supervisors = append(s.Supervisors, n)
	}
	rows.Close()
	if err := r.markMobileApps(ctx, &s); err != nil {
		return nil, err
	}
	if s.Teams, err = LoadTeams(ctx, r.DB); err != nil {
		return nil, err
	}
	markTeams(&s)
	locked, _ := store.GetSetting(ctx, r.DB, FraudLockKey, "")
	s.FraudLocked = locked != ""
	if loc, err := time.LoadLocation(s.Settings.Timezone); err == nil {
		s.Now = time.Now().In(loc)
	} else {
		s.Now = time.Now()
	}
	if s.OnCall, err = r.OnCallLive(ctx, s.Now); err != nil {
		return nil, err
	}
	return &s, nil
}

// TrunkPreset pre-fills the trunk form for a provider. The administrator still enters the
// account details the provider gave them.
type TrunkPreset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AuthMode  string `json:"authMode"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
	DIDSource string `json:"didSource"`
	Codecs    string `json:"codecs"`
	// MatchIPs are the networks the provider sends calls from (comma separated).
	MatchIPs string `json:"matchIps"`
	Note     string `json:"note"` // translation key with provider specific hints
	// Guide is the translation key of a step-by-step setup guide, "" when there is none.
	Guide string `json:"guide"`
	// DocsURL points to the provider page in the Kiestoon documentation.
	DocsURL string `json:"docsUrl,omitempty"`
}

// TrunkPresets lists the provider templates offered in the UI.
var TrunkPresets = []TrunkPreset{
	{ID: "generic", Name: "Generic (registration)", AuthMode: "register", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw,ulaw,g722", Note: "generic"},
	{ID: "generic-ip", Name: "Generic (IP authentication)", AuthMode: "ip", Port: 5060, Transport: "udp", DIDSource: "ruri", Codecs: "alaw,ulaw", Note: "ip"},
	// Voys: ha.voys.nl has DNS SRV records (UDP 6060/5060 on the VoIPGRID SIP proxies), so the port
	// is left at 0 to follow them. Calls come from 195.35.114.0/23 and 185.103.76.0/22.
	// Source: https://help.voys.nl/voip-trunk-configuratie
	{ID: "voys", Name: "Voys", AuthMode: "register", Host: "ha.voys.nl", Port: 0, Transport: "udp", DIDSource: "ruri", Codecs: "alaw,g722",
		MatchIPs: VoysNetworks, Note: "voys", Guide: "voys", DocsURL: "https://rhisijmuiden.github.io/Kiestoon/providers/voys/"},
	{ID: "voipgrid", Name: "VoIPGRID platform (Spitsgroep, partners)", AuthMode: "register", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw,g722", Note: "voipgrid"},
	// KPN (KPN EEN / VoIP Connect): IP trunk to the KPN SBC, TCP 5060 preferred, G.711 A-law only to the
	// PSTN, E.164 numbers. Standard SBC 213.162.171.164, second SBC 213.162.171.180 with high availability.
	// KPN can hand out other addresses per order. Source: KPN EEN dienst Vast (VoIP Connect) configuration
	// guidelines (innovaphone, 2017).
	{ID: "kpn", Name: "KPN SIP trunk (KPN EEN / VoIP Connect)", AuthMode: "ip", Host: KPNSBC, Port: 5060, Transport: "tcp", DIDSource: "ruri", Codecs: "alaw",
		MatchIPs: KPNNetworks, Note: "kpn", Guide: "kpn", DocsURL: "https://rhisijmuiden.github.io/Kiestoon/providers/kpn/"},
	// Cheapconnect: registration on sip.cheapconnect.net:5060 UDP, username = the number in 31... format.
	// Source: https://www.cheapconnect.nl/en/business-voip/sip-trunk-setup/
	{ID: "cheapconnect", Name: "Cheapconnect", AuthMode: "register", Host: "sip.cheapconnect.net", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw,ulaw,g722",
		Note: "cheapconnect", Guide: "cheapconnect", DocsURL: "https://rhisijmuiden.github.io/Kiestoon/providers/cheapconnect/"},
	// Speakup: registration on sip.speakup.nl, username = the number in 31... format, G.711 and RFC 2833.
	// Sources: Swyx KB "SIP Provider SpeakUp (NL)", Tuxis "Speakup trunk instellen Kerio Operator".
	{ID: "speakup", Name: "Speakup", AuthMode: "register", Host: "sip.speakup.nl", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw,ulaw",
		Note: "speakup", Guide: "speakup", DocsURL: "https://rhisijmuiden.github.io/Kiestoon/providers/speakup/"},
	// Vodafone Ziggo Zakelijk: registration on trunk.sipnl.net:5060, username and password supplied by Ziggo
	// (31... format, without @trunk.sipnl.net), T.38 for fax. Source: Panasonic KX-NS setup guide "Ziggo SIP Trunk".
	{ID: "ziggo", Name: "Vodafone Ziggo Zakelijk", AuthMode: "register", Host: "trunk.sipnl.net", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw,ulaw",
		Note: "ziggo", Guide: "ziggo", DocsURL: "https://rhisijmuiden.github.io/Kiestoon/providers/ziggo/"},
	{ID: "telnyx", Name: "Telnyx", AuthMode: "register", Host: "sip.telnyx.com", Port: 5061, Transport: "tls", DIDSource: "to", Codecs: "alaw,ulaw,g722", Note: "telnyx"},
	{ID: "twilio", Name: "Twilio Elastic SIP", AuthMode: "ip", Port: 5060, Transport: "udp", DIDSource: "ruri", Codecs: "ulaw,alaw", Note: "twilio"},
}

// VoysNetworks are the address ranges Voys (VoIPGRID) sends SIP and media from.
const VoysNetworks = "195.35.114.0/23,185.103.76.0/22"

// KPNSBC is the standard KPN SBC for SIP trunks; KPNNetworks adds the second SBC used with high availability.
const (
	KPNSBC      = "213.162.171.164"
	KPNNetworks = "213.162.171.164,213.162.171.180"
)

// FraudLockKey is set (to the time of the alert) while international calls are blocked.
const FraudLockKey = "fraud_locked"
