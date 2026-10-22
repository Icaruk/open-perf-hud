# AGENTS.md — rules for working in this repo

## Build

- **Never build the release exe by hand.** Use `task build`: it sets the flags
  that make the difference between a GUI app and a broken one
  (`-H windowsgui` on `open-perf-hud.exe`; absent on `oph-cli.exe` on purpose —
  the CLI is a console app). A manually built exe opens a `cmd` window and dies
  when the user closes it.
- Build flags live in `Taskfile.yml` (`build` task). If a build flag ever needs
  to change, change it there, nowhere else.
- Before committing: `task check` (gofmt + go vet + go test). All three clean.

## Commits

- Conventional Commits, English, imperative subject:
  `feat:`, `fix:`, `docs:`, `refactor:`, `chore:`, `test:`.
  Scope optional but encouraged: `fix(tray):`, `docs(readme):`, `refactor(menu):`.
- Subject ≤ 72 chars, no trailing period. Body explains *why*, in plain language.
- Version bumps are their own line in the same commit as the change, or their
  own commit (`chore: bump version to X.Y.Z`).

## Releases (GitHub)

1. Bump `const version` in `main.go` (semver: fix → patch, feature → minor).
2. `task check` → `task build`.
3. Commit, `git pull --rebase`, push (the remote moves — README edits happen
   from the web).
4. `gh release create vX.Y.Z open-perf-hud.exe --title vX.Y.Z --notes-file <file>`
   (long notes or special chars break `--notes` in pwsh; write them to a file).
   - Notes follow [Common Changelog](https://common-changelog.org): changes
     grouped under `### Changed`, `### Added`, `### Removed`, `### Fixed` (in
     that order, only the ones that apply), one line each, imperative mood
     ("Add…", "Fix…"), breaking changes first with a bold **Breaking:** prefix,
     skip internal noise. Categories are exactly those four — no "New".
   - Mirror every entry into `CHANGELOG.md` (latest first, `## X.Y.Z - DATE`,
     reference links to the GitHub release at the bottom) and commit it before
     tagging the release.
   - Notes are still **for non-technical users**: what changed, from the user's
     point of view ("the text could stay white and become hard to read; now it
     switches to dark"). No internals, no GDI/StretchBlt jargon.
   - Replacing a broken asset without a new release:
     `gh release upload vX.Y.Z open-perf-hud.exe --clobber`.
5. Sanity-check the exe after any release build change: PE subsystem must be 2
   (GUI). PowerShell: read the uint16 at PE header offset +0x5C.

## Repo gotchas

- `config.json` next to the exe is **the user's live config** (gitignored).
  Never overwrite it. Test builds run from a temp dir with their own config.
- The overlay is single-instance (`Local\OpenPerfHudOverlay` mutex). While an
  instance runs, launching a test exe pops a restart MessageBox — build test
  exes with a different mutex name, or test when the user's instance is closed.
- ETW (FPS) may need admin; without it `runOverlayMode` elevates via UAC. For
  automated tests of overlay rendering, build with a skip-elevate hook so no
  UAC prompt appears on the user's screen.
- Overlay render/sampling can only be verified on a real screen. Pattern that
  works: instrument the sample path behind an env var, run from a temp dir,
  paint a WPF form with the exact background colors under test, read the log.
- Code comments, identifiers and user-facing strings: English.

## Testing

- Logic that doesn't touch the screen (thresholds, luma math, config parsing)
  is covered by `go test`. Add a case there when touching it.
- GDI/Win32 behavior can't be unit-tested — verify it on screen with the
  pattern above before shipping.
