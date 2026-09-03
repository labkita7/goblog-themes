// Command build assembles a goblog-themes registry repository from a themes
// directory (docs/TASKS.md T-711, docs/THEME-AUTHORING.md §9).
//
// It copies each theme folder, packages a deterministic zip (fixed timestamps,
// sorted entries) and generates index.json with sha256 checksums. The same
// binary runs inside the publishing repo (themes at repo root) via --inplace.
//
// Usage:
//
//	go run ./registry/build --out <dir> [--themes ./themes] [--inplace]
//		[--pages-base https://labkita7.github.io/goblog-themes]
//		[--download-template "https://github.com/labkita7/goblog-themes/releases/download/{slug}-{version}/{slug}.zip"]
//		[--skeleton]  # copy registry/skeleton files + docs/THEME-AUTHORING.md into out
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// zipEpoch pins entry timestamps so rebuilds are byte-identical and the
// published sha256 stays stable.
var zipEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type manifest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Engines     struct {
		Goblog string `json:"goblog"`
	} `json:"engines"`
}

type indexEntry struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Screenshot  string   `json:"screenshot"`
	Download    string   `json:"download"`
	SHA256      string   `json:"sha256"`
	Engines     struct {
		Goblog string `json:"goblog"`
	} `json:"engines"`
}

// themeTags curates the browse tags shown in the admin registry tab.
var themeTags = map[string][]string{
	"brutal":   {"bold", "high-contrast"},
	"claude":   {"warm", "editorial"},
	"cyber":    {"dark", "terminal"},
	"graphite": {"dark", "professional"},
	"neutral":  {"minimal", "clean"},
	"paper":    {"editorial", "serif"},
	"pastel":   {"soft", "friendly"},
	"sage":     {"organic", "calm"},
	"serif":    {"classic", "typography"},
	"slate":    {"corporate", "clean"},
}

// skipNames are never copied into a theme folder or zip.
var skipNames = map[string]bool{
	"node_modules": true,
	".DS_Store":    true,
}

func main() {
	out := flag.String("out", "", "output directory (required unless --inplace)")
	themesDir := flag.String("themes", "./themes", "directory containing theme folders")
	inplace := flag.Bool("inplace", false, "assemble into --themes itself (publishing-repo mode)")
	pagesBase := flag.String("pages-base", "https://labkita7.github.io/goblog-themes", "public base URL for screenshots")
	downloadTpl := flag.String("download-template",
		"https://github.com/labkita7/goblog-themes/releases/download/{slug}-{version}/{slug}.zip",
		"download URL template; {slug} and {version} are substituted")
	skeleton := flag.Bool("skeleton", false, "copy registry/skeleton + docs/THEME-AUTHORING.md into the output")
	flag.Parse()

	root := *themesDir
	if *inplace {
		if *out != "" {
			fatal("--inplace and --out are mutually exclusive")
		}
		root = "."
		*themesDir = "."
		*out = "."
	} else if *out == "" {
		fatal("--out is required")
	}

	// Resolve skeleton source relative to this repo when assembling a
	// separate output directory.
	repoBase := "."
	if !*inplace {
		if _, err := os.Stat(filepath.Join("registry", "skeleton")); err != nil {
			repoBase = "../.."
		}
	}

	slugs, err := discoverThemes(root)
	if err != nil {
		fatal(err)
	}
	if len(slugs) == 0 {
		fatal(fmt.Sprintf("no themes found in %s (a theme is a directory containing theme.json)", root))
	}

	zipsDir := filepath.Join(*out, "zips")
	if err := os.MkdirAll(zipsDir, 0o755); err != nil {
		fatal(err)
	}

	entries := []indexEntry{}
	for _, slug := range slugs {
		src := filepath.Join(root, slug)
		m, err := readManifest(src)
		if err != nil {
			fatal(err)
		}
		if m.Slug != slug {
			fatal(fmt.Sprintf("%s: theme.json slug %q does not match folder %q", src, m.Slug, slug))
		}
		if !*inplace {
			dst := filepath.Join(*out, slug)
			if err := copyDir(dst, src); err != nil {
				fatal(err)
			}
		}
		zipPath := filepath.Join(zipsDir, slug+"-"+m.Version+".zip")
		sum, err := zipTheme(zipPath, src, slug)
		if err != nil {
			fatal(err)
		}
		tags := themeTags[slug]
		if tags == nil {
			tags = []string{"blog"}
		}
		e := indexEntry{
			Slug: slug, Name: m.Name, Version: m.Version, Author: m.Author,
			Description: m.Description, Tags: tags,
			Screenshot: strings.TrimSuffix(*pagesBase, "/") + "/" + slug + "/screenshot.png",
			Download:   strings.ReplaceAll(strings.ReplaceAll(*downloadTpl, "{slug}", slug), "{version}", m.Version),
			SHA256:     sum,
		}
		e.Engines.Goblog = m.Engines.Goblog
		if e.Engines.Goblog == "" {
			e.Engines.Goblog = ">=1.0.0 <2.0.0"
		}
		entries = append(entries, e)
		fmt.Printf("  %-12s %s  %s\n", slug, m.Version, sum[:12]+"…")
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	idx := struct {
		Version int          `json:"version"`
		Updated string       `json:"updated"`
		Themes  []indexEntry `json:"themes"`
	}{Version: 1, Updated: time.Now().UTC().Format(time.RFC3339), Themes: entries}
	blob, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		fatal(err)
	}
	blob = append(blob, '\n')
	if err := os.WriteFile(filepath.Join(*out, "index.json"), blob, 0o644); err != nil {
		fatal(err)
	}
	if *skeleton {
		if err := copySkeleton(repoBase, *out); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("registry: %d themes → %s/index.json\n", len(entries), *out)
}

// discoverThemes lists top-level directories that contain theme.json.
func discoverThemes(root string) ([]string, error) {
	des, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, de := range des {
		if !de.IsDir() || skipNames[de.Name()] || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, de.Name(), "theme.json")); err == nil {
			slugs = append(slugs, de.Name())
		}
	}
	sort.Strings(slugs)
	return slugs, nil
}

