# PACKAGING — building and deploying the agent

Phase 8. How the `aiul` binary becomes something a fleet of Macs can be given, and
what still has to happen before that is true.

**Status, 2026-09-19: everything here works except the signature.** There is no
Apple Developer account yet, so the package is unsigned. Read
[What signing changes](#what-signing-changes) before planning a pilot.

## Build

```sh
./scripts/build.sh            # version from git describe
AIUL_VERSION=0.8.0 ./scripts/build.sh
```

Produces `dist/aiul`, a **universal binary**: one file containing both the Apple
Silicon and the Intel build, joined by `lipo`. macOS picks the right half at
launch, so one package covers every Mac.

The version is stamped into the binary with the linker, so a machine can be asked
what it is running:

```sh
aiul version      # aiul 0.8.0 (darwin/arm64, go1.24.4)
```

## Package

```sh
AIUL_VERSION=0.8.0 ./scripts/package.sh
```

Produces `dist/aiul-0.8.0.pkg`. The payload is one file, `/usr/local/bin/aiul`.
Everything else is done by `packaging/scripts/postinstall`, which runs as root
after the file is placed:

1. makes sure the device has a CA the agent can use (`aiul ca ensure`): a usable
   one is kept, so a reinstall does not invalidate certificates already trusted
   here; one that cannot issue this device's signing certificate is replaced
2. runs `aiul install --apply --yes` — the same code path a person runs by hand,
   and the same one `aiul uninstall` reverses

Deployment therefore has no second implementation that can drift from the manual
one. Its log is `/var/log/aiul-install.log`.

### Install, check, remove

```sh
sudo installer -pkg dist/aiul-0.8.0.pkg -target /
aiul status
sudo aiul uninstall              # or sudo ./scripts/killswitch.sh
```

`installer` needs no signature. A double-click does, which is the first thing
signing buys.

### On an unmanaged Mac

`aiul install` refuses to run on a device with no MDM enrollment, by design, and
the postinstall inherits that. **`installer` does not pass its environment to
package scripts**, so putting the override on the command line does nothing:

```sh
# THIS DOES NOT WORK — the variable never reaches the postinstall
sudo AIUL_DEV_ALLOW_UNMANAGED=1 installer -pkg dist/aiul-0.8.0.pkg -target /
```

Use the marker file instead. It is in `/etc`, so it needs root and cannot appear
by accident on a fleet, and `scripts/killswitch.sh` removes it:

```sh
sudo touch /etc/aiul-dev-unmanaged
sudo installer -pkg dist/aiul-0.8.0.pkg -target /
sudo rm /etc/aiul-dev-unmanaged        # when finished testing
```

The postinstall log records loudly that the check was skipped.

## Deploying from an MDM

The package is a standard component package, so every MDM can deliver it —
Jamf, Kandji, Mosyle, Intune, or `InstallEnterpriseApplication` over raw MDM.
Two requirements are not negotiable:

- **the package must be signed** with a Developer ID Installer certificate. MDMs
  refuse unsigned packages, and this one is unsigned today.
- **the device must be enrolled**, which the agent verifies for itself with
  `profiles status -type enrollment`. That check is the consent boundary of the
  whole product: it intercepts traffic on company devices and refuses on personal
  ones.

### Why there is no configuration profile here

A profile carrying the CA to trust would be the obvious companion, and it is
deliberately absent: **each device still provisions its own root**, so no single
profile can carry the right certificate to trust.

The device half of D3 is built (D14): the root signs a short-lived,
name-constrained intermediate per device, and every certificate the proxy mints
comes from that intermediate. `aiul ca device` shows it. What is still missing is
the other half — one root per tenant, its key in KMS — and that needs an AWS
account. Once it exists, the profile becomes obvious and worth writing: it carries
that tenant root, the same one on every device in the fleet.

Until then the trust step is done locally by `aiul install`, which adds the
device's own root to that device's System keychain.

## EDR and security tooling

The agent will look suspicious to anything watching for credential theft, because
structurally it is doing the same thing: minting certificates and reading TLS
traffic. Tell the EDR about it before deployment, not after an alert.

| What | Value |
| --- | --- |
| Binary | `/usr/local/bin/aiul` |
| Daemons | `com.aiul.helper` (root), `com.aiul.agent` (runs as `_aiul`) |
| Plists | `/Library/LaunchDaemons/com.aiul.{helper,agent}.plist` |
| Listener | `127.0.0.1:8899`, loopback only |
| Helper socket | `/var/run/aiul-helper.sock`, `root:_aiul`, mode 0660 |
| State | `/var/db/aiul` (CA copy, spool), owned by `_aiul` |
| Logs | `/var/log/aiul/`, `/var/log/aiul-install.log` |
| Service account | `_aiul`, uid/gid 448, no shell, no home |
| System changes | HTTPS proxy on every network service; a marked block in `/etc/zshenv`; a login LaunchAgent setting the same variables for GUI apps |

Expect alerts on: a new trusted root in the System keychain, a process reading
`/etc/zshenv`, and a local proxy listener. All three are the product working.

## What signing changes

Without a Developer ID, three things are true and none of them are acceptable for
a pilot:

1. **Gatekeeper warns.** A double-clicked package is refused with "unidentified
   developer". `sudo installer` still works, which is how it is tested today.
2. **MDMs refuse it.** No fleet deployment is possible.
3. **No notarization.** Apple has not scanned the binary, and future macOS
   releases are steadily less willing to run unnotarized code.

The scripts are already written for it. Once there is an account, signing is two
environment variables and one extra command — no edit to either script:

```sh
# 1. The binary, with the hardened runtime that notarization requires
export AIUL_SIGN_IDENTITY="Developer ID Application: Your Company (TEAMID)"
./scripts/build.sh

# 2. The installer package
export AIUL_INSTALLER_IDENTITY="Developer ID Installer: Your Company (TEAMID)"
AIUL_VERSION=0.8.0 ./scripts/package.sh

# 3. Notarize and staple — the only step with no script yet, because it needs
#    credentials that must not live in this repo
xcrun notarytool store-credentials aiul-notary \
  --apple-id you@example.com --team-id TEAMID --password APP-SPECIFIC-PASSWORD
xcrun notarytool submit dist/aiul-0.8.0.pkg --keychain-profile aiul-notary --wait
xcrun stapler staple dist/aiul-0.8.0.pkg
spctl --assess --type install -vv dist/aiul-0.8.0.pkg
```

An **app-specific password**, not the Apple ID password: generated at
appleid.apple.com, revocable on its own. `store-credentials` puts it in the
keychain so it never appears in a script or a CI log.

Which certificate is which:

- **Developer ID Application** signs executables — our binary.
- **Developer ID Installer** signs `.pkg` files — our package.

Both come from the same Apple Developer Program membership (99 USD a year), and a
company account needs a D-U-N-S number, which takes longer to obtain than the
membership itself. Start that early.

## What is not built

- signing and notarization (no account)
- the tenant half of the CA chain: one root per tenant with its key in KMS, and a
  CRL so revoking a device does not mean waiting up to seven days for its
  intermediate to expire (needs an AWS account). The device half is built — see
  D14 and `aiul ca device`.
- Linux and Windows packaging; both platforms are stubs that return
  `ErrUnsupported`
- an uninstall package. Removal is `sudo aiul uninstall` or
  `sudo ./scripts/killswitch.sh`, both of which an MDM can run as a script.
