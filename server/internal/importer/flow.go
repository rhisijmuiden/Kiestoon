package importer

import (
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// flowBuilder lays out a menu tree as a flow for the IVR canvas: one column per level, one row
// per leaf.
type flowBuilder struct {
	ap  *applier
	f   *pbx.Flow
	row int
	seq int
}

func (b *flowBuilder) id(prefix string) string {
	b.seq++
	return prefix + strconv.Itoa(b.seq)
}

func (b *flowBuilder) add(n pbx.FlowNode, col int) string {
	n.X = float64(col) * 320
	n.Y = float64(b.row) * 140
	b.f.Nodes = append(b.f.Nodes, n)
	return n.ID
}

func (b *flowBuilder) edge(from, port, to string) {
	if from != "" && to != "" {
		b.f.Edges = append(b.f.Edges, pbx.FlowEdge{From: from, Port: port, To: to})
	}
}

func (b *flowBuilder) prompt(file string) int64 {
	if file == "" {
		return 0
	}
	if id, ok := b.ap.c.Prompts[file]; ok {
		return id
	}
	b.ap.warn("audio_missing", "file", file)
	return 0
}

func (b *flowBuilder) destination(key string, col int) string {
	d := b.ap.dest(key)
	id := b.add(pbx.FlowNode{ID: b.id("d"), Type: pbx.NodeDestination, Destination: &d}, col)
	b.row++
	return id
}

func keyed(s *PlanStep) []*PlanStep {
	var out []*PlanStep
	for _, c := range s.Children {
		if c.Key != "" {
			out = append(out, c)
		}
	}
	return out
}

// menu connects the keys and the timeout of a menu node (the entry or a menu step).
func (b *flowBuilder) menu(from string, s *PlanStep, col int) {
	seen := map[string]bool{}
	for _, c := range keyed(s) {
		if seen[c.Key] || len(c.Key) != 1 || !strings.Contains("0123456789*#", c.Key) {
			continue
		}
		seen[c.Key] = true
		k := b.add(pbx.FlowNode{ID: b.id("k"), Type: pbx.NodeKey, Digit: c.Key, Label: c.Title}, col)
		b.edge(from, "out", k)
		b.edge(k, "out", b.enter(c, col+1))
	}
	if s.NoAction != "" {
		t := b.add(pbx.FlowNode{ID: b.id("t"), Type: pbx.NodeTimeout}, col)
		b.edge(from, "out", t)
		b.edge(t, "out", b.destination(s.NoAction, col+1))
	}
}

// enter returns the node a call reaches when it enters the step.
func (b *flowBuilder) enter(s *PlanStep, col int) string {
	if len(keyed(s)) > 0 {
		m := b.add(pbx.FlowNode{ID: b.id("m"), Type: pbx.NodeMenu, Label: s.Title, PromptID: b.prompt(s.Audio)}, col)
		b.menu(m, s, col+1)
		return m
	}
	var audio string
	next := col
	if p := b.prompt(s.Audio); p != 0 {
		audio = b.add(pbx.FlowNode{ID: b.id("a"), Type: pbx.NodeAudio, Label: s.Title, PromptID: p}, col)
		next = col + 1
	}
	var to string
	switch {
	case s.Target != "":
		to = b.destination(s.Target, next)
	case len(s.Children) > 0:
		to = b.enter(s.Children[0], next)
	case s.NoAction != "":
		to = b.destination(s.NoAction, next)
	default:
		to = b.destination("", next)
	}
	if audio == "" {
		return to
	}
	b.edge(audio, "out", to)
	return audio
}

func (ap *applier) flow(pm PlanMenu) *pbx.Flow {
	b := &flowBuilder{ap: ap, f: &pbx.Flow{Nodes: []pbx.FlowNode{}, Edges: []pbx.FlowEdge{}}}
	s := pm.Steps
	entry := b.add(pbx.FlowNode{ID: "entry", Type: pbx.NodeEntry, PromptID: b.prompt(s.Audio)}, 0)
	if len(keyed(s)) > 0 {
		b.menu(entry, s, 1)
	} else {
		var to string
		switch {
		case s.Target != "":
			to = b.destination(s.Target, 1)
		case len(s.Children) > 0:
			to = b.enter(s.Children[0], 1)
		default:
			to = b.destination(s.NoAction, 1)
		}
		b.edge(entry, "out", to)
	}
	// Centre the entry on the rows it leads to.
	if b.row > 1 {
		b.f.Nodes[0].Y = float64(b.row-1) * 70
	}
	return b.f
}
