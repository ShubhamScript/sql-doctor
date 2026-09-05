# Contributor Guide

Hey! Thanks for taking an interest in SQL Doctor. I built this tool to make inspecting, diagnosing, and optimizing SQL databases straightforward and fast directly from the terminal. 

Whether you want to squash an edge-case bug, add support for another database engine, or write a new lint rule, this guide walks you through the codebase so you can jump right in.

---

## 🧭 Project Architecture

SQL Doctor is written in Go. The core philosophy is simple: **deterministic database facts first, AI second**.

Before ever touching an LLM, the tool relies on real numbers: actual EXPLAIN plans, table statistics, sampled rows, and real AST parses. The AI integration is strictly an optional bonus for developers who want conversational answers or summaries.

Here is the repository layout:

```text
sql-doctor/
├── cmd/
│   └── sql-doctor/
│       └── main.go                  # CLI entrypoint
├── internal/
│   ├── database/                    # Database abstraction layer
│   │   ├── driver.go                # Driver interface & domain models
│   │   ├── factory.go               # Driver registry & URI parser
│   │   ├── helpers.go               # String & numeric check helpers
│   │   ├── mysql/                   # MySQL & MariaDB driver
│   │   ├── postgres/                # PostgreSQL driver (pgx/v5)
│   │   └── sqlite/                  # SQLite driver (pure-Go modernc)
│   ├── query/
│   │   ├── analyzer/                # Execution time, rows examined ratio, scoring
│   │   ├── explain/                 # Plan tree visualizer & plain-English summary
│   │   ├── optimizer/               # Equality-Range-Sort index advisor
│   │   └── parser/                  # Vitess AST parser, linter, formatter
│   ├── schema/
│   │   ├── advisor.go               # Data-aware column type advisor
│   │   ├── analyzer.go              # Schema smells (missing PKs, unindexed FKs)
│   │   ├── diff.go                  # 2-database schema diff & DDL generator
│   │   └── snapshot.go              # JSON schema snapshot storage
│   ├── data/
│   │   └── quality.go               # Duplicate keys, casing, null analysis
│   ├── migration/
│   │   └── analyzer.go              # Migration lock risk & destructive check
│   ├── ai/
│   │   ├── provider.go              # AIProvider interface
│   │   ├── gemini/                  # Google GenAI SDK client
│   │   └── context/                 # Schema context minifier & prompt builder
│   ├── storage/
│   │   └── sqlite.go                # Local SQLite state repo (~/.sql-doctor/)
│   ├── config/
│   │   └── config.go                # Env vars & configuration loader
│   ├── ui/                          # Lip Gloss styling, scorecards, tables
│   └── cli/                         # Cobra commands & flag handlers
├── tests/
│   ├── fixtures/                    # Test schemas and migration files
│   └── unit/                        # Unit tests
├── docker-compose.yml               # Local MySQL, Postgres, MariaDB containers
└── Makefile                         # Convenience build & test targets
```

---

## 🛠️ Local Setup

### Prerequisites
- **Go 1.26+** (or modern Go with modules)
- **Git**
- **Docker** (optional, for testing against live MySQL or Postgres containers)

### Build the Binary

```bash
git clone https://github.com/sql-doctor/sql-doctor.git
cd sql-doctor

# On Linux / macOS:
go build -o sql-doctor ./cmd/sql-doctor

# On Windows:
go build -o sql-doctor.exe ./cmd/sql-doctor
```

Verify it runs:
```bash
./sql-doctor --help
# Or on Windows:
.\sql-doctor.exe --help
```

### Running Test Databases
I've included a `docker-compose.yml` pre-configured with MySQL 8.0, PostgreSQL 16, and MariaDB 10.11:

```bash
docker compose up -d
```

Ports on localhost:
- **MySQL**: `localhost:3306` (user: `devuser`, pass: `devpassword`, db: `testdb`)
- **PostgreSQL**: `localhost:5432` (user: `postgres`, pass: `devpassword`, db: `testdb`)
- **MariaDB**: `localhost:3307` (user: `devuser`, pass: `devpassword`, db: `testdb`)

