package services

import "testing"

func TestIsGroupJID(t *testing.T) {
	cases := map[string]bool{
		"120363012345678901@g.us":     true,
		"628123-1600000000@g.us":      true,
		"628123456789@s.whatsapp.net": false,
		"628123456789":                false,
		"":                            false,
		"@g.us":                       true, // suffix cocok; validasi format lanjut ada di whatsmeow ParseJID
	}
	for in, want := range cases {
		if got := IsGroupJID(in); got != want {
			t.Errorf("IsGroupJID(%q) = %v, mau %v", in, got, want)
		}
	}
}

func TestNormalizeInboxSenderPreservesGroupJID(t *testing.T) {
	group := "120363123456789012@g.us"
	if got := NormalizeInboxSender(group); got != group {
		t.Fatalf("NormalizeInboxSender(%q) = %q", group, got)
	}
	if got := NormalizeInboxSender("081220990678"); got != "6281220990678" {
		t.Fatalf("nomor personal tidak dinormalisasi: %q", got)
	}
	if got := NormalizeInboxSender("@g.us"); got != "" {
		t.Fatalf("JID grup rusak harus ditolak, got %q", got)
	}
}

func TestLegacyGroupSenderAliasOnlyAcceptsNonPhoneGroupIdentity(t *testing.T) {
	if got := legacyGroupSenderAlias("120363425256238999@g.us"); got != "120363425256238999" {
		t.Fatalf("alias grup modern = %q", got)
	}
	if got := legacyGroupSenderAlias("6281220990678@g.us"); got != "" {
		t.Fatalf("angka yang masih valid sebagai nomor tidak boleh dimigrasikan: %q", got)
	}
	if got := legacyGroupSenderAlias("120363425256238999@s.whatsapp.net"); got != "" {
		t.Fatalf("JID personal tidak boleh dianggap alias grup: %q", got)
	}
}
