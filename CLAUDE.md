# Bosho

QR table ordering app (Go + htmx). See `README.md` for setup and layout.

## Presentation stays in step with the code

`docs/presentation/FEATURES.md` (feature list), `docs/presentation/DECK.md` (slide outline) and `docs/feature-guide.md` (features by role and workflows) must always describe the code as it is.
`DECK.md`, `qr-table-ordering-plan.md` and `global-qr-ordering-research.md` are kept on this laptop only (in `.gitignore`, like `marketing/` and `outreach/`): keep them up to date, but never commit them. GitHub holds the code.
After any change that adds, removes or changes what a guest, staff member or owner can do, or changes the stack or deploy, run the `presentation-sync` skill before finishing the task.
