package asterisk

import (
	"strconv"
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
		mark(x, "fax", "id="+strconv.FormatInt(f.ID, 10))
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

// genFaxSend sends faxes the portal queues (portal upload or e-mail to fax). The portal originates
// Local/<number>@kiestoon-faxout, which dials out through the normal outbound routes with the caller
// ID of the fax box, and runs kiestoon-faxsend (exten = job number) when the far end answers.
func genFaxSend(x *w, s *pbx.Snapshot) {
	send := false
	for _, f := range s.FaxBoxes {
		send = send || (f.Enabled && f.SendEnabled)
	}
	if !send {
		return
	}
	x.f("[kiestoon-faxout]")
	x.f("exten => _X.,1,NoOp(Fax job ${KT_FAXJOB} to ${EXTEN})")
	x.f(" same => n,Set(KT_CLASS=national)")
	x.f(" same => n,Set(KT_PREMIUM=0)")
	x.f(" same => n,Set(KT_OUTCID=${KT_FAXCID})")
	x.f(" same => n,Set(CALLERID(num)=${KT_FAXCID})")
	x.f(" same => n,Set(CDR(kt_faxout)=${KT_FAXJOB})")
	x.f(" same => n,Goto(kiestoon-outbound,${EXTEN},1)")
	x.f("exten => _+X.,1,Goto(kiestoon-faxout,00${EXTEN:1},1)")
	x.nl()
	x.f("[kiestoon-faxsend]")
	x.f("exten => _X.,1,NoOp(Sending fax job ${EXTEN})")
	x.f(" same => n,Set(KT_FAXJOB=${EXTEN})")
	x.f(" same => n,Set(FAXOPT(ecm)=${KT_FAXECM})")
	x.f(" same => n,Set(FAXOPT(headerinfo)=${KT_FAXHEADER})")
	x.f(" same => n,Set(FAXOPT(localstationid)=${KT_FAXCID})")
	x.f(" same => n,SendFAX(${KT_FAXFILE},z)")
	x.f(" same => n,Hangup()")
	x.f("exten => h,1,UserEvent(KiestoonFaxOut,Job: ${KT_FAXJOB},Status: ${FAXOPT(status)},Pages: ${FAXOPT(pages)}," +
		"Error: ${FILTER(A-Za-z0-9 _.:-,${FAXOPT(error)})},Remote: ${FILTER(A-Za-z0-9 +_.-,${FAXOPT(remotestationid)})})")
	x.nl()
}
