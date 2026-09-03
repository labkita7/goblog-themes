# goblog-themes

Theme registry for [goblog](https://github.com/labkita7/go-blogging-platform) —
a self-hosted blogging platform. This repository is a **static registry**:
`index.json` and screenshots are served by GitHub Pages, theme zips by GitHub
Releases. There is no backend service.

## Install a theme (goblog users)

Admin → **Appearance → Registry** → *Install* → *Activate*. The CMS verifies
the sha256 checksum, checks the `engines` constraint, validates the manifest,
and keeps the previous version for rollback. No manual download needed.

## Publish / update a theme (theme authors)

1. Add or update a folder `<slug>/` containing `theme.json`, `screenshot.png`
   (1200×800), `templates/`, `assets/`, and `src/` — see `THEME-AUTHORING.md`.
2. Push to `main`. The [publish workflow](.github/workflows/publish.yml)
   rebuilds the zip, regenerates `index.json`, and attaches the zip to the
   `<slug>-<version>` GitHub Release automatically.
3. Bump `version` in `theme.json` for every change — it drives the update
   badge in the admin.

To rebuild locally instead: `go run ./build --inplace` (needs Go ≥ 1.25),
then commit `index.json` and upload `zips/<slug>-<version>.zip` to the
matching release.

## Layout

```
<slug>/          one folder per theme (the marketplace source of truth)
build/           the assembler (go run ./build --inplace)
zips/            generated release archives (gitignored)
index.json       generated registry index (committed)
```
