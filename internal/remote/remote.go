// Package remote implements remote rule imports.
//
// Rule directories may declare a pasta.cue manifest at their root with
// the form:
//
//	imports: {
//	    "github.com/alice/lint-rules": "v1.2.3"
//	}
//
// Versions are git tags (or branches). Resolved commit SHAs are
// recorded in a sibling pasta.lock so loads are reproducible offline.
// Module contents are cached under $XDG_CACHE_HOME/pasta/modules/.
//
// v1 is intentionally minimal:
//   - flat dependencies only (a remote module declaring its own remote
//     imports is rejected)
//   - no semver constraints; the manifest version string is treated as
//     a literal git ref
//   - no transitive resolution / no MVS
//   - the git binary is the fetcher (covers SSH agents, credential
//     helpers, private repos out of the box)
//
// All of the above can be replaced later without changing the
// declaration site or the import-path scheme rules already use.
package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// ManifestFile is the well-known filename pasta looks for at the rule
// directory root for the imports manifest.
const ManifestFile = "pasta.cue"

// LockFile is the well-known filename for the resolved-version lockfile
// pasta writes next to the manifest.
const LockFile = "pasta.lock"

// Manifest is the parsed contents of a rule directory's pasta.cue.
//
// Modules maps a module path (e.g. "github.com/alice/lint-rules") to a
// version string. The version is whatever the user wrote in the
// manifest — typically a git tag like "v1.2.3", but a branch name or
// commit SHA also works.
type Manifest struct {
	Modules map[string]string
}

// Lockfile records the resolved commit SHA for every module in the
// manifest. It's read alongside the manifest at load time and written
// by `pasta sync`.
//
// There's no schema-version field: this is v1. If a future version
// needs incompatible semantics, it'll add a `version` field then and
// treat its absence as v1.
type Lockfile struct {
	Modules map[string]LockedModule `json:"modules"`
}

// LockedModule is one entry in Lockfile.Modules.
type LockedModule struct {
	// Version is the literal version string from the manifest, copied
	// in so we can detect manifest drift without re-reading pasta.cue.
	Version string `json:"version"`
	// Commit is the full git SHA the version resolved to. The cache
	// key is (module path, commit) — re-tagging upstream after a sync
	// cannot retroactively change what we use.
	Commit string `json:"commit"`
	// Hash is sha256 over the module's tree (sorted file paths +
	// contents). Verified on cache reuse during sync and on every
	// load — a mismatch means the cache was tampered with or
	// corrupted, and the user is told to re-run `pasta sync`.
	Hash string `json:"hash"`
}

// LoadManifest reads dir/pasta.cue and returns its Manifest. The
// second return is false (with a nil error) if no manifest file
// exists; callers treat that as "no remote imports".
func LoadManifest(dir string) (*Manifest, bool, error) {
	path := filepath.Join(dir, ManifestFile)
	src, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileBytes(src, cue.Filename(path))
	if err := v.Err(); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	imports := v.LookupPath(cue.ParsePath("imports"))
	if !imports.Exists() {
		// File exists but declares no imports — treat as empty.
		return &Manifest{Modules: map[string]string{}}, true, nil
	}
	m := &Manifest{Modules: map[string]string{}}
	iter, err := imports.Fields()
	if err != nil {
		return nil, false, fmt.Errorf("%s: imports must be a struct: %w", path, err)
	}
	for iter.Next() {
		key := iter.Selector().Unquoted()
		ver, err := iter.Value().String()
		if err != nil {
			return nil, false, fmt.Errorf("%s: imports[%q] must be a string version: %w", path, key, err)
		}
		if err := validateModulePath(key); err != nil {
			return nil, false, fmt.Errorf("%s: %w", path, err)
		}
		if ver == "" {
			return nil, false, fmt.Errorf("%s: imports[%q]: version must not be empty", path, key)
		}
		m.Modules[key] = ver
	}
	return m, true, nil
}

