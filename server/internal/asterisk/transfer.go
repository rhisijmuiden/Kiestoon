package asterisk

import (
	"context"
	"strings"
)

// Call transfer over AMI. It works the same for a desk phone and for the browser softphone,
// because both are PJSIP channels on this Asterisk.

func (a *AMI) simple(ctx context.Context, m Message) error {
	r, err := a.Action(ctx, m)
	if err != nil {
		return err
	}
	if strings.EqualFold(r.Get("Response"), "Error") {
		return &ActionError{Msg: r.Get("Message")}
	}
	return nil
}

// BlindTransfer sends whoever is bridged with channel (the transferer) to exten@context. The
// transferer's leg ends.
func (a *AMI) BlindTransfer(ctx context.Context, channel, context_, exten string) error {
	return a.simple(ctx, Message{{"Action", "BlindTransfer"}, {"Channel", channel}, {"Context", context_}, {"Exten", exten}})
}

// AttendedTransfer starts a consultation call from channel to exten@context while the other
// party waits on hold. Hanging up the transferer completes the transfer; hanging up the
// consultation returns the transferer to the waiting party. It needs the atxfer feature code
// (features.conf) and transfer rights on the call (Dial option t or T).
func (a *AMI) AttendedTransfer(ctx context.Context, channel, context_, exten string) error {
	return a.simple(ctx, Message{{"Action", "Atxfer"}, {"Channel", channel}, {"Context", context_}, {"Exten", exten}})
}

// Redirect moves a channel, bridged or not, to exten@context. The switchboard uses it to move a
// call it is not part of.
func (a *AMI) Redirect(ctx context.Context, channel, context_, exten string) error {
	return a.simple(ctx, Message{{"Action", "Redirect"}, {"Channel", channel}, {"Context", context_}, {"Exten", exten}, {"Priority", "1"}})
}

// Hangup ends a channel.
func (a *AMI) Hangup(ctx context.Context, channel string) error {
	return a.simple(ctx, Message{{"Action", "Hangup"}, {"Channel", channel}, {"Cause", "16"}})
}

// LegsOf returns the device legs of the given endpoints (for example "201" and "201-web").
func (l *Live) LegsOf(endpoints ...string) []Channel {
	var out []Channel
	for _, c := range l.Calls {
		for _, leg := range c.Legs {
			if strings.HasPrefix(leg.Channel, "PJSIP/") && containsStr(endpoints, leg.Endpoint) {
				out = append(out, leg)
			}
		}
	}
	return out
}

// Channel returns the leg with this channel name.
func (l *Live) Channel(name string) (Channel, bool) {
	for _, c := range l.Calls {
		for _, leg := range c.Legs {
			if leg.Channel == name {
				return leg, true
			}
		}
	}
	return Channel{}, false
}

// BridgePeers returns the other channels in the same bridge as leg.
func (l *Live) BridgePeers(leg Channel) []Channel {
	if leg.BridgeID == "" {
		return nil
	}
	var out []Channel
	for _, c := range l.Calls {
		for _, x := range c.Legs {
			if x.BridgeID == leg.BridgeID && x.Channel != leg.Channel {
				out = append(out, x)
			}
		}
	}
	return out
}

// ConsultLegs returns the consultation call of an attended transfer to exten: the Local channel
// Asterisk dialled into context, bridged with the transferer or still ringing.
func (l *Live) ConsultLegs(transferer Channel, context_, exten string) []Channel {
	prefix := "Local/" + exten + "@" + context_ + "-"
	var out []Channel
	for _, p := range l.BridgePeers(transferer) {
		if strings.HasPrefix(p.Channel, prefix) {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, c := range l.Calls {
		for _, x := range c.Legs {
			if strings.HasPrefix(x.Channel, prefix) && strings.HasSuffix(x.Channel, ";1") {
				out = append(out, x)
			}
		}
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Reregister makes Asterisk send a fresh REGISTER for an outbound registration (the trunk's
// endpoint name), unregistering first when it is registered. It also restarts the retry timer
// of a registration that gave up after a rejection.
func (a *AMI) Reregister(ctx context.Context, registration string) error {
	return a.simple(ctx, Message{{"Action", "PJSIPRegister"}, {"Registration", registration}})
}

// RegistrationStatus returns the status of one outbound registration (Registered, Unregistered,
// Rejected, ...), or "" when Asterisk does not know it.
func (a *AMI) RegistrationStatus(ctx context.Context, registration string) (string, error) {
	evs, err := a.List(ctx, Message{{"Action", "PJSIPShowRegistrationsOutbound"}})
	if err != nil {
		return "", err
	}
	for _, e := range evs {
		if e.Get("Event") == "OutboundRegistrationDetail" && e.Get("ObjectName") == registration {
			return e.Get("Status"), nil
		}
	}
	return "", nil
}
