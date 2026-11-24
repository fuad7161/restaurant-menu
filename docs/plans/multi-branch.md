# Plan: multiple branches

Status: **step 1 built** (businesses under the hood, 24 November 2026); steps 2 to 6 planned. Written 24 November 2026 against commit `504fe35`.

This plan builds step 3 ("Branches") of `owner-verification-admin-roles.md` on its own and earlier than that plan's order. It follows the decisions already made there: **Business → Branches**, each branch is today's `restaurants` row, the menu is **copied when a branch is created and separate after that** (8.2), and a branch can override the business's logo and colour (8.3).

What changes from that plan: branches no longer wait for step 2 (accounts and memberships) and step 4 (one person at several restaurants). Those are large rewrites of login and staff. Without them, branches can go live in about a third of the work:

- **The owner** works across every branch of their business and switches between them.
- **Every other person** works at exactly one branch, as today. (One person at several branches stays in step 4.)
- **A new Manager role** runs one branch. Without it, an owner with three branches has to do every menu change and staff password themselves.

Steps 2 and 4 can still be built later on top of this without undoing it.

---

## 1. How it works today, and what breaks with branches

From the code:

| Today | Problem with branches |
| --- | --- |
| `restaurants` holds one restaurant. Tables, menu, orders, payments, reviews, audit log and settings all hang off `restaurant_id`. | None. This is why a branch can simply be a `restaurants` row: almost every query stays as it is. |
| `staff_users.restaurant_id` ties each person, including the owner, to one restaurant. Login (`currentStaff` in `auth.go`) loads that restaurant into `s.R`, and every handler uses `s.R.ID`. | The owner must be able to choose a branch. `s.R` has to come from the **session's chosen branch**, not from the staff row. |
| "Owner has confirmed their email" is worked out per restaurant (`restExtraCols` in `models.go`: an owner `WHERE o.restaurant_id=r.id`). | A second branch has no owner row of its own, so its guests would see "Not taking orders yet" while email verification is on. It must look up the owner by business. |
| One owner per restaurant (`perm.go`, `owner_staff.go`). | Becomes one owner per business. |
| Signup creates one restaurant and its owner. | Creates a business, its first branch and the owner. |
| Invoice numbers count per restaurant (`restaurants.invoice_seq`). | Fine: each branch counts on its own. A branch code on the printed bill keeps them apart ("GUL-0042"). |
| Photos in storage are never deleted; dishes only point at a file name. | Copying a menu can reuse the same photo files with no copying in storage. |
| The Redis change counter (`internal/cache`) is one counter for the whole app. | Fine as it is. |

---

## 2. What belongs to the business, and what to each branch

| Business (shared by all branches) | Each branch |
| --- | --- |
| Business name ("Kacchi Ghar") | Branch name ("Gulshan"), short code for invoices ("GUL") |
| Logo, brand colour, tagline (a branch can override them) | Address, phone, Google review link (each branch has its own Google listing) |
| The owner and their email | Tables and QR codes |
| Number of branches allowed (set by you, the platform admin) | Menu: dishes, prices, photos, options, sold out |
| | VAT %, service charge %, BIN (a branch may have its own VAT registration) |
| | Opening hours, open or closed, accept rule, ticket colours, ordering network (Wi-Fi) |
| | Staff (kitchen, waiter, cashier, manager) |
| | Orders, bills, invoice numbers, payments, discounts, reviews, closing report, activity log |

Guests see **"Kacchi Ghar · Gulshan"**: the business's branding with the branch's name, address and hours.

---

## 3. What each person can do

### Owner (whole business)
- **Branch switcher** at the top of the sidebar: "All branches", "Dhanmondi", "Gulshan" …. The choice is remembered with the login, and every page shows which branch it is working on.
- With **one branch chosen**, every page works exactly as today for that branch: kitchen board, counter, menu, tables, staff, settings, reports, closing report.
- With **"All branches"** chosen:
  - **Dashboard:** today's sales, bills and rating for each branch side by side, and in total.
  - **Reports:** sales by branch, best dishes across branches, CSV with a branch column.
  - **Closing report:** one section per branch, and a grand total.
  - **Activity log:** every branch, with a branch column.
  - Pages that only make sense for one branch (kitchen board, counter, menu, tables, settings) ask to pick a branch first.
