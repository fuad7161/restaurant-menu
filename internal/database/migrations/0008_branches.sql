-- Multiple branches (docs/plans/multi-branch.md, section 4).
-- A business groups its branches. A branch is a row in `restaurants`, as before, and every table
-- that has restaurant_id keeps it: it now means "branch".
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
ALTER TABLE restaurants ADD COLUMN branch_name TEXT NOT NULL DEFAULT '';        -- "Gulshan"; empty for a one-branch business
ALTER TABLE restaurants ADD COLUMN branch_code TEXT NOT NULL DEFAULT '';        -- "GUL", printed before invoice numbers
ALTER TABLE restaurants ADD COLUMN status      TEXT NOT NULL DEFAULT 'active';  -- active | closed | archived
ALTER TABLE restaurants ADD COLUMN sort_order  INTEGER NOT NULL DEFAULT 0;

-- Every existing restaurant becomes a business with one branch (same name, logo, colour, tagline).
-- A temporary column on businesses remembers which restaurant each one was made from.
ALTER TABLE businesses ADD COLUMN from_restaurant BIGINT;
INSERT INTO businesses(name, logo, brand_color, tagline, created_at, from_restaurant)
  SELECT name, logo, brand_color, tagline, created_at, id FROM restaurants ORDER BY id;
UPDATE restaurants r SET business_id = b.id FROM businesses b WHERE b.from_restaurant = r.id;
ALTER TABLE businesses DROP COLUMN from_restaurant;
ALTER TABLE restaurants ALTER COLUMN business_id SET NOT NULL;

CREATE INDEX idx_restaurants_business ON restaurants(business_id);

-- The session remembers the branch the person is working on. 0 = "All branches" (owner only).
ALTER TABLE staff_sessions ADD COLUMN branch_id BIGINT NOT NULL DEFAULT 0;

-- Activity log entries made in "All branches" mode (for example adding a branch) have no single
-- branch. They keep restaurant_id = the owner's home branch and get business_id for the combined log.
ALTER TABLE audit_log ADD COLUMN business_id BIGINT;
