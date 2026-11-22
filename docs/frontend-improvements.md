# Frontend Improvements

As of 2026-11-22, checked against commit 08f42e3 (`web/templates`, `web/static`)

This lists frontend patterns that QR ordering apps abroad (Toast, Square, me&u, Sunday, Chinese mini programs, Korean table tablets) treat as normal and that Bosho's UI does not have yet. For each one it says what Bosho does now and what to change. Nothing here is built yet.

**Must** means guests or staff will feel the gap from the first day. **Should** means it is common abroad and worth adding soon. **Option** means it fits only some restaurants.

## Status (2026-11-22)

Every Must and Should item is built, except the ones listed below. The database change is `internal/database/migrations/0004_menu_details.sql`; it runs by itself at the next start.

| Item | Status |
| --- | --- |
| 1–8, 10–18, 20–25, 27–32, 36–41 | Done |
| 9 Allergens, 26 Skeleton loading, 34 Bangla digits, 35 Dark mode, 42 Kiosk mode | Not built: marked Option |
| 19 Bill sheet with split and tip | Not built: waits for online payment (version 2) |
| 33 Reduced motion | Already in the app before this list (`app.css` turns animations off) |

Notes on how some items were done:

- **3 Option groups:** a group's rule is "required, choose 1", "optional, up to N" or "optional, any number". Options without a group stay free extras, so existing dishes keep working unchanged.
- **8 Tags:** the owner picks Chef's pick, New, Vegetarian and a spice level. "Popular" is added by itself to the three best sellers of the last 30 days (3 or more sold).
- **11 Hours:** shown to guests, with a notice in the last 30 minutes before the last order time. Ordering is still switched on and off by hand.
- **14 Suggestions:** dishes most often ordered with what is in the cart (last 60 days), then best sellers, then featured dishes.
- **27 Photos:** new dish photos are stored in two JPEG sizes (480 and 1200 pixels wide) and turned upright using the phone camera's rotation tag. Go has no WebP encoder, so they are JPEG. Photos uploaded before this change are shown as they are. Logos are kept as uploaded.
- **36 Screen stays on:** browsers allow this only on HTTPS or localhost. On a plain `http://` Wi-Fi address the kitchen and counter screens show a note asking staff to set the tablet's screen timeout instead.
- **41 Staff screens in Bangla:** the sidebar, kitchen, counter, take order and sold out screens switch with a button in the sidebar. The owner pages stay in English.

## 1. Guest menu

| # | Pattern used abroad | Bosho now | Change | Priority |
| --- | --- | --- | --- | --- |
| 1 | **Dish detail sheet.** Tapping a dish opens a bottom sheet with a large photo, a description, options, a quantity picker, a note and an "Add ৳ total" button. | Options, Add and the duplicate warning all sit inside the small dish card. There is no detail view. | Add a bottom sheet (a `<dialog>` loaded by htmx) opened by tapping the dish. Keep a quick "+" on the card for dishes without options. | Must |
| 2 | **Dish description.** One or two lines on what the dish is, its portion and what it comes with. | `menu_items` has no description field. | Add `desc_bn` and `desc_en` to `menu_items` and the owner menu form. Show one line on the card and the full text in the sheet. | Must |
| 3 | **Option groups with rules.** "Size: choose 1 (required)", "Extras: up to 3". Radio buttons for a single choice, checkboxes for several. | Every option is a free checkbox. You cannot make a choice required or limit it to one. | Add a group name, required flag, and minimum and maximum to `item_options`. Render a single choice as radios. Block Add until required groups are filled. | Must |
| 4 | **Quantity before adding.** A − 1 + picker in the sheet, so "3 naan" takes one tap. | Each tap adds one. Quantity changes only in the cart. | Add a quantity picker to the detail sheet. | Must |
| 5 | **Price shown with options.** The Add button shows the live total, for example "Add · ৳360". | The button says "+ Add" with no total. | Update the button total as options and quantity change. | Should |
| 6 | **Active category follows scrolling (scroll-spy).** The sticky category bar highlights and scrolls to the section on screen. | The category bar is sticky but does not show which section you are in. | Use an `IntersectionObserver` in `app.js` to mark the current category and scroll the bar to it. | Should |
| 7 | **Menu search.** | None. | Add a search box at the top that filters dishes as you type, in both languages. | Should, Must for long menus |
| 8 | **Badges and dietary tags:** Popular, Chef's pick, New, Spicy level, Vegetarian, contains nuts or dairy. | Only "Sold out". | Add tags to dishes (owner chooses) and show small icons with text. "Popular" can be calculated from sales. | Should |
| 9 | **Allergen information.** Required by law in the UK and EU. | None. | Add an allergen list per dish, shown in the detail sheet. Optional in Bangladesh, but cheap to add with the tags. | Option |
| 10 | **Photo first layout.** Large photos, with best sellers at the top of each category. One US group lost 10% on average bills when guests did not scroll the whole menu. | Photos are shown; order follows the owner's sort. | Allow a "featured" row at the top of the menu, and let the owner pin dishes to the top of a category. | Should |
| 11 | **Opening hours and "kitchen closes at" notice.** | A "closed now" warning only. | Show hours and a "last order at 10:30 pm" banner near closing. | Should |

