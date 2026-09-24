---
name: setup-skills
description: "Set up a repo created from thomasconner/repo-template: install the skills, fill AGENTS.md, create the Linear project, and pin this machine's plugins and skills. Run once, first thing in a new repo."
disable-model-invocation: true
---

# Setup Skills

Take a repo fresh from `thomasconner/repo-template` to the state where the engineering skills work
and `.claude/settings.json` names every plugin and skill on this machine. The repo's ADRs 0001–0007
explain the shape; read them before step 1.

This is a prompt-driven skill, not a script. Each step ends on its **done when** line. Ask Thomas
one question at a time, lead with the recommended answer, and write nothing that Thomas hasn't seen.

## 1. Explore

Before writing anything:

- **Set up before?** If `docs/agents/issue-tracker.md` exists, this repo was set up already. Say so,
  and offer to redo only the step Thomas names.
- **Slots**: list every `<!-- setup-skills: ... -->` in `AGENTS.md` and `README.md`.
- **Remote**: `git remote -v` gives the GitHub URL that step 5 links to the Linear project.

**Done when** you know whether this is a rerun and can list every slot.

## 2. Check Linear access

Every later step that touches Linear needs the repo's own `linear` server connected, and a problem
found now costs less than one found after the interview. From the repo root:

```bash
claude mcp get linear
```

Read the `Scope` and `Status` lines, and fix the first that applies:

- **`Scope: User config`, or `⏸ Pending approval`**: the repo's `.mcp.json` entry isn't approved
  yet, because the folder hasn't been trusted. Give Thomas these steps, then rerun the check:
  1. Exit this session
  2. Run `claude` in the repo root
  3. Accept the workspace trust dialog, and approve `linear` if asked
  4. Run `/setup-skills` again
- **`! Needs authentication`**: give Thomas these steps, then rerun the check:
  1. In a separate terminal, in the repo root, run `claude mcp login linear`
  2. Sign in to Linear in the browser tab it opens and approve access to the right workspace
  3. Back in this session, run `/mcp`, select `linear`, and choose **Reconnect**
- **`✘ Failed to connect`, or no `linear` at all**: show Thomas the `Issue:` line. Check that
  `.mcp.json` has the `linear` entry, that `allowedMcpServers` in `.claude/settings.json` includes
  `https://mcp.linear.app/*`, and that no `deniedMcpServers` or `disabledMcpjsonServers` entry names
  it. Fix and rerun.

Once it reads `Scope: Project config` and `✔ Connected`, call `get_workspace` (load the Linear
tools first if they're deferred) and ask Thomas to confirm that it's the right workspace. If it
isn't: `claude mcp logout linear`, then the login steps above, choosing the right one.

**Done when** `claude mcp get linear` shows the project scope, connected, and Thomas has confirmed
the workspace `get_workspace` returns.

## 3. Install the skills

```bash
npx skills@latest add thomasconner/skills --skill '*' --agent claude-code --copy --yes
```

Copy [THIRD-PARTY-LICENSE.txt](./THIRD-PARTY-LICENSE.txt) to `THIRD_PARTY_LICENSES.md` at the repo
root, under a first line: `The skills in .claude/skills/ are derived from mattpocock/skills and
distributed under this license.`

**Done when** `.claude/skills/` holds the fork's skills, `skills-lock.json` lists
`thomasconner/skills` as every skill's source, and `THIRD_PARTY_LICENSES.md` exists.

## 4. Fill AGENTS.md

Interview Thomas for each slot, one question per message:

1. **Repo name**, defaulting to the GitHub repo name.
2. **Purpose**: one paragraph on what the repo is for and who it serves. Draft it from what Thomas
   has said so far and ask for corrections.
3. **Guardrails**: the hard limits for this repo. Write each as the behavior to follow. A repo can
   have none yet; then the section says so in one line.

Fill the same name and purpose into `README.md`.

**Done when** no `<!-- setup-skills:` slot remains in `AGENTS.md` or `README.md` except the tracker
slot, which step 5 fills, and the Agent skills slot, which step 6 fills.

## 5. Linear

List the teams (`list_teams`) and ask which team this repo belongs to, recommending a new team when
the repo's work doesn't fit an existing one.

For a new team, the API can't create it. Give Thomas these steps (checked against Linear's UI on
2026-09-23; if a label differs, ask Thomas to describe the screen and restate every step):

