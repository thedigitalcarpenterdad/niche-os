ALTER TABLE users ADD COLUMN telegram_id TEXT;
ALTER TABLE users ADD COLUMN telegram_username TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id) WHERE telegram_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS telegram_whitelist (
  telegram_id TEXT PRIMARY KEY,
  telegram_username TEXT,
  display_name TEXT,
  added_at TEXT NOT NULL DEFAULT (datetime('now')),
  added_by TEXT,
  role TEXT NOT NULL DEFAULT 'member',
  workspace TEXT NOT NULL DEFAULT 'internal',
  notes TEXT
);

INSERT OR IGNORE INTO telegram_whitelist (telegram_id, telegram_username, display_name, role, workspace, notes)
VALUES ('7481324256', 'tdc888888', 'Joshua Neuman', 'owner', 'internal', 'Owner');

-- Link known existing users to their Telegram IDs so login maps to existing accounts
UPDATE users SET telegram_id = '7481324256', telegram_username = 'tdc888888'
  WHERE id = 'usr_01kt88y1vvvd9sbq66wshcjz8n' AND telegram_id IS NULL;