// validateModulePath rejects module paths that would let a malicious
// manifest escape the cache directory or hit unexpected URLs. We keep
// this strict on purpose — anything weird should be a load error, not
// a partial fetch.
func validateModulePath(p string) error {
	if p == "" {
		return fmt.Errorf("module path must not be empty")
	}
	if strings.ContainsAny(p, " \t\r\n") {
		return fmt.Errorf("module path %q contains whitespace", p)
	}
	if strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
		return fmt.Errorf("module path %q must be relative (no leading /)", p)
	}
	// Reject path-traversal segments. A substring check would
	// over-reject legitimate names like "foo..bar"; we only care
	// about whole segments equal to "." or "..".
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return fmt.Errorf("module path %q has an empty segment", p)
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("module path %q must not contain . or .. segments", p)
		}
	}
	// Require at least one slash so we have something that looks like
	// "<host>/<path>". This rejects bare names like "foo".
	if !strings.Contains(p, "/") {
		return fmt.Errorf("module path %q must look like <host>/<path>", p)
	}
	return nil
}

// LoadLockfile reads dir/pasta.lock. Returns (nil, false, nil) when
// the file doesn't exist — callers treat that as "lockfile needs to be
// generated".
func LoadLockfile(dir string) (*Lockfile, bool, error) {
	path := filepath.Join(dir, LockFile)
	src, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	var lf Lockfile
	if err := json.Unmarshal(src, &lf); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if lf.Modules == nil {
		lf.Modules = map[string]LockedModule{}
	}
	return &lf, true, nil
}

// WriteLockfile marshals lf and writes it to dir/pasta.lock with a
// trailing newline. Modules are emitted in sorted key order (by
// encoding/json's map handling) so the file is stable across runs
// and friendly to diffs.
func WriteLockfile(dir string, lf *Lockfile) error {
	path := filepath.Join(dir, LockFile)
	mods := lf.Modules
	if mods == nil {
		mods = map[string]LockedModule{}
	}
	data, err := json.MarshalIndent(struct {
		Modules map[string]LockedModule `json:"modules"`
	}{mods}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Fetcher resolves a (modulePath, version) pair to an on-disk
// directory plus its commit SHA. Everything else in this package goes
// through this interface so tests can supply a fake without hitting
// the network.
type Fetcher interface {
	// Fetch ensures the module is cached locally and returns
	//   - localDir: the directory containing the module's files
	//   - commit:   the full SHA the version resolved to
	Fetch(modulePath, version string) (localDir, commit string, err error)
	// FetchCommit fetches a specific commit (used when the lockfile
	// already pinned one). Implementations may share storage with
	// Fetch — the commit becomes the cache key either way.
	FetchCommit(modulePath, commit string) (localDir string, err error)
	// ListTags returns every tag advertised by the remote, in no
	// particular order. Used by `pasta bump` to find the latest
	// semver tag without cloning. Implementations should be cheap
	// (a single ls-remote --tags is enough).
	ListTags(modulePath string) ([]string, error)
}

// GitFetcher is the production Fetcher: shells out to `git` to clone
// modules into CacheDir.
//
// CacheDir is the root under which modules are stored. Layout:
//
//	<CacheDir>/modules/<modulePath>@<commit>/...
//
// Keying by commit (not tag) means that re-tagging upstream after a
// sync cannot silently change what subsequent runs see — they'll just
// re-use the existing cache directory under the locked SHA.
type GitFetcher struct {
	CacheDir string
}

// Fetch resolves version to a commit SHA, then defers to FetchCommit.
// Two-step because we want the user's version string (which may be a
// tag, branch, or short SHA) recorded in the lockfile alongside the
// resolved full SHA.
func (g *GitFetcher) Fetch(modulePath, version string) (string, string, error) {
	commit, err := g.resolve(modulePath, version)
	if err != nil {
		return "", "", err
	}
	dir, err := g.FetchCommit(modulePath, commit)
	if err != nil {
		return "", "", err
	}
	return dir, commit, nil
}

// FetchCommit is idempotent: if the cache already has the commit, it
// returns immediately. Otherwise it clones into a temp dir and renames
// into place atomically so a partial clone can't be observed as
// complete.
func (g *GitFetcher) FetchCommit(modulePath, commit string) (string, error) {
	target := g.cachePath(modulePath, commit)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("mkdir cache: %w", err)
	}
	// Clone into a temp sibling first so a crash mid-fetch can't
	// leave a half-populated dir at the canonical path.
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".pasta-fetch-*")
	if err != nil {
		return "", fmt.Errorf("mktemp: %w", err)
	}
	cloneURL := "https://" + modulePath + ".git"
	// init + fetch + checkout instead of clone --branch lets us pin
	// to a commit SHA as well as a tag, with a shallow fetch.
	steps := [][]string{
		{"git", "init", "--quiet", tmp},
		{"git", "-C", tmp, "remote", "add", "origin", cloneURL},
		{"git", "-C", tmp, "fetch", "--depth", "1", "--quiet", "origin", commit},
		{"git", "-C", tmp, "checkout", "--quiet", "FETCH_HEAD"},
	}
	for _, args := range steps {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			os.RemoveAll(tmp)
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args[1:], " "), err, strings.TrimSpace(string(out)))
		}
	}
	// Drop .git to save space — we never need history again, the
	// commit SHA is locked.
	if err := os.RemoveAll(filepath.Join(tmp, ".git")); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("clean .git: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.RemoveAll(tmp)
		// Race: another process may have populated the cache between
		// our Stat above and now. On Linux, os.Rename refuses to
		// replace an existing non-empty directory (ENOTEMPTY) — the
		// loser of the race lands here. macOS / Windows behave the
		// same way for non-empty dirs. Either way: if the target
		// exists now, the winner's contents are valid; use them.
		if _, statErr := os.Stat(target); statErr == nil {
			return target, nil
		}
		return "", fmt.Errorf("rename cache entry: %w", err)
	}
	return target, nil
}

