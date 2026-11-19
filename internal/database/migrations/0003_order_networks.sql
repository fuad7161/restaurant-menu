-- Networks guests may order from, one IP range (CIDR) per line. Empty = anywhere.
ALTER TABLE restaurants ADD COLUMN order_networks TEXT NOT NULL DEFAULT '';
