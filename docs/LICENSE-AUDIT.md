# Ngirimwa / ChatLoop — License Removal Audit

> **Purpose:** comprehensive inventory of licensing, activation, machine binding, watermark, legal restrictions, packaging, and runtime integration points that must be reviewed before converting the project to a genuinely open-source application.
>
> **Repository:** `assyauq/ngirimwa`
> **Branch audited:** `main`
> **Audit basis:** repository source currently present in GitHub plus the production/runtime context previously verified on the Tencent VPS.
>
> **Important legal note:** this document is a technical/code audit, not legal advice. Before publishing under an OSI-approved license, the copyright/licensing authority should confirm that NgertiKode.id and every other rights holder authorizes the relicensing and redistribution of the complete codebase and bundled assets.

---

## 1. Executive conclusion

The project is **not currently open-source-ready**. Licensing is implemented as a runtime subsystem and is also embedded in documentation, packaging, startup behavior, environment configuration, copyright notices, watermark metadata, machine binding, and release packaging.

The central runtime gate is:

```text
backend/main.go
    -> license.Verify()
    -> if invalid: ui.LicenseError(...)
    -> license.StartHeartbeat(...)
           -> license.Heartbeat()
           -> terminal failure => application shutdown
```

The dedicated licensing package is:

```text
backend/license/
├── heartbeat.go
├── license_contract_test.go
├── machine_id.go
├── reset.go
├── verify.go
└── watermark.go
```

The most important licensing dependencies outside `backend/license/` are:

```text
backend/main.go
backend/ui/banner.go
backend/config/config.go
.env.example
README.md
docs/EULA.md
docs/INSTALL-LOCAL.md
SOURCE-LICENSE.template.md
NOTICE
scripts/pack-master.mjs
.github/workflows/release.yml
```

There is also a **build-time development bypass** (`DevMode`) in the license package. That is useful for diagnostics but is not a substitute for removing the licensing system if the end goal is genuine open-source distribution.

---

## 2. Definition of "fully removed"

The project should only be considered converted to open source when all of the following are true:

1. A fresh installation runs without a license key.
2. Startup never calls a license verification service.
3. Application runtime never performs license heartbeats.
4. Application runtime never shuts down because of license state.
5. No machine fingerprint is created for licensing.
6. No license reset or machine-limit logic remains.
7. No watermark/license-owner fields are injected or reported.
8. No package/release process requires a license manifest.
9. No installation documentation asks users to purchase, enter, or activate a license.
10. No EULA or source restriction says redistribution is prohibited.
11. No copyright/license notices claim the source is closed/proprietary when it is intended to be open source.
12. A recognized open-source license file exists at repository root and matches the intended licensing policy.
13. CI/release artifacts are independent of the old license packaging rules.
14. Tests related only to the proprietary licensing contract are removed or replaced with tests for the new open-source behavior.
15. A repository-wide search for old license identifiers/endpoints/variables returns no functional references.

---

## 3. Dedicated licensing package — complete inventory

### 3.1 `backend/license/verify.go` — CRITICAL

**Status:** runtime licensing core.

This file contains the largest amount of licensing behavior.

Key responsibilities:

- Defines license status constants:
  - `active`
  - `revoked`
  - `expired`
  - `machine_mismatch`
  - `machine_limit_reached`
  - `not_found`
  - `network_error`
  - `rate_limited`
  - `server_error`
  - `unauthorized`
  - `invalid_request`
  - `offline_grace_expired`
  - `invalid_signature`
- Defines license verification request/response contracts.
- Stores build-time pinned license API URL.
- Stores build-time pinned Ed25519 signing public key.
- Defines `DevMode` build-time bypass.
- Performs startup verification through `Verify()`.
- Performs periodic revalidation through `Heartbeat()`.
- Calls the remote verification endpoint:

```text
POST <license API base>/api/license/verify
```

- Sends license key, current machine hash, legacy machine hash, heartbeat flag, and nonce.
- Supports `X-License-Secret` authentication.
- Verifies Ed25519-signed license responses when a public key is configured.
- Implements terminal statuses that invalidate the installation.
- Implements offline grace period logic.
- Masks license key values in logs.