## 2. Cart and placing the order

| # | Pattern used abroad | Bosho now | Change | Priority |
| --- | --- | --- | --- | --- |
| 12 | **Full total before ordering.** The cart shows subtotal, VAT, service charge and the estimated total. Hidden charges led to a lawsuit against Sunday in 2026. | The cart shows only the subtotal. VAT and service appear later, on the bill. | Show VAT, service charge and the total in the cart. Add a line on the menu: "Prices exclude VAT x% and service y%". | Must |
| 13 | **Order confirmation step.** A short screen or sheet: "Send 5 items to the kitchen?" with the list, then a clear "Order sent" screen. | Place order sends straight away and moves to the orders page. | Add a confirmation sheet, then a success screen with the order number and the estimated time. | Should |
| 14 | **"Goes well with" suggestions** in the cart (drinks, sides, dessert). | None. | Show 2 to 4 suggested dishes under the cart, chosen by the owner or from sales. | Should |
| 15 | **Undo after removing an item** ("Removed biryani · Undo"). | Removing goes to zero with no undo. | Show a short toast with Undo. | Should |
| 16 | **Name on the device** shown and editable ("Ordering as Rafi"). | The name is set on entry and shown on cart lines. | Show "Ordering as …" in the cart with a change link. | Should |

## 3. Order status and bill

| # | Pattern used abroad | Bosho now | Change | Priority |
| --- | --- | --- | --- | --- |
| 17 | **Alert the guest when food is ready** (vibration, sound, or a notification). Standard in pay-first and pickup apps. | The status changes on screen only. | When an order becomes Ready, vibrate the phone (`navigator.vibrate`) and change the tab title ("Ready! · Bosho"). Needed for the pay-first and pickup modes. | Must for pay-first |
| 18 | **Remaining time, not only a number.** A progress ring or bar counting down. | A ⏱ line with the minutes. | Add a simple progress bar that moves from Placed to Ready. Show a range ("15 to 20 min"). | Should |
| 19 | **Bill as a sheet with split and tip** (Sunday, Toast). | The bill is a section on the orders page with Request bill. | When online payment comes: a bill sheet with "Pay all", "Split evenly", "Pay my items", tip buttons (0%, 5%, 10%) and the payment methods. | Must for version 2 |
| 20 | **Digital receipt** to save or share. | A Paid screen with thanks and contact details. | Add "Download receipt" (a printable page or PDF) with the items, VAT, total, payment method and time. | Should |
| 21 | **Feedback for call-waiter.** "Waiter is coming", with the request time, and a limit on repeated calls. | The call is sent. | Show "Called at 8:42 · waiter notified" and disable the button for a minute. Show when staff have seen it. | Should |

## 4. Feedback, loading and errors (all screens)

| # | Pattern used abroad | Bosho now | Change | Priority |
| --- | --- | --- | --- | --- |
| 22 | **Busy state on buttons.** The button shows a spinner and is disabled while the request runs, so a double tap does nothing. | Buttons stay active during htmx requests. | Use `hx-disabled-elt="this"` and an `hx-indicator` spinner on every action button. | Must |
| 23 | **Connection banner.** "No internet. Reconnecting…" and a retry. | A failed poll or tap shows nothing. | Listen for `htmx:sendError`, `htmx:responseError` and the browser `offline` event, and show a banner. Retry failed actions or tell the user. | Must |
| 24 | **Toasts for short messages** ("Added", "Sent to kitchen", "Something went wrong"). | Inline text such as "✓ Added" and warning paragraphs. | Add one small toast area and use it for success and error messages. Keep inline warnings for things that need a decision. | Should |
| 25 | **In-page dialogs instead of browser pop-ups.** | `hx-confirm` and `onsubmit="return confirm(...)"` use the browser's own pop-up. It looks foreign, is in English, and some in-app browsers block it. | Replace them with a styled `<dialog>`, in Bangla or English to match the guest. | Should |
| 26 | **Skeleton loading** for the menu and orders. | Pages render complete from the server, which is fine. | Only needed where htmx loads a part; add a light placeholder for the detail sheet. | Option |

