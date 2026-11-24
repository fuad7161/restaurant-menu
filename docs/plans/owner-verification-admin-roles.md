# Plan: admin control, owner verification, branches, multi-restaurant staff and roles

Status: **step 1 and most of step 5 built.** Step 1 (23 November 2026): role permissions in `internal/server/perm.go`, checked on every route, and one owner per restaurant. Step 5 (24 November 2026): owner email at signup, email confirmation, forgot password by email, and staff passwords the owner can look up (`internal/server/email.go`, `internal/mail`). Steps 2, 3, 4 and 6 are not built yet. First written 23 November 2026, updated the same day with your decisions.

## Your decisions so far

| Topic | Decision |
|---|---|
| Email verification | **The admin (you) controls it:** on or off for the whole platform, and you can override it for each restaurant. |
| Admin power | **You control every kind of owner access:** status, limits, feature switches and what the owner can use. |
| Owner | **The owner can do everything** that the admin allows for their restaurant. |
| Staff at several restaurants | **Allowed.** One person (one phone number) can work at several restaurants and branches. |
| Branches | **The owner can create branches.** |
| Owner forgot password | **Yes, by email.** |
| Domain | **`bosho.food`** for now (cheaper). The product name is **Bosho** everywhere; a `.com` may come later. |
| Owner resets staff passwords | **Yes, with no conditions.** The owner sets every staff password and changes it whenever they want (`POST /owner/staff/{id}/password`). |
| Staff passwords | **The owner sets them and can look them up** on the Staff page (they're hard to remember). Staff can't change their own password and have no "Forgot password": they ask the owner. Built 24 November 2026. |

Decisions still open are in **section 8**.

---

## 1. How Bosho works today

Facts taken from the code (`internal/server/auth.go`, `owner_staff.go`, `migrations/0001_init.sql`):

- **Restaurants and accounts:**
  - `restaurants` holds one restaurant. Tables, menu, orders, reviews and the audit log all belong to it through `restaurant_id`.
  - `staff_users` holds one account per person: `restaurant_id, name, phone (UNIQUE), password_hash, role`. So **one phone means one restaurant**, which you want to change.
- **Roles and access:**
  - The roles are `owner`, `kitchen`, `waiter` and `cashier`, stored as plain text.
  - There is no email anywhere, no admin, and no way to recover an owner account.
- **Signup** creates the restaurant and its owner, and the owner can use it straight away.

### Gaps found in the code (fix first)

> **1. Kitchen, Waiter and Cashier have the same access.** The only check is "logged in" versus "owner" (`a.auth(ownerOnly bool, …)`). A kitchen login can mark a bill paid (`/staff/sessions/{id}/paid`), close it without payment (`/close`) or remove items (`/staff/items/{id}/{act}`). The role only decides which page opens after login.
>
> **2. An owner can create another owner** (`staffCreate` accepts `owner`), and can delete other owner accounts.

---

## 2. The new shape: Business → Branches → People

```
Admin (you)
  └─ Business (brand)  e.g. "Kacchi Ghar"            ← owner signs up for this
       ├─ Branch "Dhanmondi"   (tables, menu, orders, settings)
       ├─ Branch "Gulshan"
       └─ Branch "Uttara"

Person (one phone, one password)
  ├─ Owner of     Kacchi Ghar (all branches)
  ├─ Waiter at    Kacchi Ghar · Gulshan
  └─ Cashier at   Another Restaurant · Mirpur
```

### Why a branch is today's "restaurant" row

Everything the app already does — tables, QR codes, menu, kitchen board, counter, reports, opening hours, Wi-Fi rule — is tied to `restaurant_id`. So **each branch stays a row in `restaurants`.** A new `businesses` table groups branches under one owner. This keeps almost all existing code working, and existing restaurants become "a business with one branch".

### Why a person is separate from their jobs

Today the account and the job are the same row. To let one phone work in several places, split them:

- **`accounts`**: the person. Phone (unique), name, password, email (optional for staff, required for owners) and email verified.
- **`memberships`**: one row per job: which account, which business, which branch (or "all branches"), which role, permission changes, and active or turned off.

---

## 3. Branches

### What the owner can do

- **Create a branch** with a name, address and phone. It gets its own slug and URL.
- When creating it, **copy the menu from an existing branch** (categories, dishes, photos, options), or start empty. After that, each branch's menu is its own, so prices and sold-out can differ by branch.
- **Settings for each branch:**
  - Tables and QR codes
  - Opening hours, and open/closed now
  - VAT/service charge
  - Ordering network (Wi-Fi)
  - Kitchen-accept setting
  - Ticket colours
- **Branch switcher** at the top of owner and manager pages: "All branches / Dhanmondi / Gulshan …".
- **Reports:**
  - Each branch on its own.
  - **All branches combined:** sales by branch, and top dishes across branches.
  - CSV export for one branch or all.
- **Staff:**
  - Add a person to one branch, several branches, or all of them.
  - A manager can be limited to their own branch.
- **Turn a branch off** (temporarily closed) or **archive** it. Its data is kept.
- **The admin limits the number of branches** for each business (for example Free: 1, Pro: 5).

### Business-level settings (shared by all branches)

These are shared by all branches:

- Business name, logo, tagline and brand colour. A branch can override them (decision 8.3).
- The owner's email and the verification status.
- The plan, limits and feature switches set by the admin.

### Later (not in the first version)

These come later:

- **One shared menu** with per-branch prices and availability. It's more work, and copying the menu covers most needs first.
- Moving stock or orders between branches.

---

## 4. Owner signup, email verification and forgot password

### Signup

1. The owner enters **business name, first branch name, their name, phone, email and password**.
2. Bosho creates the business, its first branch, the owner's account and an owner membership.
3. If verification is on for this business, the account is **Pending**, and a verification email is sent.
4. **Login stays phone + password.**

### Verification: controlled by you

- **Global setting:** "Owners must verify email": **On / Off.** It is **off by default**, so you can build this now and turn it on later.
- **Per-business override:** "Use global setting / Always require / Never require".
- **You can mark an owner verified by hand,** or un-verify them.
- **Link rules:**
  - The link is single-use, and only its hash is stored.
  - It expires after **24 hours**.
  - "Send again" works at most once per minute and 5 times per day.
- **If the owner changes their email,** it needs verifying again (suggested).

### What a Pending owner can do (decision 8.1)

| Option | Log in | Set up menu/tables | Guests can order |
|---|---|---|---|
| A. Fully blocked | No | No | No |
| **B. Set up only (suggested)** | Yes | Yes | **No**: guests see "Not taking orders yet" |
| C. Reminder only | Yes | Yes | Yes |

### Forgot password

**Owner:**

1. On the login page, they click "Forgot password?" and enter their phone or email.
2. Bosho sends a reset link to the owner's **email**. The page always says "If this account exists, we've sent a link", so it doesn't reveal which accounts exist.
3. The link works once and expires after **1 hour**. Using it logs out every device.
4. **If the owner has no email or lost access to it, you (admin) reset it** from the admin panel.

**Manager / staff:**

- **The owner sets the password and can look it up** on the Staff page. It is stored encrypted (AES-GCM, key from `PASSWORD_KEY`), not only as a hash, so it can be shown again. Passwords set before this was built show "not saved" until the owner sets a new one.
- **Staff can't change their own password,** and "Forgot password" doesn't email staff. They ask the owner.
- The owner's own password is never shown or kept in a readable form.
- Later, a manager with the "reset staff password" permission can do the same as the owner.

### Email designs

| Verify email | Reset password |
|---|---|
| ![Verify email](images/email-verify.png) | ![Reset password](images/email-reset.png) |

The names, phone and restaurant in these images are examples. The images show `bosho.app`; the real domain is **`bosho.food`** (see "Name and domain" below).

### Name and domain

- The product is called **Bosho** in every page, email and message. The domain is never used as the name.
- The planned domain is **`bosho.food`**, chosen for cost; **it isn't bought yet** (24 November 2026). A `.com` may come later. Until there is a domain, email goes through Gmail.
- The domain only lives in settings (`BASE_URL`, `EMAIL_FROM`), never in the code. Moving to `.com` later is a settings change.
- **When moving:** keep `bosho.food` and redirect it to the new domain for good. QR codes already printed on tables point at `bosho.food` and must keep working.
- Printed QR cards say "Bosho", not a domain, so they don't go out of date.

### Email sending (free)

| Service | Free amount | Needs |
|---|---|---|
| **Resend** (suggested) | 3,000/month | Your own domain |
| Brevo | 300/day | Your own domain is better |
| Gmail SMTP | ~500/day | App password; mail may land in spam more often |

Settings: `EMAIL_PROVIDER`, `EMAIL_API_KEY` or `SMTP_*`, `EMAIL_FROM` (for example `Bosho <no-reply@bosho.food>`), and the existing `BASE_URL` for links. If nothing is set, the link is **printed in the server log**, which is fine for local development.

---

## 5. People at several restaurants (staff sharing)

### Login

1. The person enters their phone and password, as now.
2. **Only one job:** they go straight in, as now.
3. **Several jobs:** they see a **"Where are you working now?"** list, for example "Kacchi Ghar · Gulshan — Waiter" or "Another Restaurant · Mirpur — Cashier".
4. A **switch button** in the header changes the workplace without logging out.
5. The session remembers the chosen branch. Every page shows the branch name, so nobody marks an order at the wrong place.

### Adding someone who already has an account

1. The owner or manager enters a phone number.
2. **New phone:** they set a name and starting password, as now.
3. **Phone already registered:**
   - The owner **can't change** that person's name.
   - The owner sets the **password for their own restaurant** (see 5.4). It doesn't touch the person's other workplaces.
   - An **invitation** is created: "Kacchi Ghar invites you to work as Waiter at Gulshan."
   - The person accepts it on their next login.
   - Until they accept, they can't work at that business.

**Why invitations:** without them, any owner could type a stranger's phone and attach them to their restaurant. Sharing accounts is only safe if the person agrees.

### 5.4 Staff passwords when one person works at several restaurants

**Decided (24 November 2026): keep it as it is.** Every owner sets, sees and changes the staff passwords of their own restaurant, with no conditions, even for a person who also works somewhere else.

To keep that safe, **each workplace has its own password** for that person:

- The password is stored on the job (membership), not on the person. Owner A only ever sees and changes the password for Restaurant A.
- **Login:** phone + password. The password tells Bosho which workplace it is, and the person goes straight there. If the same password happens to open two workplaces, the "Where are you working now?" list appears.
- Owner A's password can never open Restaurant B, so seeing it gives Owner A nothing outside their own restaurant.
- The owner can also **turn the person's job off** or **remove** it, which blocks them from that restaurant straight away.
- The owner's own password stays on their account, is never shown, and is reset only by email or by you (admin).

### Removing someone

Removing a person **ends their job at this business only.** Their account and their other jobs stay, and their name stays in the activity log.

---

## 6. Admin panel (you)

### Your admin account

- **A separate `admins` table, not a restaurant account.**
- `/admin/login` with **email + password + a 6-digit email code** (2-step).
- Admin sessions last 12 hours.
- The first admin is created by a command: `go run ./cmd/... create-admin you@example.com`.

### Businesses and branches

- **List of all businesses:** owner, phone, email, verified or not, branch count, signup date, last activity, orders today and in total, plan and status. You can search and filter.
- **Business detail page:** branches, staff, menu size, storage used and recent activity.
- **Status for a business or a single branch:**
  - **Active**
  - **Pending**
  - **Suspended**: guests see "Temporarily closed", and staff can't work there.
  - **Archived**
- **Manual approval switch:** new signups wait for your OK (off by default).

### Owner and account help

- Verification: **on, off or global for each business,** and **mark the owner verified or unverified** by hand.
- Owner account:
  - **Reset the owner's password.**
  - **Change the owner's email or phone.**
  - Move ownership to another person.
- **Reset an owner's password**, or any staff password if an owner asks for help.
- **Log out all devices** of a business or of one person.
- **Log in as the owner** to help them. A red banner shows the whole time, and the action is written to both activity logs.

### Owner access control (what each business may use)

**Limits:**

| Limit | Example |
|---|---|
| Maximum branches | 1 / 3 / 10 |
| Maximum tables per branch | 15 |
| Maximum staff (total or per role) | 10 |
| Maximum menu items / categories per branch | 150 / 20 |
| Maximum photos / storage | 200 MB |
| Maximum orders per month | 3,000 |

When a business reaches a limit, **what it already has keeps working, and only adding new things is blocked**, with a clear message.

**Feature switches** (on/off for each business):

- Branches
- Manager role
- Custom permissions per person
- Staff sharing / invitations
- Reports
- CSV export
- Combined multi-branch reports
- Closing report
- Activity log
- Dish options/add-ons
- Recommended row
- Waiter ordering for guests
- Ordering network (Wi-Fi)
- Order only within opening hours
- Reviews
- Bangla/English
- Ticket colours
- Sold-out list
- Waiter-call buttons
- Kitchen-accept requirement
- Removing "Powered by Bosho"

**Plans (later):** group the limits and switches into Free / Basic / Pro, with a trial end date and an action when it ends (read-only or suspended). You can still change single settings on top of a plan.

### Platform

- **Settings:**
  - Signup open or closed.
  - Default "owners must verify email".
  - Default approval requirement.
  - Maintenance message.
  - Default plan.
- **Statistics:**
  - Number of businesses, branches and active ones.
  - Orders today and this month.
  - Guests ordering now.
  - Businesses with no activity for 14 days.
- **Admin activity log:** who did what to which business, with before and after values.

---

## 7. Roles and permissions

### How the levels fit together

```
Admin           — sets each business's limits and feature switches
  └─ Owner      — everything the admin allows, in all branches
       └─ Manager    — almost everything the owner has; can be limited to some branches
            └─ Cashier / Waiter / Kitchen — what the owner or manager allows, in their branch(es)
```

**Main rule: nobody can give more than they have.**

- The owner can only switch on permissions for features you allow.
- A manager can only give permissions the manager has, only in their own branches, and can't create or change managers or owners.

### Each role does only its own tasks

**Your rule: every person can do only the tasks their role allows, and nothing else.**

- **Kitchen** cooks.
- **Waiter** serves.
- **Cashier** takes money.
- **Manager** runs the branch.
- **Owner** runs the business.
- **Admin** runs the platform.

The permission table below is exactly what each role can do. Anything not ticked is refused by the server, not just hidden on the page.

**Per-person changes** (the owner giving one waiter one extra task) are **off by default**. They are a feature switch that you (admin) can turn on for a business (decision 8.17). Even then:

- An owner can only *remove* a task from a person, or add a task their own level is allowed to give.
- An owner can't turn a Kitchen account into a cashier by ticking boxes. To change someone's job, change their role.

Existing kitchen, waiter and cashier accounts keep their role and get exactly that role's tasks. This is **stricter than today**: in the current code, all three can do everything on `/staff/*`.

### Permission table

| Group | Permission | Owner | Manager | Cashier | Waiter | Kitchen |
|---|---|:-:|:-:|:-:|:-:|:-:|
| **Orders** | See kitchen board | ✓ | ✓ | – | ✓ | ✓ |
| | Accept / reject order | ✓ | ✓ | – | – | ✓ |
| | Mark ready | ✓ | ✓ | – | – | ✓ |
| | Mark served | ✓ | ✓ | – | ✓ | – |
| | **Undo** ready / served ⚠ | ✓ | ✓ | – | – | – |
| | Approve guest change requests | ✓ | ✓ | – | ✓ | ✓ |
| | Order for a guest | ✓ | ✓ | – | ✓ | – |
| | Cancel an order ⚠ | ✓ | ✓ | – | – | – |
| | Handle waiter calls | ✓ | ✓ | – | ✓ | – |
| **Money** | See counter / bills | ✓ | ✓ | ✓ | – | – |
| | Mark paid | ✓ | ✓ | ✓ | – | – |
| | Remove an item from a bill ⚠ | ✓ | ✓ | – | – | – |
| | Close without payment ⚠ | ✓ | ✓ | – | – | – |
| | Discount (when added) ⚠ | ✓ | ✓ | – | – | – |
| | Reopen a closed bill ⚠ | ✓ | ✓ | – | – | – |
| **Menu** | Mark sold out | ✓ | ✓ | – | ✓ | ✓ |
| | Edit prices ⚠ | ✓ | ✓ | – | – | – |
| | Add / edit / delete dishes and categories | ✓ | ✓ | – | – | – |
| | Upload photos, edit options | ✓ | ✓ | – | – | – |
| | Copy menu between branches | ✓ | – | – | – | – |
| **Tables** | See tables, open a table, order for a guest | ✓ | ✓ | – | ✓ | – |
| | See a bill (read only) | ✓ | ✓ | ✓ | ✓ | – |
| | Add / remove tables, print QR | ✓ | ✓ | – | – | – |
| | Refresh / hide / switch off a QR | ✓ | ✓ | – | – | – |
| **Staff** | Add / remove / invite staff (below manager) | ✓ | ✓ | – | – | – |
| | Set, see and reset staff passwords (5.4) | ✓ | ✓ | – | – | – |
| | Change permissions (below manager) | ✓ | ✓ | – | – | – |
| | Add / remove / change managers | ✓ | – | – | – | – |
| **Reports** | Today's sales | ✓ | ✓ | ✓ | – | – |
| | Full reports, CSV | ✓ | ✓ | – | – | – |
| | All-branches combined report | ✓ | – | – | – | – |
| | Closing report | ✓ | ✓ | ✓ | – | – |
| | Activity log, reviews | ✓ | ✓ | – | – | – |
| **Settings** | Open / closed now, opening hours | ✓ | ✓ | – | – | – |
| | Ordering network (Wi-Fi) | ✓ | ✓ | – | – | – |
| | Branch profile, ticket colours | ✓ | ✓ | – | – | – |
| | VAT / service charge ⚠ | ✓ | – | – | – | – |
| **Owner only** | Create / archive branches, business profile, own email, plan | ✓ | – | – | – | – |

⚠ = **sensitive.** These actions can be set to need a manager or owner PIN at that moment (decision 8.6).

### Rules

- **One owner per business.** It can't be deleted or demoted by anyone except you. Only you move ownership. This replaces today's "owner can create owners".
- **Turn a job off without deleting it.** The person can't work there, and their history stays.
- **Every permission, staff or branch change is written to the activity log.**
- **Login hours** (optional): a person can only work at a branch during its opening hours.

### Access control rules (apply everywhere)

1. **Refused unless allowed.** Every route has to name the permission it needs. A route without one doesn't start; a test fails at build time. Nobody can forget a check.
2. **The server decides, not the page.** Hiding a button is only for comfort. Every request is checked again on the server, including htmx fragment and polling requests.
3. **Checked on every request, against live data.** Each request checks the following:
   - The person's job is still active.
   - The business and branch are not suspended.
   - The feature is switched on.

   If the owner turns someone off, they lose access on their **next click**, not at their next login.
4. **Only your own branch's data.** Every database query is limited to the session's branch (and business). An order, table, session, dish or staff id from another branch is answered with **"Not found"**, not "Forbidden". This applies even when the person also works at that other branch but didn't choose it.
5. **The action inside a route is checked too.** `POST /staff/orders/{id}/{act}` does several things, depending on `act`:
   - `accept` and `reject`
   - `ready` and `served`
   - `undo`

   `POST /staff/items/{id}/{act}` does these:
   - `approve` and `decline` a guest change
   - `void`: remove an item from the bill

   Each `act` has its own permission, so a waiter can mark served but not void.
6. **Nobody acts above their level:**
   - A manager can't create, change or remove a manager or the owner.
   - An owner can't create another owner.
   - Nobody can change their own role or permissions.
7. **Admin is separate.** Admin routes (`/admin/*`) use a different login and session. A restaurant session can never open them, and an admin session can't place staff actions without "log in as owner", which is logged.
8. **Guests stay limited to their table.** Guest routes (`/t/{tok}/*`) only work with an active, not-hidden QR token. They only touch that table's open session, and only the guest's own cart lines and orders where the code already limits it.
9. **Refusals are logged.** Repeated refused attempts by one person show up in the activity log, so the owner can see if someone tries to do what they shouldn't.
10. **Sensitive actions are always logged** with the person's name, even when they are allowed:
    - Void
    - Close without payment
    - Undo
    - Price change
    - Staff changes
    - Permission changes

### Every current route and who can use it

Based on the routes in `internal/server` today. ✓ = allowed, – = refused. M = Manager (new).

| Route | Task | Owner | M | Cashier | Waiter | Kitchen |
|---|---|:-:|:-:|:-:|:-:|:-:|
| `GET /staff/account`, `POST …/password`, `…/logout-all`, `GET /staff/lang` | Own account, own password, own language | ✓ | ✓ | ✓ | ✓ | ✓ |
| `GET /staff/kitchen`, `GET /staff/kitchen/board` | See kitchen board | ✓ | ✓ | – | ✓ | ✓ |
| `POST /staff/orders/{id}/accept` · `/reject` | Accept / reject order | ✓ | ✓ | – | – | ✓ |
| `POST /staff/orders/{id}/ready` | Mark ready | ✓ | ✓ | – | – | ✓ |
| `POST /staff/orders/{id}/served` | Mark served | ✓ | ✓ | – | ✓ | – |
| `POST /staff/orders/{id}/undo` | Undo ready/served ⚠ | ✓ | ✓ | – | – | – |
| `POST /staff/items/{id}/approve` · `/decline` | Guest change request | ✓ | ✓ | – | ✓ | ✓ |
| `POST /staff/items/{id}/void` | Remove item from bill ⚠ | ✓ | ✓ | – | – | – |
| `POST /staff/calls/{id}/done` | Answer waiter call | ✓ | ✓ | – | ✓ | – |
| `GET /staff/stock`, `POST /staff/stock/{id}/toggle` | Sold-out list | ✓ | ✓ | – | ✓ | ✓ |
| `GET /staff/counter`, `GET /staff/counter/list` | Counter | ✓ | ✓ | ✓ | – | – |
| `GET /staff/sessions/{id}` | See a bill | ✓ | ✓ | ✓ | ✓ | – |
| `POST /staff/sessions/{id}/paid` | Mark paid | ✓ | ✓ | ✓ | – | – |
| `POST /staff/sessions/{id}/close` | Close without payment ⚠ | ✓ | ✓ | – | – | – |
| `GET /staff/tables`, `POST /staff/tables/{id}/open` | Tables, order for a guest | ✓ | ✓ | – | ✓ | – |
| `GET /owner` | Dashboard | ✓ | ✓ | – | – | – |
| `/owner/menu`, `/owner/categories*`, `/owner/items*`, `/owner/options*`, `/owner/groups*` | Menu, dishes, options | ✓ | ✓ | – | – | – |
| `POST /owner/items/{id}` with a **new price** | Edit price ⚠ | ✓ | ✓ | – | – | – |
| `/owner/tables*` (list, add, on/off, QR image, print) | Tables and QR | ✓ | ✓ | – | – | – |
| `POST /owner/tables/{id}/newqr` | Refresh a QR code | ✓ | ✓ | – | – | – |
| `GET /owner/staff`, `POST /owner/staff`, `…/delete`, `…/password` | Staff below manager | ✓ | ✓ | – | – | – |
| same, for a **manager** account | Managers | ✓ | – | – | – | – |
| `GET /owner/activity` | Activity log | ✓ | ✓ | – | – | – |
| `GET /owner/reports`, `GET /owner/export/{what}` | Reports, CSV | ✓ | ✓ | – | – | – |
| `GET /owner/closing` | Closing report | ✓ | ✓ | ✓ | – | – |
| `GET /owner/settings`, `POST /owner/settings` — **open/closed, hours, kitchen-accept** | Branch settings | ✓ | ✓ | – | – | – |
| `POST /owner/settings` — **VAT / service charge** | Tax and charges ⚠ | ✓ | – | – | – | – |
| `GET/POST /owner/networks` | Ordering network (Wi-Fi) | ✓ | ✓ | – | – | – |
| `GET/POST /owner/profile`, `…/logo/delete` | Branch profile, logo, colours | ✓ | ✓ | – | – | – |
| *new* `/owner/branches*` | Create / archive branches | ✓ | – | – | – | – |
| *new* `/owner/business*` | Business profile, owner email, plan | ✓ | – | – | – | – |
| *new* `/admin/*` | Admin panel | **admin only** | | | | |

The `/owner/*` URLs will probably be renamed to `/manage/*`, since managers use them too. The old URLs redirect to the new ones.

**Kitchen vs waiter split for ready and served:** Kitchen marks **ready**, and waiter marks **served**. If a small restaurant has one person doing both, give them the Manager role, or turn on per-person changes (8.17).

**What each role sees after login:**

| Role | Start page |
|---|---|
| Owner, Manager | Dashboard |
| Cashier | Counter |
| Waiter | Tables |
| Kitchen | Kitchen board |

The menu bar only shows the pages that role can open.

### How it works in the code

- Permission keys look like `order.accept`, `bill.mark_paid`, `bill.remove_item`, `menu.edit_price` and `branch.create`.
- **What a person can do here** = the role's defaults + their changes, limited by the business's feature switches and by the branches their job covers. This is worked out when they choose a workplace and kept with the session.
- **Every route** uses `a.need("bill.mark_paid", h)` instead of `a.auth(ownerOnly, h)`, and every query uses the **session's branch id**. This also fixes the gaps in section 1.
- **Pages only show allowed buttons.** The server still checks every request.
- **Tests:**
  - For each role, every route is allowed or refused as the table says.
  - A person at two businesses can never see data of the one they didn't choose.

---

## 8. Database changes (summary)

```sql
-- Business groups branches. A branch is today's `restaurants` row.
CREATE TABLE businesses (
  id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',           -- active|pending|suspended|archived
  verify_email TEXT NOT NULL DEFAULT 'global',     -- global|required|off
  plan TEXT NOT NULL DEFAULT 'free',
  limits JSONB NOT NULL DEFAULT '{}', features JSONB NOT NULL DEFAULT '{}',
  trial_ends_at BIGINT, created_at BIGINT NOT NULL
);
ALTER TABLE restaurants ADD COLUMN business_id BIGINT REFERENCES businesses(id);
ALTER TABLE restaurants ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

-- A person, separate from their jobs.
CREATE TABLE accounts (
  id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL,
  phone TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
  email TEXT, email_verified_at BIGINT, pin_hash TEXT, created_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX ON accounts (lower(email)) WHERE email IS NOT NULL;

CREATE TABLE memberships (
  id BIGSERIAL PRIMARY KEY,
  account_id  BIGINT NOT NULL REFERENCES accounts(id),
  business_id BIGINT NOT NULL REFERENCES businesses(id),
  role TEXT NOT NULL,                              -- owner|manager|cashier|waiter|kitchen
  all_branches BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL DEFAULT 'active',           -- invited|active|off
  password_hash TEXT,                              -- staff: this workplace's password (5.4); NULL for the owner,
  password_enc  TEXT NOT NULL DEFAULT '',          --   who logs in with accounts.password_hash
  created_at BIGINT NOT NULL,
  UNIQUE (account_id, business_id)
);
CREATE TABLE membership_branches (membership_id BIGINT REFERENCES memberships(id) ON DELETE CASCADE,
  restaurant_id BIGINT REFERENCES restaurants(id), PRIMARY KEY (membership_id, restaurant_id));
CREATE TABLE membership_permissions (membership_id BIGINT REFERENCES memberships(id) ON DELETE CASCADE,
  permission TEXT NOT NULL, allowed BOOLEAN NOT NULL, PRIMARY KEY (membership_id, permission));

-- Sessions remember the chosen workplace.
-- staff_sessions: account_id + membership_id + restaurant_id (branch)

CREATE TABLE email_tokens (
  token_hash TEXT PRIMARY KEY, account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  purpose TEXT NOT NULL,                           -- verify|reset_password
  expires_at BIGINT NOT NULL, used_at BIGINT
);

-- Admin
CREATE TABLE admins (id BIGSERIAL PRIMARY KEY, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, created_at BIGINT NOT NULL);
CREATE TABLE admin_sessions (token TEXT PRIMARY KEY, admin_id BIGINT NOT NULL REFERENCES admins(id) ON DELETE CASCADE, expires_at BIGINT NOT NULL);
CREATE TABLE admin_codes (admin_id BIGINT NOT NULL, code_hash TEXT NOT NULL, expires_at BIGINT NOT NULL);
CREATE TABLE platform_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE admin_audit (id BIGSERIAL PRIMARY KEY, admin_id BIGINT NOT NULL, business_id BIGINT,
  action TEXT NOT NULL, detail TEXT NOT NULL DEFAULT '', created_at BIGINT NOT NULL);
```

**Moving existing data** (one migration):

- Each restaurant becomes a business with one branch.
- Each `staff_users` row becomes an account plus a membership.
- Old sessions are ended, so everyone logs in once again.
- The `audit_log` keeps names as they are.

---

## 9. Build order

Each step can go live on its own. Run `presentation-sync` after each one.

1. ✅ **Fix the access gaps.** Add role checks on the `/staff/*` routes, and stop owners from creating or deleting owners. Done: `a.need(perm, h)` on every route, per-action checks in `orderAct`/`itemAct`, role-by-route test in `perm_test.go`.
2. **Accounts, memberships and the permission system:**
   - Split people from jobs.
   - `a.need(...)` on every route.
   - Manager role.
   - Per-person permissions.
   - Turning a job off.
   - Tests.
3. **Branches:** businesses table, create a branch, copy the menu, branch switcher, branch-limited staff, combined reports. Detailed plan, built ahead of step 2: `docs/plans/multi-branch.md`.
4. **Staff at several restaurants:** invitations, a password per workplace (5.4), the "where are you working" step and the workplace switcher.
5. ✅ **Email:** sending (Gmail SMTP now; Resend once there's a domain; the log when neither is set), owner email at signup, verification link, **forgot password**, verification off by default (`EMAIL_VERIFY`, and `restaurants.verify_email` per restaurant). Still to do: the admin panel switch for it (step 6), and an email field for staff if ever needed.
6. **Admin panel:**
   - Admin login with 2-step code.
   - Business list and detail.
   - Status changes.
   - Verification control.
   - Password resets.
   - Limits and feature switches.
   - Admin log.
7. **Later:** plans and trials, "log in as owner", manager PIN for sensitive actions, login alerts, daily email report, shared menu across branches.

---

## 10. Decisions still open

| # | Question | Options | Suggestion |
|---|---|---|---|
| 8.1 | What can a **Pending (unverified) owner** do? | Fully blocked · Set up but no guest orders · Everything + reminder | **Set up, no guest orders** |
| 8.2 | **Branch menu:** separate per branch, or one shared menu? | Copy then separate · Shared with per-branch price/stock | **Copy then separate** now, shared later |
| 8.3 | Can a **branch have its own logo/colour/name style**? | Always the business's · Branch can override | **Branch can override** |
| 8.4 | Adding a phone that **already has an account**: | Invitation they accept · Added straight away | **Invitation** |
| 8.5 | Owner reset of a **shared staff member's** password | **Decided (24 Nov 2026): allowed, as for everyone else.** Each workplace has its own password (5.4). | |
| 8.6 | **PIN for sensitive actions?** | No · Optional per business · Always | **Optional**, owner turns it on |
| 8.7 | Does **staff need an email**? | No · Optional (lets them reset their own password) | **Optional** |
| 8.8 | Must a **changed owner email** be verified again? | Yes · No | **Yes** |
| 8.9 | **Plans now or later?** | Per-business switches only · Free/Basic/Pro now | **Switches now**, plans later |
| 8.10 | Do **new signups need your approval**? | No · Switch, off by default | **Switch, off by default** |
| 8.11 | Can a **manager be limited to some branches**? | Yes · Always all branches | **Yes** |
| 8.12 | Can a **manager change VAT/service charge**? | Yes · Owner only | **Owner only** |
| 8.13 | **Email service** | **Decided (24 Nov 2026): Brevo's API** on Render (Render's free plan blocks SMTP), sending from the owner's Gmail address; Gmail SMTP works for local runs. Move to Resend on `bosho.food` once the domain is bought (only settings change). | |
| 8.14 | Does **"log in as owner"** need the owner's agreement? | No, log + banner · Owner approves each time | **Log + banner** |
| 8.15 | **Suspended business/branch**: what do guests see? | "Temporarily closed" · 404 | **"Temporarily closed"** |
| 8.16 | Can **one business's QR/guest pages show its branches** (e.g. a branch list at `/r/kacchi-ghar`)? | Yes · No, each branch alone | **Yes**, simple list page |
| 8.17 | **Per-person permission changes** (beyond the role)? | Never, role only · Admin can switch it on per business | **Role only by default**, admin can switch it on |
| 8.18 | **Who marks "served"**? | Waiter only · Waiter and kitchen | **Waiter only** (kitchen marks ready) |
| 8.19 | Can a **cashier see a bill's items** without the kitchen board? | Yes (read only) · No | **Yes, read only** |
| 8.20 | Can a **waiter see the counter totals**? | No · Read only | **No** |
