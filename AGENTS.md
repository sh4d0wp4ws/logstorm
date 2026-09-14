# AGENTS.md

## Project Overview

`isc4-flog` is an independently maintained fork/customization of:

- Upstream: https://github.com/mingrammer/flog

The project keeps the original synthetic log generation capabilities and extends them with additional functionality for log-pipeline and SIEM testing.

Current custom functionality includes:

- TCP output
- UDP output
- Configurable network target using `--target host:port`
- Reuse of a single TCP connection during a generation run
- One generated log per UDP datagram
- YAML configuration for concurrent TCP and UDP streams
- RFC3164, RFC5424, and CEF log formats
- Per-stream EPS pacing
- Duration-bounded streams

Treat `origin` as this project's repository and `upstream` as the original `mingrammer/flog` repository.

---

## Core Development Principles

### Keep It Simple

Follow KISS.

Prefer the smallest change that correctly solves the requested problem.

Do not introduce new abstractions, packages, interfaces, configuration layers, dependencies, or concurrency unless the task clearly requires them.

Do not redesign working code solely to make it look cleaner.

### Robustness Principle

Design features to behave predictably under both valid and invalid conditions.

Be strict and consistent in what the program produces, and tolerate harmless
input variations where doing so is safe and unambiguous.

Do not interpret robustness as silently accepting malformed or ambiguous input.

Guidelines:

- Produce stable and predictable output.
- Validate user-controlled configuration and CLI arguments early.
- Accept harmless variations such as surrounding whitespace when unambiguous.
- Reject invalid or ambiguous values with clear errors.
- Do not silently guess the user's intent.
- Do not silently fall back to another protocol, format, target, or output type.
- Preserve backward-compatible behavior where practical.
- Propagate meaningful I/O and network errors instead of ignoring them.
- Clean up resources correctly when operations fail.
- Avoid partial or inconsistent state after an error.
- Distinguish application failures from external environment failures such as
  firewall rules, unreachable hosts, or receiver configuration.

### Preserve Existing Behavior

New work must not break existing behavior unless the task explicitly requires a breaking change.

Existing output types must continue to work:

- `stdout`
- `log`
- `gz`
- `tcp`
- `udp`

Existing log formats must remain compatible unless a task explicitly changes them.

### Keep Changes Focused

Only modify files required for the current task.

Do not:

- refactor unrelated code
- rename unrelated functions or variables
- reformat unrelated files
- reorganize directories without an explicit architecture task
- add speculative features
- implement future roadmap items while working on another feature

Keep diffs small and reviewable.

---

## Repository Architecture

Respect the current responsibilities of the source files.

### `main.go`

Application entry point.

Keep business logic out of this file.

### `option.go`

CLI options, defaults, parsing, and validation.

User-facing flags and validation logic belong here.

### `flog.go`

Main generation/orchestration flow.

It coordinates log generation and output writers.

Avoid putting transport-specific or format-specific implementation details here when they can remain isolated.

### `log.go`

Log-content generation.

This file should generate log content only.

Do not put:

- TCP logic
- UDP logic
- filesystem handling
- CLI parsing
- orchestration

into `log.go`.

### `writer.go`

Output writer selection.

It maps an output type to the appropriate `io.WriteCloser`.

Keep this as the common output boundary where practical.

### `network.go`

Network transport creation.

TCP/UDP-specific connection setup belongs here.

Do not duplicate network connection logic in log generators.

### `flog_unix.go` / `flog_windows.go`

OS-specific runtime behavior.

Keep platform-specific filesystem behavior isolated here.

### Tests

Keep Go tests next to their source files:

```text
network.go
network_test.go

option.go
option_test.go

flog.go
flog_test.go
```

Do not create a separate `tests/` directory unless the project architecture later provides a clear reason.

---

## Network Output Rules

### TCP

Current TCP behavior is:

- one target per stream (one stream in single-stream CLI mode)
- one TCP connection is opened for the generation run
- the same connection is reused for generated logs
- messages are newline-delimited
- connection and write errors are returned
- no automatic reconnect
- no retry/backoff

Do not silently change these semantics.

Do not reconnect for every generated log.

### UDP

Current UDP behavior is:

- one target per stream (one stream in single-stream CLI mode)
- one UDP socket/connected UDP writer is used for the generation run
- each generated log is written as one UDP datagram
- UDP delivery is not guaranteed
- only errors actually returned by the OS/socket should be propagated

Do not treat successful UDP `Write` as proof that the receiver accepted the message.

### TCP Framing

`isc4-flog` currently sends newline-delimited TCP messages.

It does not implement RFC 6587 octet-counted framing.

When documenting Rsyslog `imtcp` examples for raw logs, preserve the current recommendation:

```rsyslog
supportOctetCountedFraming="off"
```

Do not add TCP framing modes unless they are explicitly requested as a separate feature.

---

## Current Scope and Limitations

The single-stream CLI execution model is:

```text
one flog process
    =
one log format
    +
one output type
    +
one destination
```

Optional `--config` YAML configuration runs concurrent TCP/UDP streams. Each stream has one format, one output type, and one destination.

Do not document or assume the following as implemented unless the source code actually contains them:

- multiple destinations per stream
- automatic TCP reconnect
- retry/backoff
- TLS network output
- dynamic configuration reload
- vendor-specific formats not present in the source
- load balancing
- persistent queues

Future features must not be presented as current features.

---

## Upstream Compatibility

This project is based on `mingrammer/flog`.

When practical:

- preserve upstream structure
- minimize unnecessary divergence
- avoid large file/directory moves
- avoid broad refactors that make upstream changes difficult to merge later

Before reorganizing the repository, identify a concrete architectural problem that the reorganization solves.

More files alone are not a sufficient reason to split the project into packages.

