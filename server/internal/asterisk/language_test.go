package asterisk

import (
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Queues and IVRs set the channel language, so callers hear them in the caller language (or the
// override of that queue or IVR) whatever channel the call came in on.
func TestCallerLanguage(t *testing.T) {
	s := sample()
	s.Settings.Language = "nl"
	s.Queues = []pbx.Queue{
		{ID: 7, Number: "701", Name: "Support", Strategy: "ringall", Music: "default", Enabled: true},
		{ID: 8, Number: "702", Name: "Export", Strategy: "ringall", Music: "default", Enabled: true, Options: pbx.QueueOptions{Language: "en"}},
	}
	s.IVRs = []pbx.IVR{{ID: 3, Name: "Main", Timeout: 5, Retries: 2}}
	dp := Generate(s, Options{})["extensions.conf"]
	for _, want := range []string{
		"exten => s,1,NoOp(Queue 701 Support)\n same => n,Set(CHANNEL(language)=nl)",
		"exten => s,1,NoOp(Queue 702 Export)\n same => n,Set(CHANNEL(language)=en)",
		"exten => par,1,NoOp(Queue 702 rings together with other destinations)\n same => n,Set(CHANNEL(language)=en)",
		"exten => s,1,NoOp(IVR Main)\n same => n,Set(CHANNEL(language)=nl)",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("missing %q", want)
		}
	}
	s.IVRs[0].Flow = pbx.LegacyFlow(&s.IVRs[0], nil)
	s.IVRs[0].Flow.Language = "en"
	dp = Generate(s, Options{})["extensions.conf"]
	if !strings.Contains(dp, "exten => s,1,NoOp(IVR Main)\n same => n,Set(CHANNEL(language)=en)") {
		t.Error("IVR language override not used")
	}
}
