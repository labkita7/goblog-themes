# Theme authoring guide

Everything you need to build, validate, package and publish a goblog theme,
using only this document and `themes/neutral/` as a working reference.

A theme is a directory of Go `html/template` files plus one prebuilt
Tailwind v4 stylesheet. No Go knowledge required — the server compiles your
templates and hands them data; you style it with the shadcn/tweakcn token
vocabulary.

---

## 1. Anatomy

```
mytheme/
  theme.json            # manifest (section 2) — required
  screenshot.png        # 1200x800 preview of the home page — required
  templates/            # required: index.html, post.html,
                        #           partials/{head,header,footer}.html
                        # optional: page.html, tag.html, archive.html,
                        #           author.html, 404.html, search.html,
                        #           partials/* (e.g. sidebar.html, toc.html)
  assets/               # theme.css (prebuilt), fonts/, images/
  src/                  # input.css (Tailwind v4 source) so the theme is rebuildable
```

- The engine loads themes from `data/themes/<slug>` (installed) then
  `./themes/<slug>` (bundled, `--themes` flag), and parses **all files under
  `templates/` into one shared template set**.
- **Page templates are raw files** executed by base filename
  (`index.html`, `post.html`, …). **Partials are self-defining** — each
  partial wraps its content in `{{define "name"}} … {{end}}` and pages call
  `{{template "name" .}}`. This is the ratified convention; the engine
  counts on it.
- Never reference other themes' files, CDNs, or external URLs: public pages
  ship with `Content-Security-Policy: default-src 'self'`. Bundle fonts in
  `assets/fonts/` (OFL-licensed WOFF2, referenced relatively from CSS).

## 2. Manifest (`theme.json`)

```json
{
  "slug": "neutral",
  "name": "Neutral",
  "version": "1.0.0",
  "author": "you",
  "description": "Minimal single-column blog theme",
  "engines": { "goblog": ">=1.0.0 <2.0.0" },
  "screenshot": "screenshot.png",
  "settings": [
    { "key": "accent", "label": "Accent color", "type": "color", "default": "#2563eb" },
    { "key": "font_heading", "label": "Heading font", "type": "select",
      "options": ["Inter", "Georgia"], "default": "Inter" },
    { "key": "show_reading_time", "label": "Show reading time", "type": "bool", "default": true }
  ]
}
```

Rules: `slug` matches the directory name; `engines.goblog` is a Go-style
semver range; **every setting needs a `default`** (the theme must look right
with zero saved settings). Declared settings are rendered as a form in the
admin and stored as JSON per theme.

## 3. Template data + FuncMap

Canonical, exhaustive list: **docs/THEMES.md → "Template data reference"**.
Shape summary — all timestamps are `time.Time`:

| Page | Template | Data |
|---|---|---|
| Home | `index.html` | `IndexData{Site, Nav, Posts, Page, Pages, PrevURL, NextURL}` |
| Post | `post.html` | `PostData{Site, Nav, Post, Prev, Next, Preview, TOC}` |
| Static page | `page.html` | same shape as PostData (no tags) |
| Tag | `tag.html` | `TagData{Site, Nav, Tag, Posts, Page, Pages, PrevURL, NextURL}` |
| Archive | `archive.html` | `ArchiveData{Site, Nav, Year, Posts, Page, Pages, PrevURL, NextURL}` |
| Search | `search.html` | `SearchData{Site, Nav, Query, Posts, Page, Pages, PrevURL, NextURL}` |
| 404 | `404.html` | `ErrorData{Site, Nav, Code, Message}` |

`Site.Tags []Tag` (all tags) and `Site.ArchiveYears []{Year, Count}` are
available everywhere via `.Site`. `Post.HTML` is pre-rendered trusted HTML —
the only value that renders unescaped; everything else auto-escapes. **Do not
reference any field not in the reference list.**

FuncMap (nothing else exists):

- `asset "theme.css"` → `/assets/theme/<slug>/<file>?v=<hash>` (use for CSS
  and any asset file: `asset "fonts/lora-400.woff2"`)
- `formatDate` → `Jan 2, 2006` · `formatDateTime` → `Jan 2, 2006, 15:04`
- `isoDate` → RFC 3339 — use for `<time datetime>`, `og:article:published_time`
  and JSON-LD `datePublished`/`dateModified`
- `ago` → `3 days ago`
- `setting "key"` → saved setting value or manifest default (JSON-typed:
  bools are real bools, numbers are numbers)

### The SEO head

`partials/head.html` defines one entry point per page type, because a single
shared partial cannot branch on optional fields across the different data
structs:

```
{{define "headCore"}}  charset, viewport, dark-mode bootstrap, feed links,
                       {{asset "theme.css"}}, settings style block
{{define "headIndex"}} <head> … WebSite JSON-LD … </head>
{{define "headPost"}}  <head> … BlogPosting JSON-LD, article:* og tags … </head>
{{define "headPage"}} / {{define "headTag"}} / {{define "headArchive"}} / {{define "headError"}}
```

Each page template starts `<!doctype html><html lang="{{.Site.Language}}">`
then includes its variant, e.g. `{{template "headIndex" .}}`. Copy
`themes/_shared/partials/head.html` as your starting point — it already emits
title, description, canonical, `og:*`, `twitter:card`, JSON-LD and feeds on
every page type.

