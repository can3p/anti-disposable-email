package disposable

import "testing"

func TestParseEmail(t *testing.T) {
	tests := []struct {
		in        string
		canonical string
		pref      string
		extra     string
		cat       Category
	}{
		{"John.Smith+news@gmail.com", "johnsmith@gmail.com", "John.Smith", "news", CategoryNone},
		{"j.o.h.n+x@googlemail.com", "john@gmail.com", "j.o.h.n", "x", CategoryNone},
		{"john+x@outlook.com", "john@outlook.com", "john", "x", CategoryNone},
		{"john.smith+x@outlook.com", "john.smith@outlook.com", "john.smith", "x", CategoryNone},
		{"john+x@me.com", "john@icloud.com", "john", "x", CategoryNone},
		{"john+x@pm.me", "john@proton.me", "john", "x", CategoryNone},
		{"base-keyword@yahoo.com", "base@yahoo.com", "base", "keyword", CategoryNone},
		{"anything@john.fastmail.com", "john@fastmail.com", "john", "anything", CategoryNone},
		{"+tag@gmail.com", "+tag@gmail.com", "+tag", "", CategoryNone},
		{"John+x@golang.org", "john+x@golang.org", "John+x", "", CategoryNone},
		{"a@mailinator.com", "a@mailinator.com", "a", "", CategoryDisposable},
		{"a@MAILINATOR.COM", "a@mailinator.com", "a", "", CategoryDisposable},
		{"a@foo.mailinator.com", "a@foo.mailinator.com", "a", "", CategoryDisposable},
		{"a@deep.sub.mailinator.com", "a@deep.sub.mailinator.com", "a", "", CategoryDisposable},
		{"a@guerrillamail.com", "a@guerrillamail.com", "a", "", CategoryDisposable},
		{"a@duck.com", "a@duck.com", "a", "", CategoryRelay},
		{"a@privaterelay.appleid.com", "a@privaterelay.appleid.com", "a", "", CategoryRelay},
		{"user@bücher.de", "user@xn--bcher-kva.de", "user", "", CategoryNone},
		{"üser@golang.org", "üser@golang.org", "üser", "", CategoryNone},
		{"a@x.ck", "a@x.ck", "a", "", CategoryNone},
	}
	for _, tt := range tests {
		p, err := ParseEmail(tt.in)
		if err != nil {
			t.Errorf("ParseEmail(%q): unexpected error %v", tt.in, err)
			continue
		}
		if p.Canonical != tt.canonical || p.Preferred != tt.pref || p.Extra != tt.extra || p.Category != tt.cat {
			t.Errorf("ParseEmail(%q) = canonical %q pref %q extra %q category %v; want %q %q %q %v",
				tt.in, p.Canonical, p.Preferred, p.Extra, p.Category, tt.canonical, tt.pref, tt.extra, tt.cat)
		}
		if p.Disposable != (tt.cat == CategoryDisposable) || p.Relay != (tt.cat == CategoryRelay) {
			t.Errorf("ParseEmail(%q): Disposable=%v Relay=%v disagree with category %v", tt.in, p.Disposable, p.Relay, p.Category)
		}
	}
}

func TestParseEmailCaseSensitive(t *testing.T) {
	p, err := ParseEmail("John.Smith@Example.COM", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Canonical != "John.Smith@example.com" {
		t.Errorf("got %q", p.Canonical)
	}
}

func TestParseEmailInvalid(t *testing.T) {
	long := ""
	for i := 0; i < 65; i++ {
		long += "a"
	}
	for _, in := range []string{
		"", " ", "@mailinator.com", "a@", "a@b@c.com", "a b@c.com", "a\t@c.com",
		".a@c.com", "a.@c.com", "a..b@c.com", "a(b)@c.com", `"a"@c.com`, long + "@c.com",
		"a@localhost", "a@123", "a@1.2.3.4", "a@[1.2.3.4]", "a@x_y.com", "a@-x.com", "a@x-.com",
		"a@x..com", "a@.x.com", "a@x.com.", "a@x.c", "a@x.zzz", "a@x.local",
	} {
		if _, err := ParseEmail(in); err != ErrInvalidEmail {
			t.Errorf("ParseEmail(%q): want ErrInvalidEmail, got %v", in, err)
		}
	}
}

func TestClassifyDomain(t *testing.T) {
	tests := map[string]Category{
		"mailinator.com":      CategoryDisposable,
		"Sub.Mailinator.com.": CategoryDisposable,
		"duck.com":            CategoryRelay,
		"gmail.com":           CategoryNone,
		"co.uk":               CategoryNone,
		"not a domain":        CategoryNone,
	}
	for in, want := range tests {
		if got := ClassifyDomain(in); got != want {
			t.Errorf("ClassifyDomain(%q) = %v, want %v", in, got, want)
		}
	}
	if !IsDisposableDomain("mailinator.com") || IsDisposableDomain("duck.com") || !IsRelayDomain("duck.com") {
		t.Error("IsDisposableDomain / IsRelayDomain disagree with ClassifyDomain")
	}
}

func TestCandidatesStopAtRegistrableDomain(t *testing.T) {
	got := candidates("a.b.example.co.uk")
	want := []string{"a.b.example.co.uk", "b.example.co.uk", "example.co.uk"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestListSanity guards against a broken list update.
func TestListSanity(t *testing.T) {
	d, r, a := ListSizes()
	if d < 50000 || r < 20 || a < 50 {
		t.Fatalf("lists look truncated: disposable=%d relay=%d allow=%d", d, r, a)
	}
	for _, domain := range []string{
		"gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "yahoo.com", "icloud.com",
		"proton.me", "protonmail.com", "aol.com", "gmx.de", "web.de", "mail.ru", "yandex.ru",
		"qq.com", "163.com", "fastmail.com", "zoho.com", "tutanota.com", "hey.com",
	} {
		if c := ClassifyDomain(domain); c != CategoryNone {
			t.Errorf("major provider %s classified as %v", domain, c)
		}
	}
}

func BenchmarkParseEmail(b *testing.B) {
	_, _, _ = ListSizes()
	for i := 0; i < b.N; i++ {
		_, _ = ParseEmail("john.smith+tag@sub.mailinator.com")
	}
}
