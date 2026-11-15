package pbx

import "testing"

func TestMatchIntent(t *testing.T) {
	intents := []SpeechIntent{
		{Label: "Sales", Keywords: []string{"offerte", "bestel", "prijs"}},
		{Label: "Invoices", Keywords: []string{"factu", "betaling", "invoice"}},
		{Label: "Support", Keywords: []string{"storing", "werkt niet", "kapot"}},
	}
	for said, want := range map[string]int{
		"Ik heb een vraag over mijn factuur.":                    2,
		"Mijn internet werkt niet meer sinds vanochtend":         3,
		"Ik wil graag een offerte, en wat is de prijs per maand": 1,
		"Hello, I have a question about an invoice":              2,
		"Goedemorgen": 0,
		"De betaling van de facturen klopt niet, dat werkt niet": 2,
		"Ik wil iets BESTELLEN!":                                 1,
		"Het apparaat is kapót":                                  3,
	} {
		if got := MatchIntent(said, intents); got != want {
			t.Errorf("%q: %d, want %d", said, got, want)
		}
	}
}
