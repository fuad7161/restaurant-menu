# Bosho

QR table ordering app (Go + htmx). See `README.md` for setup and layout.

## Presentation stays in step with the code

`docs/presentation/FEATURES.md` (feature list), `business/pitch/DECK.md` (slide outline) and `docs/feature-guide.md` (features by role and workflows) must always describe the code as it is.
Everything that isn't code (posters, videos, the Facebook page kit, outreach, the pitch deck `DECK.md`, `qr-table-ordering-plan.md` and `global-qr-ordering-research.md`, and the tools that make them) lives in `business/`, which is in `.gitignore` and kept on this laptop only: keep it up to date, but never commit it. New non-code material goes there too. GitHub holds the code.
After any change that adds, removes or changes what a guest, staff member or owner can do, or changes the stack or deploy, run the `presentation-sync` skill before finishing the task.
