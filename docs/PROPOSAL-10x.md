# Proposal: make anti-disposable-email 10x more effective

Status: draft · Date: 2026-11-05

## 1. Where we are today

The package does one check: an exact `map[domain]` lookup against a single
compiled-in list. I measured it against the public blocklists and probed its
behaviour.

### The list is narrow and out of date

| Source | Domains | Covered by our `list.go` |
|---|---:|---:|
| our `list.go` (last regenerated 2022) | 3,519 | n/a |
| disposable-email-domains/disposable-email-domains (our own upstream, today) | 10,143 | 3,519 (35%) |
| disposable/disposable-email-domains | 98,896 | 3,503 (3.5%) |
| FGRibreau/mailchecker | 57,702 | 3,443 (6%) |
| wesbos/burner-email-providers | 27,277 | 3,494 (13%) |
| 7c/fakefilter (tracks newly seen domains) | 4,808 | 162 (3.4%) |
| **Union of all five** | **99,049** | **3.6%** |

Just updating to the union gives about **28x** the domain coverage. A list
still goes stale within weeks, though, because disposable providers rotate
domains on purpose. Fakefilter shows this: it tracks domains that are active
now, and we cover 3.4% of it.

### The check is easy to get around (probed on `main`)

| Input | Result | Problem |
|---|---|---|
| `a@foo.mailinator.com` | not disposable | Subdomains are not matched. Many providers hand out `*.domain` addresses. |
| `@mailinator.com` | **valid**, disposable | An empty local part is accepted. |
| `a@localhost`, `a@123` | valid | Single-label and numeric domains are accepted. |
| `a@x_y.com` | valid | `_` is not legal in hostnames. |
| `j.o.h.n+x@googlemail.com` | normalized to `j.o.h.n+x` | Only `gmail.com` is normalized. `googlemail.com`, Outlook/Hotmail, iCloud, Proton, Fastmail, Yahoo (`-`) and others are not. |
| a fresh disposable domain, created yesterday | not disposable | Nothing looks past the domain name. |

Other issues:

* `toLower` builds the string by concatenating in a loop, which is O(n²).
  `strings.ToLower` does the same job.
* `go-git` is pulled into the library module only for the updater, so every
  consumer gets it in `go.sum`.