## 4. Settings in templates

Use `{{setting "key"}}`:

```html
<style>
:root{--primary:{{setting "accent"}};}
.dark{--primary:{{setting "accent"}};}
</style>
```

Put this style block **after** the `<link rel="stylesheet">` in `headCore`
so saved settings beat the manifest defaults baked into your tokens. Gate
features with truthiness (`{{if setting "show_reading_time"}}`) and compare
selects with `{{if eq (setting "list_style") "detailed"}}`. For numbers,
append units: `--shadow:{{setting "shadow_size"}}px`.

## 5. Tailwind v4 build

`src/input.css` imports Tailwind and the shared base, then defines your
token values:

```css
@import "tailwindcss";
@import "../../_shared/src/base.css";   /* when building inside themes/ */
@source "../templates";                  /* scan this theme's templates */
@source "../../_shared/src";             /* scan shared sources */

:root  { --background: #ffffff; --foreground: #171717; /* full shadcn set */ }
.dark  { --background: #0a0a0a; /* … */ }
```

`_shared/src/base.css` provides the shadcn/tweakcn token mapping
(`@theme inline`: `--color-*`, radius scale, fonts), base element styles,
`.skip-link`, `.pagination`, `.prose` article styles and Chroma
`github`/`github-dark` syntax classes. Override its rules in your own
`input.css` (copy, don't cross-import at runtime). Distributed zips ship the
prebuilt `assets/theme.css` — `src/` is for rebuilds in the repo.

Build from the repo root:

```sh
sh themes/build.sh mytheme        # or: npm run build:mytheme
```

The script installs devDependencies once (`tailwindcss` + `@tailwindcss/cli`
only) and writes minified `mytheme/assets/theme.css`. It runs the CLI inside
the theme directory so content scanning covers exactly your `templates/` plus
`_shared/src/` — no cross-theme class leakage. Commit the built CSS.

## 6. Dark mode

Strategy: `.dark` class on `<html>` + a ≤ 1 KB inline bootstrap in
`headCore` that runs before first paint:

```js
(function(){var s=null;try{s=localStorage.getItem("goblog-theme")}catch(e){}
var d=s?s==="dark":window.matchMedia("(prefers-color-scheme: dark)").matches;
document.documentElement.classList.toggle("dark",d)})();
```

…plus a `[data-theme-toggle]` button (one delegated click listener in
`partials/header.html`) that toggles the class and persists to
`localStorage`. Tailwind needs the class variant — already in `base.css`:

```css
@custom-variant dark (&:is(.dark *));
```

Define light tokens on `:root` and dark overrides on `.dark`; also set
`color-scheme` per mode (done in base.css) so form controls follow. Sites
that want a fixed default (e.g. dark-first dev themes) read a
`default_mode` setting in the bootstrap — see `themes/graphite`.

## 7. Validation checklist

Before packaging, tick all of these (`goblog theme validate <slug>` covers
the manifest checks):

- [ ] `sh themes/build.sh <slug>` succeeds and `assets/theme.css` is non-trivial
- [ ] every page type renders: index, post (cover, tags, reading time,
      prev/next), page, tag, archive, 404 — render them with the harness:
      `cd themes && go run ./_shared/dev/render <slug> <outdir>`
- [ ] templates use **only** documented data fields and FuncMap functions
- [ ] light + dark modes both readable; toggle works; `prefers-color-scheme`
      respected when no stored preference
- [ ] zero required JS (≤ 1 KB toggle allowed); exactly one `h1` per page;
      landmarks (`header`/`nav`/`main`/`footer`); skip link; `time` elements
- [ ] `screenshot.png` is a real 1200×800 home capture (the harness output +
      any headless browser at 1200×800 does it)
- [ ] ≥ 3 settings, all with defaults; no external requests (fonts bundled,
      no CDN) — CSP `default-src 'self'` compatible

## 8. Packaging a zip

From the directory that contains your theme folder:

```sh
zip -r mytheme.zip mytheme/
```

The zip must contain exactly one top-level directory named after the slug
(no `node_modules`, no build scripts). Keep it under 20 MB — the admin
installer rejects bigger archives and enforces zip-slip protection.

## 9. Publishing to the registry

The registry is a static `index.json` (GitHub Pages) whose entries point at
release zips:

```json
{
  "slug": "mytheme",
  "name": "My Theme",
  "version": "1.0.0",
  "author": "you",
  "description": "…",
  "tags": ["minimal", "professional"],
  "screenshot": "https://labkita7.github.io/goblog-themes/mytheme/screenshot.png",
  "download": "https://github.com/labkita7/goblog-themes/releases/download/mytheme-1.0.0/mytheme.zip",
  "sha256": "…64 hex chars…",
  "engines": { "goblog": ">=1.0.0 <2.0.0" }
}
```

Produce the checksum from the exact zip you attach to the release:

```sh
shasum -a 256 mytheme.zip        # macOS / Linux
```

Publish flow: tag `mytheme-1.0.0` on the `goblog-themes` repo, attach
`mytheme.zip` to that release, add the entry above to `index.json` (bump its
`updated` timestamp), and push. Readers install via Admin → Appearance →
Registry; the admin verifies `sha256`, checks `engines`, and validates the
manifest before activating.
