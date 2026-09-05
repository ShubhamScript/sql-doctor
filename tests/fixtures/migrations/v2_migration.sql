-- Sample Database Migration V2
-- Step 1: Add new column
ALTER TABLE users ADD COLUMN age_backup INT;

-- Step 2: Destructive table drop
DROP TABLE old_audit_logs;

-- Step 3: Destructive column drop
ALTER TABLE users DROP COLUMN bio;

-- Step 4: NOT NULL column added without default
ALTER TABLE orders ADD COLUMN priority VARCHAR(20) NOT NULL;

-- Step 5: Potential table rewrite
ALTER TABLE orders MODIFY COLUMN tracking_code INT;