**Removal impact:** extremely high. This is the primary file to delete once all callers are removed.

**Reference:** `backend/license/verify.go` contains the startup/heartbeat logic, remote API contract, pinned API/signing configuration, machine-aware verification, and grace-period shutdown behavior.

---

### 3.2 `backend/license/heartbeat.go` — CRITICAL

Runs an asynchronous periodic heartbeat.

Behavior:

```text
StartHeartbeat()
   -> initial random delay
   -> randomized 6–12 hour interval by caller defaults
   -> Heartbeat()
   -> if invalid / terminal / grace expired:
          onInvalid(message)
```

The current `main.go` passes a callback that stops the application's root context, which effectively shuts down the server.

**Removal:** delete the heartbeat subsystem and remove `license.StartHeartbeat(...)` from `backend/main.go`.

---

### 3.3 `backend/license/machine_id.go` — CRITICAL

Creates and persists a random installation identity at:

```text
data/.license-machine-id
```

unless `LICENSE_MACHINE_ID_PATH` overrides it.

Then derives a SHA-256 machine fingerprint from:

```text
wa-assistant-license-v2|<installation ID>|<GOOS>|<GOARCH>
```

It also maintains a legacy fingerprint based on:

```text
hostname | username | OS
```

**Licensing purpose:** binds an installation to a machine and supports migration of legacy machine bindings.

**Removal:** delete file and all references to `machineFingerprints()`.

**Important runtime side effect:** do not confuse this with application data. The machine ID exists specifically for licensing.

---

### 3.4 `backend/license/reset.go` — HIGH

Implements a CLI-only reset operation:

```text
chatloop-server license-reset
```

It:

- reads `LICENSE_KEY`
- reads `LICENSE_PUBLIC_RESET_ENABLED`
- derives machine fingerprint
- calls:

```text
POST <license API base>/api/license/reset
```

- sends license key + machine hash
- optionally sends `X-License-Secret`

This consumes a customer machine reset according to the old licensing design.

**Removal:** delete the file and remove the `license-reset` command branch from `backend/main.go`.

---

### 3.5 `backend/license/watermark.go` — HIGH

Contains placeholder ownership metadata:

```text
{{LICENSE_OWNER}}
{{LICENSE_EMAIL}}
{{LICENSE_ORDER_ID}}
{{LICENSE_FINGERPRINT_HASH}}
```

Exposes:

```go
GetWatermark()
```

`backend/main.go` logs the watermark when bound.

**Removal:** delete watermark metadata and remove the `GetWatermark()` startup logging path.

---

### 3.6 `backend/license/license_contract_test.go` — HIGH

Contains tests that explicitly define the proprietary LMS licensing contract, including:

- persistent machine fingerprinting
- `/api/license/verify` contract
- Ed25519 license response verification
- machine-limit terminal behavior
- revoked license terminal behavior
- offline grace expiration
- `/api/license/reset` contract
- legacy status inference

**Removal:** delete the test file when the license package is removed, or replace with tests for the new open-source startup contract.

---

## 4. Runtime integration outside `backend/license/`

### 4.1 `backend/main.go` — CRITICAL

This is the main runtime integration point.

Observed licensing behavior:

```go
if len(os.Args) > 1 && os.Args[1] == "license-reset" {
    license.Reset()
    return
}
```

Then at startup:

```go
if !license.Verify() {
    ui.LicenseError(license.VerifyMessage)
}
```

Then:

```go
license.StartHeartbeat(appCtx, 6*time.Hour, 12*time.Hour, func(message string) {
    stop()
})
```

And watermark reporting:

```go
if wm := license.GetWatermark(); wm.IsBound {
    log.Printf("[license] ...")
}
```

**This file must be modified before deleting the package**, otherwise the build will fail and/or startup will retain license behavior.

Desired post-open-source startup path should begin directly with normal application initialization, without any license command branch, verification, watermark logging, or heartbeat registration.

---

### 4.2 `backend/ui/banner.go` — HIGH

`LicenseError()` is a dedicated UI path that terminates the process with exit code 1.

