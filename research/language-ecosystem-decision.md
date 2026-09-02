# Product language and ecosystem decision

Validated: 2026-09-01 (Asia/Seoul)

## Decision

Use **Go for the distributed `coupangctl` product**, subject to one short
packaging proof before the schema and adapters become expensive to move.

This is a narrow win, not a claim that Go is universally better. Under the
weights that follow, Go scores 89.4/100, while C# scores 88.8, Bun scores 88.4,
and Deno and Rust score 87.8 and 87.6. Those candidates are inside the uncertainty of a judgment-based
matrix. Go is recommended because its particular combination of a small
runtime surface, ordinary per-platform executables, simple process lifecycle,
current Tier 1 MCP SDK, and viable CGO-free SQLite path best matches a local
consumer CLI that must launch and supervise a browser on three desktop OSes.

Keep the existing TypeScript files as **development-only protocol probes**.
Do not ship a TypeScript, Python, Playwright, Orca, accessibility, or agent-skill
sidecar. In particular, a bundled Python/Playwright helper observed in prior
art is evidence that such a split can work during research, not a structure to
inherit. The greenfield product should have one ownership boundary and one
distributed executable:

```text
Go executable
  typed core
    <- fixture document source
    <- native browser/CDP document source
  -> SQLite repository
  -> CLI adapter
  -> MCP stdio adapter

research/probes/*.ts   # development only; never in release artifacts
```

The language does not make the browser more or less “real.” Chrome CDP is a
JSON command/event protocol over a WebSocket, discovered through local HTTP
endpoints; Chrome itself documents the message structure, `/json/version`, and
the browser WebSocket URL. Any candidate can launch the same installed browser
with the same argv and speak the same protocol. The important controls are the
two-phase human-login architecture, dedicated profile, allowlisted read-only
navigation, and absence of browser automation during login—not the client
language. ([Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/))

## Product-specific assumptions

The decision assumes all of the following remain true:

- The user installs one CLI/MCP program on macOS, Windows, or Linux.
- Authentication opens an installed headed Chrome-family browser and leaves
  CAPTCHA, SMS, OTP, and login clicks to the user.
- Read-only sync later uses a short-lived loopback CDP connection to the same
  dedicated profile and parses structured JSON rather than DOM selectors.
- Normalized data lives in local SQLite; cookies remain browser-owned.
- MCP is local stdio first. A remote HTTP service is not a first-release goal.
- Purchase, payment, cancellation, and return automation remain out of scope.
- Release artifacts must not require Node, Python, Java, .NET, Playwright, or
  an agent runtime to be installed.

If the first-release target changes to Windows-only enterprise desktops, C#
should be reconsidered. If memory safety is made a release gate and a slower
implementation is acceptable, Rust should be reconsidered. If the team decides
that TypeScript velocity matters more than native-runtime conservatism, Bun is
the strongest TypeScript product runtime—not plain Node.

## What is independent of the language

Browser discovery and release signing are primarily OS problems:

- macOS browser discovery uses bundle metadata; release binaries need Developer
  ID signing and notarization. Apple says notarization scans signed software and
  requires appropriate signing, hardened runtime, and timestamping.
  ([Apple notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution),
  [distribution signing](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac/))
- Windows browser discovery uses registered applications/vendor paths; release
  executables can be signed and timestamped with the Windows SDK's SignTool.
  ([SignTool](https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool))
- Linux discovery uses desktop entries and `PATH`; there is no one universal
  desktop package, so publish signed/checksummed archives first and add distro
  packages selectively.
- Homebrew supports prebuilt “bottles,” independent of the implementation
  language. ([Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook))

