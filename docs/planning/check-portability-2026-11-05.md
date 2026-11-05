# Windows check portability and Go failure follow-up

- Latest result: a fresh complete `pnpm check` passed with AdGuard total
  protection off. OAuth normal/race tests also passed 20 repetitions each,
  without changing their implementation or assertions. The narrower
  `Filter localhost` configuration remains unverified; see the controlled
  environment comparison below.
- Scope: repair the checks left by R0-R2 and distinguish project defects from
  host/fixture limitations. R3 remains unimplemented.
- Baseline: [R0-R2 verification record](refactor-r0-r2-2026-11-05.md).
- No dependency upgrade, schema migration, production permission relaxation,
  production database mutation or live remote-resource operation.

## Diagnosis and changes

| Original failure | Classification | Change / retained coverage |
| --- | --- | --- |
| `GOWORK=off`, `cd`, `mkdir -p`, `rm -rf` in pnpm gates | Project script portability defect | Node standard-library runner passes argv, cwd and environment directly; every lint/test/race/build/generation/diff gate remains; temporary build output is confined and cleaned |
| New Node script test is also discovered by Vitest | Test-runner integration defect found by supplemental frontend checks | Rename it to `check-plugins.node-test.mjs` and update explicit Node/CI references; retain all six Node tests and leave Vitest's discovery configuration unchanged |
| gofmt/CRLF reports with global `core.autocrlf=true` | Missing repository checkout policy | `.gitattributes` declares LF text and CRLF native command scripts; normalize the current checkout without staging or changing global Git settings |
| Existing Model Tools fixture indentation | Actual formatting defect | Return named local fixture values so the Go formatter and pinned linter agree after LF normalization |
| MCP occupied-port rotation fails on Windows | Production platform error-classification defect | Match WinSock WSAEADDRINUSE (10048) on Windows and the native Unix sentinel elsewhere; only actual address-in-use errors permit bounded rotation |
| SSH refused TCP connection reported as unreachable | Production platform error-classification defect | Match WinSock WSAECONNREFUSED (10061) while preserving identity/authentication and non-retryable failure behavior |
| Antigravity tests directly execute POSIX `.sh` files | Test fixture portability defect | Use the Go test executable as a controlled child process; preserve stdin-only prompt, native session evidence, cancellation and protocol failure cases |
| Placement assumes `/tmp` is an absolute local root | Test fixture portability defect | Use a test-owned native temporary directory |
| In-process SSH/SFTP server receives Windows paths as remote roots | Test fixture path-model defect | Keep POSIX protocol paths distinct from native local fixture paths |
| Tunnel tampering fixture hardcodes an extensionless binary | Test fixture portability defect | Derive the entrypoint from the platform manifest |
| Plugin catalogue assumes `agy` is installed | Test expectation defect | Assert truthful inert availability; uninstalled optional backends remain unavailable |
| Terminal state polling and protocol timeout include expensive setup | Test synchronization/budget defect | Wait for fixture terminal evidence before polling; start the existing MCP protocol budget after migration/resource setup; retain terminal assertions and operation budgets |
| OAuth normal completion closes an active callback response | Production lifecycle race | Fixed-length completion/failure pages are flushed before signaling the flow; normal finish uses an independent, bounded five-second `http.Server.Shutdown`; cancellation/forced close can immediately interrupt the drain. State validation and authorization decisions remain unchanged |
| Loopback HTML is rewritten and independent socket experiments also reset | Host transport/filter interference, independently reproduced outside Mender | Assert exact HTML, no echoed private input and security/framing headers at the real handler; test actual HTTP response draining and forced interruption over `net.Pipe`; retain unskipped real loopback PKCE/token exchange and state-rejection integration tests. The resets cannot all be attributed to AdGuard from this evidence alone |

The LF normalization inspected Git's index/working-tree EOL metadata: all
affected index entries were LF, with no intentionally CRLF or mixed-EOL index
entries. It changed 1,411 tracked working-tree text files and no binary files;
line-ending-only changes do not create semantic Git diffs.

## Explicit environment-limited coverage

- A host without a verified `restricted_host` backend cannot execute the
  governed-placement success fixtures. They now skip with that precise reason.
  Host Agent and Host Sandbox rejection tests still execute. Production never
  falls back to Host mode.
- The in-process SFTP fixture uses the local filesystem. If it cannot represent
  private POSIX permissions or directory fsync, durable-job/recovery fixtures
  skip after capability probing. This does not mean a Windows Mender client
  cannot use a real Linux SSH server. Basic SSH/SFTP, host pinning, authentication,
  network failure and retry tests continue to run on Windows.