The message currently tells users to:

- buy a license
- populate `LICENSE_KEY`
- populate `LICENSE_API_SECRET`
- populate `LICENSE_API_URL`
- restart development

The success banner also currently says:

```text
Lisensi aktif.
```

**Removal:**

- remove `LicenseError()` entirely
- remove all license-specific text
- change the startup banner to a license-neutral message
- preserve normal server-status output

---

### 4.3 `backend/config/config.go`

This file is generic environment loading and does not itself enforce licensing.

It **must remain**, but the license-specific variables should no longer be needed after conversion.

Do not delete the config subsystem merely because it loads `.env`; many non-license settings use it.

---

## 5. Environment/configuration licensing surface

### `.env.example` — HIGH

Current example contains:

```text
LICENSE_KEY=lisensi_member
LICENSE_API_SECRET=secret_lisensi_member
```

**Removal:** delete these variables from `.env.example`.

Search future documentation/configuration for all other license-related variables, especially:

```text
LICENSE_KEY
LICENSE_API_SECRET
LICENSE_API_URL
LICENSE_RESPONSE_SIGNING_PUBLIC_KEY
LICENSE_SIGNATURE_MAX_AGE_SECONDS
LICENSE_OFFLINE_GRACE_HOURS
LICENSE_MACHINE_ID_PATH
LICENSE_PUBLIC_RESET_ENABLED
```

The current source clearly reads several of these through `config.Env(...)` / `config.EnvBool(...)`.

---

## 6. Documentation / legal restrictions

### `docs/EULA.md` — CRITICAL LEGAL

Current EULA explicitly makes the project proprietary and non-redistributable.

It currently says, in substance:

- license is non-exclusive and non-transferable
- internal use/modification is allowed
- redistribution/public publication is prohibited
- source resale is prohibited
- license keys are tied to updates/support
- license can be revoked
- source is not a free product

This document is **incompatible with an open-source release**.

**Action:** replace it with an OSS-oriented project policy or remove it after the new root license and contribution/governance policy are established.

Do not leave the proprietary EULA in the repository if the project is publicly distributed as open source; it creates direct contradiction about the rights granted.

---

### `SOURCE-LICENSE.template.md` — CRITICAL LEGAL

This template explicitly states:

- source belongs to a licensed buyer
- contains license key / owner / email / order / support dates
- redistribution is prohibited
- transfer is prohibited
- source remains restricted even when readable/modifiable

This is a proprietary distribution artifact and should be **removed** from an open-source repository unless there is a separate historical archive kept outside the project.

---

### `NOTICE` — CRITICAL LEGAL

Current notice says:

- source is licensed, not freely sold
- license key is required
- redistribution is prohibited
- digital fingerprint traces the buyer
- NgertiKode.id support/licensing contact applies

This must be replaced by a new neutral/OSS copyright and license notice.

**Important:** do not simply delete copyright attribution if a different rights holder still needs attribution. The final notice must be consistent with the actual copyright ownership and the selected OSS license.

---

### `README.md` — HIGH

The README describes the product and development environment. It also currently links readers into the proprietary legal/disclaimer model and may need wording updates for:

- open-source license
- contribution model
- self-hosting
- environment setup without a license key
- supported deployment

The feature description itself is not necessarily licensing logic and should be preserved.

---

### `docs/INSTALL-LOCAL.md` — HIGH

Current installation instructions explicitly instruct the user that `.env` contains a license key generated by the LMS/member dashboard.

It also tells the user to use:

```text
LICENSE_KEY=...
```

This must be rewritten so a fresh clone can run without LMS provisioning or a proprietary manifest.

The setup steps should instead generate/copy a normal `.env`, set a developer/admin secret safely, configure MySQL, and start the application.

---

### `docs/PANDUAN-INSTALASI.pdf` and `docs/panduan-template.html` — HIGH

The docs directory contains these distribution/install documents. The template and PDF should be inspected/rebuilt for license-related references after code conversion.

Do not assume binary/PDF assets are free of old legal text just because Markdown files have been cleaned.

A release audit should include text extraction or visual inspection of the PDF after the OSS transition.

