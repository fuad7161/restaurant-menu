-- Owner email: verification and "Forgot password" links (docs/plans/owner-verification-admin-roles.md, section 4).
ALTER TABLE staff_users ADD COLUMN email TEXT;
ALTER TABLE staff_users ADD COLUMN email_verified_at BIGINT NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX idx_staff_email ON staff_users (lower(email)) WHERE email IS NOT NULL;

-- Staff passwords the owner can look up on the Staff page, encrypted with the app's password key.
-- Empty for owners (never shown) and for passwords set before this was added.
ALTER TABLE staff_users ADD COLUMN password_enc TEXT NOT NULL DEFAULT '';

-- Per-restaurant email verification: 'global' follows the platform setting (EMAIL_VERIFY),
-- 'required' always, 'off' never. Changed by the platform admin.
ALTER TABLE restaurants ADD COLUMN verify_email TEXT NOT NULL DEFAULT 'global';

-- Single-use links sent by email. Only a hash of the token is stored.
CREATE TABLE email_tokens (
  token_hash TEXT PRIMARY KEY,
  staff_id   BIGINT NOT NULL REFERENCES staff_users(id) ON DELETE CASCADE,
  purpose    TEXT NOT NULL,            -- verify | reset_password
  email      TEXT NOT NULL,            -- the address the link was sent to
  created_at BIGINT NOT NULL,
  expires_at BIGINT NOT NULL,
  used_at    BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_email_tokens_staff ON email_tokens(staff_id, purpose, created_at);

-- Platform-wide values the app keeps for itself (for example the generated password key).
CREATE TABLE platform_settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
