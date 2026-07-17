# lazydiff CI and Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add version metadata, reproducible cross-platform builds, validation CI, tag-based GitHub Releases, checksums, and complete installation documentation to `lazydiff`.

**Architecture:** Keep runtime version state in a tiny `version` package and fast-path version flags before configuration or repository discovery. Put cross-platform build logic in one local Bash script used unchanged by release CI. Keep validation and release workflows separate, with fake-tool tests ensuring CI does not depend on Copilot, Delta, or `gh`.

**Tech Stack:** Go 1.24.2; standard `flag`; Bash; GitHub Actions `checkout@v4`, `setup-go@v5`, and `softprops/action-gh-release@v1`; SHA256 checksums.

## Global Constraints

- Build entrypoint: `./cmd/lazydiff`.
- Version package: `github.com/alex-irvine/lazydiff/version`.
- Validation triggers: pull requests and pushes to `main`.
- Release trigger: tags matching `v*`.
- Release targets: Linux amd64, macOS amd64, macOS arm64, Windows amd64.
- Release assets: four raw binaries and `checksums.txt`; no archives in v1.
- Local builds without linker flags report `lazydiff dev`.
- `--version` and `-version` work outside Git repositories and skip config, Git, Bubble Tea, agent, and Delta initialization.
- Version linker flag is `-ldflags "-X github.com/alex-irvine/lazydiff/version.Current=${VERSION}"`.
- CI does not install or invoke Copilot CLI, Delta, or `gh`.
- Validation commands are exactly `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and `git diff --check`.
- Release workflow has `contents: write`; validation workflow does not.
- README install commands use repository `alex-irvine/lazydiff` and `gh release download`.
- Do not add GoReleaser, release archives, Linux arm64, package-manager publishing, or runtime agent behavior beyond version fast-path.

---

## File Map

```text
version/version.go                         version value
cmd/lazydiff/main.go                       version flag fast-path and linker value
cmd/lazydiff/main_test.go                  version behavior tests
scripts/build-all.sh                       four-target builds and checksums
scripts/build-all_test.sh                  local build smoke test
.github/workflows/ci.yml                   PR/main validation
.github/workflows/release.yml              v* release publishing
README.md                                   build/install/version documentation
```

The existing product design, visual export, and application packages remain unchanged except for `cmd/lazydiff/main.go`, `cmd/lazydiff/main_test.go`, and README content required by this feature.

---

### Task 1: Add Version Package and Fast-Path Flags

**Files:**
- Create: `version/version.go`
- Modify: `cmd/lazydiff/main.go:1-88`
- Modify: `cmd/lazydiff/main_test.go:1-30`

**Interfaces:**
- Produces `version.Current` defaulting to `dev`.
- `run(ctx, args, stdin, stdout, stderr) error` handles `--version` and `-version` before config/Git startup.

- [ ] **Step 1: Write failing version tests**

Add tests that call `run` from a temporary non-Git directory with missing config:

```go
func TestRunVersionDoesNotNeedRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"lazydiff", "--version"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "lazydiff dev\n" {
		t.Fatalf("version output = %q", got)
	}
}

func TestRunShortVersionFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"lazydiff", "-version"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "lazydiff dev\n" { t.Fatalf("output = %q", stdout.String()) }
}
```

Run `go test ./cmd/lazydiff -run 'TestRun.*Version' -v`. Expected: FAIL because version package and stdout fast-path do not exist.

- [ ] **Step 2: Add version package**

Create:

```go
package version

var Current = "dev"
```

- [ ] **Step 3: Handle version before flag/config/Git setup**

Import `version` in `cmd/lazydiff/main.go`. At the start of `run`, before creating/parsing the `flag.FlagSet`, scan `args[1:]` for exactly `--version` or `-version`. Write `fmt.Fprintf(stdout, "lazydiff %s\n", version.Current)` and return nil. Keep `--help` behavior unchanged. Do not treat version strings inside unrelated arguments as flags.

- [ ] **Step 4: Run focused and full checks**

Run:

```bash
go test ./cmd/lazydiff -run 'TestRun.*Version' -v
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: all commands pass. Commit:

```bash
git add version cmd/lazydiff/main.go cmd/lazydiff/main_test.go
git commit -m "feat: add lazydiff version metadata"
```

---

### Task 2: Add Cross-Platform Build Script

**Files:**
- Create: `scripts/build-all.sh`
- Create: `scripts/build-all_test.sh`

