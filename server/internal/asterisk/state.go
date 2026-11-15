package asterisk

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ExtState is the BLF state of an extension: idle, inuse, busy, ringing, onhold, unavailable.
type ExtState struct {
	Exten  string `json:"exten"`
	Status string `json:"status"`
}

// ExtStatus turns the numeric state of an ExtensionStatus event into a BLF state.
func ExtStatus(code string) string { return extStatus(code) }

func extStatus(code string) string {
	n, _ := strconv.Atoi(code)
	switch {
	case n == -1 || n == -2:
		return "unavailable"
	case n == 0:
		return "idle"
	case n&8 != 0 && n&1 != 0:
		return "ringinuse"
	case n&8 != 0:
		return "ringing"
	case n&16 != 0:
		return "onhold"
	case n&2 != 0:
		return "busy"
	case n&1 != 0:
		return "inuse"
	case n&4 != 0:
		return "unavailable"
	}
	return "unknown"
}

// StateCache asks Asterisk for extension and queue states at most once a second.
type StateCache struct {
	AMI *AMI

	mu      sync.Mutex
	ext     map[string]string
	extAt   time.Time
	queues  []QueueState
	queueAt time.Time
}

// Extensions returns the BLF state per extension number.
func (c *StateCache) Extensions(ctx context.Context) map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ext != nil && time.Since(c.extAt) < time.Second {
		return c.ext
	}
	out := map[string]string{}
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	evs, err := c.AMI.List(cctx, Message{{"Action", "ExtensionStateList"}})
	if err == nil {
		for _, e := range evs {
			if e.Get("Event") == "ExtensionStatus" && e.Get("Context") == "kiestoon-extensions" {
				out[e.Get("Exten")] = extStatus(e.Get("Status"))
			}
		}
	}
	c.ext, c.extAt = out, time.Now()
	return out
}

// QueueMemberState is an agent in the live queue status.
type QueueMemberState struct {
	Number     string `json:"number"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Paused     bool   `json:"paused"`
	InCall     bool   `json:"inCall"`
	CallsTaken int    `json:"callsTaken"`
	LastCall   int64  `json:"lastCall"`
}

// QueueCaller is a caller waiting in a queue.
type QueueCaller struct {
	Position int    `json:"position"`
	CallerID string `json:"callerId"`
	Name     string `json:"name"`
	Wait     int    `json:"wait"`
}

// QueueState is the live status of a queue.
type QueueState struct {
	Name        string             `json:"name"` // kiestoon-q-<id>
	ID          int64              `json:"id"`
	Calls       int                `json:"calls"`
	Holdtime    int                `json:"holdtime"`
	TalkTime    int                `json:"talktime"`
	Completed   int                `json:"completed"`
	Abandoned   int                `json:"abandoned"`
	ServiceLvl  float64            `json:"serviceLevel"`
	Members     []QueueMemberState `json:"members"`
	Callers     []QueueCaller      `json:"callers"`
	LongestWait int                `json:"longestWait"`
}

var deviceStatus = map[string]string{"0": "unknown", "1": "idle", "2": "inuse", "3": "busy", "4": "invalid", "5": "unavailable",
	"6": "ringing", "7": "ringinuse", "8": "onhold"}

// Queues returns the live state of every Kiestoon queue.
func (c *StateCache) Queues(ctx context.Context) []QueueState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.queues != nil && time.Since(c.queueAt) < time.Second {
		return c.queues
	}
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out := []QueueState{}
	evs, err := c.AMI.List(cctx, Message{{"Action", "QueueStatus"}})
	if err == nil {
		byName := map[string]*QueueState{}
		order := []string{}
		get := func(name string) *QueueState {
			if q := byName[name]; q != nil {
				return q
			}
			id, _ := strconv.ParseInt(strings.TrimPrefix(name, "kiestoon-q-"), 10, 64)
			q := &QueueState{Name: name, ID: id, Members: []QueueMemberState{}, Callers: []QueueCaller{}}
			byName[name] = q
			order = append(order, name)
			return q
		}
		atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
		for _, e := range evs {
			name := e.Get("Queue")
			if !strings.HasPrefix(name, "kiestoon-q-") {
				continue
			}
			q := get(name)
			switch e.Get("Event") {
			case "QueueParams":
				q.Calls, q.Holdtime, q.TalkTime = atoi(e.Get("Calls")), atoi(e.Get("Holdtime")), atoi(e.Get("TalkTime"))
				q.Completed, q.Abandoned = atoi(e.Get("Completed")), atoi(e.Get("Abandoned"))
				q.ServiceLvl, _ = strconv.ParseFloat(e.Get("ServicelevelPerf"), 64)
			case "QueueMember":
				loc := e.Get("Location") // Local/201@kiestoon-ring/n
				num := strings.TrimPrefix(loc, "Local/")
				if i := strings.Index(num, "@"); i > 0 {
					num = num[:i]
				}
				last, _ := strconv.ParseInt(e.Get("LastCall"), 10, 64)
				q.Members = append(q.Members, QueueMemberState{Number: num, Name: e.Get("Name"), Status: deviceStatus[e.Get("Status")],
					Paused: e.Get("Paused") == "1", InCall: e.Get("InCall") == "1", CallsTaken: atoi(e.Get("CallsTaken")), LastCall: last})
			case "QueueEntry":
				w := atoi(e.Get("Wait"))
				q.Callers = append(q.Callers, QueueCaller{Position: atoi(e.Get("Position")), CallerID: e.Get("CallerIDNum"),
					Name: e.Get("CallerIDName"), Wait: w})
				if w > q.LongestWait {
					q.LongestWait = w
				}
			}
		}
		for _, n := range order {
			out = append(out, *byName[n])
		}
	}
	c.queues, c.queueAt = out, time.Now()
	return out
}

// Invalidate makes the next call ask Asterisk again (after a sign-in or pause).
func (c *StateCache) Invalidate() {
	c.mu.Lock()
	c.ext, c.queues = nil, nil
	c.mu.Unlock()
}

// Originate rings the caller's own phone(s) first and then calls number (click-to-call).
func (a *AMI) Originate(ctx context.Context, fromExt, callerName, number string) error {
	r, err := a.Action(ctx, Message{
		{"Action", "Originate"}, {"Channel", "Local/" + fromExt + "@kiestoon-ring/n"},
		{"Context", "kiestoon-click"}, {"Exten", number}, {"Priority", "1"},
		{"CallerID", `"` + strings.ReplaceAll(callerName, `"`, "") + `" <` + fromExt + `>`},
		{"Variable", "KT_CLICKFROM=" + fromExt}, {"Timeout", "30000"}, {"Async", "true"},
	})
	if err != nil {
		return err
	}
	if strings.EqualFold(r.Get("Response"), "Error") {
		return &ActionError{Msg: r.Get("Message")}
	}
	return nil
}