1. Open `https://linear.app/<workspace>/settings/teams`
2. Click **Create team**
3. In **Icon & Name**, type the team name
4. In **Identifier**, type the key (3 letters, e.g. `FAM`)
5. Leave **Parent team** as *No parent team*
6. In **Team access**, choose *Private to team members* for personal or sensitive work, otherwise
   leave *Public to workspace*
7. Leave **Timezone** and **Copy from team** as they are
8. Click **Create team**

Wait for Thomas to say it's done, then confirm with `get_team`.

Then pick the project. List the team's projects (`list_projects` with the team) and ask Thomas
whether this repo uses one of them or gets a new one. Recommend an existing project when its name
matches the repo, or `get_project` shows this repo's GitHub URL among its resources; otherwise
recommend a new one. A new team has no projects, so skip the question.

- **Existing project**: leave its name, summary, and description alone. If the GitHub URL isn't
  among its resources, add it with `save_project` (`id` and `links`; links only append).
- **New project**: `save_project` with the repo name, the team, `lead: "me"`, `state: "started"`, a
  one-line `summary`, the purpose paragraph and the working agreement as `description`, and the
  GitHub URL in `links`.

Write `docs/agents/issue-tracker.md` from [issue-tracker-linear.md](./issue-tracker-linear.md) with
the team and project filled in, and fill the tracker slot in `AGENTS.md`.

**Done when** `get_project` returns the project with this repo's GitHub URL among its resources, and
`docs/agents/issue-tracker.md` has no `<...>` placeholders.

## 6. Labels and domain docs

- **Triage labels**: one question, "keep the default triage labels?" (recommended: yes). Write
  [triage-labels.md](./triage-labels.md) to `docs/agents/triage-labels.md`, with overrides if
  Thomas gave any.
- **Domain docs**: single-context. Write [domain.md](./domain.md) to `docs/agents/domain.md`
  without asking, unless the repo shows monorepo signals (`pnpm-workspace.yaml`, a `workspaces`
  field, populated `packages/*`), in which case ask.

Replace the Agent skills slot in `AGENTS.md` with:

```markdown
## Agent skills

### Issue tracker

Linear, team <TEAM KEY>, project <PROJECT ID>. See `docs/agents/issue-tracker.md`.

### Triage labels

[default labels | the overrides]. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context. See `docs/agents/domain.md`.
```

**Done when** all three `docs/agents/` files exist and `AGENTS.md` has no slots left.

## 7. Pin this machine

Read what's installed here: the plugin keys under `plugins` in
`${CLAUDE_CONFIG_DIR:-~/.claude}/plugins/installed_plugins.json`, the `name:` of every personal
skill (`~/.claude/skills/*/SKILL.md`) and synced skill (`~/.claude/skills/synced/*/*/SKILL.md`),
and whether `~/.claude/CLAUDE.md` is a symlink (`readlink -f`). Then edit `.claude/settings.json`:

- **Plugins**: an `enabledPlugins` entry of `false` for every installed plugin. Ask Thomas which, if
  any, this repo keeps (recommended: none), and set those to `true`.
- **Skills**: a `skillOverrides` entry for every personal and synced skill: `"off"` by default,
  `"on"` for any Thomas keeps. Every one gets an explicit value, so `check-plugins` can tell a kept
  skill from a new one.
- **Global instructions**: if `~/.claude/CLAUDE.md` is a symlink, add its resolved target to
  `claudeMdExcludes` as `**/` followed by the target's path relative to the home directory.
- **Claude in Chrome**: ask whether this repo reads web pages (recommended: no). If yes, remove the
  `claude-in-chrome` entry from `deniedMcpServers`.
- **Commands**: once the stack is known, add its build and test commands to `permissions.allow`.

**Done when** every installed plugin and every personal and synced skill has an explicit entry.

## 8. Verify

- `CLAUDE_PROJECT_DIR=$PWD .claude/hooks/check-plugins` prints nothing.
- `claude -p "/context"` lists only the repo's `CLAUDE.md` and `AGENTS.md` under memory files, and
  no MCP server except `linear` (and `claude-in-chrome` if step 7 kept it).
- Every `.json` file under `.claude/` and `.mcp.json` parses.

**Done when** all three pass. On a failure, fix the setting and rerun; don't move on with a failing
check.

## 9. Land it

Show Thomas the diff. After approval: commit on a branch `chore/setup-skills`, merge into `main`
with `git merge --no-ff`, push, and post the project's first status update with `save_status_update`
(`type: "project"`) covering what was set up and what's next.

**Done when** `main` is pushed and the status update is posted.
