package asterisk

import (
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// safeName keeps a caller name usable inside Set() and UserEvent().
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ',', '(', ')', '$', '[', ']', '{', '}', '"', '\\', ';', '|', '\n', '\r':
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// genCallerIDNames looks inbound callers up in the phone book: every number of every contact, in
// all the ways a provider may send it (0201234567, 31201234567, +31201234567, 0031201234567).
func genCallerIDNames(x *w, s *pbx.Snapshot) {
	if !s.Phonebook.CallerIDLookup || len(s.Contacts) == 0 {
		return
	}
	x.f("[kiestoon-cidname]")
	done := map[string]bool{}
	for _, c := range s.Contacts {
		name := safeName(c.Name)
		if c.Company != "" {
			name = safeName(c.Name + " - " + c.Company)
		}
		if name == "" {
			continue
		}
		for _, num := range []string{c.Work, c.Mobile, c.Other} {
			for _, v := range pbx.DIDVariants(num, s.Settings.CountryCode) {
				if v == "" || done[v] || strings.ContainsAny(v, " _") {
					continue
				}
				done[v] = true
				x.f("exten => %s,1,Set(CALLERID(name)=%s)", v, name)
				x.f(" same => n,Return()")
			}
		}
	}
	x.nl()
}

// genFax receives faxes for every fax box (T.38 when the trunk offers it, otherwise G.711 audio)
// and reports the result to the portal, which converts the TIFF to PDF and mails it.
func genFax(x *w, s *pbx.Snapshot, o Options) {
	for _, f := range s.FaxBoxes {
		x.f("[kiestoon-fax-%d]", f.ID)
		x.f("exten => s,1,NoOp(Fax box %s)", esc(safeName(f.Name)))
		if !f.Enabled || o.FaxDir == "" {
			x.f(" same => n,Hangup()")
			x.nl()
			continue
		}
		ecm := "no"
		if f.ECM {
			ecm = "yes"
		}
		x.f(" same => n,Set(KT_FAXFILE=%s/${UNIQUEID}.tif)", o.FaxDir)
		x.f(" same => n,Set(FAXOPT(ecm)=%s)", ecm)
		x.f(" same => n,Set(FAXOPT(headerinfo)=%s)", esc(safeName(f.Name)))
		x.f(" same => n,Set(FAXOPT(localstationid)=${KT_DID})")
		x.f(" same => n,Set(CDR(kt_fax)=%d)", f.ID)
		x.f(" same => n,Answer()")
		x.f(" same => n,ReceiveFAX(${KT_FAXFILE},f)")
		x.f(" same => n,Hangup()")
		x.f("exten => h,1,UserEvent(KiestoonFax,Box: %d,File: ${KT_FAXFILE},Status: ${FAXOPT(status)},Pages: ${FAXOPT(pages)},"+
			"Error: ${FILTER(A-Za-z0-9 _.:-,${FAXOPT(error)})},Remote: ${FILTER(A-Za-z0-9 +_.-,${FAXOPT(remotestationid)})},"+
			"Caller: ${FILTER(0-9+,${CALLERID(num)})},DID: ${KT_DID})", f.ID)
		x.nl()
	}
}
