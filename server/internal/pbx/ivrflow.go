package pbx

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
)

// An IVR is drawn as a flow on a canvas: nodes connected by lines. The flow is the source of the
// dialplan; Options, PromptID and TimeoutDest are derived from it on save, so lists, references and
// older API clients keep working.
//
// Node types:
//   - entry:       where the call comes in. Plays its prompt, then the prompts of the connected keys
//     in key order, and waits for a key. Exactly one.
//   - key:         a key the caller can press (Digit). Its prompt is part of the menu ("press 1 for sales").
//     It is a menu option when a line runs from the entry to it. Port "out".
//   - timeout:     what happens when the caller presses nothing or a wrong key too often. Its prompt
//     plays before following the line. At most one. Port "out".
//   - audio:       plays a prompt, then follows its line. Port "out".
//   - destination: sends the call to an extension, group, queue, voicemail, other menu, ... End point.
//   - time:        checks a time condition (with its day/night switch). Ports "open" and "closed".
//   - repeat:      plays the menu again. End point.
//   - menu:        a further choice menu (for example after a time check: one menu when open, another
//     when closed). Plays its prompt and the prompts of its keys and waits for a key, like the entry.
//     Lines run from it to its keys and its timeout.
//   - speech:      asks a question ("say what you are calling about"), records the answer, has it
//     transcribed by the local Whisper and follows the line of the first matching intent (ports
//     "i1".."i8", by keywords, optionally helped by AI) or "nomatch".
//
// The entry either is a menu itself (lines to keys and a timeout) or has one line to a start step
// (a time check, audio, a menu, a speech step or a destination) that runs right after the welcome.
// A port of a key, timeout, audio, time or speech step normally has one line. It may have several
// when they all lead to an extension, ring group, queue or external number: those ring at the same
// time and the first to answer gets the call.
const (
	NodeEntry       = "entry"
	NodeKey         = "key"
	NodeTimeout     = "timeout"
	NodeAudio       = "audio"
	NodeDestination = "destination"
	NodeTime        = "time"
	NodeRepeat      = "repeat"
	NodeSpeech      = "speech"
	NodeMenu        = "menu"
)

// MaxParallel limits the destinations one port can ring at the same time.
const MaxParallel = 5

// Parallel reports whether a destination can ring together with others from one port.
func Parallel(d *Destination) bool {
	if d == nil {
		return false
	}
	switch d.Type {
	case DestExtension, DestRingGroup, DestQueue, DestExternal:
		return true
	}
	return false
}

// MaxIntents limits the intents of one speech step.
const MaxIntents = 8

// SpeechIntent is one thing a caller can say at a speech step.
type SpeechIntent struct {
	Label    string   `json:"label"`
	Keywords []string `json:"keywords"`
}

// SpeechPort is the port of intent i (from 1).
func SpeechPort(i int) string { return "i" + strconv.Itoa(i) }

// MaxFlowNodes limits the size of one menu.
const MaxFlowNodes = 200

// FlowNode is one step on the canvas.
type FlowNode struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Digit    string  `json:"digit,omitempty"`
	Label    string  `json:"label,omitempty"`
	PromptID int64   `json:"promptId,omitempty"`
	// Destination of a destination node; for a time node the time condition (type timecondition).
	Destination *Destination `json:"destination,omitempty"`
	// Intents of a speech step; AI lets the AI engine choose when no keyword matches.
	Intents []SpeechIntent `json:"intents,omitempty"`
	AI      bool           `json:"ai,omitempty"`
}

// FlowEdge is a line from a node's port to another node.
type FlowEdge struct {
	From string `json:"from"`
	Port string `json:"port"` // out | open | closed
	To   string `json:"to"`
}

// Flow is the graph of a voice menu.
type Flow struct {
	Nodes []FlowNode `json:"nodes"`
	Edges []FlowEdge `json:"edges"`
	// Language callers of this IVR hear: en or nl, or empty for the caller language of the
	// phone system.
	Language string `json:"language,omitempty"`
}