---

## 7. Packaging and release pipeline

### `scripts/pack-master.mjs` — CRITICAL

This script is explicitly a **member/LMS packaging system**.

It says it creates a distribution ZIP for members and excludes sensitive/runtime/license files such as:

- `.env`
- databases
- SQLite artifacts
- machine ID
- WhatsApp session/media
- compiled binaries
- internal development docs

This is not inherently bad for a normal OSS release, but its current purpose and naming are proprietary-product oriented.

It also explicitly excludes:

```text
.license-machine-id
backend/data/.license-machine-id
```

which confirms that machine identity is considered part of the licensing distribution model.

**Action:** decide whether this becomes a normal OSS source package builder or is removed entirely in favor of standard GitHub source/releases. If retained, rename/reword it so it does not imply member licensing.

---

### `.github/workflows/release.yml` — HIGH

The workflow is named:

```text
Package Member Release
```

and packages a downloadable member ZIP using the packaging script.

This workflow should be redesigned for OSS releases. A sensible OSS replacement would:

- run tests
- build backend/frontend
- produce normal release artifacts
- publish source tarball/ZIP
- optionally publish binaries/container images
- avoid any license/member manifest process

Also verify the artifact name and path because the current workflow references `dist/chatloop-member.zip` while the inspected packaging script declares `chatloop-master.zip`; this should be reconciled during the release refactor.

---

## 8. Build-time licensing hooks

### `verify.go` build-time variables

Two variables can be injected using Go linker flags:

```text
PinnedLicenseAPIURL
PinnedLicenseSigningPublicKey
```

and a third:

```text
DevMode=true
```

The important design issue is that this means licensing is not only an `.env` concern. Production artifacts can contain licensing configuration at build time.

When converting to OSS, remove the variables and corresponding `-ldflags` documentation/comments so future builders cannot accidentally preserve proprietary verification behavior.

---

## 9. Watermark / ownership injection

`backend/license/watermark.go` contains placeholders that are designed to be replaced during LMS packaging:

```text
{{LICENSE_OWNER}}
{{LICENSE_EMAIL}}
{{LICENSE_ORDER_ID}}
{{LICENSE_FINGERPRINT_HASH}}
```

This means the licensing model includes **per-buyer source attribution**.

The packaging process and release system must be checked for placeholder replacement, even if no replacement logic currently appears in the visible source tree.

Search for these exact tokens in the full repository and in any external/CI build tooling:

```text
LICENSE_OWNER
LICENSE_EMAIL
LICENSE_ORDER_ID
LICENSE_FINGERPRINT_HASH
```

---

## 10. Machine-bound runtime files

The licensing code creates:

```text
data/.license-machine-id
```

with restrictive permissions.

The packaging script also explicitly excludes it.

When licensing is removed, this file should no longer be created. Existing installations may retain it and should be allowed to delete it as a one-time cleanup/migration step.

**Do not blindly delete the entire `data/` directory** because it also contains legitimate application runtime data.

---

## 11. Runtime database / schema considerations

No direct license ORM model was found in the inspected `backend/database/database.go` migration list.

The licensing system is primarily remote/API + machine-file based rather than a first-class application database entity.

Nevertheless, before final release, run repository-wide searches against:

```text
license
LICENSE_
license_key
machine_hash
machine_id
fingerprint
watermark
reset_remaining
machines_used
machine_max
package_type
```

and inspect any database migration/seed code or serialized configuration not yet surfaced by repository search.

---

## 12. Startup behavior before vs after OSS conversion

### Current

```text
main()
  |
  +--> license-reset? ----------> license.Reset() ---> exit
  |
  +--> database.Init()
  |
  +--> license.Verify()
  |       |
  |       +--> remote LMS
  |       +--> machine fingerprint
  |       +--> signature verification
  |       +--> fail -> ui.LicenseError() -> exit
  |
  +--> watermark logging
  |
  +--> StartHeartbeat()
  |
  +--> normal application startup
```

### Target OSS behavior

```text
main()
  |
  +--> database.Init()
  |
  +--> normal application initialization
  |
  +--> AI / embeddings
  |
  +--> WhatsApp agents
  |
  +--> schedulers / background workers
  |
  +--> HTTP server
```

