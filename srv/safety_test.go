package srv

import "testing"

func TestFilterChat(t *testing.T) {
	cases := []struct {
		msg string
		sev int
		cat string
	}{
		{"Hallo, schöne Parzelle!", 0, ""},
		{"Ich kaufe die Wiese in Nauders um 300 Münzen", 0, ""},
		{"EZ 42 hat 8 Parzellen, 3 davon Wald", 0, ""},
		{"Das ist so eine scheiss Parzelle", 1, "profanity"},
		{"du hurensohn", 2, "hate"},
		{"du h.u.r.e.n.s.o.h.n", 2, "hate"},
		{"du HuRen50hn", 2, "hate"},
		{"Ruf mich an 0664 123 45 67", 2, "contact"},
		{"schreib mir max.muster (at) gmail (dot) com", 2, "contact"},
		{"komm auf discord.gg/abc", 2, "contact"},
		{"adde mich auf snap: maxi_12", 2, "contact"},
		{"hast du insta?", 2, "contact"},
		{"wie alt bist du?", 2, "grooming"},
		{"wie alt bist du, schöne kleine?", 3, "grooming"},
		{"schick mir mal ein foto", 2, "grooming"},
		{"das bleibt unser kleines Geheimnis", 2, "grooming"},
		{"ich bin 12 jahre alt", 2, "self_disclosure"},
		{"ich wohne in Wien", 2, "self_disclosure"},
		{"Ich bin dran, ich geh in die Schule morgen", 2, "self_disclosure"},
		{"nudes?", 2, "sexual"},
		{"ich bin 30 parzellen weit", 0, ""},
		{"ok", 0, ""},
		{"Gemeindegut 3394/1 ist 850 ha", 0, ""},
	}
	for _, c := range cases {
		r := filterChat(c.msg)
		if r.Severity != c.sev || (c.cat != "" && r.Category != c.cat) {
			t.Errorf("%q → sev %d cat %s (want %d %s)", c.msg, r.Severity, r.Category, c.sev, c.cat)
		}
	}
}

func TestFilterName(t *testing.T) {
	for _, n := range []string{"Tapferer Fuchs", "Kohlschwarz", "Wiener_Maus", "Stille Birke 7"} {
		if !filterName(n) {
			t.Errorf("name %q rejected", n)
		}
	}
	for _, n := range []string{"Hurensohn99", "N1gger", "Arschloch", "Fotze"} {
		if filterName(n) {
			t.Errorf("name %q accepted", n)
		}
	}
}