**Interfaces:**
- `scripts/build-all.sh [VERSION]` creates `dist/` or `$LAZYDIFF_OUTPUT_DIR` with four binaries and `checksums.txt`.
- `LAZYDIFF_OUTPUT_DIR` is an optional test hook; default output remains `dist`.

- [ ] **Step 1: Write failing script smoke test**

Create a Bash test that runs from repository root:

```bash
#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
LAZYDIFF_OUTPUT_DIR="$out/dist" "$root/scripts/build-all.sh" test-version
test -x "$out/dist/lazydiff-linux-amd64"
test -x "$out/dist/lazydiff-darwin-amd64"
test -x "$out/dist/lazydiff-darwin-arm64"
test -x "$out/dist/lazydiff-windows-amd64.exe"
test -s "$out/dist/checksums.txt"
(cd "$out/dist" && sha256sum -c checksums.txt)
```

Run `bash scripts/build-all_test.sh`. Expected: FAIL because build script is absent.

- [ ] **Step 2: Implement strict build script**

Create Bash script with `set -euo pipefail`, resolve repository root from script location, set `VERSION="${1:-dev}"`, and set `OUTPUT_DIR="${LAZYDIFF_OUTPUT_DIR:-$ROOT/dist}"`. Remove/recreate output directory. Use a helper:

```bash
build_target() {
  local os="$1" arch="$2" output="$3"
  echo "  -> Building ${os}/${arch}..."
  GOOS="$os" GOARCH="$arch" go build \
    -ldflags "-X github.com/alex-irvine/lazydiff/version.Current=${VERSION}" \
    -o "$OUTPUT_DIR/$output" ./cmd/lazydiff
}
```

Build exactly:

```text
linux amd64 lazydiff-linux-amd64
darwin amd64 lazydiff-darwin-amd64
darwin arm64 lazydiff-darwin-arm64
windows amd64 lazydiff-windows-amd64.exe
```

Run `sha256sum` against only these four files and write `checksums.txt` in output directory. Print `ls -lh "$OUTPUT_DIR"`. Never use an unquoted path.

- [ ] **Step 3: Run script test and inspect version injection**

Run `bash scripts/build-all_test.sh`. Expected: PASS, four executable artifacts, checksum verification passes. Build and inspect one binary:

```bash
tmp=$(mktemp -d)
GOOS=linux GOARCH=amd64 go build -ldflags "-X github.com/alex-irvine/lazydiff/version.Current=integration-test" -o "$tmp/lazydiff" ./cmd/lazydiff
test "$(cd "$tmp" && ./lazydiff --version)" = "lazydiff integration-test"
rm -rf "$tmp"
```

- [ ] **Step 4: Run full checks and commit**

Run `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and `git diff --check`. Expected: PASS. Commit:

```bash
git add scripts
git commit -m "build: add cross-platform release script"
```

---

### Task 3: Add Validation Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- GitHub Actions workflow named `CI` runs the four global validation commands on PRs and pushes to `main`.

- [ ] **Step 1: Write workflow content**

Create:

```yaml
name: CI

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - name: Test
        run: go test ./... -count=1
      - name: Vet
        run: go vet ./...
      - name: Build
        run: go build ./...
      - name: Check diff
        run: git diff --check
```

- [ ] **Step 2: Validate workflow syntax**

Run a YAML parser if available. If no parser is installed, run `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/ci.yml")'`; note that GitHub’s `on` key may be parsed as boolean by YAML 1.1, which is harmless for syntax validation. Verify workflow contains no Copilot, Delta, or `gh` setup.

- [ ] **Step 3: Commit workflow**

Run `git diff --check`, then commit:

```bash
git add .github/workflows/ci.yml
git commit -m "ci: validate lazydiff changes"
```

---

### Task 4: Add Tag Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Interfaces:**
- Tag push matching `v*` builds and publishes the four binaries plus `checksums.txt`.

- [ ] **Step 1: Write release workflow**

Create:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - name: Build binaries
        run: ./scripts/build-all.sh "${{ github.ref_name }}"
      - name: Verify checksums
        run: (cd dist && sha256sum -c checksums.txt)
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            dist/lazydiff-linux-amd64
            dist/lazydiff-darwin-amd64
            dist/lazydiff-darwin-arm64
            dist/lazydiff-windows-amd64.exe
            dist/checksums.txt
          body: |
            ## lazydiff ${{ github.ref_name }}

            Git is required. Delta is optional and raw diff is used when Delta is unavailable.
            Default AI provider is GitHub Copilot CLI; configure provider = "generic" for another local agent.
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

