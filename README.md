# Anti Disposable Email Address Checker for Go

Detects sign-ups from disposable email services and normalizes addresses so
one mailbox can't register many accounts. Everything runs offline: the domain
lists are embedded in the package, so there are no network calls and no
dependencies besides `golang.org/x/net`.

This is a fork of
[rocketlaunchr/anti-disposable-email](https://github.com/rocketlaunchr/anti-disposable-email).

**Blocking disposable addresses is a business decision.** It stops low-effort
abuse (throwaway sign-ups, repeat free trials), not a determined attacker. See
[What's missing](#whats-missing) before relying on it.

## Installation

Requires Go 1.26 or later.

```bash
go get -u github.com/can3p/anti-disposable-email
```

## Usage

```go
import disposable "github.com/can3p/anti-disposable-email"

p, err := disposable.ParseEmail("J.O.H.N+trial3@googlemail.com")
if err == disposable.ErrInvalidEmail {
	// reject: not a syntactically valid address
}

switch {
case p.Disposable:
	// reject: throwaway mailbox
case p.Relay:
	// your call: forwarding/alias service (DuckDuckGo, SimpleLogin, Apple Hide My Email, ...)
}

// Store p.Canonical with a unique index to stop duplicate accounts.
```

Result:

```text
Email:           "J.O.H.N+trial3@googlemail.com"
LocalPart:       "J.O.H.N+trial3"
Preferred:       "J.O.H.N"
Extra:           "trial3"
Normalized:      "john"
Domain:          "googlemail.com"
CanonicalDomain: "gmail.com"
Canonical:       "john@gmail.com"
Category:        CategoryNone
Disposable:      false
Relay:           false
```

| Field | Meaning |
|---|---|
| `Email` | The input, trimmed |
| `LocalPart` | Everything before `@`, unchanged |
| `Preferred` | Local part without the sub-address tag, in the user's casing |
| `Extra` | The sub-address tag the provider ignores (`trial3` above) |
| `Normalized` | Local part as the provider sees it: tag removed, dots removed for Gmail, lower-cased |
| `Domain` | Lower-cased, punycode domain |
| `CanonicalDomain` | Domain with provider aliases resolved (`googlemail.com` → `gmail.com`) |
| `Canonical` | `Normalized@CanonicalDomain`: a uniqueness key, **not an address to send mail to** |
| `Category`, `Disposable`, `Relay` | Domain classification, see below |

Pass `true` as the second argument to `ParseEmail` to keep the local part's
case in `Normalized`.

To check a domain on its own:

```go
disposable.ClassifyDomain("foo.mailinator.com") // CategoryDisposable
disposable.IsDisposableDomain("mailinator.com") // true
disposable.IsRelayDomain("duck.com")            // true
```

## What it checks

* **Syntax.** The local part must be an unquoted dot-atom of at most 64
  octets. Non-ASCII characters are allowed (RFC 6531). The domain must have at
  least two labels of `a-z`, `0-9` and `-`, and a top-level domain that is in
  the public suffix list. Unicode domains are converted to punycode. The whole
  address must be at most 254 octets.
* **Disposable domains.** About 99,000 domains, merged from five public lists
  (see [Data sources](#data-sources)). Parent domains match too, so
  `x@anything.mailinator.com` is caught. Matching stops at the registrable
  domain, so a list entry can never block a whole suffix like `co.uk`.
* **Relay domains.** About 60 forwarding and alias services, reported as
  `Relay` rather than `Disposable`. These are usually real people protecting
  their privacy.
* **Allowlist.** Major mailbox providers and known upstream false positives
  are never flagged, whatever the upstream lists say.
* **Provider normalization:**

  | Provider | Domains | Rules |
  |---|---|---|
  | Gmail | gmail.com, googlemail.com → gmail.com | `+tag` removed, dots ignored |
  | iCloud | icloud.com, me.com, mac.com → icloud.com | `+tag` removed |
  | Proton | proton.me, protonmail.com, protonmail.ch, pm.me → proton.me | `+tag` removed |
  | Microsoft | outlook.\*, hotmail.\*, live.\*, msn.com | `+tag` removed |
  | Yahoo | yahoo.\*, ymail.com, rocketmail.com | `-keyword` removed |
  | Fastmail | fastmail.com, fastmail.fm | `+tag` removed; `x@user.fastmail.com` → `user@fastmail.com` |
  | Others | hey.com, yandex, zohomail.com, mailbox.org, posteo | `+tag` removed |

  Other domains are only lower-cased, because a custom domain may use `+` or `.`
  in real, distinct addresses.

## What's missing

The package only looks at the address text and compares it with a list that is
compiled in. These gaps are known and deliberate. Each one says what a
dependent project has to do itself if the gap matters to it.

| Gap | Consequence | What to do in your project |
|---|---|---|
| **No DNS lookups.** There is no MX check and no check that the domain exists. | A domain registered by a disposable service after the last list update passes. So do typos like `gmail.co` and domains that can't receive mail. | Send a confirmation email and only activate the account once the link is clicked. If you need more, look up MX records yourself (`net.LookupMX`) and reject domains with none. |
| **No detection by mail server.** Disposable services rotate domains but keep a few mail servers. Matching on those servers would catch new domains. | Recall on fresh disposable domains is limited to what the upstream lists already know. | Not solvable without DNS. If disposable sign-ups become a real problem, add an MX-host check or use a paid verification API. [docs/PROPOSAL-10x.md](docs/PROPOSAL-10x.md) describes the design (layer 3). |
| **No domain-age or reputation checks, no heuristics, no score.** | A brand-new domain, or a random-looking address like `xk7q9z@newdomain.xyz`, is not flagged unless listed. | Combine this check with your own signals: rate limiting, CAPTCHA, payment or phone verification. |
| **The list is frozen at build time.** There is no runtime refresh. | Your binary only knows the domains from the version you compiled against. | Run `go get -u github.com/can3p/anti-disposable-email` and redeploy regularly, for example monthly after the list-update PR lands here. |
| **No custom allow/block lists in the API.** | You can't override a classification through the package. | Check your own allowlist before calling `ParseEmail` and your own blocklist after it. To fix a wrong classification for everyone, add the domain to `data/allowlist_local.txt` here. |
| **Normalization covers only the providers in the table above.** | Google Workspace and other custom domains are not normalized, because their rules can't be known from the address. Two variants of the same mailbox on such a domain get different `Canonical` keys. | Accept this, or apply your own per-domain rules on top. |
| **Not supported:** quoted local parts (`"john doe"@x.com`), IP-literal domains (`x@[1.2.3.4]`), single-label or internal domains (`x@localhost`, `x@corp.local`). | `ParseEmail` returns `ErrInvalidEmail`. | This is fine for public sign-ups. Don't use this package to validate internal addresses. |
| **Relay services are not blocked.** | `x@duck.com` passes as `Disposable == false`. | Check `p.Relay` and decide. Blocking relays rejects privacy-conscious real users. |
| **Lists can contain false positives.** The upstream lists are aggregated with only light filtering. | A small provider may be wrongly reported as disposable. | Log rejections. If a real provider is affected, add it to `data/allowlist_local.txt` and open a PR. |

## Upgrading from the previous version

These changes can break dependent projects:

* **Go 1.26 or later is required.** The latest `golang.org/x/net`, which
  supplies the public suffix list, needs it.
* **`DisposableList` (the exported map) is removed.** Use `IsDisposableDomain`
  or `ClassifyDomain`. They also match subdomains, which a map lookup did not.
* **The `update` package and `scripts/update.go` are removed.** Use
  `go run ./cmd/update-lists`. It needs only the standard library, so `go-git`
  is no longer in your `go.sum`.
* **Validation is stricter.** Addresses that `ParseEmail` and `ValidateDomain`
  accepted before are now rejected, for example: an empty local part,
  consecutive dots, `_` in the domain, single-label domains, numeric or unknown
  TLDs, and anything over the length limits.
* **`Normalized` changes for more providers.** It was only applied to
  `gmail.com` before. If you stored `Normalized` as a uniqueness key for other
  providers, recompute it for existing rows before relying on the new values.
  Better, switch to `Canonical`: it includes the domain and merges alias
  domains such as googlemail.com and gmail.com.
* **Some domains moved out of `Disposable`.**
  * Now `Relay`: e4ward.com, jetable.org, mailexpire.com, mailnull.com,
    opayq.com, spamex.com.
  * Now allowed (upstream false positives): xwaretech.com, xwaretech.info,
    xwaretech.net.

## Keeping the list up to date

`.github/workflows/update-lists.yml` runs monthly, regenerates `data/*.txt`,
runs the tests against the new lists and opens a pull request. It needs
**Settings → Actions → General → Workflow permissions → Allow GitHub Actions
to create and approve pull requests** to be enabled. After merging, tag a
release so that dependents pick it up with `go get -u`.

To update by hand:

```bash
go run ./cmd/update-lists
```

The updater aborts if a source returns implausibly few domains, or if the
disposable list would shrink by more than 20%. Pass `-force` to override the
second check.

## Data sources

The lists are merged from
[disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains) (CC0),
[disposable/disposable-email-domains](https://github.com/disposable/disposable-email-domains) (MIT),
[FGRibreau/mailchecker](https://github.com/FGRibreau/mailchecker) (MIT),
[wesbos/burner-email-providers](https://github.com/wesbos/burner-email-providers) (MIT) and
[7c/fakefilter](https://github.com/7c/fakefilter) (BSD-3-Clause).
[data/SOURCES.md](data/SOURCES.md) describes how each file is built.

## Legal Information

The license is a modified MIT license. Refer to `LICENSE` file for more details.

**© 2020-22 PJ Engineering and Business Solutions Pty. Ltd.**
