// Package importer moves the configuration of another phone system into Kiestoon.
//
// Every supported system has an Adapter that reads its backup or configuration files and
// describes the phone system in one common form, the Bundle: users (extensions), ring groups,
// queues, trunks, time conditions, voice menus with their recordings and phone numbers. From a
// Bundle, Prepare builds the plan the administrator sees (with a free Kiestoon number for
// everything) and Apply creates what they ticked. Adapters only translate; they never touch the
// database.
//
// Things inside a Bundle refer to each other by Ref: the id the source system uses (a FreePBX
// extension number, a Talk ULP id, a FreeSWITCH queue name). A Ref is only unique within its kind.
package importer

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Target kinds: where a call goes.
const (
	TargetUser      = "user"      // Ref: User.Ref
	TargetVoicemail = "voicemail" // Ref: User.Ref
	TargetGroup     = "group"     // Ref: Group.Ref
	TargetQueue     = "queue"     // Ref: Queue.Ref
	TargetMenu      = "menu"      // Ref: Step.Ref of a menu (a root step)
	TargetTime      = "time"      // Ref: TimeCondition.Ref
	TargetExternal  = "external"  // Ref: the phone number
	TargetHangup    = "hangup"
)

// Target is where a call goes. A nil *Target means hang up.
type Target struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// User is one extension with its SIP account and voicemail box.
type User struct {
	Ref          string
	Ext          string // number in the source system
	Name         string
	Email        string
	SIPPassword  string
	Voicemail    bool
	VoicemailPIN string
	// VoicemailEmail: voicemail is sent by email (to Email).
	VoicemailEmail bool
	// OutboundCID is the caller id for outside calls ("" for the trunk default).
	OutboundCID string
	Disabled    bool
}

// Group is a ring group.
type Group struct {
	Ref  string
	Name string
	Ext  string
	// Strategy: simultaneous | sequential | rotating | paging.
	Strategy    string
	RingSeconds int
	Members     []string // User.Ref in ring order
	// External members (a mobile number) ring as well.
	External []string
	Failover *Target
}

// Queue is a call queue.
type Queue struct {
	Ref  string
	Name string
	Ext  string
	// Strategy is an Asterisk queue strategy: ringall, leastrecent, fewestcalls, random, rrmemory, linear, wrandom.
	Strategy   string
	MaxWait    int // seconds, 0 = no limit
	RingTime   int // seconds an agent rings
	WrapUp     int
	MaxSize    int
	Agents     []string // User.Ref
	Overflow   *Target
	Greeting   string   // audio file played when a caller joins, "" when none
	Messages   []string // other audio files of the queue (not played by Kiestoon)
	MusicClass string
	Disabled   bool
}

// Trunk is a connection to a SIP provider.
type Trunk struct {
	Ref          string
	Name         string
	Host         string
	Port         int
	Transport    string // udp | tcp | tls
	Username     string
	AuthUsername string
	Password     string
	Register     bool
	FromDomain   string
	FromUser     string
	CallerID     string
	Codecs       string   // comma separated Asterisk codec names, "" for the default
	Networks     []string // provider addresses for IP authentication
	Numbers      []string
}

// TimeCondition is a schedule: calls go to Open during the opening hours, else to Closed.
type TimeCondition struct {
	Ref      string
	Name     string
	Schedule []pbx.TimeRange
	Open     *Target
	Closed   *Target
}

// Step is a voice menu as a tree. The root (Type "root") is the menu itself, with Ref; children
// with a Key are the keys of the menu. A step without keyed children plays its Audio and then
// goes to Target (or to its first child).
type Step struct {
	Ref      string // root steps only: what TargetMenu refers to
	ID       int
	Type     string // root | menu | audio
	Key      string // "" when the step is not a menu option
	Title    string
	Audio    string // audio file, "" when none
	Greeting string // text of a spoken greeting when there is no Audio (shown, not imported)
	Ext      string // internal number of the menu in the source system
	Numbers  []string
	Target   *Target
	// NoAction is where the call goes when the caller presses nothing or a wrong key.
	NoAction *Target
	Children []*Step
	Disabled bool
}

// Number is a phone number (DID) with where it goes.
type Number struct {
	Number string
	Target *Target
}

// Phone is a desk phone of the source system.
type Phone struct {
	MAC    string
	Vendor string // yealink, snom, grandstream, ... ("" when unknown)
	Model  string
	User   string // User.Ref of the first line, "" when none
	Label  string
}

// Note is a remark for the administrator, translated by the web app (import.notes.<code>).
type Note struct {
	Code string            `json:"code"`
	Args map[string]string `json:"args,omitempty"`
}

// NewNote makes a note from a code and key/value pairs.
func NewNote(code string, kv ...string) Note {
	n := Note{Code: code}
	if len(kv) > 1 {
		n.Args = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			n.Args[kv[i]] = kv[i+1]
		}
	}
	return n
}

// Info describes the source for the preview.
type Info struct {
	System    string `json:"system"`  // adapter id
	Product   string `json:"product"` // e.g. "FreePBX 16.0.40"
	Host      string `json:"host,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"` // milliseconds since 1970, 0 when unknown
	// Details are extra facts, shown as they are (e.g. console model, firmware).
	Details map[string]string `json:"details,omitempty"`
}

