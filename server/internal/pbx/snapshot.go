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
	// FraudLocked blocks international and premium calls after a fraud alert.
	FraudLocked bool
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
	locked, _ := store.GetSetting(ctx, r.DB, FraudLockKey, "")
	s.FraudLocked = locked != ""
	if loc, err := time.LoadLocation(s.Settings.Timezone); err == nil {
		s.Now = time.Now().In(loc)
	} else {
		s.Now = time.Now()
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
	Note      string `json:"note"` // translation key with provider specific hints
}

// TrunkPresets lists the provider templates offered in the UI.
var TrunkPresets = []TrunkPreset{
	{ID: "generic", Name: "Generic (registration)", AuthMode: "register", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw,ulaw,g722", Note: "generic"},
	{ID: "generic-ip", Name: "Generic (IP authentication)", AuthMode: "ip", Port: 5060, Transport: "udp", DIDSource: "ruri", Codecs: "alaw,ulaw", Note: "ip"},
	{ID: "voipgrid", Name: "VoIPGRID platform (Voys, Spitsgroep, partners)", AuthMode: "register", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw,g722", Note: "voipgrid"},
	{ID: "kpn", Name: "KPN SIP trunk", AuthMode: "ip", Port: 5060, Transport: "udp", DIDSource: "ruri", Codecs: "alaw", Note: "kpn"},
	{ID: "telnyx", Name: "Telnyx", AuthMode: "register", Host: "sip.telnyx.com", Port: 5061, Transport: "tls", DIDSource: "to", Codecs: "alaw,ulaw,g722", Note: "telnyx"},
	{ID: "twilio", Name: "Twilio Elastic SIP", AuthMode: "ip", Port: 5060, Transport: "udp", DIDSource: "ruri", Codecs: "ulaw,alaw", Note: "twilio"},
}

// FraudLockKey is set (to the time of the alert) while international calls are blocked.
const FraudLockKey = "fraud_locked"
