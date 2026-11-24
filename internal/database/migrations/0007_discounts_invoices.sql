-- VAT registration number (BIN) printed on bills and receipts, and a running invoice number per restaurant.
ALTER TABLE restaurants ADD COLUMN bin TEXT NOT NULL DEFAULT '';
ALTER TABLE restaurants ADD COLUMN invoice_seq BIGINT NOT NULL DEFAULT 0;
-- Given when the bill is paid: 1, 2, 3 ... per restaurant. 0 = not paid (or paid before this was added).
ALTER TABLE table_sessions ADD COLUMN invoice_no BIGINT NOT NULL DEFAULT 0;

-- A discount on a group's bill, set by the cashier or the owner with a reason.
-- Either a percent of the subtotal or a fixed amount in poisha; VAT and service charge are worked out after it.
ALTER TABLE table_sessions ADD COLUMN discount_percent DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE table_sessions ADD COLUMN discount_amount  BIGINT NOT NULL DEFAULT 0;
ALTER TABLE table_sessions ADD COLUMN discount_reason  TEXT NOT NULL DEFAULT '';
ALTER TABLE table_sessions ADD COLUMN discount_by      TEXT NOT NULL DEFAULT '';
-- The discount taken off when the bill was paid, so reports don't depend on later changes.
ALTER TABLE payments ADD COLUMN discount BIGINT NOT NULL DEFAULT 0;