There should be **no licensing decision in the startup critical path**.

---

## 13. `DevMode` is not the final solution

The code already provides:

```text
DevMode=true
```

through linker flags to skip license verification.

This may appear to make the application "free", but it does not satisfy the OSS goal because:

- proprietary package still exists
- heartbeat still exists
- legal files still prohibit redistribution
- machine fingerprinting remains in source
- packaging still targets licensed members
- branding/watermark code remains
- `.env.example` still asks for license values
- fresh users still depend conceptually on a proprietary license flow

Therefore **do not ship `DevMode` as the public OSS mechanism**. Remove the proprietary system itself.

---

## 14. Repository-wide search checklist

Because text search results can miss generated files, binaries, PDFs, or external build-time substitutions, use all of the following searches during implementation.

### Exact identifiers

```text
LICENSE_KEY
LICENSE_API_URL
LICENSE_API_SECRET
LICENSE_RESPONSE_SIGNING_PUBLIC_KEY
LICENSE_SIGNATURE_MAX_AGE_SECONDS
LICENSE_OFFLINE_GRACE_HOURS
LICENSE_MACHINE_ID_PATH
LICENSE_PUBLIC_RESET_ENABLED
```

### Code symbols

```text
license.Verify
license.Heartbeat
license.StartHeartbeat
license.Reset
license.GetWatermark
machineFingerprints
DevMode
PinnedLicenseAPIURL
PinnedLicenseSigningPublicKey
LicenseError
VerifyResult
VerifyMessage
VerifyStatus
VerifyPackageType
```

### Protocols/endpoints

```text
/api/license/verify
/api/license/reset
api.ngertikode.id
ngertikode.id
X-License-Secret
```

### Metadata/placeholders

```text
LICENSE_OWNER
LICENSE_EMAIL
LICENSE_ORDER_ID
LICENSE_FINGERPRINT_HASH
{{LICENSE_OWNER}}
{{LICENSE_EMAIL}}
{{LICENSE_ORDER_ID}}
{{LICENSE_FINGERPRINT_HASH}}
```

### Legal words / branding to review

```text
EULA
lisensi
license
licensed
member
LMS
support until
updates until
source code license
redistribut
dijual ulang
distribusi
```

---

## 15. Files to remove, replace, or modify

### Delete after migration

```text
backend/license/heartbeat.go
backend/license/license_contract_test.go
backend/license/machine_id.go
backend/license/reset.go
backend/license/verify.go
backend/license/watermark.go
SOURCE-LICENSE.template.md
```

### Replace/rewrite

```text
backend/main.go
backend/ui/banner.go
.env.example
README.md
docs/EULA.md
docs/INSTALL-LOCAL.md
NOTICE
scripts/pack-master.mjs
.github/workflows/release.yml
docs/PANDUAN-INSTALASI.pdf
docs/panduan-template.html
```

### Inspect for secondary references

```text
docs/PROJECT-MAP.md
all backend files
all scripts
all GitHub workflows
all frontend files
all tests
package/build scripts
```

---

## 16. Recommended OSS target

The project should choose a recognized license rather than writing a custom license.

Typical permissive choices:

- MIT — simple and highly permissive.
- Apache-2.0 — permissive with explicit patent language.

If the project should require derivative distributions to stay open, GPL family licenses may be considered, but that is a legal/governance decision rather than an engineering choice.

**Do not declare an OSI license until rights to all source/assets/dependencies are confirmed.**

Recommended repository state:

```text
LICENSE           # chosen OSS license text
NOTICE            # only if needed by chosen license / attribution policy
README.md         # OSS project description
CONTRIBUTING.md   # optional but recommended
SECURITY.md       # recommended
```

---

## 17. Dependencies and third-party licenses

Open-sourcing the project also requires reviewing dependency licenses.

`go.mod` includes a broad dependency graph, including:

- Gin
- JWT
- OpenAI-compatible client
- WhatsMeow
- GORM
- MySQL driver
- modernc SQLite
- Google APIs
- OpenTelemetry-related dependencies
- QUIC/websocket packages
- numerous transitive packages