To shut them down:
```bash
docker compose down
```

---

## 🧪 Running Tests

Unit tests run fast in-memory using the SQLite driver and test fixtures:

```bash
go test -v ./...
```

To run a specific test:
```bash
go test -v ./tests/unit/...
```

Check formatting and static issues before committing:
```bash
go vet ./...
```

---

## 🔨 Adding New Features

### Adding a Database Driver
Every database engine implements the `Driver` interface in `internal/database/driver.go`:

```go
type Driver interface {
    Dialect() Dialect
    DSN(cfg *ConnectionConfig) string
    Connect(ctx context.Context, cfg *ConnectionConfig) (*sql.DB, error)
    Ping(ctx context.Context, db *sql.DB) error
    Version(ctx context.Context, db *sql.DB) (string, error)
    Tables(ctx context.Context, db *sql.DB) ([]TableInfo, error)
    DescribeTable(ctx context.Context, db *sql.DB, table string) (*TableDetail, error)
    Indexes(ctx context.Context, db *sql.DB, table string) ([]IndexInfo, error)
    ForeignKeys(ctx context.Context, db *sql.DB, table string) ([]ForeignKeyInfo, error)
    Relationships(ctx context.Context, db *sql.DB) ([]RelationshipInfo, error)
    Explain(ctx context.Context, db *sql.DB, query string, analyze bool) (*ExplainResult, error)
    TableStats(ctx context.Context, db *sql.DB, table string) (*TableStats, error)
    SampleColumnData(ctx context.Context, db *sql.DB, table, col string, limit int) (*ColumnSampleStats, error)
}
```

Steps to add an engine (e.g. SQL Server or Oracle):
1. Create `internal/database/<engine>/driver.go`.
2. Implement the interface methods using native `database/sql` queries against that database's system catalog.
3. Register the new driver in `internal/cli/root.go` inside `database.InitRegistry(...)`.
4. Add the URL scheme mapping in `internal/database/factory.go`.

### Adding a SQL Lint Rule
All lint rules live in `internal/query/parser/linter.go`.

1. Open `internal/query/parser/linter.go`.
2. Review existing rules (`L001` through `L007`).
3. Inspect `ParsedQuery` or pattern-match `sqlText` to detect the smell.
4. Append a `LintFinding`:
   - `RuleID` (e.g. `L008`)
   - `Severity` (`SeverityCritical`, `SeverityWarning`, or `SeverityInfo`)
   - `Title`, `Description`, and actionable `Suggestion`
5. Add a test case in `tests/unit/linter_test.go`.

### Adding a Data Quality Check
Inspect `internal/data/quality.go`. Inside `AnalyzeTable`, add new checks against sampled column data (e.g. detecting bad date formats, corrupted strings, or range violations) and append a `QualityAnomaly`.

---

## 📌 Coding Conventions

1. **Keep SQLite Pure Go**: I chose `modernc.org/sqlite` specifically so anyone on Windows or Linux can compile this project with `CGO_ENABLED=0` without having to mess with GCC or MinGW toolchains.
2. **Resource Cleanup**: Always close database rows and check for iteration errors:
   ```go
   rows, err := db.QueryContext(ctx, query)
   if err != nil {
       return nil, err
   }
   defer rows.Close()

   for rows.Next() {
       // scan fields
   }
   if err := rows.Err(); err != nil {
       return nil, err
   }
   ```
3. **No Leaked Credentials**: Never print database passwords or API keys to stdout, stderr, or log outputs. Mask sensitive strings before rendering.
4. **Machine-Readable Mode**: Every CLI command should handle `--json` cleanly by passing output through `OutputResult(data, textRenderFunc)`.

---

## 🚀 Submitting a Pull Request

1. Fork the repo and create your branch (`git checkout -b my-feature`).
2. Write clean code and add tests under `tests/unit/`.
3. Make sure tests pass:
   ```bash
   go test -v ./...
   go vet ./...
   ```
4. Push your branch and open a Pull Request.

Don't stress about making your PR 100% perfect on the first shot. I'm happy to review, test, and polish it with you!
