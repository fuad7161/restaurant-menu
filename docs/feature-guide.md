# Bosho (বসো): Complete Feature Guide

QR table ordering for restaurants. A guest scans the QR code on the table, orders from their phone's browser, follows the order while it cooks, asks for the bill and leaves a review. No app to install, no signup for guests.

This guide lists everything Bosho does, role by role, and then walks through each workflow from start to end. Written against commit `9607a4c` (2026-11-24).

---

## Roles at a glance

| Role | Where they work | Starts on | Main job |
| --- | --- | --- | --- |
| **Guest** | Their own phone, after scanning the table QR | Menu | Order, track food, call staff, ask for the bill, review |
| **Kitchen** | Tablet in the kitchen | Kitchen board | Accept or reject orders, cook, mark ready, mark dishes sold out |
| **Waiter** | Phone or tablet on the floor | Take order | Order for guests who don't scan, serve food, answer calls |
| **Cashier** | Counter tablet or computer | Counter | Discounts, take payment, print bills, closing report |
| **Owner** | Any device | Dashboard | Everything above, plus menu, tables, staff, settings, reports |

Each role sees only the screens it may use, and the server checks the role on every action, so hiding a button is never the only protection.

---

## 1. Guest

### Getting in
- Scan the table QR and the restaurant's menu opens, already knowing the table.
- The restaurant's own branding on every screen: logo, brand colour, tagline, address, phone.
- Bangla or English, switchable at any time.
- Opening hours are shown, with a "last orders at …" notice in the last 30 minutes before.
- A privacy note: only the name the guest types is kept. No phone number, no account.
- If the restaurant allows ordering only on its Wi-Fi, a guest outside it is asked to connect first.

### Sharing a table
- The first guest at a table starts a group. Everyone in the group shares one cart and one bill.
- A newcomer at a busy table chooses:
  - **Join**: someone already in the group must tap **Allow**.
  - **Separate order**: a new group (Group B) with its own bill.

