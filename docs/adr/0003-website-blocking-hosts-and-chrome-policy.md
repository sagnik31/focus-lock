# 0003. Block websites with the `hosts` file plus Chrome's `URLBlocklist` policy

- **Status:** Accepted
- **Date:** 2026-11-17 (the `hosts` layer dates from 2025-12; the Chrome policy layer was added then)

## Context

Website blocking has to cover every browser, including private windows, without asking the
user to install anything per browser. Blocking with the `hosts` file alone leaks in two
ways. Chrome's Secure DNS (DNS over HTTPS) resolves names without reading `hosts`, and sites
with offline support can load from cache.

## Decision

- **`hosts` file as the base layer.** During a session, blocked domains and their common
  subdomains (`www.`, `m.` and known popular-site hosts, matched on a dot boundary) are
  written between `#### FOCUS LOCK ####` markers, then DNS is flushed. Everything outside
  the markers is left alone.
- **Chrome `URLBlocklist` policy as a second layer.** During a session the same sites are
  written to `HKLM\SOFTWARE\Policies\Google\Chrome\URLBlocklist`. Focus Lock records which
  entries it owns and removes only those when the session ends. Entries the user already
  had stay.
- **Leave Secure DNS alone.** The policy layer covers it, so there's no need to change the
  user's DNS settings.
- Both layers are reapplied every five seconds by the enforcer, so manual edits are undone.

## Alternatives considered

- **Local proxy or PAC file.** Can block by URL, but HTTPS needs a trusted root
  certificate, and apps can ignore the system proxy.
- **Browser extensions.** Precise, but there's one per browser, they can be disabled in
  seconds, and they don't load in private windows by default.
- **Windows Filtering Platform or firewall rules.** Work on IP addresses, which are shared
  and churn across CDNs. Very easy to over- or under-block.
- **Changing DNS servers or turning off Secure DNS.** Interferes with the user's network
  setup beyond the session and is easy to reverse.

## Consequences

- Blocking applies system-wide, and in Chrome it holds against Secure DNS and cached pages.
- Chrome shows "Managed by your organization" while a session is active.
- A tab already open on a blocked site keeps its page until it is navigated or refreshed.
- Other Chromium browsers (Edge, Brave) and Firefox use only the `hosts` layer. Each would
  need its own policy key to get the second layer.
- Editing `hosts` and HKLM needs admin rights, so this runs in the elevated enforcer.
