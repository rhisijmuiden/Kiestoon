package provision

import (
	"strings"
)

// vendorMarks are the words in a User-Agent that name the vendor.
var vendorMarks = []struct{ mark, vendor string }{
	{"YEALINK", "yealink"}, {"SNOM", "snom"}, {"GRANDSTREAM", "grandstream"}, {"FANVIL", "fanvil"},
	{"GIGASET", "gigaset"}, {"POLYCOM", "poly"}, {"POLY", "poly"}, {"HTEK", "htek"}, {"AKUVOX", "akuvox"},
	{"CISCO", "cisco"},
}

func squash(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return -1
	}, s)
}

// GuessModel names the vendor and model of an unknown phone from the file it asked for and its
// User-Agent. Either may be "" when it cannot be told.
func GuessModel(req Request, mac, userAgent string) (vendor, model string) {
	ua := squash(userAgent)
	// The MAC address is often part of the User-Agent; its digits must not look like a model.
	ua = strings.ReplaceAll(ua, strings.ToUpper(mac), " ")
	for _, v := range vendorMarks {
		if strings.Contains(ua, v.mark) {
			vendor = v.vendor
			break
		}
	}
	if vendor == "" {
		switch {
		case len(req.Shared) > 0:
			// Grandstream and Htek ask for the same file: without a User-Agent it stays open.
		case req.Vendor != "":
			vendor = req.Vendor
		}
	}
	if req.Model != "" && vendor == "snom" {
		if m := FindModel("snom", strings.ToUpper(req.Model)); m != nil {
			return vendor, m.ID
		}
	}
	best := ""
	for _, m := range Models {
		if vendor != "" && m.Vendor != vendor {
			continue
		}
		if id := squash(m.ID); strings.Contains(ua, id) && len(id) > len(squash(best)) {
			best, model = m.ID, m.ID
			if vendor == "" {
				vendor = m.Vendor
			}
		}
	}
	if vendor == "" && model == "" {
		return "", ""
	}
	if model != "" && FindModel(vendor, model) == nil {
		model = ""
	}
	return vendor, model
}

// ResyncEvent is the SIP NOTIFY event that makes a phone of this vendor fetch its configuration
// again without a full reboot where the vendor supports that.
func ResyncEvent(vendor string) string {
	switch vendor {
	case "yealink", "snom", "fanvil", "htek":
		return "check-sync;reboot=false"
	case "cisco":
		return "resync"
	}
	return "check-sync"
}
