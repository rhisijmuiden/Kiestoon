package ai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/speech"
)

// TestLocalModel runs a real summary with llama-server. It only runs when
// KIESTOON_TEST_SPEECH_HOME (with llama/bin/llama-server) and KIESTOON_TEST_SPEECH_DIR (with the
// Qwen3 1.7B model) are set.
func TestLocalModel(t *testing.T) {
	home, dir := os.Getenv("KIESTOON_TEST_SPEECH_HOME"), os.Getenv("KIESTOON_TEST_SPEECH_DIR")
	if home == "" || dir == "" {
		t.Skip("no local model")
	}
	z := &Summarizer{Speech: &speech.Engine{Home: home, Dir: dir}}
	st := Defaults()
	st.LLM = "qwen3-1.7b"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for _, text := range []string{
		"Goedemorgen, u spreekt met Mark de Vries van Bakkerij De Korrel. Onze bestelling van afgelopen dinsdag is nog niet binnen. " +
			"Kunt u nagaan waar die blijft en mij voor drie uur terugbellen? Mijn nummer is 0503141592. Dank u wel.",
		"Hi, this is Sarah from Northwind. The invoice you sent last week has the wrong VAT number. Could you send a corrected one? Thanks.",
	} {
		start := time.Now()
		sum, engine, err := z.Summarize(ctx, st, KindVoicemail, text)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s in %s: %+v", engine, time.Since(start).Round(time.Millisecond), sum)
	}
	speech.StopLLM()
}
