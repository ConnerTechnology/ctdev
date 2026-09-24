---
name: implement-spec
description: "Implement a specification in code."
disable-model-invocation: true
---

You have been provided a spec. This spec should have tickets associated with it, describing how to implement the spec.

The goal is a PR which implements the entire spec on a single branch.

The tickets are not a list of steps. They are a **task graph** with blocking relationships between them. This means there is always a **frontier** of tickets which are ready to be grabbed.

Communication to and from subagents should be sparse. Communicate primarily through **context pointers**: to the spec, tickets, research notes, and previous commits. Don't duplicate information already available via pointers.

**Implementer subagents** should be run in the background where possible for **maximum concurrency**.

## Steps

1. Read the spec and tickets. Read enough to understand the task graph.

2. (optional) Use an **exploration subagent** to conduct any exploration required by the tickets - relevant codebase files or external documentation. Ensure the exploration subagent can save files - it should save its markdown notes in a directory outside the repo, accessible by all future subagents. This lets **implementer subagents** focus on implementation rather than exploration.

3. Create a branch, and a draft PR. The PR should be marked as 'closing' the spec issue and tickets.

4. Use **implementer subagents** to implement each ticket. Each implementer subagent should work in its own worktree, on its own branch. End each implementer's prompt with the standing instruction in **Keeping implementers working**, below.

5. Once an **implementer subagent** completes, treat its final message as a report, not proof: check its ticket's acceptance criteria against its branch. If items are still open and it named no blocker, send it one message naming them, such as "Your ticket still has open items: <items>. Continue with them. If one is blocked, say what is blocking it." After two such messages on the same ticket, stop and raise it with the user. Once the criteria are met, merge its work to the PR branch with a **merger subagent**.

6. If this changes the **frontier** of available tickets, kick off more **implementer subagents** to work on the new tickets. This allows for maximum concurrency.

7. Once all tickets are complete, run /code-review on the PR branch. Fix all issues raised by the code review in a single **implementer subagent**.

8. Mark the PR as ready for review.

9. Clean up all **implementer subagent** worktrees.

## Keeping implementers working

Implementers run unattended, and a message with no tool call ends their run. Add this, verbatim, at the end of each implementer's prompt:

> A standing instruction from the coordinator, who is running you unattended. It is about how your turns end. A message with no tool call in it ends your run, and the work stops there. Do not end your run in any of these four ways while work on your ticket is still owed. One: a long summary of what was done that closes by announcing the next step and has no tool call, so the next thing never starts. Two: an offer to carry on with something unless the coordinator would prefer otherwise, which stops to wait for an answer nobody was going to give. Three: a list of decisions when, by your own account, none of them blocks the rest of the work. Four: deciding that this is a good place to report, because the run has been long or a milestone is done. Status notes are welcome, and so are your recommendations on open decisions, but put them in the same message as your next tool call and carry on with whatever does not depend on an answer. The stops that are wanted are the ones where nothing can move without an answer, or where the thing blocking you is deliberately protected from you. This does not override the need for confirmation on risky or destructive actions.

This follows Anthropic's guidance for unattended runs on Claude Opus 5.5: https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prompting-claude-opus-5-5#unattended-agentic-runs
