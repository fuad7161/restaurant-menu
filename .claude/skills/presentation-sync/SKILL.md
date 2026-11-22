---
name: presentation-sync
description: Keep Bosho's presentation context (docs/presentation/FEATURES.md and DECK.md) and the README feature sections in step with the code. Use after any change that adds, removes or changes what a guest, staff member or owner can do, or changes the stack, deploy or rules (routes, handlers in internal/server, templates in web/templates, migrations, config, i18n texts, Makefile, Dockerfile, fly.toml). Also use when the user asks to update, check or refresh the feature list, the deck, the presentation or the pitch, or asks "is the presentation up to date".
---

# Presentation sync

Bosho's presentation material lives in two files that must always describe the code as it is:

- `docs/presentation/FEATURES.md`: the feature list. Source of truth. Every feature has a stable ID (`G-` guest, `K-` kitchen and floor staff, `O-` owner, `A-` accounts, `P-` platform) and a "Where in code" column.
- `docs/presentation/DECK.md`: the slide-by-slide outline. Slides cite feature IDs in brackets, and the "Feature → slide map" at the bottom says which slides each ID appears on.

Both files carry a marker on line 3: `<!-- presentation-sync: last synced at commit <sha> on <date> -->`.

The README's "Screens", "Rules built in" and "Not built yet" sections say the same things in shorter form and must not contradict `FEATURES.md`.

## Steps

1. **Find what changed since the last sync.**
   Read the commit from the marker in `FEATURES.md`, then run:
   ```sh
   git log --oneline <sha>..HEAD
   git diff --stat <sha>..HEAD
   git status --short
   ```
   Include uncommitted changes (`git diff`, `git diff --cached`, untracked files) when the sync runs right after an edit in the same session. If the marker's commit is missing from history, compare against the whole tree instead.

2. **Keep only changes a user or a presenter would notice.** Relevant:
   - routes (`mux.HandleFunc` lines in `internal/server/*.go`) added, removed or renamed;
   - handler behaviour: what a screen shows, what an action allows, rules and limits;
   - templates in `web/templates/` that change what a screen shows or how it is laid out;
   - migrations in `internal/database/migrations/` (new columns usually mean a new setting or feature);
   - `internal/i18n` (new guest texts often mean a new guest feature);
   - stack and deploy: `go.mod`, `internal/config`, `Dockerfile`, `docker-compose.yml`, `fly.toml`, `Makefile`;
   - the README's "Not built yet" list.
   Ignore pure refactors, tests, comments, formatting and CSS tweaks that do not change what a screen offers. A visible redesign (for example "show bills as cards", "sidebar instead of top menu") is relevant: update the feature's wording and flag the slide's screenshot.

3. **Read the code, not the commit message.** For each relevant change, open the changed handler or template and confirm what it actually does now. Never describe a feature from a commit title or from `qr-table-ordering-plan.md` alone; the plan describes intentions and parts of it predate the Go build.

4. **Update `FEATURES.md`.**
   - Changed feature: rewrite its line in plain words and fix the "Where in code" column. Keep the ID.
   - New feature: add a row with the next free number in its group (do not renumber others).
   - Removed feature: delete the row and add `- <ID>: <old text> (removed <date>, <short sha>)` under "Removed". Never reuse a removed ID.
   - Built something from "Not built yet": take it off that list and add it as a feature.
   - Update the marker to the current `git rev-parse --short HEAD` and today's date.

5. **Update `DECK.md`.**
   - Use the "Feature → slide map" to find every slide that cites a changed, new or removed ID, and rewrite those bullets.
   - A new feature goes on the slide whose topic fits it; add its ID to the bullet and to the map. If no slide fits and the feature is a selling point, add a slide in the right place and renumber the following slides and the map.
   - A removed feature: take it off every slide and out of the map.
   - Slide 13 ("What comes next") mirrors "Not built yet".
   - When a screen's look changed, add `(screenshot needs retaking)` after the slide's "Picture:" line.
   - Keep the deck at about 15 slides and each slide at 4 bullets or fewer. Update the marker.

6. **Update the README** "Screens", "Rules built in" and "Not built yet" sections if they now disagree with `FEATURES.md`. Do not rewrite other README sections unless the change made them wrong.

7. **Check.**
   - Every ID cited in `DECK.md` exists in `FEATURES.md`, and every ID in the map exists:
     ```sh
     comm -23 <(grep -o '[GKOAP]-[0-9][0-9]' docs/presentation/DECK.md | sort -u) \
              <(grep -o '^| [GKOAP]-[0-9][0-9]' docs/presentation/FEATURES.md | cut -c3- | sort -u)
     ```
     This must print nothing (the map's ranges such as "G-01 to G-07" are fine as long as both ends exist).
   - Every route in `grep -rhn 'HandleFunc("' internal/server/*.go` belongs to some feature, unless it is plumbing (`/healthz`, `/static/`, `/uploads/`, polling fragments, `/logout`).
   - Every path in "Where in code" still exists.

8. **Report** to the user in a few lines: which IDs changed, were added or removed, which slides were edited, and which screenshots need retaking. If nothing user-visible changed, say so and only move the markers forward.

## Writing rules

- Plain words for a restaurant owner. Name the screen and what the person can do, not the function. Code names belong only in the "Where in code" column and on slide 11.
- State only what the code does today. Things that are recorded but not processed (for example bKash or Nagad marked as the payment method) must say so.
- Business, pricing and market facts come only from `qr-table-ordering-plan.md` and `global-qr-ordering-research.md`, and the slide must name which one.
- Do not commit unless the user asks; when they do, a separate commit such as "Update presentation for <change>" is clearest.