- **Branches page** (`/owner/branches`):
  - **Add a branch:** name, code, address, phone. Optionally copy from an existing branch: the menu (categories, dishes, photos, options, tags), settings (VAT, service charge, hours, ticket colours, accept rule) and profile overrides. Tables are not copied (they get new QR codes) and neither are staff.
  - **Edit** a branch's name, code, address and phone.
  - **Close temporarily:** guests see "Temporarily closed", staff can still log in to see their reports.
  - **Archive:** hidden everywhere except reports and CSV; its staff can't log in; its QR codes stop working. Data is kept. An archived branch can be brought back.
- **Business profile:** name, logo, colour, tagline for all branches. A branch's own Profile page can override the logo and colour (8.3).
- **Staff:** the Staff page lists staff of the chosen branch (or all, with a branch column). Adding a person asks for the branch. **Move to another branch** changes their branch and logs them out.

### Manager (new role, one branch)
Everything the owner can do **in their own branch**, except:
- No branch switcher, no "All branches" view, no Branches page, no business profile.
- Can't change VAT or service charge (decision 8.12 in the earlier plan).
- Can add, remove and set passwords only for kitchen, waiter and cashier, not for managers or the owner.
- Starts on the Dashboard of their branch.

### Kitchen, Waiter, Cashier
No change, except that every screen shows the branch name. They only ever see their own branch.

### Guest
No change in how ordering works. The QR code belongs to one table in one branch, so the guest is in the right branch from the first scan.

---

## 4. Database changes

One migration, `0008_branches.sql`:

```sql
-- A business groups its branches. A branch is a row in `restaurants`, as before.
CREATE TABLE businesses (
  id           BIGSERIAL PRIMARY KEY,
  name         TEXT NOT NULL,
  logo         TEXT NOT NULL DEFAULT '',
  brand_color  TEXT NOT NULL DEFAULT '#8f1d21',
  tagline      TEXT NOT NULL DEFAULT '',
  max_branches INTEGER NOT NULL DEFAULT 3,   -- set by the platform admin (by SQL until the admin panel exists)
  created_at   BIGINT NOT NULL
);

ALTER TABLE restaurants ADD COLUMN business_id BIGINT REFERENCES businesses(id);
ALTER TABLE restaurants ADD COLUMN branch_name TEXT NOT NULL DEFAULT '';   -- "Gulshan"; empty for a one-branch business
ALTER TABLE restaurants ADD COLUMN branch_code TEXT NOT NULL DEFAULT '';   -- "GUL", printed before invoice numbers
ALTER TABLE restaurants ADD COLUMN status      TEXT NOT NULL DEFAULT 'active'; -- active | closed | archived
ALTER TABLE restaurants ADD COLUMN sort_order  INTEGER NOT NULL DEFAULT 0;

-- Every existing restaurant becomes a business with one branch (same name, logo, colour, tagline).
-- (Done in the migration with INSERT ... SELECT and an UPDATE joining back on a temporary column.)
-- Then: ALTER TABLE restaurants ALTER COLUMN business_id SET NOT NULL;

-- The session remembers the branch the person is working on. 0 = "All branches" (owner only).
ALTER TABLE staff_sessions ADD COLUMN branch_id BIGINT NOT NULL DEFAULT 0;

-- Activity log entries made in "All branches" mode (for example adding a branch) have no single branch.
-- They keep restaurant_id = the owner's home branch and get business_id for the combined log.
ALTER TABLE audit_log ADD COLUMN business_id BIGINT;

CREATE INDEX idx_restaurants_business ON restaurants(business_id);
```

What stays the same: every other table keeps `restaurant_id`, which now means "branch". `staff_users.restaurant_id` stays: for staff and managers it is their branch; for the owner it is their first branch (a home branch for old code paths), and their reach is the whole business.

**Profile overrides (8.3):** the branch's existing `logo`, `brand_color` and `tagline` columns stay. Empty means "use the business's". When loading a branch, the code fills empty ones from the business.

---

## 5. Code changes

