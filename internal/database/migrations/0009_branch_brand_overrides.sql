-- A branch's logo, colour and tagline override its business's; empty means "use the business's"
-- (docs/plans/multi-branch.md, 8.3). 0008 copied each restaurant's branding to its new business,
-- so a value equal to the business's is not an override: clear it, and a later change to the
-- business's branding reaches the branch.
UPDATE restaurants r SET logo='' FROM businesses b WHERE b.id=r.business_id AND r.logo=b.logo;
UPDATE restaurants r SET brand_color='' FROM businesses b WHERE b.id=r.business_id AND r.brand_color=b.brand_color;
UPDATE restaurants r SET tagline='' FROM businesses b WHERE b.id=r.business_id AND r.tagline=b.tagline;
ALTER TABLE restaurants ALTER COLUMN brand_color SET DEFAULT '';