The workflow must not invoke Copilot, Delta, or `gh`. `contents: write` applies only to release workflow.

- [ ] **Step 2: Validate assets and permissions**

Check each artifact in `files` matches the build script exactly. Confirm trigger is only `push.tags: ["v*"]`, `permissions.contents` is `write`, and no archive or Linux arm64 asset is present.

- [ ] **Step 3: Commit workflow**

Run `git diff --check`, then commit:

```bash
git add .github/workflows/release.yml
git commit -m "ci: publish lazydiff releases"
```

---

### Task 5: Document Release Installation and Verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- README exposes source build, version output, release install, checksums, upgrade, prerequisites, configuration, and development checks.

- [ ] **Step 1: Add version and source-build documentation**

Update Build section:

```bash
go build -o lazydiff ./cmd/lazydiff
./lazydiff --version
```

State that local builds print `lazydiff dev` and release binaries print their tag version.

- [ ] **Step 2: Add platform release commands**

Add Linux and macOS commands:

```bash
mkdir -p ~/.local/bin
gh release download vX.Y.Z --repo alex-irvine/lazydiff --pattern 'lazydiff-linux-amd64' --output ~/.local/bin/lazydiff --clobber
chmod +x ~/.local/bin/lazydiff
```

Use `lazydiff-darwin-arm64` for Apple Silicon and `lazydiff-darwin-amd64` for Intel. Add Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force "$HOME\.local\bin"
gh release download vX.Y.Z --repo alex-irvine/lazydiff --pattern "lazydiff-windows-amd64.exe" --output "$HOME\.local\bin\lazydiff.exe" --clobber
```

- [ ] **Step 3: Add checksums, upgrade, prerequisites, and CI commands**

Add:

```bash
gh release download vX.Y.Z --repo alex-irvine/lazydiff --pattern checksums.txt
sha256sum -c checksums.txt
```

Document macOS `shasum -a 256 -c checksums.txt`, repeat install with newer tag/`--clobber`, Git required, Delta optional, Copilot required only for default provider, generic stdin alternative, config paths, and:

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Link `docs/lazydiff-visual.html` and the two design specs.

- [ ] **Step 4: Review README and commit**

Run `grep -nE 'lazydiff-(linux|darwin|windows)|checksums|--version|gh release download|go test ./\.\.\.' README.md` and verify names match release workflow. Run `git diff --check`, then commit:

```bash
git add README.md
git commit -m "docs: document release installation"
```

---

### Task 6: Final Verification

**Files:**
- Test: all repository files

- [ ] **Step 1: Run version smoke tests**

From a non-Git temporary directory, run the built binary with `--version` and `-version`. Expected: both exit 0 and print `lazydiff dev`.

- [ ] **Step 2: Run build script smoke test**

Run `bash scripts/build-all_test.sh`. Expected: all four binaries exist, are executable, and `sha256sum -c checksums.txt` passes.

- [ ] **Step 3: Run complete checks**

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
git status --short
```

Expected: all checks pass and only intentional release/README files are present before final commit.

- [ ] **Step 4: Check final workflow consistency**

Verify `ci.yml` commands equal README commands, release asset list equals build-script output, `v*` is the only release trigger, and release workflow alone has `contents: write`.

---

## Plan Self-Review

### Spec coverage

- Version package, defaults, linker injection, and fast-path flags: Task 1.
- Local four-target script, output override, and checksums: Task 2.
- Pull request/main validation workflow: Task 3.
- Tag release workflow, permissions, assets, and release body: Task 4.
- README source build, gh installation, checksums, requirements, upgrades, and links: Task 5.
- Version/build/script/CI/release consistency verification: Task 6.

### Placeholder scan

No implementation instruction uses `TBD`, `TODO`, `FIXME`, or an unspecified file. `vX.Y.Z` appears only as an explicit README command placeholder users replace with release tag.

### Type and path consistency

- Linker package path matches `go.mod`: `github.com/alex-irvine/lazydiff/version`.
- Build entrypoint matches `cmd/lazydiff`.
- Script output names match release workflow and README commands.
- CI commands match README verification commands.
- Release tag expression and version argument both use `${{ github.ref_name }}`.

### Scope check

Plan contains one cohesive release-infrastructure subsystem with six independently verifiable tasks. No runtime agent, Git, Delta, or UI behavior changes beyond version fast-path are included.