func readManifest(dir string) (manifest, error) {
	var m manifest
	raw, err := os.ReadFile(filepath.Join(dir, "theme.json"))
	if err != nil {
		return m, fmt.Errorf("read %s/theme.json: %w", dir, err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("parse %s/theme.json: %w", dir, err)
	}
	if m.Slug == "" || m.Name == "" || m.Version == "" {
		return m, fmt.Errorf("%s/theme.json needs slug, name, and version", dir)
	}
	return m, nil
}

// zipTheme writes <slug>/… entries with fixed timestamps in sorted order so
// the same sources always produce identical bytes (and a stable sha256).
func zipTheme(dst, src, slug string) (string, error) {
	var files []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if skipNames[name] {
				return fs.SkipDir
			}
			return nil
		}
		if skipNames[name] {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range files {
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		hdr := &zip.FileHeader{Name: slug + "/" + filepath.ToSlash(rel), Method: zip.Deflate, Modified: zipEpoch}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return "", err
		}
		if _, err := w.Write(data); err != nil {
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

func copyDir(dst, src string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "." {
				return os.MkdirAll(dst, 0o755)
			}
			if skipNames[d.Name()] {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if skipNames[d.Name()] {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}

// copySkeleton brings the publishing-repo scaffolding (README, workflow,
// go.mod, this builder, authoring docs) into the assembled output.
func copySkeleton(repoBase, out string) error {
	copies := map[string]string{
		filepath.Join(repoBase, "registry", "skeleton", "README.md"):   "README.md",
		filepath.Join(repoBase, "registry", "skeleton", "gitignore"):   ".gitignore",
		filepath.Join(repoBase, "registry", "skeleton", "nojekyll"):    ".nojekyll",
		filepath.Join(repoBase, "registry", "skeleton", "go.mod"):      "go.mod",
		filepath.Join(repoBase, "registry", "build", "main.go"):        filepath.Join("build", "main.go"),
		filepath.Join(repoBase, "docs", "THEME-AUTHORING.md"):          "THEME-AUTHORING.md",
		filepath.Join(repoBase, "registry", "skeleton", "publish.yml"): filepath.Join(".github", "workflows", "publish.yml"),
	}
	for src, rel := range copies {
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("skeleton %s: %w", src, err)
		}
		dst := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func fatal(v any) {
	fmt.Fprintln(os.Stderr, "registry/build:", v)
	os.Exit(1)
}
