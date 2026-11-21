package crm

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

// Result is what a look-up found: the contacts of the first CRM in the order that knows the number.
type Result struct {
	Number   string    `json:"number"`
	CRM      string    `json:"crm,omitempty"`
	Contacts []Contact `json:"contacts"`
	At       time.Time `json:"-"`
	// Took is how long the look-up took (ms), for the sync log.
	Took int64 `json:"-"`
}

// Found tells whether a contact was found.
func (r *Result) Found() bool { return r != nil && len(r.Contacts) > 0 }

// First returns the first contact, or an empty one.
func (r *Result) First() Contact {
	if !r.Found() {
		return Contact{}
	}
	return r.Contacts[0]
}

// CacheTTL is how long a look-up result is reused.
var CacheTTL = 10 * time.Minute

// ownersTTL is how long the owner list of a CRM is reused.
const ownersTTL = time.Hour

// LogFunc records a CRM request for the sync log.
type LogFunc func(kind, action, detail string, took time.Duration, err error)

// Service holds the connectors and caches look-ups and owner lists.
type Service struct {
	// SaveSecret stores a secret that a connector changed (refreshed tokens, expired sign-in).
	SaveSecret func(kind string, s Secret)
	// Log records requests (may be nil).
	Log LogFunc

	mu       sync.Mutex
	settings Settings
	secrets  map[string]Secret
	conns    map[string]Connector
	cache    map[string]*Result
	flight   map[string]*flight
	owners   map[string]ownerList
}

type flight struct {
	done chan struct{}
	res  *Result
	err  error
}

type ownerList struct {
	at   time.Time
	list []Owner
}

// NewService makes an empty service; call Reload with the settings.
func NewService() *Service {
	return &Service{conns: map[string]Connector{}, cache: map[string]*Result{}, flight: map[string]*flight{}, owners: map[string]ownerList{}, secrets: map[string]Secret{}}
}

// Reload replaces the settings and secrets. Connectors and caches start fresh.
func (s *Service) Reload(st Settings, secrets map[string]Secret) {
	st.Normalize()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings, s.secrets = st, secrets
	s.conns = map[string]Connector{}
	s.cache = map[string]*Result{}
	s.owners = map[string]ownerList{}
	for _, k := range Kinds {
		cfg, sec := st.Connectors[k], secrets[k]
		if !cfg.Enabled || !sec.Configured(k) {
			continue
		}
		kind := k
		c, err := New(k, cfg, sec, func(n Secret) {
			s.mu.Lock()
			s.secrets[kind] = n
			s.mu.Unlock()
			if s.SaveSecret != nil {
				s.SaveSecret(kind, n)
			}
		})
		if err == nil {
			s.conns[k] = c
		}
	}
}

// Settings returns the current settings.
func (s *Service) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// Active tells whether at least one CRM is connected.
func (s *Service) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.conns {
		if !s.secrets[k].LoginExpired {
			return true
		}
	}
	return false
}

// Connector returns the connector and config of a kind (nil when it is not connected).
func (s *Service) Connector(kind string) (Connector, *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.conns[kind]
	if c == nil {
		return nil, nil
	}
	return c, s.settings.Connectors[kind]
}

// Fail handles an error from a connector: a refused sign-in marks the CRM as expired, so look-ups
// skip it until an admin signs in again.
func (s *Service) Fail(kind string, err error) {
	if !errors.Is(err, ErrAuth) {
		return
	}
	s.mu.Lock()
	sec, ok := s.secrets[kind]
	if !ok || sec.LoginExpired {
		s.mu.Unlock()
		return
	}
	sec.LoginExpired = true
	s.secrets[kind] = sec
	s.mu.Unlock()
	if s.SaveSecret != nil {
		s.SaveSecret(kind, sec)
	}
}

func (s *Service) log(kind, action, detail string, took time.Duration, err error) {
	if s.Log != nil {
		s.Log(kind, action, detail, took, err)
	}
}

// Cached returns a cached result without asking the CRMs.
func (s *Service) Cached(n Number) *Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.cache[n.Key]; r != nil && time.Since(r.At) < CacheTTL {
		return r
	}
	return nil
}

// Forget drops the cached result of a number (after a contact was created for it).
func (s *Service) Forget(n Number) {
	s.mu.Lock()
	delete(s.cache, n.Key)
	s.mu.Unlock()
}