// ActionError is an AMI action that Asterisk refused.
type ActionError struct{ Msg string }

func (e *ActionError) Error() string { return "asterisk: " + e.Msg }

// QueuePause pauses or unpauses an agent in a queue.
func (a *AMI) QueuePause(ctx context.Context, queue, ext string, paused bool) error {
	p := "false"
	if paused {
		p = "true"
	}
	_, err := a.Action(ctx, Message{{"Action", "QueuePause"}, {"Queue", queue}, {"Interface", "Local/" + ext + "@kiestoon-ring/n"},
		{"Paused", p}, {"Reason", "Kiestoon"}})
	return err
}

// VoicemailRefresh makes Asterisk recount the messages of a mailbox (message waiting light).
func (a *AMI) VoicemailRefresh(ctx context.Context, mailbox string) {
	_, _ = a.Action(ctx, Message{{"Action", "VoicemailRefresh"}, {"Context", "default"}, {"Mailbox", mailbox}})
}

// SetDeviceState sets a custom device state (for example the BLF of the day/night switch).
func (a *AMI) SetDeviceState(ctx context.Context, device, state string) {
	_, _ = a.Action(ctx, Message{{"Action", "Setvar"}, {"Variable", "DEVICE_STATE(" + device + ")"}, {"Value", state}})
}

// OriginateApp calls channel and, when it answers, runs context/exten in the dialplan. vars are
// NAME=value channel variables. Async: the result arrives as an OriginateResponse event.
func (a *AMI) OriginateApp(ctx context.Context, channel, context_, exten, callerID string, timeout time.Duration, vars ...string) error {
	m := Message{
		{"Action", "Originate"}, {"Channel", channel}, {"Context", context_}, {"Exten", exten}, {"Priority", "1"},
		{"CallerID", callerID}, {"Timeout", strconv.Itoa(int(timeout.Milliseconds()))}, {"Async", "true"},
	}
	for _, v := range vars {
		m = append(m, KV{"Variable", v})
	}
	r, err := a.Action(ctx, m)
	if err != nil {
		return err
	}
	if strings.EqualFold(r.Get("Response"), "Error") {
		return &ActionError{Msg: r.Get("Message")}
	}
	return nil
}