When updating from upstream, inspect the changes carefully before merging.

---

## Dependency Policy

Prefer the Go standard library.

Add a third-party dependency only when it provides clear value and the task cannot be implemented cleanly with the standard library.

Before adding a dependency:

1. explain why it is needed
2. check whether the standard library is sufficient
3. keep the dependency scope minimal
4. avoid adding a framework for a small feature

Do not update unrelated dependencies during feature work.

Do not add defensive complexity for failures that can be handled clearly with
validation and explicit errors.

---

## Error Handling

Do not ignore meaningful errors.

In particular:

- propagate writer creation errors
- propagate network connection errors
- propagate writer `Write` errors
- handle `Close` errors where they can affect correctness
- preserve the original/primary error if cleanup also fails

Avoid double-closing writers.

Do not add complex retry/recovery behavior unless explicitly requested.

Prefer explicit failure over silently producing incomplete, redirected, or
ambiguous output.

---

## Resource Lifecycle

Every created writer or network connection must have clear ownership.

A writer should be closed at most once.

When replacing a writer, such as during file splitting:

1. transfer/clear ownership
2. close the old writer
3. handle the close error
4. create the replacement writer
5. assign ownership to the replacement

Do not leave cleanup behavior ambiguous.

---

## Testing Requirements

Every user-visible feature or bug fix should include appropriate tests.

Before considering a code task complete, run:

```bash
gofmt -w <modified-go-files>
go test ./...
go vet ./...
git diff --check
```

Prefer formatting only modified Go files when practical.

### Network Tests

Network integration tests must:

- use localhost
- use dynamically allocated ports
- avoid hard-coded ports such as `514` or `515`
- not require external network access
- not require a real Rsyslog server

TCP tests should verify relevant behavior such as:

- data is received
- logs are newline-delimited
- one connection is reused where expected
- connection/write errors are propagated

UDP tests should verify relevant behavior such as:

- one log corresponds to one datagram
- newline behavior is correct
- multiple logs reuse the same UDP writer/socket behavior

Do not make automated tests depend on firewall, SELinux, or external services.

---

## Manual Acceptance Testing

For network-related changes, automated tests should be supplemented with manual acceptance testing when practical.

Typical acceptance path:

```text
isc4-flog
   |
   +-- TCP/UDP
   |
Rsyslog listener
   |
output file
```

Manual environment issues such as firewall rules must be distinguished from application bugs.

Do not weaken application tests just because an external environment is misconfigured.

---

## Documentation Rules

`README.md` is user documentation, not a development diary.

Review and update README when a change affects:

- installation
- build commands
- CLI syntax
- flags
- supported formats
- supported output types
- user-visible behavior
- configuration
- examples
- limitations
- integration instructions

README does not normally need changes for:

- internal refactors
- variable renames
- test-only changes
- implementation cleanup with no user-visible effect

When user-facing behavior changes, documentation should be part of the same feature/PR when practical.

### README Style

Write README for a new user.

Prioritize:

1. what the project is
2. how to install/build it
3. how to use it quickly
4. supported capabilities
5. practical examples
6. limitations
7. development instructions
8. upstream attribution and license

Examples should be copy-paste friendly.

Do not document unimplemented features.

Do not write the README as if this repository were `mingrammer/flog`.

The upstream repository should be clearly credited and referenced, while the README primarily describes `isc4-flog`.

---

## Git Workflow

`main` is the stable/default development baseline for this repository.

Do not develop features directly on `main`.

Use focused branches such as:

```text
feature/network-output
feature/multi-stream
feature/new-log-format
fix/network-write-error
docs/rewrite-readme
refactor/<specific-purpose>
```

Before starting new work:

```bash
git switch main
git pull --ff-only origin main
git switch -c <branch-name>
```

Keep commits focused.

Use clear commit messages, for example:

```text
feat: add TCP and UDP log output
fix: prevent writer double close
docs: update network output examples
test: add TCP connection reuse coverage
refactor: isolate output writer creation
```

Do not commit or push unless the user explicitly asks you to do so.

Do not force-push unless explicitly requested and the consequences have been reviewed.

---

## Origin and Upstream

Expected remote roles:

```text
origin
  -> this isc4-flog repository

upstream
  -> https://github.com/mingrammer/flog.git
```

Do not push custom project changes to `upstream`.

When asked to synchronize with the original project, fetch from `upstream` and review differences before merging.

---

## Before Implementing a Non-Trivial Feature

For a non-trivial task, first inspect the current implementation and produce a short plan.

The plan should identify:

- current data flow
- affected files/functions
- smallest viable implementation
- compatibility impact
- error cases
- tests
- documentation impact

Do not modify files during the planning phase when the user explicitly requests analysis/plan first.

Once a plan is approved, implement only the approved scope.

---

## Repository Reorganization

Do not reorganize the repository simply because the number of files increases.

Consider extracting packages only when there are stable, meaningful boundaries such as:

- log generation
- configuration
- transport/output
- application orchestration

Before moving files or introducing `internal/`, `cmd/`, or additional packages:

1. explain the concrete problem with the current structure
2. propose the target structure
3. identify migration impact
4. evaluate upstream merge cost
5. obtain approval before making structural changes

Architecture refactors should be separate from feature work whenever possible.

---

## Completion Checklist

Before reporting a coding task as complete:

- confirm only intended files changed
- confirm no temporary/debug artifacts remain
- run `gofmt` for modified Go files
- run `go test ./...`
- run `go vet ./...`
- run `git diff --check`
- review `git status --short`
- summarize modified/created files
- explain behavior changes
- mention any unresolved limitations
- review whether README needs updating
- do not commit or push unless explicitly requested

If any verification step cannot be run, state that explicitly instead of claiming it passed.