// KeyOrder is the order in which key prompts are played and keys are listed.
var KeyOrder = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "*", "#"}

func keyRank(d string) int { return slices.Index(KeyOrder, d) }

var reNodeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

// Node returns the node with the id, or nil.
func (f *Flow) Node(id string) *FlowNode {
	for i := range f.Nodes {
		if f.Nodes[i].ID == id {
			return &f.Nodes[i]
		}
	}
	return nil
}

// Entry returns the entry node.
func (f *Flow) Entry() *FlowNode {
	for i := range f.Nodes {
		if f.Nodes[i].Type == NodeEntry {
			return &f.Nodes[i]
		}
	}
	return nil
}

// Next returns the node a port leads to, or nil.
func (f *Flow) Next(from, port string) *FlowNode {
	for _, e := range f.Edges {
		if e.From == from && e.Port == port {
			return f.Node(e.To)
		}
	}
	return nil
}

// NextAll returns every node a port leads to (several when they ring at the same time).
func (f *Flow) NextAll(from, port string) []*FlowNode {
	var out []*FlowNode
	for _, e := range f.Edges {
		if e.From == from && e.Port == port {
			if n := f.Node(e.To); n != nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// isMenu reports whether keys and a timeout hang off a node (the entry or a menu step).
func isMenu(n *FlowNode) bool { return n != nil && (n.Type == NodeEntry || n.Type == NodeMenu) }

// StartStep returns the step the entry runs right after the welcome instead of a menu, or nil.
func (f *Flow) StartStep() *FlowNode {
	entry := f.Entry()
	if entry == nil {
		return nil
	}
	for _, e := range f.Edges {
		if n := f.Node(e.To); e.From == entry.ID && n != nil && n.Type != NodeKey && n.Type != NodeTimeout {
			return n
		}
	}
	return nil
}

// KeysOf returns the key nodes connected to a menu (the entry or a menu step), in key order.
func (f *Flow) KeysOf(menu string) []*FlowNode {
	var keys []*FlowNode
	for _, e := range f.Edges {
		if e.From != menu {
			continue
		}
		if n := f.Node(e.To); n != nil && n.Type == NodeKey {
			keys = append(keys, n)
		}
	}
	slices.SortFunc(keys, func(a, b *FlowNode) int { return keyRank(a.Digit) - keyRank(b.Digit) })
	return keys
}

// TimeoutOf returns the timeout node connected to a menu, or nil.
func (f *Flow) TimeoutOf(menu string) *FlowNode {
	for _, e := range f.Edges {
		if n := f.Node(e.To); e.From == menu && n != nil && n.Type == NodeTimeout {
			return n
		}
	}
	return nil
}

// MenuKeys returns the key nodes connected to the entry, in key order (none when the entry has a
// start step).
func (f *Flow) MenuKeys() []*FlowNode {
	entry := f.Entry()
	if entry == nil || f.StartStep() != nil {
		return nil
	}
	return f.KeysOf(entry.ID)
}

// TimeoutNode returns the timeout node connected to the entry, or nil.
func (f *Flow) TimeoutNode() *FlowNode {
	entry := f.Entry()
	if entry == nil || f.StartStep() != nil {
		return nil
	}
	return f.TimeoutOf(entry.ID)
}

// Steps are the nodes a call can reach after a key or the timeout (audio, destination, time,
// repeat), numbered from 1 in node order. The dialplan uses the number as the extension.
func (f *Flow) Steps() map[string]int {
	m := map[string]int{}
	for _, n := range f.Nodes {
		switch n.Type {
		case NodeAudio, NodeDestination, NodeTime, NodeRepeat, NodeSpeech, NodeMenu:
			m[n.ID] = len(m) + 1
		}
	}
	return m
}

// FirstDestination follows the lines from a node to the first destination, for lists and
// the derived Options. Hangup when there is none.
func (f *Flow) FirstDestination(from *FlowNode) Destination {
	seen := map[string]bool{}
	n := from
	for n != nil && !seen[n.ID] {
		seen[n.ID] = true
		switch n.Type {
		case NodeDestination:
			if n.Destination != nil {
				return *n.Destination
			}
			return Destination{Type: DestHangup}
		case NodeTime:
			if n.Destination != nil {
				return *n.Destination
			}
			return Destination{Type: DestHangup}
		case NodeRepeat, NodeMenu:
			return Destination{Type: DestHangup}
		case NodeSpeech:
			n = f.Next(n.ID, "nomatch")
			continue
		}
		n = f.Next(n.ID, "out")
	}
	return Destination{Type: DestHangup}
}

// PromptIDs returns every prompt the flow plays.
func (f *Flow) PromptIDs() []int64 {
	var ids []int64
	for _, n := range f.Nodes {
		if n.PromptID != 0 && !slices.Contains(ids, n.PromptID) {
			ids = append(ids, n.PromptID)
		}
	}
	return ids
}

// LegacyFlow builds the flow of a menu saved before the canvas existed (or by an API client that
// only sends options): entry, one key per option with its destination, and the timeout.
func LegacyFlow(v *IVR, labels map[string]string) *Flow {
	f := &Flow{Nodes: []FlowNode{}, Edges: []FlowEdge{}}
	f.Nodes = append(f.Nodes, FlowNode{ID: "entry", Type: NodeEntry, X: 0, Y: 0, PromptID: v.PromptID})
	opts := slices.Clone(v.Options)
	slices.SortFunc(opts, func(a, b IVROption) int { return keyRank(a.Digit) - keyRank(b.Digit) })
	rows := len(opts) + 1
	y := func(i int) float64 { return float64(i)*120 - float64(rows-1)*60 }
	for i, o := range opts {
		key := "k" + strconv.Itoa(i+1)
		dest := "d" + strconv.Itoa(i+1)
		d := o.Destination
		f.Nodes = append(f.Nodes,
			FlowNode{ID: key, Type: NodeKey, X: 300, Y: y(i), Digit: o.Digit, Label: labels[o.Digit]},
			FlowNode{ID: dest, Type: NodeDestination, X: 600, Y: y(i), Destination: &d})
		f.Edges = append(f.Edges, FlowEdge{From: "entry", Port: "out", To: key}, FlowEdge{From: key, Port: "out", To: dest})
	}
	td := v.TimeoutDest
	if td.Type == "" {
		td = Destination{Type: DestHangup}
	}
	f.Nodes = append(f.Nodes,
		FlowNode{ID: "timeout", Type: NodeTimeout, X: 300, Y: y(len(opts))},
		FlowNode{ID: "dt", Type: NodeDestination, X: 600, Y: y(len(opts)), Destination: &td})
	f.Edges = append(f.Edges, FlowEdge{From: "entry", Port: "out", To: "timeout"}, FlowEdge{From: "timeout", Port: "out", To: "dt"})
	return f
}

// ivrLayout is what the layout column holds.
type ivrLayout struct {
	Flow   *Flow             `json:"flow,omitempty"`
	Labels map[string]string `json:"labels,omitempty"` // before the canvas
}

// validateFlow checks the flow and fills the fields derived from it.
func (r *Repo) validateFlow(ctx context.Context, v *IVR) error {
	f := v.Flow
	if f.Nodes == nil {
		f.Nodes = []FlowNode{}
	}
	if f.Edges == nil {
		f.Edges = []FlowEdge{}
	}
	if len(f.Nodes) > MaxFlowNodes || len(f.Edges) > MaxFlowNodes*3 {
		return invalid("too_large", "flow")
	}
	if !ValidLanguageOverride(f.Language) {
		return invalid("invalid", "flow.language")
	}
	ids := map[string]bool{}
	var entries int
	prompts := map[int64]bool{}
	for i := range f.Nodes {
		n := &f.Nodes[i]
		if !reNodeID.MatchString(n.ID) || ids[n.ID] {
			return invalid("invalid", "flow")
		}
		ids[n.ID] = true
		n.Label = cleanText(n.Label, 40)
		if n.X != n.X || n.Y != n.Y || n.X > 1e6 || n.X < -1e6 || n.Y > 1e6 || n.Y < -1e6 {
			return invalid("invalid", "flow")
		}
		if n.Type != NodeDestination && n.Type != NodeTime {
			n.Destination = nil
		}
		if n.Type != NodeKey {
			n.Digit = ""
		}
		if n.Type != NodeSpeech {
			n.Intents, n.AI = nil, false
		}
		switch n.Type {
		case NodeEntry:
			entries++
		case NodeTimeout, NodeMenu:
		case NodeKey:
			if !reIVRDigit.MatchString(n.Digit) {
				return invalid("invalid_digit", "flow")
			}
		case NodeAudio, NodeRepeat:
		case NodeSpeech:
			if len(n.Intents) > MaxIntents {
				return invalid("too_large", "flow")
			}
			for j := range n.Intents {
				in := &n.Intents[j]
				in.Label = cleanText(in.Label, 40)
				kws := []string{}
				for _, k := range in.Keywords {
					if k = cleanText(k, 40); k != "" && len(kws) < 30 && !slices.Contains(kws, k) {
						kws = append(kws, k)
					}
				}
				in.Keywords = kws
				if in.Label == "" {
					return invalid("invalid", "flow")
				}
			}
		case NodeDestination:
			if n.Destination == nil {
				n.Destination = &Destination{Type: DestHangup}
			}
			if err := r.ValidateDestination(ctx, n.Destination, "flow"); err != nil {
				return err
			}
		case NodeTime:
			if n.Destination == nil || n.Destination.Type != DestTimeCondition {
				return invalid("unknown_destination", "flow")
			}
			if err := r.ValidateDestination(ctx, n.Destination, "flow"); err != nil {
				return err
			}
		default:
			return invalid("invalid", "flow")
		}
		switch n.Type {
		case NodeEntry, NodeKey, NodeTimeout, NodeAudio, NodeSpeech, NodeMenu:
		default:
			n.PromptID = 0
		}
		if n.PromptID != 0 && !prompts[n.PromptID] {
			if _, err := r.Prompt(ctx, n.PromptID); err != nil {
				return invalid("unknown_prompt", "flow")
			}
			prompts[n.PromptID] = true
		}
	}
	if entries != 1 {
		return invalid("invalid", "flow")
	}
	// Lines: from a known node and port to a known node; one line per port, except a menu (a line
	// to each key, its timeout and at most one start step) and destinations that ring together.
	used := map[string]bool{}
	ports := map[string][]*FlowNode{}
	parent := map[string]bool{} // key or timeout already in a menu
	starts := 0
	edges := f.Edges[:0]
	for _, e := range f.Edges {
		from, to := f.Node(e.From), f.Node(e.To)
		if from == nil || to == nil || from.ID == to.ID {
			return invalid("invalid", "flow")
		}
		if to.Type == NodeEntry {
			return invalid("invalid_line", "flow")
		}
		switch from.Type {
		case NodeEntry, NodeMenu:
			if e.Port != "out" {
				return invalid("invalid_line", "flow")
			}
			if to.Type == NodeKey || to.Type == NodeTimeout {
				if parent[to.ID] {
					return invalid("invalid_line", "flow")
				}
				parent[to.ID] = true
			} else if from.Type == NodeMenu || to.Type == NodeRepeat {
				return invalid("invalid_line", "flow")
			} else {
				starts++
			}
		case NodeKey, NodeTimeout, NodeAudio:
			if e.Port != "out" || to.Type == NodeKey || to.Type == NodeTimeout {
				return invalid("invalid_line", "flow")
			}
		case NodeTime:
			if (e.Port != "open" && e.Port != "closed") || to.Type == NodeKey || to.Type == NodeTimeout {
				return invalid("invalid_line", "flow")
			}
		case NodeSpeech:
			ok := e.Port == "nomatch"
			for j := range from.Intents {
				ok = ok || e.Port == SpeechPort(j+1)
			}
			if !ok || to.Type == NodeKey || to.Type == NodeTimeout {
				return invalid("invalid_line", "flow")
			}
		default:
			return invalid("invalid_line", "flow")
		}
		k := e.From + "\x00" + e.Port + "\x00" + e.To
		if used[k] {
			return invalid("invalid_line", "flow")
		}
		used[k] = true
		if !isMenu(from) {
			pk := e.From + "\x00" + e.Port
			ports[pk] = append(ports[pk], to)
		}
		edges = append(edges, e)
	}
	if starts > 1 {
		return invalid("invalid_line", "flow")
	}
	// Several lines from one port only to destinations that can ring together.
	for _, to := range ports {
		if len(to) < 2 {
			continue
		}
		if len(to) > MaxParallel {
			return invalid("too_many_parallel", "flow")
		}
		for _, n := range to {
			if n.Type != NodeDestination || !Parallel(n.Destination) {
				return invalid("parallel_only_ring", "flow")
			}
		}
	}
	f.Edges = edges
	// Keys of one menu have different digits; a menu has at most one timeout.
	for i := range f.Nodes {
		if n := &f.Nodes[i]; isMenu(n) {
			seen := map[string]bool{}
			for _, k := range f.KeysOf(n.ID) {
				if seen[k.Digit] {
					return invalid("duplicate_digit", "flow")
				}
				seen[k.Digit] = true
			}
			t := 0
			for _, e := range f.Edges {
				if to := f.Node(e.To); e.From == n.ID && to != nil && to.Type == NodeTimeout {
					t++
				}
			}
			if t > 1 {
				return invalid("invalid", "flow")
			}
		}
	}

	// Derived fields.
	v.PromptID = f.Entry().PromptID
	v.Options = []IVROption{}
	for _, k := range f.MenuKeys() {
		v.Options = append(v.Options, IVROption{Digit: k.Digit, Destination: f.FirstDestination(f.Next(k.ID, "out"))})
	}
	v.TimeoutDest = Destination{Type: DestHangup}
	if t := f.TimeoutNode(); t != nil {
		v.TimeoutDest = f.FirstDestination(f.Next(t.ID, "out"))
	}
	return nil
}

// replaceFlowDestinations applies fn to every destination in a flow; true when one changed.
// A time node whose time condition is gone becomes a destination node.
func replaceFlowDestinations(f *Flow, fn func(*Destination) bool) bool {
	changed := false
	for i := range f.Nodes {
		n := &f.Nodes[i]
		if n.Destination == nil || (n.Type != NodeDestination && n.Type != NodeTime) {
			continue
		}
		if fn(n.Destination) {
			changed = true
			if n.Type == NodeTime && n.Destination.Type != DestTimeCondition {
				n.Type = NodeDestination
				f.Edges = slices.DeleteFunc(f.Edges, func(e FlowEdge) bool { return e.From == n.ID })
			}
		}
	}
	return changed
}

// replaceLayoutDestinations rewrites the flow inside a stored layout.
func replaceLayoutDestinations(raw string, fn func(*Destination) bool) (string, bool) {
	var l ivrLayout
	if json.Unmarshal([]byte(raw), &l) != nil || l.Flow == nil {
		return raw, false
	}
	if !replaceFlowDestinations(l.Flow, fn) {
		return raw, false
	}
	b, _ := json.Marshal(l)
	return string(b), true
}

// promptUsedByFlows reports whether a stored flow plays the prompt.
func (r *Repo) promptUsedByFlows(ctx context.Context, id int64) (bool, error) {
	ivrs, err := r.IVRs(ctx)
	if err != nil {
		return false, err
	}
	for _, v := range ivrs {
		if v.Flow != nil && slices.Contains(v.Flow.PromptIDs(), id) {
			return true, nil
		}
	}
	return false, nil
}
