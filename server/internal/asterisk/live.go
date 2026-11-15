package asterisk

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Channel is one active call leg.
type Channel struct {
	Channel   string `json:"channel"`
	Endpoint  string `json:"endpoint"`
	CallerNum string `json:"callerNum"`
	CallerNam string `json:"callerName"`
	Connected string `json:"connected"`
	Exten     string `json:"exten"`
	State     string `json:"state"`
	App       string `json:"app"`
	Duration  int    `json:"duration"`
	LinkedID  string `json:"-"`
}

// Call groups the legs of one call.
type Call struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	FromName  string    `json:"fromName"`
	To        string    `json:"to"`
	Direction string    `json:"direction"`
	State     string    `json:"state"`
	Duration  int       `json:"duration"`
	Legs      []Channel `json:"legs"`
}

// Contact is a registered phone.
type Contact struct {
	Endpoint  string  `json:"endpoint"`
	URI       string  `json:"uri"`
	Status    string  `json:"status"`
	RTTMs     float64 `json:"rttMs"`
	UserAgent string  `json:"userAgent"`
}

// Registration is the state of a trunk.
type Registration struct {
	Endpoint string `json:"endpoint"`
	Status   string `json:"status"`
}

// Live is what the dashboard shows.
type Live struct {
	Connected     bool           `json:"connected"`
	Calls         []Call         `json:"calls"`
	Contacts      []Contact      `json:"contacts"`
	Registrations []Registration `json:"registrations"`
	At            time.Time      `json:"at"`
}

// LiveCache asks Asterisk for the live state at most once a second.
type LiveCache struct {
	AMI  *AMI
	mu   sync.Mutex
	last *Live
}

func endpointOf(channel string) string {
	// PJSIP/201-0000001a -> 201
	s := strings.TrimPrefix(channel, "PJSIP/")
	if i := strings.LastIndex(s, "-"); i > 0 {
		s = s[:i]
	}
	return s
}

// Get returns the live state.
func (c *LiveCache) Get(ctx context.Context) *Live {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last != nil && time.Since(c.last.At) < time.Second {
		return c.last
	}
	l := &Live{Calls: []Call{}, Contacts: []Contact{}, Registrations: []Registration{}, At: time.Now()}
	l.Connected = c.AMI.Connected()
	if !l.Connected {
		c.last = l
		return l
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	if evs, err := c.AMI.List(ctx, Message{{"Action", "CoreShowChannels"}}); err == nil {
		byID := map[string]*Call{}
		order := []string{}
		for _, e := range evs {
			if e.Get("Event") != "CoreShowChannel" {
				continue
			}
			d := parseDuration(e.Get("Duration"))
			ch := Channel{
				Channel: e.Get("Channel"), Endpoint: endpointOf(e.Get("Channel")), CallerNum: e.Get("CallerIDNum"),
				CallerNam: e.Get("CallerIDName"), Connected: e.Get("ConnectedLineNum"), Exten: e.Get("Exten"),
				State: e.Get("ChannelStateDesc"), App: e.Get("Application"), Duration: d, LinkedID: e.Get("Linkedid"),
			}
			id := ch.LinkedID
			if id == "" {
				id = e.Get("Uniqueid")
			}
			call := byID[id]
			if call == nil {
				call = &Call{ID: id}
				byID[id] = call
				order = append(order, id)
			}
			call.Legs = append(call.Legs, ch)
		}
		for _, id := range order {
			call := byID[id]
			// The leg that started the call is the one with the longest duration.
			sort.SliceStable(call.Legs, func(i, j int) bool { return call.Legs[i].Duration > call.Legs[j].Duration })
			first := call.Legs[0]
			call.From, call.FromName, call.Duration = first.CallerNum, first.CallerNam, first.Duration
			call.To = first.Exten
			if first.Connected != "" && first.Connected != "<unknown>" {
				call.To = first.Connected
			}
			call.State = "ringing"
			for _, leg := range call.Legs {
				if leg.State == "Up" && len(call.Legs) > 1 {
					call.State = "talking"
				}
			}
			if len(call.Legs) == 1 && first.State == "Up" {
				call.State = first.App
			}
			call.Direction = "internal"
			for i, leg := range call.Legs {
				if strings.HasPrefix(leg.Endpoint, "trunk-") {
					if i == 0 {
						call.Direction = "inbound"
					} else {
						call.Direction = "outbound"
					}
				}
			}
			l.Calls = append(l.Calls, *call)
		}
	}
	if evs, err := c.AMI.List(ctx, Message{{"Action", "PJSIPShowContacts"}}); err == nil {
		for _, e := range evs {
			if e.Get("Event") != "ContactList" {
				continue
			}
			rtt, _ := strconv.ParseFloat(e.Get("RoundtripUsec"), 64)
			l.Contacts = append(l.Contacts, Contact{
				Endpoint: e.Get("Endpoint"), URI: e.Get("Uri"), Status: e.Get("Status"), RTTMs: rtt / 1000, UserAgent: e.Get("UserAgent"),
			})
		}
	}
	if evs, err := c.AMI.List(ctx, Message{{"Action", "PJSIPShowRegistrationsOutbound"}}); err == nil {
		for _, e := range evs {
			if e.Get("Event") != "OutboundRegistrationDetail" {
				continue
			}
			l.Registrations = append(l.Registrations, Registration{Endpoint: e.Get("ObjectName"), Status: e.Get("Status")})
		}
	}
	c.last = l
	return l
}

func parseDuration(s string) int {
	// "00:01:23" or seconds
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	parts := strings.Split(s, ":")
	total := 0
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		total = total*60 + n
	}
	return total
}
