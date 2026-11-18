-- Restaurant profile: logo, brand colour and the details guests see.
ALTER TABLE restaurants ADD COLUMN logo        TEXT NOT NULL DEFAULT '';
ALTER TABLE restaurants ADD COLUMN brand_color TEXT NOT NULL DEFAULT '#8f1d21';
ALTER TABLE restaurants ADD COLUMN tagline     TEXT NOT NULL DEFAULT '';
ALTER TABLE restaurants ADD COLUMN address     TEXT NOT NULL DEFAULT '';
ALTER TABLE restaurants ADD COLUMN phone       TEXT NOT NULL DEFAULT '';
