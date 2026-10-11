-- Add audit timestamps without modifying an applied initial schema.
ALTER TABLE developers ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE webhook_endpoints ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE transactions ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
UPDATE developers SET updated_at=created_at;
UPDATE projects SET updated_at=created_at;
UPDATE webhook_endpoints SET updated_at=created_at;
UPDATE transactions SET updated_at=created_at;
CREATE TRIGGER developers_updated AFTER UPDATE OF name,password_hash ON developers
BEGIN UPDATE developers SET updated_at=unixepoch() WHERE id=NEW.id; END;
CREATE TRIGGER projects_updated AFTER UPDATE OF name,merchant_name,description,payment_ttl,webhook_max_attempts ON projects
BEGIN UPDATE projects SET updated_at=unixepoch() WHERE id=NEW.id; END;
CREATE TRIGGER endpoints_updated AFTER UPDATE OF url,enabled,events ON webhook_endpoints
BEGIN UPDATE webhook_endpoints SET updated_at=unixepoch() WHERE id=NEW.id; END;
CREATE TRIGGER transactions_updated AFTER UPDATE OF status ON transactions
BEGIN UPDATE transactions SET updated_at=unixepoch() WHERE id=NEW.id; END;
