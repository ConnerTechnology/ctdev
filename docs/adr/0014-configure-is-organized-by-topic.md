---
status: accepted
date: 2026-10-15
---

# Configure is organized by topic, not by OS

`ctdev configure` categories are topics (keyboard, desktop, power, ssh), not operating systems.
No `configure linux` wrapper is added, and `configure macos` goes away. Each setting's platform check already
hides what doesn't apply, which is the reason the record gives. Decided by Thomas on 2026-10-15 in
CTD-90, decision 7, during the review of every configure category.

## Consequences

The `macos` category is broken up by topic: Dock, Finder and animations move into `desktop`, and
typing into `keyboard`. That work is CTD-94 and CTD-95. Until both land, the code still has a
`macos` category (`ctdev/cmd/configure_wizard.go`), so the code and this decision disagree.
