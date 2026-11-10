# Native npm Launcher Upgrades

Investigated on November 10, 2026, on Windows with npm 12.2.2.

## Confirmed Cause

The installed Claude Code 2.1.322 and OpenCode 2.2.6 packages both declare a
postinstall script and platform-specific optional dependencies. Their scripts
replace the packaged placeholder with the matching native executable.

The local npm debug log for a Claude Code installation contained:

```text
1 package had install scripts blocked because they are not covered by allowScripts:
  @anthropic-ai/claude-code@2.1.322 (postinstall: node install.cjs)
```

That invocation already included `--include=optional --ignore-scripts=false`.
Those options alone did not authorize the script. A subsequent invocation with
`--allow-scripts=@anthropic-ai/claude-code` ran postinstall successfully.
The broken launcher's error about ignore-scripts/optional dependencies therefore
does not fully describe the npm 12 script-approval failure.

AgentUp previously generated bare `npm install -g <package>@latest`, inheriting
this default and allowing npm to exit successfully without installing a runnable
CLI. The same command construction exposed both tools to the problem. The
retained logs directly establish the Claude failure; they do not establish the
historical cause of the reported OpenCode failure.

## Fix

- Claude Code and OpenCode npm upgrades include `--include=optional` and
  `--ignore-scripts=false`.
- npm 12+ gets `--allow-scripts=<official-package>`. Earlier npm releases are
  probed for the setting, avoiding unknown flags on older npm versions.
- Only the configured tool package is approved. AgentUp does not allow every
  dependency script or persist changes to the user's npm configuration.
- pnpm upgrades explicitly enable scripts and optional dependencies and
  approve only the official package with `--allow-build=<official-package>`.
  pnpm 9 does not receive the unsupported approval flag; pnpm 10+ must expose
  it in `pnpm add --help`, otherwise AgentUp stops before installation and
  asks the user to update pnpm.
- Web UI command previews and execution use the same builder. A changed npm
  approval requirement invalidates an earlier confirmation.

## Broken Launchers

An unreadable CLI version is not evidence that the installation is healthy.
When an official manifest can be matched to the active executable or npm shim,
AgentUp retains its version separately as `package_version`. This is used only
to select the release line and verify that recovery does not downgrade or
migrate major versions.

The visible runtime `version` stays `unknown`. A successful registry query does
not mark the broken CLI as up to date, and the single-tool upgrade remains
available. An unrelated leftover package, an unrecognized major, malformed
metadata or a prerelease cannot authorize a guessed OpenCode recovery.

After installation, AgentUp runs version detection again. A readable package
manifest cannot substitute for a working `--version`; an unreadable executable
remains `unverified` and produces a failing command exit status.

## Manual npm 12 Recovery

Use these only for an existing npm installation:

```sh
npm install -g --include=optional --ignore-scripts=false --allow-scripts=@anthropic-ai/claude-code @anthropic-ai/claude-code@latest
npm install -g --include=optional --ignore-scripts=false --allow-scripts=@opencode/cli @opencode/cli@latest
claude --version
opencode --version
```

The OpenCode example is V2. For an existing V1 installation, use `opencode-ai`
in both places instead. Do not use this procedure to migrate major versions.

## References

- npm CLI v12.2.1, `docs/lib/content/commands/npm-install-scripts.md`:
  global installs require explicit `--allow-scripts` approval.
- Installed npm 12.2.2, `@npmcli/config/lib/definitions/definitions.js`:
  `allow-scripts`, `ignore-scripts` and `include` definitions.
- Installed Claude Code 2.1.322: `package.json` and `install.cjs`.
- Installed OpenCode 2.2.6: `package.json` and `postinstall.mjs`.
- Installed pnpm 10.28.1: `pnpm add --help`, `--allow-build` documentation.
