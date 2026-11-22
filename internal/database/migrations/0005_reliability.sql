-- Who did what: price changes, removed items, payments, closing without payment, staff changes.
CREATE TABLE audit_log (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  staff_name    TEXT NOT NULL,
  action        TEXT NOT NULL,
  detail        TEXT NOT NULL DEFAULT '',
  created_at    BIGINT NOT NULL
);
CREATE INDEX idx_audit_restaurant ON audit_log(restaurant_id, created_at);

-- Take guest orders only during the opening hours (needs open_time and close_time).
ALTER TABLE restaurants ADD COLUMN orders_in_hours BOOLEAN NOT NULL DEFAULT FALSE;
-- Google review link offered to guests who gave 4 or 5 stars.
ALTER TABLE restaurants ADD COLUMN review_url TEXT NOT NULL DEFAULT '';
