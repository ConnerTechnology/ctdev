# Triage labels

The plugin skills speak in five canonical triage roles and two categories. This repo has one
reporter — Thomas — so most of the inbound-queue machinery has nothing to do, and the roles map
onto what Linear already has. Both labels exist on the CTDev team (2026-09-24).

| Role in mattpocock/skills | In our tracker                                  | Meaning here                                                                          |
| ------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------- |
| `needs-triage`            | State **Backlog**, before `/to-tickets`         | Nobody has shaped it into a ticket yet                                                |
| `needs-info`              | Label `ready-for-human`                         | The missing information is always a question for Thomas                               |
| `ready-for-agent`         | Label `ready-for-agent`                         | The plan is on the issue and Thomas approved it; an agent can take it unattended      |
| `ready-for-human`         | Label `ready-for-human`                         | Only Thomas can do it — the issue says why, and gives him numbered steps              |
| `wontfix`                 | State **Canceled**                              | Ruled out. There is no `.out-of-scope/` folder and no parked-ideas project here       |
| `bug`                     | Label `bug`                                     | Something is broken                                                                   |
| `enhancement`             | Label `enhancement`                             | New behavior, or a better version of existing behavior                                |

When a skill mentions a role, use the mapping above. Do not create the canonical label strings in
Linear.
