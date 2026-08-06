# flowcontrol brand files

Mark: **one node unblocks two** — the cascade the engine performs when a node
turns DONE. Filled node = satisfied, hollow nodes = released.

## Files

| File | Use |
| --- | --- |
| `logo-mark-dark.svg` | mark on dark backgrounds (scales to any size) |
| `logo-mark-light.svg` | mark on light backgrounds |
| `logo-mark-dark-512.png` / `-light-512.png` | favicon / avatar source, transparent |
| `logo-lockup-dark.svg` / `-light.svg` | mark + wordmark + `fctrl` |
| `logo-lockup-dark@4x.png` | lockup raster, transparent, 1216×256 |
| `social-preview-1280x640.png` | **GitHub social preview** — upload this one |
| `social-preview.svg` | editable vector of the same image |
| `logo-monogram-accent.svg` / `-ink.svg` | **alternate mark (1b)** — `fc` tile |
| `logo-monogram-accent-256.png` / `-ink-256.png` | monogram raster, 256×256 |
| `logo-monogram-lockup-dark@3x.png` | monogram lockup raster, 912×192 |

## GitHub social preview

Settings → General → Social preview → Upload an image. GitHub takes PNG/JPG/GIF
(not SVG), recommends 1280×640, and caps the file at 1MB — the PNG here is well
under. Edit `FlowControl Social Preview.dc.html` and re-export if the tagline
changes.

## Colours

```
mark    #5ad1e6   one flat blue: filled node, edges and rings alike
                  (light-bg variant #0f7a8c)
ink     #e6e8ec   wordmark   (light-bg variant #16181b)

READY #3ecf8e   BLOCKED #ef6a5a   DEFERRED #7c8394   DONE #3f6b8f
```

## Type

IBM Plex Sans 600 for the wordmark (`flow` at full strength, `control` at 50%),
IBM Plex Mono 500 for `flowcli`. The lockup SVGs reference both by name — if a
consumer doesn't have Plex installed the text falls back to system UI, so use
`logo-lockup-dark.png` where fidelity matters, or outline the text in your
vector editor before shipping the SVG externally.

## Rules

- Clear space: one node diameter on all sides.
- Minimum mark size 16px; the wordmark comes off below 96px wide — use the mark alone.
- The mark is a single flat blue by design. Don't tint the edges differently from
  the nodes, don't recolour per status, don't add a container to the mark, don't
  set the wordmark in anything but Plex Sans.

## Alternate mark — the monogram (1b)

Exported alongside the primary mark: the `fc` tile the app rail already uses.
Accent tile for dark surfaces, ink tile for light ones. It survives small sizes
as a shape but the letters go illegible under ~20px, so use the fork mark for
favicons and the monogram for app tiles and avatars where it has room.

The monogram PNGs top out at 256×256 — they're captured from the live design, and
the glyph is type, not vector. For larger sizes render `logo-monogram-accent.svg`
with IBM Plex Mono installed, or outline the text in a vector editor first.

Other directions explored (histogram, gate, terminal prompt, progress ring) live
in `FlowControl Logo.dc.html`.