// Bundle is a phone system read from a backup, in the form every adapter produces.
type Bundle struct {
	Info           Info
	Users          []User
	Groups         []Group
	Queues         []Queue
	Trunks         []Trunk
	TimeConditions []TimeCondition
	Menus          []*Step
	Numbers        []Number
	Phones         []Phone
	// Audio maps a file name (as used by Step.Audio and Queue.Greeting) to its contents. Files a
	// browser cannot decode (G.711, GSM, raw) are converted to WAV by NormalizeAudio.
	Audio map[string][]byte
	// VoicemailGreeting is the audio file of a greeting for every voicemail box, "" when none.
	VoicemailGreeting string
	Notes             []Note
}

// Note adds a remark.
func (b *Bundle) Note(code string, kv ...string) { b.Notes = append(b.Notes, NewNote(code, kv...)) }

// User returns the user with the ref, or nil.
func (b *Bundle) User(ref string) *User {
	for i := range b.Users {
		if b.Users[i].Ref == ref {
			return &b.Users[i]
		}
	}
	return nil
}

// UserByExt returns the user with the extension number, or nil.
func (b *Bundle) UserByExt(ext string) *User {
	for i := range b.Users {
		if b.Users[i].Ext == ext {
			return &b.Users[i]
		}
	}
	return nil
}

// Menu returns the root step with the ref, or nil.
func (b *Bundle) Menu(ref string) *Step {
	for _, m := range b.Menus {
		if m.Ref == ref {
			return m
		}
	}
	return nil
}

func (b *Bundle) queue(ref string) *Queue {
	for i := range b.Queues {
		if b.Queues[i].Ref == ref {
			return &b.Queues[i]
		}
	}
	return nil
}

func (b *Bundle) group(ref string) *Group {
	for i := range b.Groups {
		if b.Groups[i].Ref == ref {
			return &b.Groups[i]
		}
	}
	return nil
}

func (b *Bundle) trunk(ref string) *Trunk {
	for i := range b.Trunks {
		if b.Trunks[i].Ref == ref {
			return &b.Trunks[i]
		}
	}
	return nil
}

func (b *Bundle) timeCondition(ref string) *TimeCondition {
	for i := range b.TimeConditions {
		if b.TimeConditions[i].Ref == ref {
			return &b.TimeConditions[i]
		}
	}
	return nil
}

// ---- adapters -----------------------------------------------------------------

// Errors for the user (translated by the web app as import.errors.<code>).
var (
	ErrUnknownFormat = errors.New("unknown_format")
	ErrEmpty         = errors.New("nothing_to_import")
	ErrTooLarge      = errors.New("too_large")
	ErrDamaged       = errors.New("damaged_file")
)

// Adapter reads the backup or configuration files of one kind of phone system.
type Adapter interface {
	// ID is the stable name of the system (unifi-talk, asterisk, freepbx, ...).
	ID() string
	// Detect reports how sure the adapter is that the upload is its format: 0 not at all, 100 certain.
	Detect(u *Upload) int
	// Read turns the upload into a Bundle.
	Read(ctx context.Context, u *Upload) (*Bundle, error)
}

var (
	mu       sync.RWMutex
	adapters []Adapter
)

// Register adds an adapter. Adapters register themselves in an init function.
func Register(a Adapter) {
	mu.Lock()
	defer mu.Unlock()
	adapters = slices.DeleteFunc(adapters, func(x Adapter) bool { return x.ID() == a.ID() })
	adapters = append(adapters, a)
	slices.SortFunc(adapters, func(x, y Adapter) int { return strings.Compare(x.ID(), y.ID()) })
}

// Adapters returns the ids of the registered adapters.
func Adapters() []string {
	mu.RLock()
	defer mu.RUnlock()
	var out []string
	for _, a := range adapters {
		out = append(out, a.ID())
	}
	return out
}

// Find returns the adapter with the id, or nil.
func Find(id string) Adapter {
	mu.RLock()
	defer mu.RUnlock()
	for _, a := range adapters {
		if a.ID() == id {
			return a
		}
	}
	return nil
}

// Detect returns the adapter that fits the upload best, or nil.
func Detect(u *Upload) Adapter {
	mu.RLock()
	defer mu.RUnlock()
	var best Adapter
	score := 0
	for _, a := range adapters {
		if s := a.Detect(u); s > score {
			best, score = a, s
		}
	}
	return best
}

// Read detects the format (or uses the adapter with the id) and reads the upload.
func Read(ctx context.Context, u *Upload, system string) (*Bundle, error) {
	var a Adapter
	if system != "" && system != "auto" {
		a = Find(system)
	} else {
		a = Detect(u)
	}
	if a == nil {
		return nil, ErrUnknownFormat
	}
	b, err := a.Read(ctx, u)
	if err != nil {
		return nil, err
	}
	b.Info.System = a.ID()
	if b.Audio == nil {
		b.Audio = map[string][]byte{}
	}
	for name, data := range b.Audio {
		b.Audio[name] = NormalizeAudio(name, data)
	}
	if len(b.Users)+len(b.Groups)+len(b.Queues)+len(b.Trunks)+len(b.Menus)+len(b.Numbers)+len(b.TimeConditions) == 0 {
		return nil, ErrEmpty
	}
	return b, nil
}
