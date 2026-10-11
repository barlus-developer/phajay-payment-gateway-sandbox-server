-- Initialize timestamps for newly created records as well as migrated records.
CREATE TRIGGER developers_created AFTER INSERT ON developers
BEGIN UPDATE developers SET updated_at=NEW.created_at WHERE id=NEW.id; END;
CREATE TRIGGER projects_created AFTER INSERT ON projects
BEGIN UPDATE projects SET updated_at=NEW.created_at WHERE id=NEW.id; END;
CREATE TRIGGER endpoints_created AFTER INSERT ON webhook_endpoints
BEGIN UPDATE webhook_endpoints SET updated_at=NEW.created_at WHERE id=NEW.id; END;
CREATE TRIGGER transactions_created AFTER INSERT ON transactions
BEGIN UPDATE transactions SET updated_at=NEW.created_at WHERE id=NEW.id; END;
