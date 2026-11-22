# Bosho (বসো): Presentation Format

<!-- presentation-sync: last synced at commit 1cc0df1 (plus the minimal-ui redesign) on 2026-11-23 -->

The slide-by-slide outline for presenting Bosho. Facts come from `FEATURES.md` (feature IDs in brackets), `qr-table-ordering-plan.md` (business plan) and `global-qr-ordering-research.md` (market). Do not put a claim on a slide that is not in one of those three.

**Length:** 15 slides, about 12 minutes plus a 3-minute live demo.
**Audience:** restaurant owners and investors. Plain words, no code on slides except slide 11.
**Look:** the app's own style. White page, Inter font, sindoor maroon `#8f1d21` as the one accent, thin grey borders, no decoration. Bangla title next to English.
**Each slide:** one title that states the point, at most 4 bullets or one picture, speaker notes underneath.

---

## 1. Title

- **Bosho (বসো)**: "Sit down. Scan. Order."
- QR table ordering for restaurants in Bangladesh.
- Picture: a phone over a table QR card with the restaurant mark in the middle [O-07].

*Notes:* "Bosho" is what a host says to a guest: please sit.

## 2. The problem

- Guests wait to catch a waiter's eye, to order, to order more, to get the bill.
- Orders get misheard; the kitchen gets slips in a pile.
- Owners do not know what sold today or what guests thought.

*Source:* plan, Summary and section 2 (guest workflow).

## 3. The answer in one line

- Scan the QR on the table, the menu opens in the phone browser, the whole meal runs from there.
- No app to install, no signup to order [G-01].
- The waiter stays; the waiting goes.

## 4. The guest's meal, step by step

Picture: a row of 6 phone screenshots. (screenshot needs retaking)
1. Scan, search the menu in Bangla or English, tap a dish for photo, options and total [G-01, G-03, G-04, G-16, G-17]
2. Build one cart together, see the full total with VAT before sending [G-05, G-06, G-07, G-09, G-18]
3. Watch it cook with a progress bar; the phone buzzes when it is ready [G-10]
4. Call a waiter, ask for the bill, pay, review, keep the receipt [G-12, G-13, G-14, G-20]

## 5. Real tables are messy, and Bosho handles it

| Situation | What happens |
| --- | --- |
| Strangers share your table | They start Group B with their own bill, or join yours only if you tap Allow [G-08, P-04] |
| One biryani too many | Change it yourself until the kitchen accepts; after that, request a change [G-11, K-03] |
| Guest won't scan | The waiter orders for them on a staff phone, same kitchen queue [K-07] |

## 6. The kitchen screen

- New, Cooking, Ready columns; tickets turn amber, then red, as they wait [K-01].
- Accept with a ready time, or reject with a reason; undo a mistaken tap [K-02, K-11].
- Waiter calls and change requests in the same place, with a sound alert [K-03, K-04, K-05].
- In Bangla or English, and the tablet stays awake [K-12, K-13]. Picture: kitchen board screenshot. (screenshot needs retaking)

## 7. The counter and the floor

- Every open bill as a card; remove an item with a recorded reason [K-08].
- Mark paid by cash, bKash, Nagad or card [K-09].
- Mark a dish sold out and guests see it at once [K-06].
- Staff sidebar, each role lands on its own page [K-10].

## 8. The owner's view

- Today: sales, bills paid, open bills, average rating; takings by method for the cash drawer [O-01, O-02].
- Reports by day, dish and hour, with CSV export for the accountant [O-13, O-14].
- Closing report each night: what was taken, removed or left unpaid [O-15].
- Activity log of every price change, removal and payment [O-16]. Picture: reports screenshot. (screenshot needs retaking)

## 9. Set up in an afternoon

- Get-started checklist on the first login [O-04].
- Menu with photos, descriptions, tags and option rules [O-05], tables [O-06], staff with roles [O-09].
- Logo and brand colour on every guest screen and QR card [O-10, G-02, O-07].
- Print the QR cards and put them on the tables [O-08].

## 10. No fake orders

- Staff accept each guest order before it is cooked (on by default) [O-11].
- Order only on the restaurant's Wi-Fi, set with one tap [O-12, G-15].
- Renew a table's QR so an old photo stops working [O-08].
- Prices and totals are always computed on the server; guest taps are rate-limited and other websites cannot post [P-03, P-12].

## 11. How it is built

Picture: a diagram. Phone browser → Go server (htmx pages) → PostgreSQL (Supabase), photos (Backblaze B2), Redis change counter (Upstash).
- One installation serves many restaurants [P-02].
- Screens that refresh skip the database when nothing changed [P-05].
- Free online services, deploys with Docker or Fly.io [P-06, P-07].
- Built for weak internet and slow phones; checked writes, nightly backups, load-tested [P-09, P-10, P-11, P-13].

## 12. How it compares

- QR ordering is proven abroad (China, Korea, Australia, UK, US); guests resisted only when it was forced on them.
- Bosho keeps the waiter and paper option (hybrid), shows every charge up front, needs no guest signup.
- In Bangladesh, QR is mostly view-only menus or a small POS add-on.

*Source:* research, Summary, sections 1 and 4.

## 13. What comes next

- From "Not built yet" in `FEATURES.md`: online payment (bKash, Nagad, card), split bill, tips, discounts, kitchen ticket printing, subscription billing, multiple branches.

## 14. Business plan

- First customers: mid-range restaurants and cafes in Dhaka.
- Pilot free with 3 to 5 restaurants, then a flat monthly fee per outlet (suggested 1,500 to 3,000 taka, tiered by tables; still to be tested with owners).
- Cash and "mark as paid" first; payment gateway after the trade licence.

*Source:* plan, Summary and section 5. The plan's architecture section (6) predates the Go build; take tech facts from `FEATURES.md`, not the plan.

## 15. Live demo and ask

- Demo script: owner logs in → shows the menu → guest scans table 1 on a phone → places an order → kitchen accepts → guest sees the ready time → guest asks for the bill → counter marks it paid → guest leaves a review → dashboard shows the sale.
- Demo data: `DEMO=1`, owner phone `01700000000`, guest link `/t/demo-table-1` [P-08].
- The ask: pilot restaurants / funding / feedback (fill in per audience).

---

## Feature → slide map

Used by the `presentation-sync` skill to find slides that a feature change affects.

| Feature IDs | Slides |
| --- | --- |
| G-01 to G-07 | 3, 4 |
| G-08, G-11, P-04 | 5 |
| G-09, G-10, G-12, G-13, G-14 | 4 |
| G-16 to G-18, G-20 | 4 |
| G-19 | (not on a slide) |
| G-02 | 9 |
| G-15 | 10 |
| K-01 to K-05 | 6 |
| K-06 to K-10 | 7 (K-07 also 5) |
| K-11 to K-13 | 6 |
| O-01 to O-03 | 8 |
| O-13 to O-16 | 8 |
| O-04 to O-10 | 9 (O-07 also 1, O-08 also 10) |
| O-11, O-12 | 10 |
| A-01, A-02 | 9 |
| P-01 to P-07 | 11 (P-03 also 10) |
| P-09, P-10, P-11, P-13 | 11 |
| P-12 | 10 |
| G-21, K-14 | (not on a slide) |
| P-08 | 15 |
| Not built yet | 13 |
