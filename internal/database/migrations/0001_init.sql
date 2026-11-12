-- Times are Unix seconds (BIGINT). Money is poisha (BIGINT).

CREATE TABLE restaurants (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL,
  slug            TEXT NOT NULL UNIQUE,
  vat_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
  service_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
  is_open         BOOLEAN NOT NULL DEFAULT TRUE,
  require_accept  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      BIGINT NOT NULL
);

CREATE TABLE staff_users (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  name          TEXT NOT NULL,
  phone         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL
);

CREATE TABLE staff_sessions (
  token      TEXT PRIMARY KEY,
  staff_id   BIGINT NOT NULL REFERENCES staff_users(id) ON DELETE CASCADE,
  expires_at BIGINT NOT NULL
);

CREATE TABLE tables (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  label         TEXT NOT NULL,
  qr_token      TEXT NOT NULL UNIQUE,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE menu_categories (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  name_bn       TEXT NOT NULL DEFAULT '',
  name_en       TEXT NOT NULL DEFAULT '',
  sort_order    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE menu_items (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  category_id   BIGINT NOT NULL REFERENCES menu_categories(id),
  name_bn       TEXT NOT NULL DEFAULT '',
  name_en       TEXT NOT NULL DEFAULT '',
  price         BIGINT NOT NULL,
  photo         TEXT NOT NULL DEFAULT '',
  prep_minutes  INTEGER NOT NULL DEFAULT 15,
  is_available  BOOLEAN NOT NULL DEFAULT TRUE,
  deleted       BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_menu_items_restaurant ON menu_items(restaurant_id);

CREATE TABLE item_options (
  id           BIGSERIAL PRIMARY KEY,
  menu_item_id BIGINT NOT NULL REFERENCES menu_items(id),
  name         TEXT NOT NULL,
  extra_price  BIGINT NOT NULL DEFAULT 0
);

-- One visit by one group. A table can hold several open sessions (Group A, Group B).
CREATE TABLE table_sessions (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  table_id      BIGINT NOT NULL REFERENCES tables(id),
  group_label   TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'open',   -- open, bill_requested, paid, closed
  opened_at     BIGINT NOT NULL,
  closed_at     BIGINT NOT NULL DEFAULT 0,
  last_activity BIGINT NOT NULL
);
CREATE INDEX idx_sessions_table ON table_sessions(table_id, status);
CREATE INDEX idx_sessions_restaurant ON table_sessions(restaurant_id, status);

CREATE TABLE session_guests (
  id           BIGSERIAL PRIMARY KEY,
  session_id   BIGINT NOT NULL REFERENCES table_sessions(id),
  display_name TEXT NOT NULL,
  device_token TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'joined',  -- pending, joined, declined, left
  is_staff     BOOLEAN NOT NULL DEFAULT FALSE,
  last_seen    BIGINT NOT NULL
);
CREATE INDEX idx_guests_device ON session_guests(device_token);
CREATE INDEX idx_guests_session ON session_guests(session_id);

CREATE TABLE cart_items (
  id            BIGSERIAL PRIMARY KEY,
  session_id    BIGINT NOT NULL REFERENCES table_sessions(id),
  guest_id      BIGINT NOT NULL REFERENCES session_guests(id),
  menu_item_id  BIGINT NOT NULL REFERENCES menu_items(id),
  quantity      INTEGER NOT NULL,
  options_text  TEXT NOT NULL DEFAULT '',
  options_extra BIGINT NOT NULL DEFAULT 0,
  note          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_cart_session ON cart_items(session_id);

CREATE TABLE orders (
  id                 BIGSERIAL PRIMARY KEY,
  restaurant_id      BIGINT NOT NULL REFERENCES restaurants(id),
  session_id         BIGINT NOT NULL REFERENCES table_sessions(id),
  status             TEXT NOT NULL,             -- placed, cooking, ready, served, rejected, cancelled
  placed_by          TEXT NOT NULL DEFAULT '',
  is_addon           BOOLEAN NOT NULL DEFAULT FALSE,
  reject_reason      TEXT NOT NULL DEFAULT '',
  placed_at          BIGINT NOT NULL,
  estimated_ready_at BIGINT NOT NULL DEFAULT 0,
  ready_at           BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_orders_restaurant ON orders(restaurant_id, status);
CREATE INDEX idx_orders_session ON orders(session_id);

-- Name and price are copied at order time so old bills never change.
CREATE TABLE order_items (
  id            BIGSERIAL PRIMARY KEY,
  order_id      BIGINT NOT NULL REFERENCES orders(id),
  menu_item_id  BIGINT NOT NULL,
  name_bn       TEXT NOT NULL,
  name_en       TEXT NOT NULL,
  unit_price    BIGINT NOT NULL,
  quantity      INTEGER NOT NULL,
  options_text  TEXT NOT NULL DEFAULT '',
  note          TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'active', -- active, cancel_requested, cancelled
  req_qty       INTEGER NOT NULL DEFAULT 0,
  last_change   TEXT NOT NULL DEFAULT '',
  cancelled_by  TEXT NOT NULL DEFAULT '',
  cancel_reason TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_items_order ON order_items(order_id);

CREATE TABLE payments (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  session_id    BIGINT NOT NULL REFERENCES table_sessions(id),
  amount        BIGINT NOT NULL,
  method        TEXT NOT NULL,                  -- cash, bkash, nagad, card
  marked_by     TEXT NOT NULL,
  created_at    BIGINT NOT NULL
);
CREATE INDEX idx_payments_restaurant ON payments(restaurant_id, created_at);

CREATE TABLE reviews (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  session_id    BIGINT NOT NULL REFERENCES table_sessions(id),
  menu_item_id  BIGINT,                         -- NULL means the overall rating
  rating        INTEGER NOT NULL,
  comment       TEXT NOT NULL DEFAULT '',
  created_at    BIGINT NOT NULL
);
CREATE INDEX idx_reviews_restaurant ON reviews(restaurant_id);

CREATE TABLE waiter_calls (
  id            BIGSERIAL PRIMARY KEY,
  restaurant_id BIGINT NOT NULL REFERENCES restaurants(id),
  session_id    BIGINT NOT NULL REFERENCES table_sessions(id),
  reason        TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'pending',
  created_at    BIGINT NOT NULL
);
CREATE INDEX idx_calls_restaurant ON waiter_calls(restaurant_id, status);
