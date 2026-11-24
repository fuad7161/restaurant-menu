-- Platform admin panel, first part (docs/plans/multi-branch.md step 7; the full panel is section 6
-- of owner-verification-admin-roles.md). Admins are not restaurant accounts: own table, own login.
CREATE TABLE admins (
  id            BIGSERIAL PRIMARY KEY,
  email         TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at    BIGINT NOT NULL
);
CREATE UNIQUE INDEX idx_admins_email ON admins (lower(email));

CREATE TABLE admin_sessions (
  token      TEXT PRIMARY KEY,           -- a hash of the cookie value
  admin_id   BIGINT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  expires_at BIGINT NOT NULL
);

-- The 6-digit code emailed after the password (2-step login). pending is a hash of the cookie that
-- ties the code to the browser that gave the password.
CREATE TABLE admin_codes (
  pending    TEXT PRIMARY KEY,
  admin_id   BIGINT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  code_hash  TEXT NOT NULL,
  tries      INTEGER NOT NULL DEFAULT 0,
  expires_at BIGINT NOT NULL
);

-- What each admin changed, with the value before and after.
CREATE TABLE admin_audit (
  id          BIGSERIAL PRIMARY KEY,
  admin_id    BIGINT NOT NULL REFERENCES admins(id),
  business_id BIGINT REFERENCES businesses(id),
  action      TEXT NOT NULL,
  detail      TEXT NOT NULL DEFAULT '',
  created_at  BIGINT NOT NULL
);
CREATE INDEX idx_admin_audit_business ON admin_audit(business_id, created_at);
