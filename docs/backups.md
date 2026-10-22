# Backups

Per-folder backup is being rebuilt under
[CTD-87](https://linear.app/conner-technology/issue/CTD-87). Until it lands,
`ctdev backup` prints one line saying it is coming.

`ctdev install restic` installs the restic binary and nothing else, with no
root: on Linux the official 0.19.1 release, checked against its `SHA256SUMS`
and put in `~/.local/bin` (apt's restic is too old for per-folder backup); on
macOS, Homebrew's.