The project should generate a third-party software inventory before publication and preserve required attribution notices.

Frontend dependencies in `frontend/package.json` / lockfile require the same review.

This is separate from the current proprietary ChatLoop license but is mandatory for a clean OSS release.

---

## 18. Production migration strategy

Do **not** immediately delete the licensing package on the live Tencent VPS.

Recommended sequence:

1. Create an OSS migration branch.
2. Remove license startup gate and heartbeat in code.
3. Remove license package and tests.
4. Remove machine-ID creation.
5. Remove proprietary docs/packaging/license notices.
6. Add the chosen OSS `LICENSE` file.
7. Update installation instructions.
8. Update CI/release workflow.
9. Run all tests.
10. Build frontend and backend.
11. Verify a clean fresh installation without license values.
12. Deploy to a staging path/VPS or maintenance window.
13. Verify WhatsApp session recovery, AI, database, REST API, broadcast, scheduler and CRM.
14. Only then replace the production binary/service.
15. Remove old `data/.license-machine-id` as a one-time cleanup on existing installations.

---

## 19. Acceptance tests for OSS conversion

A release candidate should pass all of the following:

### Fresh install

```text
LICENSE_KEY absent
LICENSE_API_* absent
```

Application still starts successfully.

### Network independence

Block the old license API host. Application still starts and remains operational.

### Machine independence

Run on a new machine without any license machine file. Application starts normally.

### No shutdown hook

Wait beyond the old heartbeat interval. No license-related shutdown occurs.

### No license UI

No "Beli lisensi", "Lisensi aktif", activation/reset prompt, or license error appears.

### Packaging

OSS release contains source code and permitted assets without member-specific metadata.

### Search gate

Repository search finds zero functional references to the old license subsystem and zero forbidden proprietary licensing text.

### Documentation gate

README/install docs do not require a proprietary purchase or LMS account.

---

## 20. Important distinction: source open vs legal relicense

Removing technical license enforcement does **not** automatically make a project legally open source.

There are two independent layers:

```text
Technical layer
  -> remove verification / heartbeat / machine binding / packaging restrictions

Legal layer
  -> obtain rights / decide copyright ownership
  -> choose OSS license
  -> add LICENSE
  -> replace/remove incompatible EULA and restrictions
```

Both must be completed.

---

## 21. Current audit status

| Area | Status | Priority |
|---|---|---:|
| Runtime license verifier | Found | CRITICAL |
| License heartbeat | Found | CRITICAL |
| Machine binding | Found | CRITICAL |
| License reset | Found | HIGH |
| Watermark metadata | Found | HIGH |
| Startup integration | Found | CRITICAL |
| License error UI | Found | HIGH |
| License env vars | Found | HIGH |
| Proprietary EULA | Found | CRITICAL LEGAL |
| Source license template | Found | CRITICAL LEGAL |
| Proprietary NOTICE | Found | CRITICAL LEGAL |
| Member packaging | Found | HIGH |
| GitHub release workflow | Found | HIGH |
| Install docs | Found | HIGH |
| Binary/PDF documentation review | Pending implementation | HIGH |
| Full repository text search | Connector search is incomplete/returns sparse results; must be repeated locally | CRITICAL |
| Third-party dependency license audit | Pending | CRITICAL |
| Final OSS license selection | Pending | CRITICAL |

---

## 22. Final engineering recommendation

Treat `backend/license/` as an **isolated removable subsystem**, but treat its callers and documentation as a dependency graph.

Do not delete only the directory and attempt to repair compile errors afterward. The correct migration is:

```text
1. Remove startup gate
2. Remove heartbeat
3. Remove watermark reporting
4. Remove reset CLI
5. Remove machine fingerprint dependency
6. Remove license UI
7. Remove env vars
8. Remove proprietary packaging
9. Remove proprietary legal docs
10. Replace with OSS legal policy
11. Add recognized LICENSE
12. Add OSS-focused release/install process
13. Run clean-install + integration tests
```

**Do not modify production until the complete migration has passed the acceptance tests above.**
