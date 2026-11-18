# 0005. Sign the config and make the elevated enforcer the only signer

- **Status:** Accepted
- **Date:** 2026-11-03 (HMAC signing since 2025-12; the `%ProgramData%` copy added 2026-11-18)

## Context

The config (block lists, schedules, the lock end time) is a JSON file in the user's
profile. If the user can edit it, they can end a session by changing one field. Signing
only helps if the user can't read the secret. A per-user secret in `HKCU` or a file can be
read by the same user who wants to cheat, and so can anything the medium-integrity UI
process can read.

## Decision

- The config is **HMAC-SHA256 signed** and written atomically (temp file and rename), with
  a signed `.bak` and a registry backup of the lock timing. A config that fails
  verification is rejected.
- **Strong mode.** The one-time elevated setup stores the secret in
  `HKLM\SOFTWARE\FocusLock\Security`, with an ACL that allows only SYSTEM and
  Administrators. Only the elevated enforcer can read it, so **the enforcer is the only
  signer**. The UI can't sign (`Store.CanSign() == false`). It sends each change as a
  request file, and the enforcer validates and commits it.
- In strong mode the enforcer also writes a signed copy to **`%ProgramData%\FocusLock`**
  (admins only) first, and trusts that copy first on load. It rewrites the profile files
  from it, so deleting the profile files or replaying an older signed copy doesn't work.
- **Reduced mode** is the fallback when no HKLM secret exists, for example on a
  standard-user account or when setup didn't run. A per-user `HKCU` secret is used and the
  UI signs directly. The app still works but is easier to tamper with.

## Alternatives considered

- **No signing, just hide the file.** Trivial to defeat.
- **Secret in `HKCU` or embedded in the binary.** Readable by the user. A secret in the
  binary can be pulled out once and works for every install.
- **DPAPI.** Protects the secret from other users, not from the user it's protecting
  against.
- **Store the config only in an admin-only location.** The UI couldn't read its own
  settings without elevation. A signed profile copy plus an admin-only authoritative copy
  keeps reads simple.

## Consequences

- In strong mode the UI must never call `Store.Save`. Every change goes through the request
  channel, which handles one request at a time per process.
- Anything that writes the config, including stats, has to handle `CanSign() == false`.
- Strong mode assumes the signed-in user is a local administrator who approves UAC.
  Standard users get reduced mode.
- An admin can still read the secret, which is accepted and outside the threat model.
