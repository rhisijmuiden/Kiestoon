package threecx

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
)

const backupXML = `<?xml version="1.0" encoding="utf-8"?>
<PhoneSystem Version="18.0.9.20">
  <Extensions>
    <Extension Number="100"><FirstName>Anna</FirstName><LastName>de Vries</LastName><EmailAddress>anna@example.nl</EmailAddress><AuthPassword>Zx8kLmn42Qa</AuthPassword><VMPin>4321</VMPin></Extension>
    <Extension Number="101" FirstName="Bram" LastName="Jansen" AuthPassword="Yq7abcd99Rt"/>
  </Extensions>
  <RingGroups>
    <RingGroup Number="800" Name="Sales" RingStrategy="Hunt" RingTime="20">
      <Members><Member Number="100"/><Member Number="101"/></Members>
      <NoAnswer Type="VoiceMail" To="100"/>
    </RingGroup>
  </RingGroups>
  <CallQueues>
    <CallQueue Number="801" Name="Support" PollingStrategy="RingAll">
      <Agents><Agent Number="101"/></Agents>
      <NoAnswer Type="Extension" To="100"/>
    </CallQueue>
  </CallQueues>
  <DigitalReceptionists>
    <DigitalReceptionist Number="802" Name="Main menu" PromptFilename="welcome.wav">
      <Forwards>
        <Forward Input="1" Type="RingGroup" To="800"/>
        <Forward Input="2" Type="Queue" To="801"/>
        <Forward Input="9" Type="External" To="0612345678"/>
      </Forwards>
      <Timeout Type="EndCall"/>
    </DigitalReceptionist>
  </DigitalReceptionists>
  <InboundRules><InboundRule Number="+31201234567"><Destination Type="IVR" To="802"/></InboundRule></InboundRules>
  <Gateways><Gateway Name="Voys" Host="sip.voys.nl" AuthID="12345" AuthPassword="secret99"/></Gateways>
</PhoneSystem>`

func writeZip(t *testing.T, files map[string][]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "backup.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, b := range files {
		w, _ := zw.Create(n)
		w.Write(b)
	}
	zw.Close()
	f.Close()
	return p
}

func TestBackup(t *testing.T) {
	wav, err := os.ReadFile("../freeswitch/testdata/sounds/custom/welcome.wav")
	if err != nil {
		t.Fatal(err)
	}
	p := writeZip(t, map[string][]byte{"backup.xml": []byte(backupXML), "httpprompts/welcome.wav": wav, "vmailprompts/x.wav": wav})
	u, err := importer.NewUpload("3cx_backup.zip", p)
	if err != nil {
		t.Fatal(err)
	}
	if a := importer.Detect(u); a == nil || a.ID() != "3cx" {
		t.Fatalf("detect %v", a)
	}
	b, err := importer.Read(context.Background(), u, "")
	if err != nil {
		t.Fatal(err)
	}
	if b.Info.Product != "3CX 18.0.9.20" {
		t.Errorf("product %q", b.Info.Product)
	}
	if len(b.Users) != 2 || b.Users[0].Name != "Anna de Vries" || b.Users[0].Email != "anna@example.nl" || b.Users[0].VoicemailPIN != "4321" || b.Users[1].SIPPassword != "Yq7abcd99Rt" {
		t.Errorf("users %+v", b.Users)
	}
	if len(b.Groups) != 1 || b.Groups[0].Strategy != "sequential" || len(b.Groups[0].Members) != 2 || b.Groups[0].Failover == nil || b.Groups[0].Failover.Kind != importer.TargetVoicemail {
		t.Errorf("groups %+v", b.Groups)
	}
	if len(b.Queues) != 1 || b.Queues[0].Strategy != "ringall" || b.Queues[0].Overflow == nil || b.Queues[0].Overflow.Ref != "100" {
		t.Errorf("queues %+v", b.Queues)
	}
	if len(b.Menus) != 1 || b.Menus[0].Audio != "welcome.wav" || len(b.Menus[0].Children) != 3 || b.Menus[0].NoAction == nil ||
		b.Menus[0].Children[2].Target.Kind != importer.TargetExternal {
		t.Fatalf("menus %+v", b.Menus)
	}
	if len(b.Numbers) != 1 || b.Numbers[0].Target == nil || b.Numbers[0].Target.Kind != importer.TargetMenu {
		t.Errorf("numbers %+v", b.Numbers)
	}
	if len(b.Trunks) != 1 || b.Trunks[0].Host != "sip.voys.nl" || !b.Trunks[0].Register {
		t.Errorf("trunks %+v", b.Trunks)
	}
}

func TestCSVExport(t *testing.T) {
	p := filepath.Join(t.TempDir(), "users.csv")
	os.WriteFile(p, []byte("\xef\xbb\xbfNumber;FirstName;LastName;EmailAddress;MobileNumber\n100;Anna;de Vries;anna@example.nl;0612345678\n101;Bram;Jansen;;\n"), 0o600)
	u, err := importer.NewUpload("users.csv", p)
	if err != nil {
		t.Fatal(err)
	}
	if a := importer.Detect(u); a == nil || a.ID() != "3cx" {
		t.Fatalf("detect %v", a)
	}
	b, err := importer.Read(context.Background(), u, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Users) != 2 || b.Users[1].Name != "Bram Jansen" {
		t.Errorf("users %+v", b.Users)
	}
}

func TestEncrypted(t *testing.T) {
	junk := []byte(strings.Repeat("\x01\x9f\x00\xfe\x13", 400))
	p := writeZip(t, map[string][]byte{"backup.xml": junk, "httpprompts/a.wav": []byte("RIFF")})
	u, _ := importer.NewUpload("b.zip", p)
	if _, err := importer.Read(context.Background(), u, "3cx"); err == nil || err.Error() != "encrypted_backup" {
		t.Fatalf("err %v", err)
	}
}