// resolve runs `git ls-remote` to translate a tag/branch/short-SHA
// into a full commit SHA without cloning. Cheaper than fetching, and
// we'd need the full SHA to key the cache anyway.
func (g *GitFetcher) resolve(modulePath, version string) (string, error) {
	cloneURL := "https://" + modulePath + ".git"
	// ls-remote prints "<sha>\trefs/tags/v1.2.3" lines. Trying both
	// tag and branch namespaces in one call is what `git ls-remote`
	// already does when given a ref pattern.
	cmd := exec.Command("git", "ls-remote", "--exit-code", cloneURL,
		"refs/tags/"+version, "refs/tags/"+version+"^{}",
		"refs/heads/"+version)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err == nil {
		// Prefer a peeled tag (^{}) when present — that's the commit
		// the tag points to, even for annotated tags.
		var tagSHA, peeledSHA, branchSHA string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			sha, ref := fields[0], fields[1]
			switch {
			case strings.HasSuffix(ref, "^{}"):
				peeledSHA = sha
			case strings.HasPrefix(ref, "refs/tags/"):
				tagSHA = sha
			case strings.HasPrefix(ref, "refs/heads/"):
				branchSHA = sha
			}
		}
		switch {
		case peeledSHA != "":
			return peeledSHA, nil
		case tagSHA != "":
			return tagSHA, nil
		case branchSHA != "":
			return branchSHA, nil
		}
	}
	// Fall through: maybe the user pinned to a full commit SHA.
	if looksLikeFullSHA(version) {
		return strings.ToLower(version), nil
	}
	return "", fmt.Errorf("resolve %s@%s: no matching tag, branch, or commit", modulePath, version)
}

// ListTags returns the set of tag names advertised by the upstream
// remote. Used by `pasta bump` to find the highest semver tag
// without cloning. Annotated-tag peels (`^{}`) are stripped so each
// tag appears once.
func (g *GitFetcher) ListTags(modulePath string) ([]string, error) {
	cloneURL := "https://" + modulePath + ".git"
	cmd := exec.Command("git", "ls-remote", "--tags", "--refs", cloneURL)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ls-remote tags %s: %w", modulePath, err)
	}
	seen := map[string]bool{}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ref := strings.TrimSuffix(fields[1], "^{}")
		name := strings.TrimPrefix(ref, "refs/tags/")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		tags = append(tags, name)
	}
	return tags, nil
}

func looksLikeFullSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func (g *GitFetcher) cachePath(modulePath, commit string) string {
	// Encode the @ separator so paths are filesystem-safe on Windows.
	return filepath.Join(g.CacheDir, "modules", filepath.FromSlash(modulePath)+"@"+commit)
}

// DefaultCacheDir returns the cache root used when none is overridden.
//
// Honors the PASTA_CACHE_DIR env var (intended for tests / CI
// hermeticity). Otherwise defers to os.UserCacheDir, which uses
// XDG_CACHE_HOME on Linux, ~/Library/Caches on macOS, %LocalAppData%
// on Windows.
func DefaultCacheDir() (string, error) {
	if env := os.Getenv("PASTA_CACHE_DIR"); env != "" {
		return env, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache dir: %w", err)
	}
	return filepath.Join(base, "pasta"), nil
}

// HashTree computes a sha256 over the module's file tree: sorted
// relative paths followed by per-file sha256 hex digests, fed back
// through sha256. Stable across machines (paths are forward-slash
// normalized) and detects any change to file content or layout.
//
// Files are streamed through sha256 rather than read into memory so
// embedded fixtures don't blow up the heap.
func HashTree(dir string) (string, error) {
	type entry struct{ path, hash string }
	var entries []entry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		sum, err := hashFile(p)
		if err != nil {
			return err
		}
		entries = append(entries, entry{path: filepath.ToSlash(rel), hash: sum})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s %s\n", e.hash, e.path)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// hashFile streams f through sha256 and returns the hex digest.
func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CheckFlat scans dir for nested pasta.cue files declaring further
// imports. v1 disallows transitive remote imports — if a remote
// module wants to depend on another, the importer must declare both
// directly. Returns nil if dir contains zero nested manifests with
// non-empty imports.
func CheckFlat(dir string) error {
	var bad []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != ManifestFile {
			return nil
		}
		// LoadManifest takes the parent dir.
		m, ok, lerr := LoadManifest(filepath.Dir(p))
		if lerr != nil {
			return lerr
		}
		if ok && len(m.Modules) > 0 {
			rel, _ := filepath.Rel(dir, p)
			bad = append(bad, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("transitive remote imports are not supported in v1 (found in: %s)", strings.Join(bad, ", "))
	}
	return nil
}

// Sync ensures the cache and lockfile are up to date with the
// manifest:
//   - every manifest entry has a lockfile entry at its current version
//   - every locked commit is on disk under cacheDir
//   - the lockfile is written back to dir
//
// Returns the (possibly updated) lockfile.
//
// f is the fetcher used to resolve and download modules. Pass a
// GitFetcher with cacheDir set; tests pass a fake.
func Sync(dir string, m *Manifest, f Fetcher) (*Lockfile, error) {
	existing, _, err := LoadLockfile(dir)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &Lockfile{Modules: map[string]LockedModule{}}
	}
	out := &Lockfile{Modules: map[string]LockedModule{}}
	for path, ver := range m.Modules {
		// If the lockfile already records this exact version and the
		// cache has its commit, reuse — but verify the on-disk hash
		// before trusting it. A mismatch means the cache was
		// tampered with or corrupted; we refuse to silently
		// overwrite it (auto-recovery could mask an attack) and
		// tell the user to clear the cache entry.
		if prev, ok := existing.Modules[path]; ok && prev.Version == ver && prev.Commit != "" {
			if localDir, ferr := f.FetchCommit(path, prev.Commit); ferr == nil {
				if err := CheckFlat(localDir); err != nil {
					return nil, fmt.Errorf("module %s: %w", path, err)
				}
				if prev.Hash != "" {
					gotHash, err := HashTree(localDir)
					if err != nil {
						return nil, fmt.Errorf("hash %s: %w", path, err)
					}
					if gotHash != prev.Hash {
						return nil, fmt.Errorf("module %s@%s: cached files hash to %s but lockfile records %s; remove %s and re-run `pasta sync`",
							path, prev.Commit, gotHash, prev.Hash, localDir)
					}
				}
				out.Modules[path] = prev
				continue
			}
			// Cache miss — fall through to a fresh fetch.
		}
		localDir, commit, err := f.Fetch(path, ver)
		if err != nil {
			return nil, fmt.Errorf("fetch %s@%s: %w", path, ver, err)
		}
		if err := CheckFlat(localDir); err != nil {
			return nil, fmt.Errorf("module %s: %w", path, err)
		}
		hash, err := HashTree(localDir)
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", path, err)
		}
		out.Modules[path] = LockedModule{Version: ver, Commit: commit, Hash: hash}
	}
	if err := WriteLockfile(dir, out); err != nil {
		return nil, err
	}
	return out, nil
}

