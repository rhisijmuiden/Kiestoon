package pbx

import "context"

// Snapshot is everything the Asterisk configuration is generated from.
type Snapshot struct {
	Settings   Settings
	Extensions []Extension
	Trunks     []Trunk
	Inbound    []InboundRoute
	Outbound   []OutboundRoute
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