// Lookup asks the CRMs in order for a number. Results (also "not found") are cached; concurrent
// look-ups of the same number share one request.
func (s *Service) Lookup(ctx context.Context, n Number) (*Result, error) {
	if r := s.Cached(n); r != nil {
		return r, nil
	}
	s.mu.Lock()
	if f := s.flight[n.Key]; f != nil {
		s.mu.Unlock()
		select {
		case <-f.done:
			return f.res, f.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	s.flight[n.Key] = f
	order := slices.Clone(s.settings.Order)
	s.mu.Unlock()

	// The shared request is not cut short by the first caller's deadline.
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	f.res, f.err = s.lookup(lctx, n, order)
	cancel()
	s.mu.Lock()
	delete(s.flight, n.Key)
	if f.err == nil {
		s.cache[n.Key] = f.res
		// Keep the cache small.
		if len(s.cache) > 5000 {
			for k, r := range s.cache {
				if time.Since(r.At) > CacheTTL {
					delete(s.cache, k)
				}
			}
		}
	}
	s.mu.Unlock()
	close(f.done)
	return f.res, f.err
}

func (s *Service) lookup(ctx context.Context, n Number, order []string) (*Result, error) {
	start := time.Now()
	res := &Result{Number: n.Raw, Contacts: []Contact{}}
	var firstErr error
	for _, k := range order {
		c, cfg := s.Connector(k)
		if c == nil || s.expired(k) {
			continue
		}
		t := time.Now()
		found, err := c.Lookup(ctx, n, cfg)
		if err != nil {
			s.Fail(k, err)
			s.log(k, "lookup", n.Raw, time.Since(t), err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if len(found) == 0 {
			s.log(k, "lookup", n.Raw, time.Since(t), nil)
			continue
		}
		if len(found) > 10 {
			found = found[:10]
		}
		// Deals, tickets and the owner's name only for the contact the pop-up opens with.
		if err := c.Details(ctx, &found[0], cfg); err != nil {
			s.Fail(k, err)
		}
		for i := range found {
			if found[i].Owner == "" && found[i].OwnerID != "" {
				found[i].Owner = s.ownerName(ctx, k, found[i].OwnerID)
			}
		}
		s.log(k, "lookup", n.Raw+" → "+found[0].Name, time.Since(t), nil)
		res.CRM, res.Contacts = k, found
		break
	}
	res.At, res.Took = time.Now(), time.Since(start).Milliseconds()
	if !res.Found() && firstErr != nil {
		// Do not cache "not found" when a CRM failed: the next call tries again.
		return res, firstErr
	}
	return res, nil
}

func (s *Service) expired(kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secrets[kind].LoginExpired
}

// Owners returns the owner list of a CRM (cached for an hour).
func (s *Service) Owners(ctx context.Context, kind string) ([]Owner, error) {
	s.mu.Lock()
	if o, ok := s.owners[kind]; ok && time.Since(o.at) < ownersTTL {
		s.mu.Unlock()
		return o.list, nil
	}
	s.mu.Unlock()
	c, _ := s.Connector(kind)
	if c == nil {
		return nil, errors.New("crm: not connected")
	}
	list, err := c.Owners(ctx)
	if err != nil {
		s.Fail(kind, err)
		return nil, err
	}
	s.mu.Lock()
	s.owners[kind] = ownerList{at: time.Now(), list: list}
	s.mu.Unlock()
	return list, nil
}

func (s *Service) ownerName(ctx context.Context, kind, id string) string {
	list, _ := s.Owners(ctx, kind)
	for _, o := range list {
		if o.ID == id {
			return o.Name
		}
	}
	return ""
}

// OwnerFor finds the CRM owner of a Kiestoon user: the manual mapping first, then the e-mail address.
func (s *Service) OwnerFor(ctx context.Context, kind string, userID string, email string) string {
	s.mu.Lock()
	cfg := s.settings.Connectors[kind]
	var manual string
	if cfg != nil {
		manual = cfg.Owners[userID]
	}
	s.mu.Unlock()
	if manual != "" {
		return manual
	}
	if email == "" {
		return ""
	}
	list, _ := s.Owners(ctx, kind)
	for _, o := range list {
		if strings.EqualFold(o.Email, email) {
			return o.ID
		}
	}
	return ""
}

// DeskName waits at most wait for a look-up and returns the name for a desk phone display ("" when
// it is unknown or too slow; the look-up goes on and fills the cache for the pop-up).
func (s *Service) DeskName(n Number, wait time.Duration) string {
	if !s.Settings().DeskName || !s.Active() {
		return ""
	}
	ch := make(chan *Result, 1)
	go func() {
		r, _ := s.Lookup(context.Background(), n)
		ch <- r
	}()
	select {
	case r := <-ch:
		if r.Found() {
			return r.First().DisplayName()
		}
	case <-time.After(wait):
	}
	return ""
}
