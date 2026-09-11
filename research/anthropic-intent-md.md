# Anthropic intent.md guidance

Checked 2026-09-07 against Anthropic primary sources. This is research and a proposed local application, not an approval record or implementation result.

## What the guidance says

Anthropic's August 21, 2026 [AI-Native SDLC playbook](https://claude.com/blog/the-ai-native-sdlc-playbook) describes a chain of reviewable artifacts: intent, requirements/design specification, implementation plan, code/tests, and review findings. Each stage reads the previous artifact. The plays are modular; the article does not require adopting an enterprise workflow wholesale.

`intent.md` records the originator's problem, desired outcome, and constraints in their own terms. The suggested template includes affected users/systems and open questions, with author/status metadata in the example. The team agrees its template and version-controlled location; an `intent/` directory is suggested for a single product. These are examples and conventions, not a universal mandatory schema or filename casing rule. The product owner reviews/corrects the agent draft, and acceptance is recorded through the repository review process. See [Capture as intent.md](https://academy.claude.com/courses/ai-native-sdlc-playbook/capture-intent).

The next artifact, `spec.md`, converts accepted intent into requirements and design under applicable policies, explicitly identifying concerns and unresolved questions. The product owner checks that it solves the original problem and decides whether it proceeds to engineering. This is a separate decision from having an agent produce a draft. See [Requirements and design](https://academy.claude.com/courses/ai-native-sdlc-playbook/requirements-and-design).

`plan.md` describes implementation: files to change, work order, risks, alternatives, and checks that demonstrate success. An engineer reviews the plan before implementation; deviations update the plan. The lesson also allows accepted plans to support autonomous routine edits, so approval is not synonymous with prompting for every edit. See [Claude Code plan mode](https://academy.claude.com/courses/ai-native-sdlc-playbook/plan-mode).

## Convention versus automatic loading

The playbook tells users to attach or explicitly point Claude at intent/spec documents. Claude Code's [memory documentation](https://code.claude.com/docs/en/memory) separately documents automatic loading for `CLAUDE.md` and auto memory. Inference from these sources: do not claim that merely naming a file `INTENT.md` makes Claude Code automatically load or enforce it. Explicitly reference it in task instructions or a documented project entry point.

## Minimal application to this task

The following is a local recommendation based on the user's request, not an Anthropic product requirement:

1. Record the current intent in one short `INTENT.md`: the user can search products repeatedly through the ordinary Chrome extension bridge from the CLI without a search window interrupting their work. A work window may remain open, but must remain minimized across repeated searches. Browser closure and headless execution are not acceptance criteria.
2. Carry forward explicit constraints: no CDP; preserve the existing Chrome profile/session and cookies; preserve unrelated dirty-worktree changes; maintain typed core/CLI/MCP boundaries; never automate final purchase/payment or capture sensitive live payloads.
3. Link a bounded specification describing new and reused search windows, repeat searches, foreground/focus behavior, and observable failures. Distinguish requested behavior from behavior already verified. Keep Chrome lifecycle details in the narrow browser adapter.
4. Link a short implementation plan naming actual files and regression checks after inspecting the code. Verify initial and repeated CLI searches, minimized state throughout relevant transitions, and successful search completion. Record redacted metadata or synthetic evidence only.
5. Attribute existing user decisions to this conversation. Label newly proposed design choices accurately; an agent drafting a document is not evidence that a human reviewed that document. Do not retroactively invent an approval or add unrelated deployment/commit steps.

The smallest useful result is a readable intent → specification → plan linkage with honest decision and verification status. Existing task authorization remains the authority for local work; this research does not independently authorize external actions or impose new approval gates.
