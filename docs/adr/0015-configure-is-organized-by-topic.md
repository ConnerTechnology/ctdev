---
status: accepted
date: 2026-10-15
---

# Configure is organized by topic, not by OS

`ctdev configure` categories are topics (keyboard, desktop, power, ssh), not operating systems.
No `configure linux` wrapper is added, and `configure macos` goes away. Each setting's platform
check already hides what doesn't apply, which is the reason the record gives. Decided by Thomas on
2026-10-15 in CTD-90, decision 7, during the review of every configure category.

## Consequences

The `macos` category is broken up by topic: Dock, Finder and animations move into `desktop`
(CTD-94), and typing into `keyboard` (CTD-95). Its last three settings went by topic too, as
Thomas decided on 2026-10-22 in CTD-95: the volume click into `audio`, expanded save and print
dialogs into `desktop`, and the password right after the screensaver into `power`. The code has
no `macos` category; `ctdev configure macos` reports an unknown category.