## 5. Speed on slow phones and 4G

| # | Pattern used abroad | Bosho now | Change | Priority |
| --- | --- | --- | --- | --- |
| 27 | **Responsive images** (`srcset`, WebP, fixed width and height). | One uploaded file, served as is, with `loading="lazy"`. | Resize uploads to two or three sizes in WebP, use `srcset`, and set `width` and `height` so the page does not jump. | Must |
| 28 | **Self-hosted fonts.** | Hind Siliguri and Tiro Bangla load from Google Fonts. | Serve a subset of the fonts from `/static` with `font-display: swap`. Saves a slow extra connection on 4G. | Should |
| 29 | **Cache static files.** | Not checked. | Add long cache headers and file-version names for CSS, JS and images. | Should |

## 6. Accessibility and readability

The main complaint about QR menus abroad was that they were hard to read and use. These follow WCAG 2.2 AA.

| # | Pattern used abroad | Bosho now | Change | Priority |
| --- | --- | --- | --- | --- |
| 30 | **Tap targets of at least 44 px.** | Small buttons (`.btn.sm`, 6 px padding) and option labels are below that. | Make every tap target at least 44 px high on guest and kitchen screens. | Must |
| 31 | **Minimum text size of about 14 px.** | Badges, step labels and some meta text are about 11 to 12 px (`.72rem` to `.78rem`). | Raise the smallest sizes to `.875rem`. | Should |
| 32 | **Labels in the guest's language.** | Some `aria-label`s are fixed English ("less", "more"). | Move them into `i18n`. | Should |
| 33 | **Respect reduced motion.** | Animations (`pop`, slide-in) always run. | Wrap animations in `@media (prefers-reduced-motion: no-preference)`. | Should |
| 34 | **Bangla digits in Bangla mode** (৳১২০ instead of ৳120), as local apps such as bKash do. | Numbers show as Latin digits. | Optional switch; many Bangladeshi users read both. Test with pilot guests before choosing. | Option |
| 35 | **Dark mode.** | Light only. | Low priority for guests in a lit restaurant. More useful for the kitchen screen at night. | Option |

## 7. Staff screens

| # | Pattern used abroad | Bosho now | Change | Priority |
| --- | --- | --- | --- | --- |
| 36 | **Screen stays on.** Kitchen displays never sleep. | The tablet screen can turn off and miss orders. | Use the Screen Wake Lock API on the kitchen and counter screens, with a reminder if it is not supported. | Must |
| 37 | **Ticket timers with colour.** Green, then amber, then red as an order waits (standard in kitchen display systems). | Shows "5 min ago" text. | Colour each ticket by waiting time, with limits the owner can set. | Must |
| 38 | **Large type and big buttons for the kitchen.** Read from a metre away, tapped with wet hands. | Same card style as the other staff screens. | A kitchen layout with bigger text, item quantities in bold and full-width Accept and Ready buttons. | Should |
| 39 | **Undo for staff actions** (marked ready or served by mistake). | No undo. | Show "Marked served · Undo" for about 10 seconds. | Should |
| 40 | **Install as an app on the tablet** (web manifest, home screen icon, full screen). | No manifest. | Add `manifest.json` and icons so the kitchen tablet opens full screen. Also needed for the table-tablet option. | Should |
| 41 | **Staff screens in Bangla.** | Staff templates are English only. | Many kitchen and waiter staff in Bangladesh prefer Bangla. Add a language switch to the staff screens. | Should |
| 42 | **Kiosk mode for table tablets.** A locked full-screen guest menu tied to one table, which resets after the bill is paid. | Not available. | Belongs with the table-tablet option in `missing-features.md`. | Option |

## Suggested order of work

1. Busy buttons (22), connection banner (23), full total in the cart (12), screen stays on (36). These are small and protect against real mistakes.
2. Dish detail sheet with descriptions, option groups and quantity (1 to 5). The biggest visible change, and what guests abroad expect.
3. Ticket timers and kitchen layout (37, 38), tap targets and text size (30, 31).
4. Responsive images and self-hosted fonts (27, 28).
5. The rest, then the bill sheet with split and tip when online payment arrives (19).