### 5.1 Who is working where (`auth.go`, `models.go`)
- `Restaurant` gets `BusinessID`, `BusinessName`, `BranchName`, `BranchCode`, `Status`. A `DisplayName()` gives "Kacchi Ghar · Gulshan", or just the business name for a one-branch business.
- `Staff` gets `BusinessID`, `AllBranches bool` (owner in "All branches" mode) and `Branches []Restaurant` (owner only, for the switcher).
- `currentStaff`:
  - Staff and managers: load the branch from `staff_users.restaurant_id`, as today. Refuse the login if that branch is archived.
  - Owner: load the branch from `staff_sessions.branch_id`, **checked to belong to the owner's business**. If it is 0, the owner is in "All branches" mode and `s.R` is their home branch, used only by pages that need some restaurant for the layout.
- **Switch branch:** `POST /owner/branch` with a branch id (or `all`). It checks the branch belongs to the business and is not archived, saves it on the session and returns to the same page.
- **Pages that need one branch:** a wrapper `a.oneBranch(h)`. In "All branches" mode it shows "Pick a branch" with buttons instead of the page. It goes on the kitchen board, counter, bill pages, take order, sold out, menu, tables, settings, networks, branch profile, staff and closing report for one branch.

### 5.2 Permissions (`perm.go`)
- New role `manager` with every owner permission except: `branch.manage` (new: Branches page and switcher), `business.profile` (new), `settings.charges` (new: VAT and service charge, split out of `settings.edit`), `reports.all_branches` (new) and `staff.manage_managers` (new).
- `staffRoles` (what can be given) becomes: owner may give `manager`, `kitchen`, `waiter`, `cashier`; manager may give `kitchen`, `waiter`, `cashier`.
- `staffHome("manager")` is the Dashboard.
- `perm_test.go`: add the manager column for every route, and the new routes.

### 5.3 Branches page (`owner_branches.go`, new)
- `GET /owner/branches`: list with status, today's sales, tables count, staff count.
- `POST /owner/branches`: create. Checks `max_branches`. If a source branch is chosen, copies in one transaction:
  - `menu_categories` → `menu_items` → `option_groups` → `item_options`, mapping old ids to new ones (categories first, then dishes, then groups, then options with their new group id). Photo file names are reused as they are.
  - Settings columns: VAT, service, hours, last order, ticket minutes, accept rule, orders-in-hours. Not the ordering network (each branch has its own Wi-Fi).
  - Logo, colour, tagline overrides.
- `POST /owner/branches/{id}`: edit name, code, address, phone.
- `POST /owner/branches/{id}/status`: active, closed, archived. Archiving a branch with open bills is refused until they are closed.
- Each action goes into the activity log.

### 5.4 Things that must be fixed for correctness
- **Owner email check** (`restExtraCols`): find the owner by `business_id`, not `restaurant_id`.
- **Guest entry** (`guest.go`, the QR lookup): refuse an archived branch (like a switched-off table) and show "Temporarily closed" for a closed one.
- **One owner per business:** `owner_staff.go` and signup check the business, not the restaurant.
- **Staff pages:** listing, adding, deleting and passwords stay limited to the chosen branch. A manager can't touch managers or the owner. Moving a person to another branch is owner only and checks the target branch is in the same business.
- **Printed bill and receipt:** show "Kacchi Ghar · Gulshan", the branch's address and BIN, and the invoice as `GUL-0042` when the branch has a code.

### 5.5 Combined views (`reports.go`, `owner_dashboard.go`, `audit.go`)
- Every report query today is `WHERE restaurant_id=?`. Give each one a branch filter that is either `restaurant_id=?` or `restaurant_id IN (SELECT id FROM restaurants WHERE business_id=?)`, chosen from `s.AllBranches`. Add `GROUP BY restaurant_id` where the page shows one line per branch.
- CSV exports get a `branch` column in all modes, so files from different months line up.

### 5.6 Signup and demo
- Signup asks for **business name** and **first branch name** (optional; empty means a one-branch business, and the page shows only the business name). It creates the business, the branch and the owner.
- Demo mode creates "Demo Kitchen" with two branches ("Dhanmondi", "Gulshan"), so the switcher and combined reports can be shown in a sales demo.

