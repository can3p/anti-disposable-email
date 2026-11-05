package disposable

import "strings"

// provider describes how a mailbox provider treats the local part.
type provider struct {
	// canonical is the domain that all of this provider's alias domains map
	// to, e.g. googlemail.com -> gmail.com.
	canonical string

	// tag is the sub-addressing separator: everything from the first tag
	// character onwards is ignored by the provider. Zero means none.
	tag byte

	// ignoreDots is true when the provider ignores '.' in the local part.
	ignoreDots bool
}

// providers lists the mailbox providers whose addressing rules we know.
// Domains not listed here are left alone apart from lower-casing, because a
// custom domain may treat '+' or '.' as part of a real address.
var providers = map[string]provider{}

func addProvider(p provider, domains ...string) {
	for _, d := range domains {
		providers[d] = p
	}
}

func init() {
	addProvider(provider{canonical: "gmail.com", tag: '+', ignoreDots: true},
		"gmail.com", "googlemail.com")

	// iCloud addresses share one namespace across the three domains.
	addProvider(provider{canonical: "icloud.com", tag: '+'},
		"icloud.com", "me.com", "mac.com")

	// Proton addresses share one namespace across these domains.
	addProvider(provider{canonical: "proton.me", tag: '+'},
		"proton.me", "protonmail.com", "protonmail.ch", "pm.me")

	// Microsoft domains are separate namespaces, so each keeps its own domain.
	for _, d := range []string{
		"outlook.com", "hotmail.com", "live.com", "msn.com", "passport.com",
		"outlook.co.uk", "outlook.de", "outlook.fr", "outlook.es", "outlook.it", "outlook.jp",
		"hotmail.co.uk", "hotmail.de", "hotmail.fr", "hotmail.es", "hotmail.it", "hotmail.nl", "hotmail.be", "hotmail.ca",
		"live.co.uk", "live.de", "live.fr", "live.it", "live.nl", "live.ca", "live.com.au",
	} {
		addProvider(provider{canonical: d, tag: '+'}, d)
	}

	// Yahoo's "disposable addresses" feature uses basename-keyword@. Yahoo
	// account names cannot contain '-', so the part before it is stable.
	for _, d := range []string{
		"yahoo.com", "yahoo.co.uk", "yahoo.de", "yahoo.fr", "yahoo.es", "yahoo.it",
		"yahoo.ca", "yahoo.com.au", "yahoo.co.jp", "yahoo.co.in", "ymail.com", "rocketmail.com",
	} {
		addProvider(provider{canonical: d, tag: '-'}, d)
	}

	for _, d := range []string{"fastmail.com", "fastmail.fm", "hey.com", "yandex.ru", "yandex.com", "ya.ru", "zohomail.com", "mailbox.org", "posteo.de", "posteo.net"} {
		addProvider(provider{canonical: d, tag: '+'}, d)
	}
}

// fastmailSubdomain handles Fastmail's subdomain addressing, where
// anything@user.fastmail.com is delivered to user@fastmail.com. It returns
// the user and base domain, or ok == false.
func fastmailSubdomain(domain string) (user, base string, ok bool) {
	for _, b := range []string{"fastmail.com", "fastmail.fm"} {
		if strings.HasSuffix(domain, "."+b) {
			user = strings.TrimSuffix(domain, "."+b)
			if user != "" && !strings.Contains(user, ".") {
				return user, b, true
			}
		}
	}
	return "", "", false
}