* The doc comment still points at the old `martenson/...` repository.
* The result is a single bool. Callers can't tell a forwarding service such as
  `duck.com` (which some lists block and others don't) from Mailinator, and
  they can't tune strictness.
* There are no tests.

## 2. Goals and how we measure "10x"

We define effectiveness as **recall on disposable signups at a fixed
false-positive rate**, measured with a benchmark kept in the repo:

* **Positive set:** domains seen by fakefilter in the last 90 days, plus
  domains taken from live temp-mail sites (a snapshot of their public domain
  pickers). These are held out from the list we ship.
* **Negative set:** the top 10k email domains by real-world use (the
  Tranco-ranked MX-bearing domains, ISP mail, and big webmail providers), plus
  the disposable-email-domains `allowlist.conf`.
* **Target:** at least 10x the current recall on the held-out positive set,
  with false positives on the negative set at or below 0.1%.

The list alone cannot reach this, because held-out domains are by definition
not on the list. The infrastructure signals in section 3, layer 3 are what
catch those.

## 3. The proposal: layered detection

Each layer is cheap to run before the next, slower one. The offline layers run
on every call. The network layers are opt-in.

### Layer 1: correct parsing and normalization (offline, no-brainer)

* Validate the local part: it must be non-empty, at most 64 octets, and contain
  only allowed characters. The total address must be at most 254 octets.
* Validate the domain properly: labels of 1 to 63 characters, letters, digits
  and `-` only (no `_`), at least two labels, a TLD that is not all digits, and
  a TLD that exists in the public suffix list (`golang.org/x/net/publicsuffix`).
* Use a provider table for normalization (sub-addressing separator, whether
  dots are ignored, and domain aliases):
  * gmail.com and googlemail.com: `+`, dots ignored, normalized to gmail.com
  * outlook.com, hotmail.\*, live.\*: `+`
  * icloud.com, me.com, mac.com: `+`
  * proton.me, protonmail.com, pm.me: `+`, all normalized to the same domain
  * yahoo.\*: `-`
  * fastmail: `+`, plus subdomain addressing (`x@user.fastmail.com`)
  * Add a `Canonical` field (normalized local part + canonical domain) so
    callers can deduplicate accounts.
* Use `strings.ToLower`.

### Layer 2: a broader, smarter, fresher list (offline)

* **Aggregate sources:** the five lists above, with each domain tagged by the
  sources that list it. Store the result as an embedded, gzip-compressed
  sorted text file loaded at init (via `go:embed`). A 100k-entry map literal
  would slow compilation badly. Embedded size is about 600 KB gzipped.
* **Apply allowlists:** use disposable-email-domains' allowlist and our own
  `allowlist.txt`. Major providers must never be flagged.
* **Categories instead of a bool:** `disposable`, `forwarding/relay` (duck.com,
  SimpleLogin, addy.io, Firefox Relay, iCloud Hide My Email), and
  `free-webmail`. Sources disagree about relays (wesbos and mailchecker list
  `duck.com`; others don't), so we keep relays as a separate category and let
  the caller decide.
* **Confidence from agreement:** a domain listed by 3 or more independent
  sources is high confidence. One listed by a single source is medium.
* **Suffix matching:** walk up the labels against the list, so
  `foo.mailinator.com` matches `mailinator.com`. Stop at the registrable
  domain from the public suffix list, so that blocking `xyz.co.uk` does not
  block all of `co.uk`.
* **Freshness:**
  * A GitHub Action regenerates the list daily and opens a PR or tags a
    release, so `go get -u` picks it up.
  * An optional runtime updater (`checker.WithRemoteList(url, interval)`)
    reloads from a URL we publish (GitHub release asset, ETag-cached) without
    a redeploy. The embedded list stays as the fallback.

### Layer 3: infrastructure signals (opt-in, network, cached)

This layer catches domains that are not on any list yet. Disposable providers
rotate domains all the time, but they rarely rotate their mail servers.

* **MX host blocklist:** resolve MX records and match the hosts (and their IPs
  or /24 ranges) against known disposable mail infrastructure. Mailinator,
  Guerrilla Mail, temp-mail, 10minutemail and friends each serve thousands of
  domains from a handful of MX hosts. We build the MX blocklist automatically:
  resolve MX for every domain in the aggregated list, and any MX host serving
  N or more listed domains goes on the MX blocklist. In my experience this is
  the single biggest recall win.
* **NS blocklist:** the same idea applied to authoritative nameservers. It
  catches providers that park throwaway domains behind the same DNS.
* **Deliverability:** flag a domain with no MX record and no A/AAAA fallback,
  or a null MX (`MX 0 .`, RFC 7505), as `undeliverable`.
* **Domain age (RDAP):** a domain registered less than 30 days ago is a strong
  risk signal (opt-in, because it is slow; cache it for a long time).
* **Caching and timeouts:** use a pluggable `Cache` interface with an
  in-memory LRU by default and TTLs that respect DNS. Every lookup is
  `context`-aware with a strict timeout. If a lookup fails, we fall back to
  the offline verdict and never fail the call.

### Layer 4: heuristics (offline, low weight)

* Signals from the domain name itself: tokens such as `temp`, `trash`, `10min`
  or `throwaway`, high digit ratio, high entropy, and free or abused TLDs (`.tk`,
  `.ml`, `.ga`, `.cf`, `.gq`).
* Signals from the local part: a random-looking string (entropy or consonant
  runs), and the long-random-string pattern that temp-mail sites generate.
* These only add to a score. They never flag an address on their own.

## 4. API (backwards compatible)

`ParseEmail` and `ParsedEmail.Disposable` keep working, now backed by layers 1
and 2. The new API:

```go
c := disposable.NewChecker(
    disposable.WithDNS(true),                 // layer 3: MX, NS, deliverability
    disposable.WithRDAP(false),
    disposable.WithRemoteList(disposable.DefaultListURL, 24*time.Hour),
    disposable.WithAllow("mycompany.com"),
    disposable.WithBlock("competitor-trash.io"),
    disposable.WithCache(myRedisCache),
)

v, err := c.Check(ctx, "j.o.h.n+x@googlemail.com")
// v.Parsed.Canonical == "john@gmail.com"
// v.Category        == disposable.CategoryNone | Disposable | Relay | FreeWebmail
// v.Score           == 0..100
// v.Reasons         == []Reason{{Code: "mx_blocklisted", Detail: "mx.mailinator.com"}, ...}
// v.Undeliverable   == false
```

Returning `Reasons` matters for product teams: they can show "please use a
permanent address", log why an address was blocked, and handle relays
separately.

Repository layout changes:

* Move the updater to its own module (`./cmd/update` with its own `go.mod`), so
  the library depends only on `golang.org/x/net`.
* Ship the data in `data/domains.txt.gz`, `data/mx.txt` and `data/allow.txt`,
  each with source attribution.
* Raise `go` to 1.21 or later (needed for `go:embed` and `slices`).

## 5. Roadmap

| Phase | Scope | Expected impact |
|---|---|---|
| **1. Quick wins** (1 to 2 days) | Fix the parsing bugs, provider normalization table, suffix matching, aggregate the five lists plus the allowlist, embed as gzip, add tests, add the daily update Action | About 28x domain coverage; subdomain bypass closed |
| **2. Checker API** (2 to 3 days) | `Checker`, `Verdict`, categories, confidence, custom allow/block lists, remote list refresh | No-redeploy freshness; relays separated from disposable |
| **3. Infrastructure** (3 to 5 days) | MX and NS blocklist generation in CI, DNS checks with caching, null-MX and no-MX detection | Catches newly rotated domains that no list has yet |
| **4. Scoring** (2 days) | Heuristics, optional RDAP domain age, benchmark harness plus a CI recall and false-positive report | Measurable, tunable precision and recall |

## 6. Risks

* **False positives from aggressive lists:** handled by the allowlist,
  confidence tiers, a separate relay category, and the false-positive gate in
  the benchmark.
* **Licensing:** every source must be checked before it is bundled. The current
  ones are MIT, CC0 or similar. Record attribution in `data/SOURCES.md`.
* **DNS latency and abuse:** network checks are opt-in, have timeouts, are
  cached, and fall back to the offline verdict.
* **Binary size:** about 600 KB gzipped for 100k domains. For anyone who cares,
  a `-tags nolist` build can drop the embedded list and rely only on the
  remote list.