// IsInSync reports whether lf records the exact same set of modules
// at the exact same versions as m. Returns false (and a reason) when
// the lockfile is stale, missing entries, or has extras the manifest
// no longer asks for.
//
// This is the gate the loader uses to decide whether to run an
// implicit Sync before vendoring: if everything matches, the cached
// commits are still authoritative and no network is needed.
//
// Note this only compares manifest-vs-lockfile metadata. It does NOT
// verify that the cached files still exist on disk or hash to the
// recorded digest — VendorDirs already does both, so we don't
// duplicate the work here.
func IsInSync(m *Manifest, lf *Lockfile) (bool, string) {
	if lf == nil {
		return false, "no lockfile"
	}
	for path, ver := range m.Modules {
		entry, ok := lf.Modules[path]
		if !ok {
			return false, fmt.Sprintf("manifest declares %q but lockfile has no entry", path)
		}
		if entry.Version != ver {
			return false, fmt.Sprintf("%q: manifest=%s, lockfile=%s", path, ver, entry.Version)
		}
	}
	for path := range lf.Modules {
		if _, ok := m.Modules[path]; !ok {
			return false, fmt.Sprintf("lockfile has %q but manifest no longer declares it", path)
		}
	}
	return true, ""
}

// VendorDirs returns, for each module in lf, the on-disk path to its
// cached contents. Used by the loader to vendor remote modules into
// the CUE overlay.
//
// Errors if any module in m is missing a lock entry (manifest drift),
// if its locked commit isn't on disk (run `pasta sync`), or if the
// cached files no longer hash to the locked digest (cache tampering
// / corruption — also a `pasta sync` away from healthy).
func VendorDirs(m *Manifest, lf *Lockfile, f Fetcher) (map[string]string, error) {
	out := map[string]string{}
	for path, ver := range m.Modules {
		entry, ok := lf.Modules[path]
		if !ok {
			return nil, fmt.Errorf("module %q is in pasta.cue but not in pasta.lock; run `pasta sync`", path)
		}
		if entry.Version != ver {
			return nil, fmt.Errorf("module %q: pasta.cue requests %s but pasta.lock has %s; run `pasta sync`", path, ver, entry.Version)
		}
		if entry.Commit == "" {
			return nil, fmt.Errorf("module %q: lockfile entry has no commit", path)
		}
		dir, err := f.FetchCommit(path, entry.Commit)
		if err != nil {
			return nil, fmt.Errorf("module %q@%s not in cache; run `pasta sync`: %w", path, entry.Commit, err)
		}
		// Skip verification when no hash is recorded — older
		// lockfiles (or hand-written ones for tests) may omit it. A
		// recorded hash that doesn't match is always an error.
		if entry.Hash != "" {
			gotHash, err := HashTree(dir)
			if err != nil {
				return nil, fmt.Errorf("hash %s: %w", path, err)
			}
			if gotHash != entry.Hash {
				return nil, fmt.Errorf("module %q@%s: cached files hash to %s but lockfile records %s; run `pasta sync` to refetch", path, entry.Commit, gotHash, entry.Hash)
			}
		}
		out[path] = dir
	}
	return out, nil
}
