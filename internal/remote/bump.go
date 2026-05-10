package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"golang.org/x/mod/semver"
)

// BumpResult describes what happened to one module during a Bump
// call. Exactly one of NewVersion (with the bump applied) or Skipped
// (with a human-readable reason like "no semver tags" or "already
// up to date") is set.
type BumpResult struct {
	Module     string
	OldVersion string
	NewVersion string
	Skipped    string
}

// Bump is the library-level implementation of `pasta bump`. For
// every module in dir's manifest (optionally narrowed by filter), it
// asks the fetcher for the upstream tag list, picks the highest
// semver tag, rewrites the manifest, then runs Sync to refresh the
// lockfile.
//
// Returns one BumpResult per module considered, in stable
// (sorted-by-path) order, plus any error from the rewrite or sync
// step. The CLI prints these as "bump X v1 -> v2" / "skip X (...)"
// lines; tests inspect them directly.
//
// filter may be nil or empty to mean "all modules". Filter keys that
// don't match any module in the manifest are reported as a skipped
// result with reason "not in manifest" rather than an error, so a
// shell-level typo doesn't leave the lockfile stale.
func Bump(dir string, filter map[string]bool, f Fetcher) ([]BumpResult, error) {
	m, ok, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	if !ok || len(m.Modules) == 0 {
		return nil, nil
	}
	// Build the path list in stable order, applying the filter.
	var paths []string
	if len(filter) == 0 {
		for p := range m.Modules {
			paths = append(paths, p)
		}
	} else {
		for p := range filter {
			if _, ok := m.Modules[p]; !ok {
				// Surface the typo as a result rather than failing
				// the whole bump — a partial bump is better than
				// "did nothing because of one bad arg".
				paths = append(paths, p)
				continue
			}
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	results := make([]BumpResult, 0, len(paths))
	updates := map[string]string{}
	for _, p := range paths {
		cur, inManifest := m.Modules[p]
		if !inManifest {
			results = append(results, BumpResult{Module: p, Skipped: "not in manifest"})
			continue
		}
		tags, err := f.ListTags(p)
		if err != nil {
			return nil, fmt.Errorf("list tags %s: %w", p, err)
		}
		latest, ok := LatestSemverTag(tags)
		if !ok {
			results = append(results, BumpResult{Module: p, OldVersion: cur, Skipped: "no semver tags"})
			continue
		}
		if latest == cur {
			results = append(results, BumpResult{Module: p, OldVersion: cur, NewVersion: latest, Skipped: "already up to date"})
			continue
		}
		results = append(results, BumpResult{Module: p, OldVersion: cur, NewVersion: latest})
		updates[p] = latest
	}
	if len(updates) > 0 {
		if err := RewriteManifestVersions(dir, updates); err != nil {
			return results, fmt.Errorf("rewrite manifest: %w", err)
		}
		// Re-load + sync so the lockfile pins the new commits.
		m2, _, err := LoadManifest(dir)
		if err != nil {
			return results, fmt.Errorf("reload manifest: %w", err)
		}
		if _, err := Sync(dir, m2, f); err != nil {
			return results, fmt.Errorf("sync after bump: %w", err)
		}
	}
	return results, nil
}

// LatestSemverTag picks the highest semver-looking tag from tags
// using golang.org/x/mod/semver ordering. Tags missing a leading
// "v" are accepted too (canonicalized for comparison only — the
// returned name is verbatim from the input). Returns ("", false)
// if no tag is recognizable as semver.
//
// Used by `pasta bump` to choose the new ref for each module.
// Prereleases ("v1.0.0-rc1") are deliberately filtered out: bumping
// to a prerelease silently moves users off stable, which is rarely
// what they want.
func LatestSemverTag(tags []string) (string, bool) {
	type entry struct {
		tag       string // original tag as given
		canonical string // "vX.Y.Z" form for semver.Compare
	}
	var candidates []entry
	for _, t := range tags {
		c := t
		if c == "" {
			continue
		}
		if c[0] != 'v' {
			c = "v" + c
		}
		if !semver.IsValid(c) {
			continue
		}
		// Skip prereleases: a v2.0.0-rc1 sitting next to v1.5.0
		// shouldn't bump a stable user onto the rc.
		if semver.Prerelease(c) != "" {
			continue
		}
		candidates = append(candidates, entry{tag: t, canonical: c})
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return semver.Compare(candidates[i].canonical, candidates[j].canonical) < 0
	})
	return candidates[len(candidates)-1].tag, true
}

// manifestVersionPattern builds a regex that matches a single
// imports entry's value: "<path>": "<some version>". Capture group 1
// is the prefix (everything up to the opening quote of the
// version), capture group 2 is the closing quote. The version
// itself is replaced wholesale.
//
// The regex is permissive about whitespace between the path, colon,
// and value so we round-trip whatever formatting the user wrote
// (single-line `imports: { "x": "v1" }` and multi-line both work).
func manifestVersionPattern(path string) *regexp.Regexp {
	return regexp.MustCompile(`("` + regexp.QuoteMeta(path) + `"\s*:\s*")[^"]*(")`)
}

// RewriteManifestVersions reads dir/pasta.cue, replaces the version
// string for each (path → newVersion) entry in updates, and writes
// the result back. Preserves all surrounding formatting (comments,
// alignment, ordering) by doing surgical regex replacement on the
// raw file bytes rather than a CUE round-trip.
//
// Returns an error if any path in updates isn't found in the file
// — `pasta bump` reads the manifest before calling, so a miss here
// is a real bug (concurrent edit, malformed file, etc.) and not
// something to silently skip.
func RewriteManifestVersions(dir string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	path := filepath.Join(dir, ManifestFile)
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out := src
	for modPath, newVer := range updates {
		pat := manifestVersionPattern(modPath)
		replaced := pat.ReplaceAll(out, []byte("${1}"+newVer+"${2}"))
		// Detect "no change" by comparing lengths AND contents. A
		// successful replace with the same version is a no-op too,
		// which is fine; what we're catching here is "the path
		// wasn't in the file at all", which would silently leave
		// the manifest stale.
		if string(replaced) == string(out) {
			// Confirm the path is genuinely absent (vs. already at
			// newVer) by re-running the pattern as a finder.
			if !pat.Match(src) {
				return fmt.Errorf("module %q not found in %s; manifest may be malformed", modPath, path)
			}
		}
		out = replaced
	}
	if string(out) == string(src) {
		// Nothing changed — caller can decide whether to print
		// "already up to date" or just stay quiet.
		return nil
	}
	// Write atomically: temp file + rename. If pasta is interrupted
	// mid-write the original manifest survives.
	tmp, err := os.CreateTemp(dir, ".pasta-cue-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