- The local SSH fixture's POSIX shell subtest skips only when `sh` is missing;
  structural SFTP file operations continue to run.
- Live SSH/remote account tests retain explicit operator opt-in. No native
  Wails UI, real Agent account, Docker/Daytona resource or remote host was used.

No test treats a host capability declaration as proof of strong isolation or
remote durability. Unsupported proof remains a production rejection.

## Independent loopback environment evidence

Two separate issues were found. The previous normal OAuth `finish` path could
call `server.Close` while the accepted callback handler was still completing its
response. That internal race is repaired by explicit `Content-Length` and
`Flush`, followed by a graceful drain with a context independent of the completed
flow's cancellation and bounded to five seconds. The hard-close path remains
separate, so explicit cancellation does not have to wait for that drain.

`TestFlowFinishDrainsResponseAndHardCloseCanInterrupt` runs a real
`http.Server`/`http.Transport` over `net.Pipe`: it starts a blocked accepted
handler, closes the listener through normal finish, then releases and verifies
the entire HTTP response. Its second case verifies that forced close interrupts
the response immediately. These deterministic lifecycle cases and the callback
HTML/framing/flush-order/private-input assertions passed targeted normal/race
checks; the lifecycle cases also passed 20 race-enabled repetitions. This is
evidence about Mender's internal lifecycle, not proof that this host's TCP path
is reliable. Real TCP OAuth integration remains a separate, enabled check.

A temporary Go-standard-library program, importing no Mender code, tested this
host separately. For the shutdown cases the handler had already started; it was
released to write its response only after `Shutdown` closed the listener. Both
HTTP and TLS used explicit response length and flushing. Clients connected
directly without an HTTP proxy. The TLS client trusted only the httptest server
certificate and retained certificate verification.

| Independent control | Trials | WSAECONNRESET | Exact response bodies |
| --- | ---: | ---: | ---: |
| Plain HTTP, no shutdown until after the response | 10 | 1 | 0 |
| Plain HTTP, accepted response drained with `Shutdown` | 30 | 15 | 0 |
| TLS, no shutdown until after the response | 10 | 2 | 8 |
| TLS, accepted response drained with `Shutdown` | 30 | 3 | 27 |
| Raw TCP, close listener after accept/read, then reply on accepted connection | 30 | 15 | 20 |

Every received plain-HTTP 200 body differed from the server's fixed original.
The raw-TCP exact-body and reset counts overlap: five clients received the whole
payload before `ReadAll` reported reset. No handler write/flush or graceful
shutdown in these controls reported an error. TLS reduced, but did not eliminate,
the observed resets.

A separate long-HTML HTTP probe without shutdown had two resets in three
requests. Its received 200 response contained an injected script from
`//local.adguard.org` whose URL identified `app=bodyprobe.exe`. The received body
was changed despite retaining the declared 8,268-byte length. This independently
establishes HTML interception outside Mender; it does not establish that AdGuard
caused every reset, especially those also observed with TLS and raw TCP.

Local diagnostic programs and observations were retained in
`%TEMP%/mender-loopback-drain-666bb72c84924cb183ec3905c15ca7ec/`:
`main.go`, `bodyprobe.go`, `results.json` and `http-body-probe.txt`. They were run
as `go run main.go` and `go run bodyprobe.go`, with `GOWORK=off` set in the
subprocess environment. No repository source or security-software
settings were changed by these independent experiments.

## Validation strategy and evidence

Observed targeted checks:

- Portable script tests: 6 passing cases, including gate ordering, short-circuit
  failures, exit codes and success/failure temporary-output cleanup.
- Real Windows protocol lint/generation/parity, Provider SDK tests and example
  build passed.
- Antigravity, placement and Agent fairness targeted normal/race tests passed.
- Agent/MCP/Catalogue and Desktop targeted normal/race tests passed, with the
  restricted-host success fixtures explicitly skipped on this host.
- SSH remote full package normal/race and Bootstrap SSH/Tunnel targeted
  normal/race tests passed with the capability limitations listed above.
- Actual occupied TCP port rotation passed. Callback handler output/framing and
  deterministic HTTP drain/hard-close checks passed targeted normal/race runs.
  Real loopback OAuth integrations remain enabled; they have not been uniformly
  passing on this host. Their final normal/race status belongs to the complete
  run below, not to a blanket claim that handler and network OAuth checks passed.

Previous full-suite results, before the AdGuard environment comparison:

| Check | Result |
| --- | --- |
| `pnpm check` | Exit 1 at `go:test`; every preceding gate passed, including all plugin modules/examples, protocol regeneration/parity, module isolation, SDK/Wails parity, architecture gates and backend lint (0 issues) |
| Full `go test ./backend/...` within that run | 96 test-bearing packages passed, only `credential/infrastructure/oauthflow` failed; `TestBadStateFailsTheFlowClosed` received WSAECONNRESET at the real callback GET |
| Separate full `go test -race ./backend/...` | 96 test-bearing packages passed, only OAuth failed; `TestOAuthFlowRejectsInvalidStateWithoutSecret` received the same socket reset. No data race was reported |
| Bootstrap full normal/race | Passed in 83.943s / 431.736s; the three former timing failures did not recur in their full-package normal/race runs |
| Final `pnpm scripts:test` after renaming the Node fixture | All 6 tests passed |
| Supplemental `pnpm lint`, `pnpm typecheck` | Passed for all existing frontend scopes |
| Final `pnpm test` | Passed: 71 files, 467 tests; i18n passed with 1,594 required/translated messages |
| `pnpm build` | Web and Desktop frontend production builds passed; the existing minified-chunk-over-500-kB warning remains, with no performance work added to this scope |
| `gofmt -l`, `git diff --check`, tracked text EOL scan | Clean; no tracked working-tree CRLF/mixed text, and no generated-contract, binding, dependency or lockfile content diff |

The complete command deliberately retains its failing exit status. It did not
reach the backend race or frontend gates after the ordinary OAuth failure; those
gates are run separately, without treating them as a passing aggregate command.
The capability skips above are still skips, rather than claimed proof of the
unsupported host behavior. The remaining real TCP OAuth tests require a rerun on
a host whose loopback transport is not subject to this interference before the
entire gate can be considered green.

Full command logs and exit statuses are in
`%TEMP%/mender-check-portability-complete.{log,exit}` and
`%TEMP%/mender-check-portability-full-race.{log,exit}`. The independent environment
evidence above is retained separately from these project test results.

The initial supplemental frontend run passed lint/typecheck and all 467 frontend
tests, but Vitest also picked up the Node-only test and reported an empty suite.
After renaming that fixture and updating its explicit package/CI references,
Node's six tests and the full frontend suite passed. The Vitest discovery rules
were not narrowed. Backend and generated-code inputs did not change during this
final runner correction, so the full Go and parity results above remain valid.
The initial frontend log is
`%TEMP%/mender-check-portability-frontend.{log,exit}`; final frontend test/build
evidence is in `%TEMP%/mender-check-portability-frontend-test.{log,exit}` and
`%TEMP%/mender-check-portability-build.{log,exit}`.

## CI

Linux gates retain all existing checks. Script unit tests also run in the Web
quality job. A Windows job runs the portable script tests, real example build,
Provider SDK tests and full backend normal suite. Existing Linux race gates
remain enabled. CI configuration is prepared locally; no remote CI run is
claimed by this record.

## Follow-up: controlled AdGuard environment comparison

The preceding full-suite failures and independent controls are historical
results, before this follow-up. AdGuard 8.0.5570 was installed and WinHTTP was
configured for direct access. No OAuth implementation or assertions were
changed during the environment comparison below. In particular,
real loopback PKCE/token exchange and rejected-state tests remained enabled;
there was no new retry, response sanitization, relaxed assertion or skip to make
them pass.

| Environment state | OAuth normal, `-count=20` | OAuth race, `-count=20` | Other observation |
| --- | --- | --- | --- |
| A: Original AdGuard protection/filtering state | Exit 1; 5 socket resets | Exit 1; 5 socket resets | Baseline logs contain 135 / 134 unsolicited-response messages respectively; a separate baseline HTML probe confirms injection |
| B: Protection remains on; one `127.0.0.1` plus process-name network exception enabled | Exit 1; 8 socket resets | Exit 1; 6 socket resets | Injection/unsolicited-response markers disappeared, but real callback resets persisted |
| C: AdGuard total protection switched off, as last confirmed by the user | Exit 0; no reset | Exit 0; no reset | All independent controls also received exact original bodies without reset |

These reset counts are failures across the selected repeated test cases, not
percentages for a single request per repetition. The change from 5 to 8 or 6
failures is not evidence that the scoped exception made transport worse.

The independent standard-library control was also run with the same executable
process name as the scoped exception, so that the exception could apply to it.
It imports no Mender code. Its detailed observations were:

| Independent control | Trials | B: resets / exact bodies | C: resets / exact bodies |
| --- | ---: | ---: | ---: |
| HTTP, normal completion | 10 | 2 / 8 | 0 / 10 |
| HTTP, accepted response drained with `Shutdown` | 30 | 0 / 30 | 0 / 30 |
| TLS, normal completion | 10 | 0 / 10 | 0 / 10 |
| TLS, accepted response drained with `Shutdown` | 30 | 1 / 29 | 0 / 30 |
| Raw TCP, listener closed after accept/read | 30 | 8 / 30 | 0 / 30 |

All eight B-state raw-TCP resets occurred after the complete payload was
received: exact-body counts do not imply that `ReadAll` returned without an
error. In state C, all 110 independent trials had an exact body and no reset.
Together with unchanged project tests, these results strongly associate this
machine's AdGuard processing path with the remaining loopback instability.
They do not identify a particular WFP, redirect, driver or filtering-engine
component as its cause.

The scoped exception changed filtering behavior without resolving every
transport error. The official [`$network` documentation](https://adguard.com/kb/general/ad-filtering/create-own-filters/#network)
promises bypassing further filtering for a matched endpoint; it does not
establish which driver/redirect path this host took. AdGuard also documents
[routing, filtering and HTTPS filtering as separate layers](https://adguard.com/kb/adguard-for-windows/app-management/).
Disappearance of injected HTML therefore is not proof that all network
interception was bypassed.

The only successful environment configuration verified in this follow-up is
**AdGuard total protection off**. The official
[`Filter localhost` setting](https://adguard.com/kb/adguard-for-windows/settings/app-settings/advanced-settings/#filter-localhost)
provides a narrower loopback-filtering control, but the combination of total
protection on and only that setting off was not tested here. It must not be
described as a verified minimal fix, or as an exception limited to the single
IPv4 address `127.0.0.1`. The final check uses state C above.

For rollback tracking, the earlier diagnostic exception was exactly:

```text
@@/^127\.0\.0\.1(:[0-9]+)?$/$network,app=oauthflow.test.exe
```

Its rollback is deletion or disabling of that one user-rule entry in the
[AdGuard filter editor](https://adguard.com/kb/adguard-for-windows/protection/ad-blocking/).
Removal has not been verified. No additional app or IP-range exclusion was
configured during the comparison.

The local evidence directory is `%TEMP%/mender-oauth-loopback-20261105/`:

- A: `oauth-baseline-normal-repeat.{log,exit}`,
  `oauth-baseline-race-repeat.{log,exit}` and `bodyprobe-baseline.log`.
- B: `oauth-scoped-rule-normal.{log,exit}`,
  `oauth-scoped-rule-race.{log,exit}`, `scoped-rule-stdlib.log` and
  `scoped-rule-stdlib-results.json`.
- C: `oauth-protection-off-normal.{log,exit}`,
  `oauth-protection-off-race.{log,exit}`, `protection-off-stdlib.log` and
  `protection-off-stdlib-results.json`.
- `oauth-source-before.json` records the OAuth source hashes before comparison;
  the single initial `oauth-baseline-normal.log` success is retained separately
  and was not treated as proof that the intermittent problem had disappeared.

Before starting the current complete `pnpm check`, `go clean -testcache` was run
so successes from a different protection state would not be reused. The final
results in state C were:

| Check | Result |
| --- | --- |
| Complete `pnpm check` | Passed, exit 0; log and status: `pnpm-check.{log,exit}` in the evidence directory above |
| OAuth normal / race, explicit `-count=20` | Both passed; no assertions changed and no network tests skipped |
| Full backend ordinary tests | All 97 test-bearing packages passed; Bootstrap 116.978s, OAuth 0.569s |
| Full backend race tests | All 97 test-bearing packages passed; Bootstrap 430.045s, OAuth 1.649s; no data race reported |
| Script / architecture / plugin / module / generation gates | All passed; six script tests and all existing protocol/SDK/Wails parity gates retained |
| Backend lint / frontend lint / typecheck | Passed; backend lint reported 0 issues |
| Frontend tests / i18n | 71 files and 467 tests passed; all 1,594 required messages translated |
| Web / Desktop frontend builds | Passed; the existing chunk-size-over-500-kB warnings remain |

The earlier capability-based restricted-host and local POSIX fixture skips
remain explicit skips, not additional platform guarantees. No R3 work, code
change, test-assertion change, dependency change or broader exclusion rule was
added in this follow-up. The 13 OAuth source/test files still match the saved
SHA-256 hashes. The earlier failed aggregate runs remain historical failures;
the successful result applies to the reported protection-off environment only.
