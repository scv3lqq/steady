# Session retrospective: account access contracts

## Goal

Prevent backend-only account provisioning and permissive auth test doubles from
being reported as complete access. Apply two demonstrated improvements, with no
runtime, credential, account, proxy or live database changes.

Base: `f3f47e6`. Branch: `feat/retro-account-access`.

## Evidence and expected effect

1. The initial account passed backend authentication but had no corresponding
   edge-gateway identity, leading to two access follow-ups. Add a complete public
   ingress provisioning procedure, covering every existing gate, credential
   handoff and the initial account. Expected effect: accounts work on the normal
   entry path when reported ready, with no corrective login turns.
2. The frontend test double accepted logout without the real handler's required
   binding header; real-browser validation found a surviving server session.
   Add a contract-sensitive test convention and real-handler lifecycle proof.
   Expected effect: permissive mocks do not mask auth integration failures.

The session also encountered a missing credential lookup before a GUI attempt,
a scoped linter lock conflict and oversized literal documentation plans. The
procedure addresses the credential lookup at its operational entry point. Shared
runtime workflow changes are excluded because their canonical repository is on
an existing unfinished feature branch with unrelated dirty files.

## Constraints and shared definitions

Read `AGENTS.md`, `CLAUDE.md` and `docs/spec.md`. There are no matching project
scoped rule files in this checkout. English public artifacts, no literal U+2014,
private identities, hosts, credential paths or session excerpts. Preserve existing
cookie, session-binding, rate-limit and CLI contracts. Instructions do not grant
new access or change approval policy. No production commands during this task.

`$PLAN_EDITS` is the existing canonical exact prose-edit helper, supplied in agent
briefs. Exact old blocks were generated from the base, never retyped. Task outputs
must equal base plus their own old/new edits byte for byte. The shared procedure
anchor is `deploy/README.md#account-access-through-additional-proxy-authentication`.

## Tasks

### T1: project instruction contract

Own only `AGENTS.md`. Apply `plans/retro-account-access/agents.md` against base.
The two new bullets cover HTTP-faithful auth doubles/real lifecycle checks and
whole-ingress account provisioning including the initial account. Preserve every
other byte. No speculative instruction cleanup. DoD: exact helper check and
`make check-text` pass. Gate: `$PLAN_EDITS check plans/retro-account-access/agents.md AGENTS.md f3f47e6`.

### T2: operator procedure

Own only `deploy/README.md`. Apply `plans/retro-account-access/operations.md`
against base. Add the five-step procedure exactly as planned, before Reaching the
app. It covers existing authorized credential sources, protected handoff, hidden
CLI input, application-scoped proxy access, existing identities, public HTTPS and
separating gateway/header rejection from actual session revocation. Preserve every
other byte. DoD: exact helper check and `make check-text` pass. Gate:
`$PLAN_EDITS check plans/retro-account-access/operations.md deploy/README.md f3f47e6`.

## Seams

| Producer | Consumer | Shared definition |
| --- | --- | --- |
| T2 procedure heading | T1 account instruction link | exact shared procedure anchor |
| T1 HTTP test rule | T2 final verification | valid proxy auth while testing Steady session rejection |

## Edge cases

- Shared proxy credential store: require explicit authorization for broader access;
  never silently add an application account to another service's gate.
- Bootstrap preserves inherited habits/settings; only ordinary create starts empty
  with defaults. Existing identities or passwords: preserve entries; no silent resets or disabling
  protections. Respect existing policy if credentials must differ between layers.
- Missing or locked password manager: normal authorized access flow; no unrelated
  secret discovery and no bypass of denied GUI access.
- Proxy 401 or a missing private-request binding is not proof of logout/reset.
  Retain the old cookie only in verifier memory and use GET /api/session, with
  currently valid proxy authentication, first proving 200 for that same identity
  and then requiring the actual Steady JSON 401 after logout/reset.
- Missing auth doubles/real handler proof is not PASS for changed auth behavior.
- macOS/Linux and missing tools: prose uses existing project commands and no new
  platform dependency. No tools or runtime defaults are installed or changed.
- Dirty shared instruction repository remains untouched; only this clean project
  branch is in scope. No worktrees or edits to live symlink targets elsewhere.

## Acceptance criteria and Gates

1. Both exact helper checks exit 0 with EDITS OK (two and one replacements).
2. `make check` exits 0, including generated freshness and existing regression
   coverage; optional missing shellcheck is reported honestly.
3. `git diff --check f3f47e6` exits 0. Diff contains only the two prose targets and
   these three plan artifacts; generated output is unchanged.
4. Independent skill-dry-run checks the plan before implementation, then simulates
   initial provisioning, later accounts, resets, shared proxy stores and gateway
   rejection against the finished instructions. It must find the single shared
   heading and no weakened boundary or contradictory lifecycle statement.
5. Independent acceptance validates final hashes, complete scope, exact prose
   checks and its own serial full Gates. Exact transcription tasks need no separate
   per-task reviewer; the validator checks their frozen expected prose.
6. Signed Conventional Commits, push feature branch and inspect CI through a draft
   PR. Do not merge main or trigger application deployment in this retrospective.

Baseline: the new instructions/procedure are absent on base; existing `make check`
is run and archived before the plan commit. These are prose-only edits, so no
behavioral test RED or code mutations are required. Instruction behavior is checked
through the independent dry runs and exact prose gates.

## Not doing

Changing shared runtime skills/native roles/global guidance, editing another
session's dirty branch, changing models, weakening permissions or approval policy,
editing application code/tests, live account changes, deployment or main merge.
