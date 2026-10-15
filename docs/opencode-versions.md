# OpenCode V1/V2 Compatibility

Investigated on October 15, 2026. This covers the CLI, not the desktop app,
SDK version numbers, or the retired Go implementation.

## Findings

V1 and V2 are separate release lines. They share the `opencode` command but
use different distribution packages. The V2 npm manifest also exports an
`opencode2` alias. A shared executable name is not evidence of a V1 install.

The official migration guide says package-managed V1 should be removed before
installing V2. The V2 curl installer replaces the V1 binary at the shared
installation path. This is a migration, not an ordinary same-package update.

| Item | V1 | V2 |
| --- | --- | --- |
| Preferred executable | `opencode` | `opencode` |
| npm package | `opencode-ai` | `@opencode/cli` |
| Homebrew | `opencode`, also distributed through `anomalyco/tap/opencode` | `anomalyco/tap/opencode-v2` |
| Scoop | `opencode` | Not assumed by AgentUp |
| Official installer | `https://opencode.ai/install` | `https://opencode.ai/v2/install` |
| Installed version | `opencode --version` | `opencode --version` |
| Native same-line update | `opencode upgrade --method curl` | `opencode upgrade --method curl` |

The registry metadata observed during this investigation reported V1 `1.19.0`
and V2 `2.0.29`. These are dated observations, not versions pinned in AgentUp.
The V2 package requires its postinstall script. Its official pnpm installation
explicitly allows that script with `--allow-build=@opencode/cli`.

V2's introductory documentation currently says Windows package managers are
not supported, while the development updater source contains WinGet support.
AgentUp does not infer a V2 Scoop/WinGet mapping from this disagreement. Its
existing npm-family execution path remains supported when the V2 package can
be positively detected.

V1 `1.19.0` has a separate `opencode upgrade --major` path for migration. It can
remove the old package and install V2, and attempts to restore V1 if the new
installation fails. This must not be silently substituted for AgentUp's normal
upgrade action.

## AgentUp Behavior

1. Resolve `opencode` from PATH first; use `opencode2` only if it is absent.
2. Read the executable version and select the V1 or V2 distribution settings.
   Missing, unknown, unsupported future majors, and prerelease versions do not
   produce guessed upgrade commands.
3. Detect only the package belonging to that version line. A leftover V1 npm
   package must not classify the active V2 executable as a V1 package install.
4. Query the selected package/formula. A result from another major line is a
   failed check, not an update or proof that the installation is current.
   Post-upgrade verification also rejects a changed major or a prerelease
   version even if the upgrade command exits successfully.
5. Use the same resolved configuration for CLI execution and Web UI previews.
   Re-detection after UI confirmation rejects a changed upgrade command.
6. Update the active installation only. AgentUp does not enumerate multiple
   installations, switch the PATH winner, uninstall V1, or migrate user files.

### Package Manager Commands

| Source | V1 | V2 |
| --- | --- | --- |
| npm | `npm install -g opencode-ai@latest` | `npm install -g @opencode/cli@latest` |
| pnpm | `pnpm add -g opencode-ai` | `pnpm add -g --allow-build=@opencode/cli @opencode/cli@latest` |
| yarn | `yarn global upgrade opencode-ai` | `yarn global add @opencode/cli@latest` |
| Homebrew | `brew upgrade opencode` | `brew upgrade anomalyco/tap/opencode-v2` |
| Scoop | `scoop update opencode` | No automatic mapping |

Homebrew's JSON identifies a tapped formula with a short `name` and a
qualified `full_name`; both are recognized for the V2 lookup.

### Native Installations

An executable at the current user's official `~/.opencode/bin/opencode` location, including
the Windows `.exe` variant, uses the executable's own
`upgrade --method curl` command. This check precedes package-manager detection
so an unrelated global package does not take ownership of the native binary.
Matching only a path suffix is insufficient: another user's home or a custom
directory containing `.opencode/bin` is not the official installation location.

Latest-version checks use the selected stable package's official npm registry
metadata over HTTP, independently of a local npm/Node installation. Requests
are cancellable, time-bounded, size-bounded, and reject malformed responses,
HTTP errors, and cross-major results. Offline detection makes no HTTP request.

An arbitrary downloaded executable is not automatically labeled a curl
installation: V2's updater only recognizes the official location for that
method. Custom binary locations remain manual upgrades. AgentUp also leaves
prerelease channels manual because a version number alone does not establish
which channel/package should be updated.

### Migration Boundary

The official V2 guide identifies breaking changes in the plugin API, server
API/client contracts, and terminal client settings (`tui.json(c)` to global
`cli.json`). Much supported V1 configuration remains accepted, but a package
upgrade is not sufficient to migrate plugins or API integrations.

AgentUp intentionally does not run `--major`, remove an existing package, or
rewrite any OpenCode configuration.

## Primary Sources

- [V1 CLI documentation](https://opencode.ai/docs/cli/)
- [V2 installation documentation](https://opencode.ai/v2/docs)
- [Official V1-to-V2 migration guide](https://opencode.ai/v2/docs/migrate-v1)
- [V2 CLI commands](https://opencode.ai/v2/docs/cli/commands)
- [V1 npm stable metadata](https://registry.npmjs.org/opencode-ai/latest)
- [V2 npm stable metadata](https://registry.npmjs.org/@opencode%2fcli/latest)
- [V1 1.19.0 installer/update source](https://github.com/anomalyco/opencode/blob/v1.19.0/packages/opencode/src/installation/index.ts)
- [V1 1.19.0 upgrade command](https://github.com/anomalyco/opencode/blob/v1.19.0/packages/opencode/src/cli/cmd/upgrade.ts)
- [V2 development updater source](https://github.com/anomalyco/opencode/blob/dev/packages/cli/src/services/updater.ts)
- [V2 development Homebrew publisher](https://github.com/anomalyco/opencode/blob/dev/packages/cli/script/publish-homebrew.ts)

Documentation and development-branch sources can change. Recheck them before
adding channels, package aliases, or new platform-specific installation routes.

## Regression Coverage

- Configuration tests validate release-line selection and stable-version boundaries.
- Detector tests cover npm/pnpm/yarn, tapped Homebrew formulae, executable aliases,
  native paths, malformed registry responses, cancellation, and cross-major results.
- Upgrader tests cover command previews, stale confirmations, native upgrades,
  and post-upgrade release-line verification.
- Service tests exercise V1/V2 previews and execution, offline native previews,
  and refusal to execute after the detected release line changes.
- Browser tests verify V1, V2 pnpm, and Windows native commands in both the
  details view and confirmation dialog on desktop and mobile.
