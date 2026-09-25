# SQL Doctor | Database Diagnostics & SQL Intelligence CLI

> A developer-first command-line toolkit to inspect, diagnose, optimize, and safely manage your SQL databases.

[![Go Report](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](https://golang.org)
[![Status](https://img.shields.io/badge/Status-Active%20Development-orange.svg)](#-under-active-development)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

### 🚧 Under Active Development

> **Note:** SQL Doctor is an early-stage project under active development.  
> I'm actively testing it across different database engines and query patterns, but edge cases and bugs are expected. If you hit a query that parses oddly, an execution plan that renders weirdly, or an unhandled engine difference, please [open an issue](https://github.com/sql-doctor/sql-doctor/issues) with the details so I can fix it!

---

## Why I Built SQL Doctor

As a software developer, I built SQL Doctor to solve the common database problems I run into every day:

- **Struggling to understand why a query is slow** — SQL Doctor analyzes query performance, visualizes execution plans, and identifies bottlenecks.
- **Checking whether two databases have the same structure** — SQL Doctor compares them side-by-side and clearly highlights missing or mismatched tables, columns, and indexes.
- **Dealing with poorly chosen data types or oversized columns** — SQL Doctor inspects your actual data records and recommends better, more compact definitions based on real statistics.
- **Worrying about what could break when applying a migration** — SQL Doctor checks for potential data loss, table locks, and compatibility issues beforehand.
- **Knowing something is wrong with a database but not knowing where** — SQL Doctor runs a full diagnostic across schema health, performance, indexes, data quality, and referential integrity.
- **Wanting an interactive terminal environment without typing `sql-doctor` before every command** — SQL Doctor includes an interactive REPL shell with engine selection, database switching, direct SQL execution, and clean box table rendering.

It gives you clear, deterministic answers straight in your terminal without needing heavy GUI clients or cloud dashboards. And if you want conversational assistance, you can optionally connect your own Gemini API key for query explanations and natural-language query generation.

---

## 🔒 Security & Privacy First

I built this tool with production safety in mind:

- **Read-only by default**: Database inspections and health checks run in read-only mode wherever the database engine supports it.
- **Passwords are never displayed**: Credentials are saved in your local user directory (`~/.sql-doctor/`) with restricted permissions and masked on screen.
- **Zero telemetry**: SQL Doctor does not call home, does not track your database, and has no tracking backend. It runs entirely on your machine.
- **User-owned AI keys**: I don't provide a shared API key or proxy your requests through any external server. You configure your own Gemini API key. Nothing touches an LLM unless you explicitly pass the `--ai` flag or run `sql-doctor ask`.
- **Never runs destructive AI queries automatically**: If you ask AI to write a query and it generates a `DELETE`, `UPDATE`, or `DROP`, SQL Doctor warns you in bold red text and requires interactive confirmation before anything touches your database.

---

## 📦 Installation & Setup

### Building from Source (All Platforms)

Make sure you have [Go](https://go.dev/dl/) installed (version 1.26 or modern Go with modules):

```bash
git clone https://github.com/sql-doctor/sql-doctor.git
cd sql-doctor
go build -o sql-doctor ./cmd/sql-doctor
```

---

### Running on Linux & macOS

Once compiled, move the binary somewhere in your `$PATH`:

```bash
# Move to local bin
sudo mv sql-doctor /usr/local/bin/

# Check installation
sql-doctor --help
```

---

### Running on Windows

You can build the `.exe` directly in PowerShell or Command Prompt:

```powershell
# In PowerShell:
go build -o sql-doctor.exe ./cmd/sql-doctor

# Test it:
.\sql-doctor.exe --help
```

To use it from anywhere on Windows, add the folder containing `sql-doctor.exe` to your User `PATH` environment variable, or move it to a directory already on your PATH (like `C:\Users\<YourUser>\go\bin`).

---

## 🚀 Everyday Usage & Examples

### 1. Interactive Shell REPL (`shell`)
If you prefer an interactive environment like the `mysql` or `psql` CLI where you can type queries and inspect schemas without prefixing every command with `sql-doctor`:

```bash
# Launch interactive shell with database engine picker
sql-doctor shell

# Or jump straight into a specific connection / database:
sql-doctor shell -c local-mysql -d <database_name>
```

Inside the shell, your prompt reflects your active engine and database:
```text
sql-doctor [mysql@<database_name>]> tables
sql-doctor [mysql@<database_name>]> select * from users;
sql-doctor [mysql@<database_name>]> select * from users\G   # Vertical format (one column per line)
sql-doctor [mysql@<database_name>]> use <other_database>    # Switch database on the fly
sql-doctor [mysql@<other_database>]> analyze SELECT * FROM orders WHERE status = 'pending'
sql-doctor [mysql@<other_database>]> help                   # View categorized commands
sql-doctor [mysql@<other_database>]> exit                   # Clean exit (discards in-memory session)
```

All session state (active connection and selected database) lives in memory during your shell session and is cleanly discarded upon exit.

---

### 2. Connecting to a Database
SQL Doctor works out of the box with **MySQL**, **MariaDB**, **PostgreSQL**, and **SQLite**.

Specifying a database name upfront is completely optional. If you connect to a server without picking a database, you can select one later:

```bash
# Connect to MySQL / MariaDB (specifying a database is optional)
sql-doctor connect --type mysql --host 127.0.0.1 --port 3306 --user root -p

# Connect to PostgreSQL
sql-doctor connect --type postgres --host localhost --port 5432 --user postgres -p

# Connect to SQLite (local file)
sql-doctor connect --type sqlite --file ./my-app.db --name my-local-db --save

# Or run directly against a database URL without saving:
sql-doctor --db-url "postgres://user:pass@localhost:5432/<database_name>" db tables
```

> **Tip:** You don't need `--save` just to try a connection. Running `connect` without `--save` starts an ephemeral session so you can immediately run subsequent commands in that terminal.

#### Managing Databases on the Server:
```bash
# List all databases on the connected server
sql-doctor databases

# Switch the active database
sql-doctor use <database_name>

# Clear session memory
sql-doctor disconnect
```

To see your saved connection profiles or switch between them:
```bash
# List saved connection profiles
sql-doctor connections

# Switch active profile
sql-doctor connections --use local-pg

# Quick connectivity test
sql-doctor ping
```

---

### 3. Full Health Diagnostic (`doctor`)
Run a quick diagnostic across your whole database. It checks for tables missing primary keys, unindexed foreign keys, redundant indexes, and sampled data anomalies:

```bash
sql-doctor doctor
```

Output gives you a composite health score, along with critical issues and remedies:
```text
Overall Database Health Score:
[███████████████░░░░░]  76 / 100  [NEEDS ATTENTION]
  • Schema Quality:       70 / 100
  • Data Cleanliness:     86 / 100

Critical Issues Detected:
  CRITICAL  [legacy_items] Missing Primary Key: Table does not have a defined Primary Key.
  CRITICAL  [orders.tracking_code] Duplicate Candidate Key Values

Warnings:
  WARNING   [orders] Unindexed Foreign Key: Column 'user_id' lacks a covering index.
```

---

### 4. Query Performance & EXPLAIN Analysis
Profile slow queries to see execution time, rows examined vs returned, and unindexed table scans:

```bash
# Profile a query and get a score
sql-doctor query analyze "SELECT * FROM users WHERE email LIKE '%@gmail.com'"

# Print an execution plan tree in plain English
sql-doctor query explain "SELECT u.name, o.amount FROM users u JOIN orders o ON u.id = o.user_id WHERE o.amount > 100"

# Get composite index recommendations (Equality -> Range -> Sort)
sql-doctor query optimize "SELECT * FROM users WHERE status = 'active' AND age > 21 ORDER BY created_at DESC"
```

---

### 5. Data-Aware Datatype Advisor
Don't guess what column type you should have used. SQL Doctor samples actual records and checks value lengths, patterns (like UUIDs, ISO dates, and booleans), and recommends tighter types:

```bash
sql-doctor schema datatypes users
```

Example recommendation:
```text
INFO  Column 'user_uuid'
  Current Type:   VARCHAR(255)
  Suggested Type: CHAR(36) or UUID (Confidence: 95%)
  Observed Stats: All values match standard UUID regex (fixed length 36).
  Rationale:      Storing in fixed CHAR(36) or native UUID reduces variable-length overhead.
```

---

### 6. Checking Foreign Keys & Orphan Rows
Find broken referential integrity before your app hits a foreign key error:

```bash
sql-doctor db relationships
```

This lists all foreign keys (plus inferred relationships like `user_id -> users.id`) and counts any orphan records pointing to missing parents.

---

### 7. Comparing Two Database Schemas (`diff`)
Need to verify if your staging database matches production?

```bash
sql-doctor db diff production staging
```

This compares tables, columns, data types, nullability, defaults, and indexes, and generates the exact `ALTER TABLE` and `CREATE TABLE` migration SQL to sync them.

---

### 8. Migration Safety Checks
Before running a migration script on production, check it for destructive commands or locking hazards:

```bash
sql-doctor migration check ./migrations/2024_add_user_field.sql
```

Catches issues like:
- `DROP TABLE` or `DROP COLUMN`
- `ALTER TABLE ... ADD COLUMN NOT NULL` without a `DEFAULT` (which rewrites the table or fails on populated tables)
- Incompatible type changes that trigger exclusive metadata locks

---

### 9. SQL Linter & Formatter
Quick static checks without needing a live connection:

```bash
# Lint for anti-patterns (SELECT *, implicit joins, cartesian products, non-sargable functions)
sql-doctor lint "SELECT * FROM users, orders WHERE users.id = orders.user_id"

# Format and pretty-print SQL
sql-doctor format "select id,name from users where status='active' and age>21 order by name asc"
```

---

### 10. Multi-Model AI Assistant (Gemini, OpenAI, Claude, Ollama)
If you want AI explanations or natural-language query generation, SQL Doctor supports **Google Gemini**, **OpenAI (ChatGPT)**, **Anthropic Claude**, and **Ollama / Local LLMs** (OpenAI-compatible):

```bash
# Open the interactive AI configuration dashboard
sql-doctor config ai

# Or switch provider and model directly:
sql-doctor config ai switch openai gpt-4o-mini
sql-doctor config ai switch claude claude-3-5-haiku-20241022
sql-doctor config ai switch gemini gemini-3.8-flash
sql-doctor config ai switch ollama deepseek-r1:8b

# Configure API keys (prompts with masked input if key omitted):
sql-doctor config ai set-key openai
sql-doctor config ai set-key claude
sql-doctor config ai set-key gemini

# Set custom endpoint for Ollama / local models:
sql-doctor config ai set-endpoint http://localhost:11434/v1

# Test connection and latency:
sql-doctor config ai test

# Ask questions grounded in your schema:
sql-doctor ask "Which tables store customer billing records?"

# Generate queries (with interactive execution prompt):
sql-doctor ask "Write a query to find the top 5 customers by revenue this year"
```

---

### 11. Machine-Readable Output (`--json`)
Every single command supports the `--json` flag. You can pipe the output into `jq` or plug it into your CI/CD pipelines:

```bash
sql-doctor doctor --json
sql-doctor query analyze "SELECT * FROM users" --json
sql-doctor migration check ./migration.sql --json
```

---

## 🤝 Want to Contribute?

I built SQL Doctor as an individual developer for other developers. Database engineering has endless quirks across versions, engines, and edge cases, and I'd love your help making it better!

Good places to jump in:
- **Found a bug?** Open an issue with your database version and query.
- **Want a new lint rule?** Adding rules in the AST linter is straightforward.
- **Want to add a database engine?** I'd love drivers for SQL Server, Oracle, or CockroachDB!
- **Feedback & ideas**: Let me know what feels rough or could be diagnosed better.

👉 **Check out the [Contributor Guide (DEVELOPMENT.md)](DEVELOPMENT.md)** for how the codebase is organized, how to add drivers, and how to run tests locally.

---

## 📜 License

SQL Doctor is open source software released under the [MIT License](LICENSE).
