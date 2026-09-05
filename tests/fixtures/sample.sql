-- Sample SQLite Test Schema for SQL Doctor
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    age INTEGER,
    is_verified VARCHAR(10) NOT NULL,
    user_uuid VARCHAR(255) NOT NULL,
    bio TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    amount DECIMAL(10,2) NOT NULL,
    tracking_code VARCHAR(50),
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Table without Primary Key to test schema smell analyzer
CREATE TABLE legacy_items (
    title VARCHAR(100),
    price REAL,
    category VARCHAR(50)
);

-- Insert Sample Data
INSERT INTO users (name, email, status, age, is_verified, user_uuid, bio) VALUES
('Alice Smith', 'alice@example.com', 'active', 28, 'true', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'DevOps engineer at TechCorp'),
('Bob Jones', 'bob@example.com', 'Active', 35, 'false', 'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22', 'Backend developer'),
('Charlie Brown', 'charlie@domain.org', 'ACTIVE', 42, 'true', 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a33', 'DBA and architect'),
('Diana Prince', 'diana@hero.net', 'inactive', 30, 'true', 'd0eebc99-9c0b-4ef8-bb6d-6bb9bd380a44', 'Data scientist'),
('Evan Wright', 'evan@domain.org', 'active', 24, 'false', 'e0eebc99-9c0b-4ef8-bb6d-6bb9bd380a55', 'Junior engineer');

INSERT INTO orders (user_id, amount, tracking_code, notes) VALUES
(1, 199.99, 'TRACK-100', 'Expedited delivery'),
(1, 49.50, 'TRACK-100', ''), -- Duplicate tracking code and empty string notes
(2, 850.00, 'TRACK-200', NULL),
(3, 120.00, 'TRACK-300', ''),
(999, 45.00, 'TRACK-400', 'Orphan order'); -- Orphan user_id 999!

INSERT INTO legacy_items (title, price, category) VALUES
('Vintage Lamp', 45.50, 'Home'),
('Desk Chair', 120.00, 'Office');
