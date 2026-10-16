# Grok Build Remote — MSIX / Microsoft Store packaging

Packages the Windows tray app (`gbr-tray.exe`, GUI subsystem) plus the
background agent (`gbr-agent.exe`) as an MSIX with a **desktop StartupTask**, so
the Store install auto-starts the tray at sign-in (no HKCU Run entry needed in
the packaged variant).

## Build + sign + WACK (on a Windows PC with the SDK + cert)

```powershell
# builds gbr-tray.exe / gbr-agent.exe first (repo root):
#   go build -ldflags "-H=windowsgui -X main.version=0.6.5" -o gbr-tray.exe ./cmd/gbr-tray
#   go build            -X main.version=0.6.5  -o gbr-agent.exe ./cmd/gbr-agent
packaging\msix\Build-Msix.ps1 -RunWack
```

- `makeappx pack` builds the `.msix` from `AppxManifest.xml` + `Assets\`.
- `signtool` signs with the `CN=Linespotting` code-signing cert (CurrentUser\My).
- `appcert` (WACK) validates; report written next to the MSIX.

## Identity / publisher

- Local sign + WACK: `Publisher="CN=Linespotting"` (matches the cert).
- Legal publisher display name: **Linespotting AB** (same as Cookie Cleaner).
- At Store reservation Partner Center assigns the final Package Identity
  (Name + Publisher). Update `AppxManifest.xml` `<Identity>` to the assigned
  values before uploading the Store MSIX.

## Privacy policy

https://linespotting.com/privacy-policy/

## Store listing

`store/<locale>/` holds the per-locale listing text, mirroring the Cookie Cleaner
layout, for all 32 locales. English (`store/en/`) is authored (~3.8k-char
description, short description, what's-new, keywords, copyright, license,
developer, privacy/support URLs). The 31 non-English locales currently carry the
English baseline plus a `_TRANSLATION-PENDING.txt` marker and still need
translation before submission.

Store assets (tiles/splash) are generated PNGs under `Assets\`. Listing
screenshots of the real tray/status UI are captured on the target PC (≥1366×768).

## Submission status / what is needed

Cookie Cleaner was submitted **manually in the Partner Center dashboard**
(browser + Microsoft account sign-in; "Submit = human confirm"), as a Win32 EXE
Package URL — not via an API. There are **no Partner Center / Microsoft Store
submission API credentials in the vault** (`_secrets` has Apple, Google Play,
GitHub, Cloudflare, Expo only). So the final steps require a human:

1. Reserve the product name in Partner Center (assigns Package Identity).
2. Update `<Identity>` to the assigned Name/Publisher and rebuild the MSIX.
3. Create the submission, upload the MSIX, paste the localized listings,
   upload the screenshots, set Privacy policy URL.
4. Submit for certification (human confirm).

These need Partner Center sign-in in a browser; they cannot be done headlessly
with the credentials currently available.
