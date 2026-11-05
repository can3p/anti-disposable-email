// Copyright 2020-22 PJ Engineering and Business Solutions Pty. Ltd. All rights reserved.

package disposable

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// ErrInvalidEmail is returned if the email address is invalid.
var ErrInvalidEmail = errors.New("invalid email")

// ParsedEmail returns a parsed email address.
//
// An email address is made up of 3 components: <local-part>@<domain>.
// The local-part is case-sensitive according to the specs, but most
// (if not all) reputable email services will treat it as case-insensitive.
// The domain is case-insensitive.
type ParsedEmail struct {
	// Email represents the input email (after white-space has been trimmed).
	Email string

	// Preferred represents the local-part in the way the user seems to prefer it,
	// without any sub-address tag. For example if the local-part is
	// case-insensitive, the user may prefer their email address all upper-case
	// even if it does not matter.
	Preferred string

	// Normalized represents the local-part normalized such that it can be
	// compared for uniqueness within CanonicalDomain.
	//
	// For gmail, since john.smith@gmail.com, johnsmith@gmail.com, and JohnSmith+x@gmail.com
	// are all equivalent, the normalized local-part is 'johnsmith'.
	Normalized string

	// Extra represents the sub-address tag that the provider ignores, for the
	// providers whose rules are known (see providers.go).
	//
	// adam+junk@gmail.com => adam@gmail.com (Extra: junk)
	Extra string

	// Disposable is true if the domain (or a parent domain) belongs to a
	// disposable email service. Relay services are not counted; see Relay.
	Disposable bool

	// Relay is true if the domain (or a parent domain) belongs to an email
	// forwarding or alias service such as DuckDuckGo, SimpleLogin or
	// Apple Hide My Email.
	Relay bool

	// Category is the domain's category. It agrees with Disposable and Relay.
	Category Category

	// Domain represents the component after the '@' character.
	// It is lower-cased and converted to punycode.
	Domain string

	// CanonicalDomain is Domain with provider aliases resolved, e.g.
	// googlemail.com becomes gmail.com and pm.me becomes proton.me.
	CanonicalDomain string

	// Canonical is Normalized@CanonicalDomain. Store it with a unique index to
	// stop one person registering many accounts with variants of one mailbox.
	// It is an identity key, not an address to send mail to: use Email for that.
	Canonical string

	// LocalPart represents the component before the '@' character.
	LocalPart string
}

// ParseEmail parses a given email address. Set caseSensitive to true if you want the local-part
// to be considered case-sensitive. The default value is false.
//
// Validation is syntactic only: it does not check that the domain exists or
// accepts mail. Quoted local-parts and IP-literal domains are rejected.
func ParseEmail(email string, caseSensitive ...bool) (ParsedEmail, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return ParsedEmail{}, ErrInvalidEmail
	}

	invalid := ParsedEmail{Email: email}

	if strings.IndexFunc(email, unicode.IsSpace) >= 0 {
		return invalid, ErrInvalidEmail
	}

	cs := len(caseSensitive) > 0 && caseSensitive[0]

	splits := strings.Split(email, "@")
	if len(splits) != 2 {
		return invalid, ErrInvalidEmail
	}
	localPart := splits[0]

	if !validLocalPart(localPart) {
		return invalid, ErrInvalidEmail
	}

	domain, err := idna.ToASCII(strings.ToLower(splits[1]))
	if err != nil {
		return invalid, ErrInvalidEmail
	}

	if !ValidateDomain(domain) {
		return invalid, ErrInvalidEmail
	}

	// RFC 5321: a path is at most 256 octets including the angle brackets.
	if len(localPart)+1+len(domain) > 254 {
		return invalid, ErrInvalidEmail
	}

	p := ParsedEmail{
		Email:     email,
		Domain:    domain,
		LocalPart: localPart,
	}

	p.Normalized, p.Preferred, p.Extra, p.CanonicalDomain = normalize(localPart, domain, cs)
	p.Canonical = p.Normalized + "@" + p.CanonicalDomain

	p.Category = classify(domain)
	p.Disposable = p.Category == CategoryDisposable
	p.Relay = p.Category == CategoryRelay

	return p, nil
}

func normalize(localPart, domain string, caseSensitive bool) (norm, pref, extra, canonDomain string) {
	pref = localPart
	canonDomain = domain
	norm = localPart

	if user, base, ok := fastmailSubdomain(domain); ok {
		// anything@user.fastmail.com is delivered to user@fastmail.com.
		pref, extra, canonDomain, norm = user, localPart, base, user
	} else if p, ok := providers[domain]; ok {
		canonDomain = p.canonical
		if p.tag != 0 {
			// A local part starting with the tag character is left whole.
			if i := strings.IndexByte(localPart, p.tag); i > 0 {
				pref, extra = localPart[:i], localPart[i+1:]
				norm = pref
			}
		}
		if p.ignoreDots {
			norm = strings.ReplaceAll(norm, ".", "")
		}
	}

	if !caseSensitive {
		norm = strings.ToLower(norm)
	}
	return
}

// atext from RFC 5322, plus '.' which is checked separately.
const atextSpecials = "!#$%&'*+-/=?^_`{|}~."

// validLocalPart checks an unquoted (dot-atom) local part. Non-ASCII
// characters are allowed for internationalized addresses (RFC 6531).
func validLocalPart(s string) bool {
	if s == "" || len(s) > 64 || !utf8.ValidString(s) {
		return false
	}
	if s[0] == '.' || s[len(s)-1] == '.' || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r < utf8.RuneSelf:
			if !strings.ContainsRune(atextSpecials, r) {
				return false
			}
		default:
			if !unicode.IsPrint(r) || unicode.IsSpace(r) {
				return false
			}
		}
	}
	return true
}

// ValidateDomain returns true if the domain component of an email address is valid.
// domain must be already lower-case and white-space trimmed. For domains
// containing unicode characters, you must perform punycode conversion
// beforehand. See: https://godoc.org/golang.org/x/net/idna#ToASCII
//
// The domain must be a hostname of at least two labels made of a-z, 0-9 and
// '-', and its top-level domain must be in the public suffix list. The check
// is syntactic: it does not look up DNS.
func ValidateDomain(domain string) bool {
	if domain == "" || len(domain) > 253 {
		return false
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}

	for _, l := range labels {
		if len(l) == 0 || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}

	return knownTLD(labels[len(labels)-1])
}

func knownTLD(tld string) bool {
	if strings.Trim(tld, "0123456789") == "" {
		return false
	}
	// Probe with a second label: TLDs listed only through a wildcard rule
	// such as *.ck are not reported as ICANN suffixes on their own.
	_, icann := publicsuffix.PublicSuffix("x." + tld)
	return icann
}