### Menu
- Dishes by category, with photos, short descriptions, prices in taka and tags (Popular, Chef's pick, New, Vegetarian, spice level).
- A "Recommended" row of the owner's featured dishes.
- Search in Bangla or English, and a category bar that follows the scroll.
- Sold-out dishes are greyed out.
- A line says which charges (VAT, service charge) are added on top.
- Tapping a dish opens a sheet with a big photo, the full description, options with their rules ("Size: required, choose 1", "up to 3"), a note, quantity and a live total.

### Cart and ordering
- One shared cart. Every item shows who added it.
- A warning if someone already added the same dish ("Rafi already added 1 biryani. Add another?").
- Notes per item ("less spicy"), quantity changes, and **Undo** after removing an item.
- "Goes well with" suggestions under the cart.
- A confirmation that lists the items and the estimated total with VAT and service charge, then "Order sent".
- Order again later: each new round is added to the same bill.

### After ordering
- Live status: Received → Cooking → Ready → Served, with an estimated ready time and a progress bar.
- When the food is ready, the phone vibrates, beeps if it can and shows "Ready" in the tab title.
- Change or cancel an item freely until the kitchen accepts it. After that, the guest sends a request and staff approve or decline it.
- Call a waiter, or ask for water or tissue. The button shows when it was called and when staff answered.

### Bill, payment and review
- Ask for the bill: itemised, with any discount, VAT and service charge. Can be withdrawn to keep ordering.
- After the cashier marks it paid, the guest sees a receipt to print or save as a PDF. It shows the restaurant's VAT number (BIN), the invoice number, items, discount, charges, total, payment method and time.
- Review: 1 to 5 stars for the visit with a comment, plus optional stars for each dish. Guests who give 4 or 5 stars are offered the restaurant's Google review page.

---

## 2. Kitchen

- **Kitchen board** with New, Cooking and Ready columns. It refreshes every 3 seconds, with large text and buttons.
- Each ticket shows how long it has been waiting, and turns amber, then red, after the times set by the owner.
- **Accept** an order with a ready time, or **Reject** it with a reason.
- **Mark ready**, with **Undo** for one minute.
- Approve or decline guests' change requests on accepted items.
- A sound alert for new orders and change requests. It repeats every 15 seconds until the order is accepted.
- **Sold out**: switch a dish off or back on, and guests see the change at once.
- Keeps the tablet screen awake, and can be installed on the home screen like an app.
- Bangla or English.

---

## 3. Waiter

- **Take order**: open a table and order for guests who don't scan, from the same menu. These orders go straight to the kitchen without waiting for Accept.
- Sees the kitchen board: **Mark served**, with **Undo** for one minute.
- Answers guest calls (waiter, water, tissue) and marks them done.
- Approves or declines change requests.
- Marks dishes sold out.
- Can view a bill and print it.
- Bangla or English.

---

## 4. Cashier

- **Counter**: every open bill as a card. Bills where guests asked to pay come first, with a sound alert.
- **Bill detail**: every order and item, subtotal, discount, service charge, VAT and total.
- **Discount**: a percent or a taka amount with a required reason. It comes off the subtotal before service charge and VAT, can be changed or removed until the bill is paid, and is recorded in the activity log.
- **Take payment**: mark paid by cash, bKash, Nagad or card. The method is recorded; no money moves through Bosho. Tapping twice records only one payment.
- **Print bill**: for a receipt printer or A4. Before payment it prints the table's bill; after payment, the receipt with the BIN and invoice number.
- Each paid bill gets the restaurant's next invoice number (1, 2, 3 …).
- **Closing report** for any day.
- Bangla or English.

---

## 5. Owner

The owner can do everything the other roles can, plus the following.

### Dashboard
- Today: sales, bills paid, open bills, average rating.
- Today's takings by payment method, to check against the cash drawer.
- Best sellers of the last 30 days, and the latest reviews.
- A get-started checklist for a new restaurant: profile, menu, tables, VAT, kitchen screen.

### Menu
- Categories, and dishes in Bangla and English with descriptions, price, photo, preparation time, tags and "featured".
- Photos are resized for phones automatically.
- Options with extra prices, optionally in groups with rules: required (choose 1), up to N, or any.

### Tables and QR codes
- Add tables, or switch one off (it is hidden from guests).
- Branded QR codes with the restaurant's logo or initial in the middle.
- Print QR cards for all tables or for one.
- Renew a table's QR code, so old photos of it stop working.

### Staff
- Add kitchen, waiter and cashier accounts.
- Set each staff member's password, look it up again later (it is stored encrypted), or change it, which logs that person out everywhere.
- One owner per restaurant.

### Profile
- Name, logo, brand colour (12 ready colours, all easy to read), tagline, address, phone.
- VAT registration number (BIN) for bills and receipts.
- Google review link.

### Settings
- VAT % and service charge %.
- Open or closed for orders.
- Staff must accept each guest order (on by default, which stops fake orders).
- Opening hours shown to guests, and an option to take guest orders only during those hours.
- The minutes after which kitchen tickets turn amber and red.
- **Ordering network**: guests can order only on the restaurant's Wi-Fi. One tap allows the current network.

### Reports and records
- Reports for the last 7, 30 or 90 days: sales and bills per day, average bill, sales per dish, orders per hour.
- CSV downloads for any dates: orders, payments (with invoice number and discount) and reviews. They open in Excel with Bangla intact.
- **Closing report**: money by payment method, bills paid, discounts, items removed by staff, rejected orders, groups closed without payment, bills still open. Printable.
- **Activity log**: price changes, deleted dishes, items removed from bills, discounts, payments, bills closed without payment, rejected orders, and changes to staff, passwords and charges.

### Owner-only actions on a bill
- Remove an item from a bill, with a recorded reason.
- Close a bill without payment (for example, guests who scanned and left).

---

## 6. Accounts and access (all staff)

- **Sign up** creates a restaurant and its owner account, with the owner's email.
- Staff log in with phone number and password. Five wrong passwords lock that number for 15 minutes.
- **My account**: log out on all devices. The owner also changes their own password and email there.
- **Forgot password** (owner): an emailed link that works once, for 1 hour, and logs out every device. Staff ask their owner instead.
- **Email confirmation** (owner): off by default. When switched on, the owner can set up the restaurant, but guests can't order until the email is confirmed.

---

## 7. Workflows, start to end

### A. A new restaurant gets set up (owner)
1. Owner opens `/signup` and enters the restaurant name, their name, phone, email and password.
2. The dashboard opens with a get-started checklist.
3. **Profile**: logo, brand colour, tagline, address, phone, BIN, Google review link.
4. **Settings**: VAT %, service charge %, opening hours, accept rule, ticket colour timings, and optionally Wi-Fi-only ordering.
5. **Menu**: add categories, then dishes with photos, prices, tags and options.
6. **Tables**: add tables and print the QR cards; place one card on each table.
7. **Staff**: add kitchen, waiter and cashier accounts and give them their passwords.
8. Open the Kitchen board on the kitchen tablet and turn on its sound.

### B. A guest dines (the main flow)
1. The guest scans the table QR. The menu opens in Bangla (English is one tap away).
2. They type their name. If the table already has a group, they choose to join it (the group taps Allow) or to start a separate order.
3. They browse, open dishes, choose options and add to the shared cart.
4. They check the cart and the estimated total, and place the order.
5. **Kitchen** hears the alert and taps **Accept** with a ready time, or **Reject** with a reason.
6. The guest sees "Cooking" and the estimated ready time.
7. **Kitchen** taps **Mark ready**. The guest's phone vibrates and beeps.
8. **Waiter** serves the food and taps **Mark served**.
9. The guest can order more rounds, which are added to the same bill.
10. The guest taps **Ask for the bill**. The counter hears an alert and the bill moves to the top.
11. **Cashier** opens the bill, adds a discount if needed (with a reason), prints the bill if the guest wants paper, and marks it paid with the method used.
12. The guest's phone shows the receipt with the invoice number, then the review form.
13. A guest who gives 4 or 5 stars is offered the restaurant's Google review page.

### C. A guest who doesn't scan (waiter)
1. **Waiter** opens **Take order** and taps the table (a new group, or an existing one).
2. The waiter orders from the same menu. The order goes straight to the kitchen as accepted.
3. Cooking, serving and payment continue as in workflow B from step 6.

### D. A guest changes their mind
1. **Before the kitchen accepts:** the guest changes or cancels the item themselves.
2. **After the kitchen accepts:** the guest sends a change request. Kitchen or waiter approves or declines it, and the guest sees the answer.
3. **After serving:** only the owner can remove the item, with a recorded reason. It appears in the closing report and the activity log.

### E. Calling a waiter
1. The guest taps **Call waiter**, **Water** or **Tissue**.
2. The call appears on the board for waiters, with a sound alert.
3. The waiter taps **Done**, and the guest's button shows that staff answered.

### F. A dish runs out
1. **Kitchen** or **Waiter** opens **Sold out** and switches the dish off.
2. Guests see it greyed out at once and can't order it.
3. Switch it back on when it is available again.

### G. A table leaves without paying
1. **Owner** opens the bill and taps **Close without payment**.
2. Unserved orders are cancelled.
3. It is recorded in the activity log and in that day's closing report.

### H. End of the day (cashier or owner)
1. Check the Counter: any bills still open must be paid or closed.
2. Open **Closing report**: money by payment method, bills paid, discounts, items removed, rejected orders, unpaid closures.
3. Count the cash drawer against the cash line; check bKash, Nagad and card against their statements.
4. Print the report.

### I. Weekly or monthly review (owner)
1. **Reports**: sales per day, average bill, best and worst dishes, busiest hours.
2. **Dashboard**: ratings and the latest reviews.
3. Download CSVs for the accountant.
4. **Activity log**: check price changes, discounts and removed items.

### J. Staff changes (owner)
1. **New staff**: Staff → add a name, phone, role and password, then tell them the password.
2. **Forgotten staff password**: look it up on the Staff page, or set a new one.
3. **Someone leaves**: remove their account; they are logged out everywhere.
4. **Owner forgets their password**: "Forgot password?" on the login page sends a reset link by email.

### K. Security incidents
1. **QR photo shared outside**: Tables → renew that table's QR, then print the new card.
2. **Fake orders from outside**: keep "staff must accept" on, and switch on Wi-Fi-only ordering.
3. **Lost phone or tablet**: My account → log out on all devices, or the owner changes that person's password.

---

## 8. Built in behind the scenes

- **Money is exact**: amounts are stored in poisha, and prices, discounts, VAT and service charge are always worked out on the server. A dish's name and price are copied onto each order, so old bills never change.
- **The bill belongs to a group, not a table**, so two groups at one table get separate bills.
- **Safe on weak internet**: a "No internet, reconnecting" banner, buttons that can't send twice, and screens that catch up when the connection is back.
- **Fast on slow phones**: the app serves its own fonts, files are cached, and photos come in two sizes.
- **Safe writes**: money changes are saved all at once or not at all, a second tap changes nothing, and a cleanup job runs every 10 minutes.
- **Security**: guest phones are limited to 60 taps a minute, logins lock after 5 wrong passwords, posts from other websites are refused, and strict security headers are set.
- **Many restaurants on one installation**: each restaurant's data is kept apart.
- **Keeps working when parts fail**: the app runs on if photo storage or Redis is down.
- **Operations**: request logs, a time limit on every request, a database backup command with a restore check, a busy-evening load test, and automatic tests on every push.

---

## 9. Not built yet

Online payment (bKash, Nagad or card through a payment gateway), split bill, tips, kitchen ticket printing, subscription billing, live push updates (screens refresh every 3 to 4 seconds instead), allergen information, dark mode, table tablet (kiosk) mode, multiple branches.
