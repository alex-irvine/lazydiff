# lazydiff CI and Release Design

## Status

Approved design. This document defines validation CI, cross-platform release builds, version metadata, and README release instructions for `lazydiff`.

## Objective

Provide reproducible local and GitHub Actions builds for `lazydiff`, validate every pull request and push to `main`, and publish versioned binaries plus checksums when a `v*` tag is pushed.

The design ports the established `lazyorc` release pattern while adapting module paths, binary names, runtime prerequisites, and the `lazydiff` command entrypoint.

## Decisions

- Build entrypoint: `./cmd/lazydiff`.
- Version package: `github.com/alex-irvine/lazydiff/version`.
- Local build script: `scripts/build-all.sh`.
- Validation workflow: `.github/workflows/ci.yml`.
- Release workflow: `.github/workflows/release.yml`.
- Validation triggers: pull requests and pushes to `main`.
- Release trigger: tags matching `v*`.
- Release targets: Linux amd64, macOS amd64, macOS arm64, Windows amd64.
- Release assets: four raw binaries and `checksums.txt`; no archives in v1.
- Release publisher: `softprops/action-gh-release`.
- Release permission: `contents: write` only on release workflow.
- Version default: `dev`.
- Version injection: Go linker flag using the release tag.
- Installation instructions: GitHub CLI (`gh`) commands, with checksum verification.
- CI does not require Copilot CLI, Delta, or `gh`; tests use temporary repositories and fake executables.

## Version Metadata

Create `version/version.go`:

```go
package version

var Current = "dev"
```

`--version` and `-version` must:

- print `lazydiff <version>`;
- exit successfully;
- work outside a Git repository;
- avoid loading TOML, opening Git, starting Bubble Tea, or checking agent/Delta availability.

Release builds inject the tag:

```bash
-ldflags "-X github.com/alex-irvine/lazydiff/version.Current=${VERSION}"
```

Local builds without linker flags report `lazydiff dev`.

## Local Build Script

Create `scripts/build-all.sh` with strict shell behavior and quoted variables.

Invocation:

```bash
./scripts/build-all.sh [VERSION]
```

Default version is `dev`. Script removes and recreates `dist`, then builds `./cmd/lazydiff` into:

```text
dist/lazydiff-linux-amd64
dist/lazydiff-darwin-amd64
dist/lazydiff-darwin-arm64
dist/lazydiff-windows-amd64.exe
```

Each build sets `GOOS`, `GOARCH`, and the version linker flag. The script must fail immediately if any build fails. It then creates:

```text
dist/checksums.txt
```

with SHA256 entries for all release binaries. It prints artifact names and sizes after completion.

## Validation CI

Create `.github/workflows/ci.yml`:

- Name: `CI`.
- Trigger on `pull_request` and `push.branches: [main]`.
- Runner: `ubuntu-latest`.
- Checkout with `actions/checkout@v4`.
- Setup Go with `actions/setup-go@v5`, version `1.24`.
- Run checks in this order:

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

The workflow does not install or invoke Copilot, Delta, or `gh`. Existing integration tests create fake commands and temporary Git repositories.

## Release CI

Create `.github/workflows/release.yml`:

- Name: `Release`.
- Trigger on `push.tags: ["v*"]`.
- Set job-level `permissions.contents: write`.
- Checkout with `actions/checkout@v4`.
- Setup Go with `actions/setup-go@v5`, version `1.24`.
- Run `./scripts/build-all.sh "${{ github.ref_name }}"`.
- Print and validate `dist/checksums.txt`.
- Publish four binaries and `dist/checksums.txt` with `softprops/action-gh-release@v1`.
- Pass `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` to release action.

Release asset list:

```text
dist/lazydiff-linux-amd64
dist/lazydiff-darwin-amd64
dist/lazydiff-darwin-arm64
dist/lazydiff-windows-amd64.exe
dist/checksums.txt
```

Release body must identify:

- the release tag;
- Git requirement;
- Delta as optional with raw-diff fallback;
- Copilot requirement for default provider;
- generic-agent alternative;
- platform install commands;
- checksum file.

No release job should require credentials for Copilot, Delta, or GitHub CLI.

## README Release UX

Update `README.md` with:

### Build from source

```bash
go build -o lazydiff ./cmd/lazydiff
./lazydiff --version
```

### Install from GitHub release

Use repository `alex-irvine/lazydiff` and asset names matching the build script.

Linux:

```bash
mkdir -p ~/.local/bin
gh release download vX.Y.Z --repo alex-irvine/lazydiff --pattern 'lazydiff-linux-amd64' --output ~/.local/bin/lazydiff --clobber
chmod +x ~/.local/bin/lazydiff
```

macOS Apple Silicon:

```bash
mkdir -p ~/.local/bin
gh release download vX.Y.Z --repo alex-irvine/lazydiff --pattern 'lazydiff-darwin-arm64' --output ~/.local/bin/lazydiff --clobber
chmod +x ~/.local/bin/lazydiff
```

macOS Intel uses `lazydiff-darwin-amd64`.

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force "$HOME\.local\bin"
gh release download vX.Y.Z --repo alex-irvine/lazydiff --pattern "lazydiff-windows-amd64.exe" --output "$HOME\.local\bin\lazydiff.exe" --clobber
```

### Checksum verification

```bash
gh release download vX.Y.Z --repo alex-irvine/lazydiff --pattern checksums.txt
sha256sum -c checksums.txt
```

Document that `shasum -a 256 -c checksums.txt` is the macOS equivalent when `sha256sum` is unavailable.

### Upgrade

Repeat platform install command with a newer tag and `--clobber`.

### Requirements and configuration

README must state:

- Git is required.
- Delta is optional; raw diff is used if unavailable.
- Copilot CLI is required only for default `provider = "copilot"`.
- `provider = "generic"` supports configured local agents through stdin.
- Configuration path is `$XDG_CONFIG_HOME/lazydiff/config.toml` or `$HOME/.config/lazydiff/config.toml`.

### Development checks

Document:

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Link static visual reference at `docs/lazydiff-visual.html` and full product design at `docs/superpowers/specs/2026-07-17-lazydiff-design.md`.

## Error Handling

- Build script exits non-zero on any failed target build.
- Release workflow does not publish partial artifacts because build and checksum steps precede release action.
- Missing optional runtime tools are documented and handled by application fallback/configuration; CI does not treat them as build dependencies.
- Unsupported host shell behavior is avoided by using POSIX Bash in the script and GitHub Ubuntu runner.
- Invalid version input is passed through as linker metadata; tag-trigger restriction prevents ordinary branch builds from publishing.

## Testing

Add or update tests for:

- `--version` and `-version` before repository discovery;
- local build script output names and checksum contents using a temporary output directory or shell-level smoke test;
- release linker injection by building a temporary binary and checking version output;
- CI commands remaining identical to README commands.

Workflow syntax should be checked with a YAML parser or a local action linter when available. Full Go verification remains:

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

## Scope Exclusions

- No GoReleaser.
- No release archives.
- No Linux arm64 artifact in v1.
- No Homebrew, Scoop, apt, or package-manager publishing.
- No automatic installation of Copilot, Delta, or `gh` in CI.
- No change to runtime agent behavior beyond adding version fast-path.