### 5.7 Templates
- Sidebar: the branch switcher for owners; the branch name under the restaurant name for everyone else.
- New: `owner_branches` page, "Pick a branch" page, business profile page.
- Dashboard, reports, closing report and activity log: an "All branches" layout with one row or section per branch.
- Guest pages: `DisplayName` instead of the restaurant name in the header, receipt and review.

---

## 6. Build order

Each step can go live on its own. Run `presentation-sync` after each step that users can see.

1. **Businesses under the hood.** Migration (every restaurant becomes a business with one branch), `Restaurant.BusinessID`, owner email check by business, signup creating a business. **Nothing changes on screen.** Tests: every existing test still passes.
2. **Branches for the owner.** Branches page with "copy from", branch switcher, branch on the session, `oneBranch` wrapper, status (closed, archived) on guest and staff screens, branch name and code on bills and receipts. Tests: below.
3. **Manager role.** Role, permissions, staff page rules, start page. Tests: the manager column in `perm_test.go`.
4. **"All branches" views.** Combined dashboard, reports, closing report, activity log, CSV branch column.
5. **Business profile and branch overrides.** Business logo and colour, branch override on the branch Profile page.
6. **Demo with two branches**, and the docs.

Rough size: steps 1 and 2 are the bulk (about 2 to 3 days of work); 3 to 6 are about half a day each.

---

## 7. Tests to add

In `flow_test.go`, against PostgreSQL:
- **Isolation:** an owner of business A can't switch to, or open any table, bill, dish or staff member of, business B (answer: "Not found"). A waiter in branch 1 can't open a bill of branch 2 by changing the id in the address.
- **Switching:** after switching to branch 2, the counter, kitchen board and menu show only branch 2.
- **Copy:** a copied branch has the same categories, dishes, prices, option groups with their rules, and options in the right groups, all with new ids. Changing a price in the copy leaves the original alone.
- **Invoices:** branch 1 and branch 2 each start at 1.
- **Archive:** a QR code of an archived branch doesn't open the menu; its staff can't log in; archiving with an open bill is refused.
- **Combined report:** the all-branches total equals the sum of the branches.
- **Limit:** creating a branch past `max_branches` is refused with a message.
- **Migration:** an existing restaurant with orders becomes one business with one branch, and its reports are unchanged.

---

## 8. Not in this plan (later)

- **One person at several branches or businesses** (step 4 of the earlier plan: accounts, memberships, invitations, a password per workplace). Until then, a person who works at two branches needs two phone numbers, or the Manager role at one branch.
- **One shared menu** with per-branch prices and sold-out. Copy-then-separate covers it for now; a "push this dish to all branches" button is a cheap middle step if owners ask.
- **Admin panel** for `max_branches` and other limits. Until then, set it with SQL.
- Moving stock or orders between branches. Central kitchen.
- A public page listing all branches of a business (decision 8.16).

---

## 9. Decisions needed from you

| # | Question | Options | Suggestion |
| --- | --- | --- | --- |
| B1 | Build the **Manager role** with branches, or later? | With branches (step 3) · Later | **With branches.** An owner with several branches can't do every task alone. |
| B2 | **Copy from another branch** when creating one: what is copied? | Menu only · Menu and settings · Ask each time with tick boxes | **Tick boxes:** menu, settings, logo/colour, each on by default |
| B3 | **Invoice numbers** | Per branch with a code ("GUL-0042") · One series for the whole business | **Per branch with a code.** Each branch prints its own bills, and a branch may have its own BIN. Check with an accountant. |
| B4 | **Default branch limit** before the admin panel exists | 1 · 3 · No limit | **3**, raised by you with SQL for bigger customers |
| B5 | Can **cashiers give discounts**? Built on 24 Nov 2026 for owner and cashier; the permission table in `owner-verification-admin-roles.md` says owner and manager only. | Owner, manager and cashier · Owner and manager only | Your call. Many restaurants let the cashier do it; it is logged and in the closing report either way. |
| B6 | Does the **owner's login land** on "All branches" or on the last branch used? | All branches · Last branch | **Last branch used**, "All branches" for a new login |
| B7 | Can a **manager see the activity log** of their branch? | Yes · No | **Yes**, their branch only |