Likewise, no candidate supplies one universal desktop keychain API. A narrow
platform adapter must use Apple Keychain Services, Windows Credential Manager,
and Linux Secret Service where available. Apple describes Keychain as encrypted
storage for small secrets; Windows `CredWrite` writes to the current user's
credential set; Linux Secret Service defines a D-Bus collection/item API.
([Apple Keychain Services](https://developer.apple.com/documentation/Security/keychain-services),
[Windows `CredWrite`](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew),
[Secret Service specification](https://specifications.freedesktop.org/secret-service/latest/))

For `coupangctl`, this adapter should not copy or re-store Chrome cookies. It is
only needed if the product later creates a small application secret, such as a
database-encryption key. Browser session data stays in the dedicated profile.

## Weighted decision model

Scores are 1 (poor) through 5 (excellent). They are architectural judgments
grounded in the facts below, not benchmark measurements. The weights total 100.

| Criterion | Weight | Why it matters here |
| --- | ---: | --- |
| Consumer distribution and packaging | 18 | A nontechnical user should receive one signed program with no runtime setup. |
| Native browser/process/CDP work | 15 | Browser discovery, safe argv launch, locking, shutdown, and a narrow WebSocket client are on the critical path. |
| SQLite portability | 12 | The local order ledger is the product; native-library surprises directly damage installation reliability. |
| Official MCP SDK/spec alignment | 14 | MCP is a first-class adapter, and hand-maintaining a fast-moving wire protocol is wasted risk. |
| Typed JSON/schema/domain modeling | 8 | Unstable upstream payloads must stop at narrow typed adapters. |
| OS secret/security integration | 8 | The product handles a sensitive browser profile and private local commerce data. |
| Long-running reliability/concurrency | 8 | Sync cancellation, process cleanup, SQLite serialization, and stdio lifetime must remain predictable. |
| Testing and contributor velocity | 8 | A small OSS project needs accessible tooling and fixture-driven tests. |
| Supply-chain/audit surface | 5 | Fewer transitive/runtime dependencies make a credential-adjacent program easier to audit. |
| Startup, binary size, runtime footprint | 4 | Relevant to CLI/MCP ergonomics, but less important than correctness and distribution. |

### Baseline matrix

| Candidate | Dist. 18 | Browser 15 | SQLite 12 | MCP 14 | Types 8 | Secrets 8 | Runtime 8 | DX 8 | Supply 5 | Footprint 4 | Weighted /100 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Go** | 5 | 5 | 4 | 5 | 4 | 3 | 5 | 4 | 4 | 4 | **89.4** |
| **C#/.NET** | 4 | 5 | 5 | 5 | 5 | 4 | 5 | 4 | 3 | 2 | **88.8** |
| **TypeScript/Bun** | 5 | 5 | 5 | 5 | 5 | 2 | 4 | 4 | 3 | 3 | **88.4** |
| **TypeScript/Deno** | 5 | 5 | 4 | 5 | 5 | 3 | 4 | 4 | 4 | 2 | **87.8** |
| **Rust** | 4 | 4 | 5 | 5 | 5 | 4 | 5 | 3 | 4 | 5 | **87.6** |
| TypeScript/Node | 2 | 5 | 3 | 5 | 5 | 2 | 4 | 5 | 2 | 2 | 72.6 |
| C++ | 5 | 3 | 5 | 1 | 3 | 4 | 5 | 2 | 4 | 5 | 72.2 |
| Kotlin/JVM | 3 | 4 | 3 | 4 | 4 | 3 | 5 | 3 | 3 | 1 | 69.0 |
| Python | 1 | 4 | 4 | 5 | 4 | 3 | 3 | 5 | 2 | 2 | 66.8 |
| Zig | 5 | 3 | 4 | 1 | 3 | 3 | 5 | 1 | 4 | 5 | 66.6 |
| Swift | 2 | 4 | 3 | 3 | 5 | 4 | 4 | 2 | 4 | 3 | 65.2 |

The top five are a practical tie. The matrix rules out weak fits; it does not
remove engineering judgment between close leaders.

## Candidate review

### Go

Go produces ordinary executables and directly supports many OS/architecture
targets through `GOOS`/`GOARCH`. Its standard library covers argv-based process
launch, HTTP, JSON, contexts/cancellation, and testing. The module system records
dependency hashes in `go.sum` and verifies public modules against a checksum
database by default. ([Go supported targets](https://go.dev/doc/install/source),
[Go executable build](https://go.dev/doc/tutorial/compile-install),
[Go module authentication](https://go.dev/ref/mod#authenticating))

The official Go MCP SDK is maintained with Google, has typed tool inputs and
outputs, stdio transport, and supports the current `2026-07-28` protocol plus
older revisions. ([official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk),
[version compatibility](https://github.com/modelcontextprotocol/go-sdk#version-compatibility))

SQLite is the one qualification. The common `mattn/go-sqlite3` route uses CGO,
which complicates cross-builds. `modernc.org/sqlite` is a CGo-free port and lists
the required desktop targets, but its own package documentation warns that the
matching `modernc.org/libc` version must be pinned exactly. That deserves a
release-matrix test and explains the 4 rather than 5.
([`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite))

Go's weaknesses are less expressive sum types than Rust/Swift and no first-party
cross-platform keychain abstraction. Keep upstream payload DTOs separate from
validated domain types, model state with explicit tagged structs, and isolate
per-OS secret code.

### TypeScript on Node, Bun, or Deno

The official TypeScript MCP SDK explicitly runs on Node, Bun, and Deno and ships
stdio server/client transports. Its schema layer accepts Standard Schema
implementations such as Zod and therefore offers the shortest route from the
existing probes to strongly validated adapter boundaries.
([official TypeScript MCP SDK](https://github.com/modelcontextprotocol/typescript-sdk))

The runtime choice changes the result materially:

- **Node:** familiar and mature, with a built-in `node:sqlite` API, although the
  current API is still marked release candidate. Its single-executable feature
  is also marked active development and embeds one CommonJS entry script. This
  is not the cleanest consumer distribution boundary.
  ([Node SQLite](https://nodejs.org/api/sqlite.html),
  [Node single executable applications](https://nodejs.org/api/single-executable-applications.html))
- **Bun:** `bun build --compile` creates a standalone executable, supports
  cross-target builds for Windows/macOS/Linux and x64/arm64, and includes
  `bun:sqlite`. This makes Bun a genuinely strong product candidate, not merely
  “Node with a different package manager.” Its deductions are ecosystem/runtime
  maturity, embedded-runtime footprint, and the need to verify that every MCP
  and OS-integration dependency survives compiled mode.
  ([Bun standalone executables](https://bun.sh/docs/bundler/executables),
  [Bun SQLite](https://bun.sh/docs/runtime/sqlite))
- **Deno:** `deno compile` creates a self-contained executable and cross-compiles
  to the target desktop triples. Deno implements `node:sqlite` and the MCP SDK
  supports Deno. Deno's own example says a compiled V8 executable is roughly
  70 MB. Its permission model is useful in development, but this product must
  grant process, profile filesystem, SQLite, and loopback network access, so it
  is not a complete security boundary.
  ([Deno compile](https://docs.deno.com/runtime/reference/cli/compile/),
  [compiled binary size example](https://docs.deno.com/examples/deno_compile/),
  [Deno SQLite](https://docs.deno.com/examples/sqlite/))

If Go's packaging proof fails, **Bun is the preferred fallback**, followed by
Deno. Do not call “TypeScript” one option without naming its runtime.

### Rust

Rust is the strongest choice for memory safety, explicit domain states, small
native executables, and controlled resource ownership. Cargo produces binary
targets and supports target triples. `rusqlite`'s `bundled` feature compiles and
links its included SQLite, avoiding dependence on the user's SQLite version.
([Cargo build targets](https://doc.rust-lang.org/cargo/commands/cargo-build.html),
[`rusqlite` bundled SQLite](https://github.com/rusqlite/rusqlite#usage))

The official Rust MCP SDK provides server/client functionality, stdio transport,
macros, and optional JSON Schema generation. MCP's 2026-07-28 launch announcement
initially described Rust support as beta, but the current official SDK index now
classifies Rust as Tier 1 alongside Go, TypeScript, Python, and C#. The score uses
the current Tier 1 status. Contributor ramp-up and async/FFI build complexity are
still higher for this small project, and Rust cross-compilation commonly needs a
target linker/toolchain in addition to `rustup target add`; this is why its
distribution score is 4 rather than 5.
([official Rust MCP SDK](https://github.com/modelcontextprotocol/rust-sdk),
[current official MCP SDK tiers](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/docs/2026-07-28/sdk.mdx),
[Rust cross-compilation](https://rust-lang.github.io/rustup/cross-compilation.html))

Rust would beat Go if memory-safety assurance and minimum footprint were assigned
more weight than implementation speed and cross-build simplicity. It does not need to
be a sidecar: if selected, it should own the whole product.

### C#/.NET

C# is closer to Go than the earlier quick assessment suggested. .NET can publish
self-contained single-file programs and Native AOT programs. Microsoft documents
the trade-off: single-file self-contained apps are larger and may start more
slowly; Native AOT improves startup and memory use but not every library is AOT
compatible. Builds are OS/architecture-specific.
([.NET publishing modes](https://learn.microsoft.com/en-us/dotnet/core/deploying/))

The official C# MCP SDK is maintained with Microsoft, and MCP lists C# as Tier 1
for the current revision. `Microsoft.Data.Sqlite` brings an SQLitePCLRaw native
bundle by default. `System.Text.Json`, source generation, `Process`,
`ClientWebSocket`, cancellation tokens, and strong Windows integration make this
technically excellent.
([official C# MCP SDK](https://github.com/modelcontextprotocol/csharp-sdk),
[`Microsoft.Data.Sqlite` native bundle](https://learn.microsoft.com/en-us/dotnet/standard/data/sqlite/custom-versions))

It loses narrowly because the conservative self-contained path embeds more
runtime, Native AOT must be proven across MCP/SQLite/keychain dependencies, and
the product is not Windows-first. If a clean Native AOT packaging spike passes on
all target OSes, C# is a fully defensible alternative.

### Python

Python has an excellent official MCP SDK, fast fixture/test iteration, type-hint
driven validation, and `sqlite3` in the standard library.
([official Python MCP SDK](https://github.com/modelcontextprotocol/python-sdk),
[Python `sqlite3`](https://docs.python.org/3/library/sqlite3.html))

Its problem is the product boundary. Python's packaging guide describes
standalone delivery as “freezing,” generally embedding the interpreter and often
requiring multiple third-party technologies. That recreates the sidecar/runtime
complexity this product should remove. Python remains useful for disposable
research, but it should not be shipped.
([Python application packaging](https://packaging.python.org/en/latest/overview/#bringing-your-own-python-executable))

### Kotlin/JVM

Kotlin can use the official Java MCP SDK, which includes stdio and asynchronous
and synchronous APIs. `jpackage` can build self-contained application images and
native installers, and GraalVM Native Image can remove the runtime dependency.
([official Java MCP SDK](https://github.com/modelcontextprotocol/java-sdk),
[Java MCP stdio server](https://github.com/modelcontextprotocol/java-sdk/blob/main/docs/server.md),
[`jpackage` guide](https://docs.oracle.com/en/java/javase/25/jpackage/packaging-tool-user-guide.pdf))

The widely used Xerial SQLite JDBC driver bundles per-OS native libraries and
extracts the selected library to a temporary directory. It supports GraalVM, but
that combination still adds JNI/native-image packaging cases. The JVM image is
large for a local CLI; GraalVM shifts complexity into build metadata and library
compatibility. Kotlin/JVM is strong for an existing JVM team, not this greenfield
consumer binary.
([Xerial SQLite JDBC](https://github.com/xerial/sqlite-jdbc))

### Swift

Swift has an expressive type system, native Apple security APIs, official MCP
SDK, and modern concurrency. Swift itself supports development on macOS, Linux,
and Windows, but its platform table shows builds are ordinarily native to each
development platform; Windows development also requires the C++ toolchain and
Windows SDK. The official MCP Swift SDK currently documents stdio transport for
Apple platforms and Linux with glibc—not Windows.
([Swift platform support](https://www.swift.org/platform-support/),
[Swift Windows requirements](https://www.swift.org/install/windows/),
[official Swift MCP SDK](https://github.com/modelcontextprotocol/swift-sdk))

Swift would be attractive for a macOS-only app. Its current cross-platform MCP,
SQLite/system-library, and release-CI shape is a poor fit for a three-OS CLI.

### C++ and Zig

Both can produce small native executables, call every OS API directly, and embed
SQLite. They are relevant only if minimum footprint or an existing native codebase
dominates every other concern. As of this review, MCP's official repository list
has no C++ or Zig SDK, and the official current-spec announcement names neither;
the product would own protocol generation, conformance, transports, and upgrades.
([official MCP repositories](https://github.com/orgs/modelcontextprotocol/repositories),
[MCP 2026-07-28 SDK status](https://blog.modelcontextprotocol.io/posts/2026-07-28/))

C++ also increases memory-safety review cost around private JSON and WebSocket
buffers. Zig has a smaller contributor/package ecosystem and would require more
FFI and platform code. Neither is justified for `coupangctl`.

## Sensitivity analysis

The same 1–5 scores were recomputed under three alternative weight sets:

| Scenario | Changed emphasis | Top results |
| --- | --- | --- |
| Installation-first | Distribution 28%, footprint 6%, DX 3% | Go 91.2, Bun 90.0, Deno 88.6 |
| Security/reliability-first | Security 15%, runtime 12%, DX 5% | C# 89.2, Rust 88.0, Go 87.4 |
| Velocity-first | Testing/DX 20%, types 12%, distribution 10% | C# 88.0, Bun 87.8, Go 87.6 |

Interpretation:

- Go is robust under the baseline and installation-first priorities.
- Rust moves into second place when safety and long-running reliability dominate,
  but its cross-toolchain distribution cost keeps it from the baseline lead.
- C# wins a security/reliability model because of strong platform/runtime APIs,
  but that assumes its Native AOT dependency proof succeeds.
- Bun nearly wins when preserving TypeScript development speed is emphasized.
- No reasonable weighting makes plain Node, Python, JVM, Swift, C++, or Zig the
  best balanced choice for the stated three-OS product.

Because the leaders are close, an unmeasured binary-size or AOT claim should not
decide the language. Measure the actual dependency graph.

## Required proof before committing to Go

Time-box this to a thin vertical slice; it is not a second research project.

1. Build signed-ready `darwin-arm64`, `darwin-amd64`, `windows-amd64`,
   `windows-arm64`, `linux-amd64`, and `linux-arm64` executables with CGO off.
2. Use `modernc.org/sqlite` with one migration and a synthetic insert/query;
   verify race tests, WAL behavior, clean-machine startup, and exact dependency
   pinning on every release target.
3. Launch a synthetic local Chrome/Chromium page with an isolated temporary
   profile, read `DevToolsActivePort`, connect over loopback WebSocket, obtain one
   structured JSON value, and shut down cleanly. Do not access Coupang or private
   state in this packaging proof.
4. Start an MCP stdio server using the official Go SDK and run the official
   conformance/Inspector smoke path without writing protocol output to stdout.
5. Verify the release archive contains only `coupangctl`, licenses, shell
   completions, and documentation—no Node/Python runtime, probes, Playwright,
   browser, profile, credentials, or native sidecar.
6. Record startup time, idle MCP memory, archive size, notarization result, and
   Windows signature verification. These are release baselines, not marketing
   comparisons.

If Go fails because of `modernc.org/sqlite` or release-target support, repeat
only this slice in Bun. If Go passes, proceed without maintaining two product
implementations.

## Implementation guardrails implied by the decision

- One Go module may contain multiple internal packages, but dependency direction
  remains `adapters -> application/core`; the core must not import CLI or CDP.
- Use the official Go MCP SDK; do not hand-roll MCP JSON-RPC.
- Keep the CDP adapter deliberately small: target discovery, allowlisted
  navigation, lifecycle wait, structured-document extraction, and shutdown.
- Generate or pin only the CDP methods actually used. Do not introduce a full
  browser automation framework.
- Use synthetic fixtures for parsers and SQLite tests. Never record real HARs,
  cookies, OTPs, raw order payloads, customer data, or stable identifiers.
- Treat the dedicated browser profile as sensitive OS-managed application state;
  never expose it through core types, CLI JSON, MCP, logs, exports, or backups.
- TypeScript probes can discover unstable endpoints, but a probe result enters
  production only through a documented adapter response shape and synthetic
  contract tests.

## Final recommendation in one sentence

Build a greenfield, single-binary Go product with TypeScript retained only for
non-shipping research probes; validate Go's CGO-free SQLite and six-target release
slice immediately, and use Bun—not Node/Python sidecars—as the fallback only if
that proof fails.
