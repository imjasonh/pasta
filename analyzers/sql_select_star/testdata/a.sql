SELECT * FROM users;        -- want "SELECT * is fragile"
SELECT id, name FROM users;
SELECT COUNT(*) FROM users; -- OK: COUNT(*) is an aggregate, not a projection
SELECT * FROM orders        -- want "SELECT * is fragile"
WHERE id = 1;
