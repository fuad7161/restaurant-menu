-- Dish details shown in the guest's dish sheet, and tags the owner picks.
ALTER TABLE menu_items ADD COLUMN desc_bn  TEXT NOT NULL DEFAULT '';
ALTER TABLE menu_items ADD COLUMN desc_en  TEXT NOT NULL DEFAULT '';
ALTER TABLE menu_items ADD COLUMN tags     TEXT NOT NULL DEFAULT '';      -- comma list: chef, new, veg, spicy1, spicy2, spicy3
ALTER TABLE menu_items ADD COLUMN featured BOOLEAN NOT NULL DEFAULT FALSE; -- shown first in its category and in the top row

-- Option groups with rules: "Size: choose 1 (required)", "Extras: up to 3".
-- Options without a group are free extras: any number, none required.
CREATE TABLE option_groups (
  id           BIGSERIAL PRIMARY KEY,
  menu_item_id BIGINT NOT NULL REFERENCES menu_items(id),
  name         TEXT NOT NULL,
  min_choices  INTEGER NOT NULL DEFAULT 0, -- 1 or more = required
  max_choices  INTEGER NOT NULL DEFAULT 0  -- 0 = no limit, 1 = one choice only
);
CREATE INDEX idx_option_groups_item ON option_groups(menu_item_id);
ALTER TABLE item_options ADD COLUMN group_id BIGINT REFERENCES option_groups(id);

-- Opening hours shown to guests ("HH:MM", restaurant time; empty = not shown).
ALTER TABLE restaurants ADD COLUMN open_time  TEXT NOT NULL DEFAULT '';
ALTER TABLE restaurants ADD COLUMN close_time TEXT NOT NULL DEFAULT '';
ALTER TABLE restaurants ADD COLUMN last_order TEXT NOT NULL DEFAULT '';
-- Kitchen ticket colours: amber after warn_minutes, red after late_minutes of waiting.
ALTER TABLE restaurants ADD COLUMN warn_minutes INTEGER NOT NULL DEFAULT 10;
ALTER TABLE restaurants ADD COLUMN late_minutes INTEGER NOT NULL DEFAULT 20;

-- When staff handled a waiter call, so the guest sees it was answered.
ALTER TABLE waiter_calls ADD COLUMN done_at BIGINT NOT NULL DEFAULT 0;
-- When an order was served, so a mistaken "Mark served" can be undone for a short time.
ALTER TABLE orders ADD COLUMN served_at BIGINT NOT NULL DEFAULT 0;
