package cli

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	aiContext "github.com/sql-doctor/sql-doctor/internal/ai/context"
	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/query/analyzer"
	"github.com/sql-doctor/sql-doctor/internal/query/explain"
	"github.com/sql-doctor/sql-doctor/internal/query/optimizer"
	"github.com/sql-doctor/sql-doctor/internal/schema"
	"github.com/sql-doctor/sql-doctor/internal/storage"
	"github.com/sql-doctor/sql-doctor/internal/ui"
	"golang.org/x/term"
)

var (
	shellFlagConn     string
	shellFlagDatabase string
)

type ShellSession struct {
	DB     *sql.DB
	Driver database.Driver
	Config *database.ConnectionConfig
}

var shellCmd = &cobra.Command{
	Use:     "shell",
	Aliases: []string{"console", "repl"},
	Short:   "Start an interactive SQL Doctor shell / REPL session",
	Long: `Start an interactive shell where you can execute queries and database commands
directly without prefixing each command with 'sql-doctor'.

Session state (active database and memory) is kept in memory during the shell session
and is completely discarded when you type 'exit' or 'quit'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		if shellFlagConn != "" {
			flagConn = shellFlagConn
		}

		var cfg *database.ConnectionConfig

		// If user specified --conn or --db-url, connect directly
		if flagConn != "" || flagDBURL != "" {
			c, _, _, err := GetActiveDB(ctx)
			if err != nil {
				return err
			}
			c.Close()
			// Fetch config
			db, driver, resolvedCfg, err := GetActiveDB(ctx)
			if err != nil {
				return err
			}
			defer db.Close()
			return RunShell(ctx, db, driver, resolvedCfg)
		}

		// Otherwise, launch interactive Driver & Connection Wizard
		reader := bufio.NewReader(os.Stdin)
		var list []storage.ConnectionRecord
		var sess *storage.ConnectionRecord
		if appStorage != nil {
			list, _ = appStorage.ListConnections(ctx)
			sess, _ = appStorage.GetSessionConnection(ctx)
		}

		chosenCfg, err := promptDriverWizard(ctx, reader, list, sess)
		if err != nil {
			return err
		}
		cfg = chosenCfg

		db, driver, err := database.OpenConnection(ctx, cfg)
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		defer db.Close()

		return RunShell(ctx, db, driver, cfg)
	},
}

var disconnectCmd = &cobra.Command{
	Use:   "disconnect",
	Short: "Disconnect and clear the current active session memory",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if appStorage != nil {
			_ = appStorage.ClearSessionConnection(ctx)
		}
		fmt.Println(ui.Success("Active session disconnected. Memory cleared."))
		return nil
	},
}

func init() {
	shellCmd.Flags().StringVarP(&shellFlagConn, "conn", "c", "", "Connection profile to use for shell session")
	shellCmd.Flags().StringVarP(&shellFlagDatabase, "database", "d", "", "Database to select upon entering shell")

	RootCmd.AddCommand(shellCmd)
	RootCmd.AddCommand(disconnectCmd)
}

// RunShell starts the interactive loop
func RunShell(ctx context.Context, db *sql.DB, driver database.Driver, cfg *database.ConnectionConfig) error {
	shellCfg := *cfg
	if shellFlagDatabase != "" {
		shellCfg.Database = shellFlagDatabase
	}

	session := &ShellSession{
		DB:     db,
		Driver: driver,
		Config: &shellCfg,
	}

	ver, _ := session.Driver.Version(ctx, session.DB)
	primaryStyle := lipgloss.NewStyle().Bold(true).Foreground(ui.PrimaryColor)

	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("SQL Doctor Interactive Shell"))
	fmt.Printf("Connected to: %s (%s)\n", primaryStyle.Render(session.Config.Name), ver)
	if session.Config.Database != "" {
		fmt.Printf("Active Database: %s\n", primaryStyle.Render(session.Config.Database))
	} else if session.Config.Dialect != database.DialectSQLite {
		fmt.Println(ui.Info("No database selected yet. Type 'use <dbname>' or 'databases' to choose one."))
	}
	fmt.Println("Type " + primaryStyle.Render("help") + " for commands, or " + primaryStyle.Render("exit") + " to quit (session memory is discarded on exit).\n")

	reader := bufio.NewReader(os.Stdin)

	for {
		prompt := renderPrompt(session.Config)
		fmt.Print(prompt)

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("\nExiting shell session...")
			break
		}

		line := strings.TrimSpace(input)
		if line == "" {
			continue
		}

		if line == "exit" || line == "quit" || line == "\\q" {
			fmt.Println(ui.Info("Disconnected. Shell session memory cleared. Bye!"))
			break
		}

		if line == "clear" || line == "cls" {
			fmt.Print("\033[H\033[2J")
			continue
		}

		lowerTrimmed := strings.TrimSuffix(strings.ToLower(line), ";")
		fields := strings.Fields(lowerTrimmed)
		if len(fields) > 0 && (fields[0] == "help" || fields[0] == "?" || fields[0] == "\\h" || fields[0] == "\\?") {
			printShellHelp(fields[1:]...)
			continue
		}

		if err := handleShellCommand(ctx, session, line); err != nil {
			fmt.Println(ui.Error("%v", err))
		}
		fmt.Println()
	}

	return nil
}

func renderPrompt(cfg *database.ConnectionConfig) string {
	engineStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6366F1"))
	dbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	noneStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))

	dbPart := noneStyle.Render("(none)")
	if cfg.Database != "" {
		dbPart = dbStyle.Render(cfg.Database)
	}

	return fmt.Sprintf("%s [%s@%s]> ",
		engineStyle.Render("sql-doctor"),
		cfg.Dialect,
		dbPart,
	)
}

func printShellHelp(args ...string) {
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	cmdStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#10B981"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	exampleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8"))
	syntaxStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EC4899"))

	if len(args) > 0 {
		topic := strings.ToLower(args[0])
		fmt.Println()
		switch topic {
		case "use":
			fmt.Println(sectionStyle.Render("COMMAND: use"))
			fmt.Println(descStyle.Render("Switch active database for the current connection (in-memory session only)."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  use <database-name>")
			fmt.Println(exampleStyle.Render("Example:") + " use rolerift")

		case "databases", "dbs":
			fmt.Println(sectionStyle.Render("COMMAND: databases (or dbs)"))
			fmt.Println(descStyle.Render("List all available databases on the connected database server."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  databases")

		case "connections", "profiles":
			fmt.Println(sectionStyle.Render("COMMAND: connections (or profiles)"))
			fmt.Println(descStyle.Render("List all saved connection profiles with dialect, host, and database."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  connections")

		case "connect":
			fmt.Println(sectionStyle.Render("COMMAND: connect"))
			fmt.Println(descStyle.Render("Switch active connection to a different saved connection profile without restarting."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  connect <profile-name>")
			fmt.Println(exampleStyle.Render("Example:") + " connect prod-db")

		case "tables":
			fmt.Println(sectionStyle.Render("COMMAND: tables"))
			fmt.Println(descStyle.Render("List all tables in the active database with type, row counts, and storage engine."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  tables")

		case "describe", "desc":
			fmt.Println(sectionStyle.Render("COMMAND: describe (or desc)"))
			fmt.Println(descStyle.Render("Inspect table schema, column types, nullability, default values, and keys."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  describe <table-name>")
			fmt.Println(exampleStyle.Render("Example:") + " describe users")

		case "indexes":
			fmt.Println(sectionStyle.Render("COMMAND: indexes"))
			fmt.Println(descStyle.Render("List indexes on a table, including column order, uniqueness, and primary status."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  indexes <table-name>")
			fmt.Println(exampleStyle.Render("Example:") + " indexes users")

		case "relationships":
			fmt.Println(sectionStyle.Render("COMMAND: relationships"))
			fmt.Println(descStyle.Render("Map explicit foreign keys and detect naming-based inferred relationships."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  relationships")

		case "analyze":
			fmt.Println(sectionStyle.Render("COMMAND: analyze"))
			fmt.Println(descStyle.Render("Run deep performance analysis on a query (measures time, row scans, full table scans, performance score)."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  analyze <sql-query>")
			fmt.Println(exampleStyle.Render("Example:") + " analyze SELECT * FROM users WHERE email = 'test@example.com'")

		case "explain":
			fmt.Println(sectionStyle.Render("COMMAND: explain"))
			fmt.Println(descStyle.Render("Visualize the hierarchical execution plan tree for a SQL query."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  explain <sql-query>")
			fmt.Println(exampleStyle.Render("Example:") + " explain SELECT * FROM users JOIN roles ON users.role_id = roles.id")

		case "optimize":
			fmt.Println(sectionStyle.Render("COMMAND: optimize"))
			fmt.Println(descStyle.Render("Analyze query plan and recommend composite indexes or SQL query rewrites."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  optimize <sql-query>")
			fmt.Println(exampleStyle.Render("Example:") + " optimize SELECT * FROM orders WHERE status = 'paid' AND created_at > '2026-01-01'")

		case "doctor":
			fmt.Println(sectionStyle.Render("COMMAND: doctor"))
			fmt.Println(descStyle.Render("Run full automated health diagnostics across the active database."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  doctor")

		case "ask":
			fmt.Println(sectionStyle.Render("COMMAND: ask"))
			fmt.Println(descStyle.Render("Use Gemini AI to answer questions or generate SQL based on schema."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  ask <natural language prompt>")
			fmt.Println(exampleStyle.Render("Example:") + " ask find the 5 newest registered users")

		case "sql", "query", "select":
			fmt.Println(sectionStyle.Render("DIRECT SQL EXECUTION"))
			fmt.Println(descStyle.Render("Execute any SQL query directly against the active database."))
			fmt.Println()
			fmt.Println(syntaxStyle.Render("Syntax:") + "  <SELECT | SHOW | INSERT | UPDATE | DELETE | CREATE | ALTER | DROP ...>")
			fmt.Println(exampleStyle.Render("Example:") + " SELECT id, name, email FROM users LIMIT 10")
			fmt.Println(exampleStyle.Render("Vertical:") + " SELECT * FROM users\\G  (one column per line)")

		default:
			fmt.Println(ui.Warning("No specific help topic for '%s'. Type 'help' for full command list.", topic))
		}
		fmt.Println()
		return
	}

	fmt.Println()
	fmt.Println(styleLogo.Render(AsciiLogo))
	fmt.Println()
	badge := styleBadge.Render("SQL DOCTOR")
	ver := styleVer.Render("SHELL")
	subtitle := styleGray.Render("— Interactive Query Console & Diagnostic REPL")
	fmt.Printf("  %s  %s  %s\n\n", badge, ver, subtitle)

	printCategory := func(name string, items [][2]string) {
		fmt.Println(sectionStyle.Render(name))
		for _, item := range items {
			cmd := item[0]
			desc := item[1]
			fmt.Printf("  %-30s %s\n", cmdStyle.Render(cmd), descStyle.Render(desc))
		}
		fmt.Println()
	}

	printCategory("DATABASE & CONNECTION", [][2]string{
		{"use <database>", "Switch active database for current connection"},
		{"databases (or dbs)", "List all databases available on server"},
		{"connections (or profiles)", "List all saved connection profiles"},
		{"connect <profile>", "Switch active connection to a saved profile"},
	})

	printCategory("SCHEMA INSPECTION", [][2]string{
		{"tables", "List all tables in active database with row counts & engines"},
		{"describe (or desc) <table>", "Inspect table columns, types, nullability, keys, and defaults"},
		{"indexes <table>", "List table indexes, uniqueness, and composite column orders"},
		{"relationships", "Inspect detected foreign keys and inferred relationships"},
	})

	printCategory("PERFORMANCE & DIAGNOSTICS", [][2]string{
		{"analyze <sql>", "Run deep performance audit, rows scanned, and smell detection"},
		{"explain <sql>", "Visualize hierarchical query execution plan tree"},
		{"optimize <sql>", "Recommend missing indexes and query rewrite improvements"},
		{"doctor", "Run full end-to-end diagnostic health check on active database"},
		{"ask <prompt>", "Generate SQL or ask schema questions using Gemini AI"},
	})

	printCategory("DIRECT SQL & UTILITIES", [][2]string{
		{"<any SQL query>", "Run standard SQL directly (SELECT, SHOW, INSERT, UPDATE, etc.)"},
		{"<sql query>\\G", "Display query results in vertical format (one column per line)"},
		{"clear (or cls)", "Clear terminal screen"},
		{"help [command]", "Show full reference or detailed help for a specific command"},
		{"exit (or quit / \\q)", "Disconnect and leave shell session"},
	})

	fmt.Println(sectionStyle.Render("EXAMPLES"))
	fmt.Printf("  %s\n", exampleStyle.Render("sql-doctor> use rolerift"))
	fmt.Printf("  %s\n", exampleStyle.Render("sql-doctor> tables"))
	fmt.Printf("  %s\n", exampleStyle.Render("sql-doctor> select * from users\\G"))
	fmt.Printf("  %s\n", exampleStyle.Render("sql-doctor> analyze SELECT * FROM users WHERE email = 'test@example.com'"))
	fmt.Printf("  %s\n", exampleStyle.Render("sql-doctor> help analyze"))
	fmt.Println()
}

func handleShellCommand(ctx context.Context, session *ShellSession, line string) error {
	trimmed := strings.TrimSuffix(line, ";")
	lower := strings.ToLower(trimmed)
	parts := strings.Fields(trimmed)
	cmdName := strings.ToLower(parts[0])

	switch cmdName {
	case "connections", "profiles":
		if appStorage == nil {
			return fmt.Errorf("local storage unavailable")
		}
		list, err := appStorage.ListConnections(ctx)
		if err != nil {
			return err
		}
		tbl := ui.NewTable("CURRENT", "NAME", "DIALECT", "HOST/FILE", "DATABASE", "USER")
		for _, c := range list {
			marker := ""
			if c.Name == session.Config.Name {
				marker = "  ★  "
			}
			hostOrFile := c.Host
			if c.FilePath != "" {
				hostOrFile = c.FilePath
			}
			dbName := c.Database
			if dbName == "" {
				dbName = "(none)"
			}
			tbl.AddRow(marker, c.Name, string(c.Dialect), hostOrFile, dbName, c.User)
		}
		fmt.Println(tbl.Render())
		fmt.Println(ui.Info("Tip: Switch connection using 'connect <profile-name>'"))
		return nil

	case "connect":
		if len(parts) < 2 {
			return fmt.Errorf("usage: connect <profile-name> (type 'connections' to view available profiles)")
		}
		targetName := parts[1]
		if appStorage == nil {
			return fmt.Errorf("local storage unavailable")
		}
		rec, err := appStorage.GetConnection(ctx, targetName)
		if err != nil {
			return fmt.Errorf("connection profile '%s' not found: %w", targetName, err)
		}
		newCfg := &database.ConnectionConfig{
			Name:     rec.Name,
			Dialect:  rec.Dialect,
			Host:     rec.Host,
			Port:     rec.Port,
			User:     rec.User,
			Password: rec.Password,
			Database: rec.Database,
			FilePath: rec.FilePath,
			SSLMode:  rec.SSLMode,
		}
		newDB, newDriver, err := database.OpenConnection(ctx, newCfg)
		if err != nil {
			return fmt.Errorf("failed to connect to '%s': %w", targetName, err)
		}
		ver, _ := newDriver.Version(ctx, newDB)
		session.DB.Close()
		session.DB = newDB
		session.Driver = newDriver
		session.Config = newCfg
		fmt.Println(ui.Success("Switched connection to '%s' (%s)", targetName, ver))
		if session.Config.Database != "" {
			fmt.Println(ui.Info("Active database: %s", session.Config.Database))
		}
		return nil

	case "use":
		if len(parts) < 2 {
			return fmt.Errorf("usage: use <database-name>")
		}
		targetDB := parts[1]
		if session.Config.Dialect == database.DialectSQLite {
			return fmt.Errorf("SQLite uses single-file database. Use of multiple databases is not supported.")
		}
		dbs, err := session.Driver.Databases(ctx, session.DB)
		if err == nil && len(dbs) > 0 {
			found := false
			for _, d := range dbs {
				if strings.EqualFold(d, targetDB) {
					targetDB = d
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("database '%s' not found on server", targetDB)
			}
		}
		session.Config.Database = targetDB
		if session.Driver.Dialect() == database.DialectMySQL || session.Driver.Dialect() == database.DialectMariaDB {
			_, _ = session.DB.ExecContext(ctx, fmt.Sprintf("USE `%s`", targetDB))
		}
		fmt.Println(ui.Success("Database switched to '%s'", targetDB))
		return nil

	case "databases", "dbs":
		dbs, err := session.Driver.Databases(ctx, session.DB)
		if err != nil {
			return err
		}
		tbl := ui.NewTable("ACTIVE", "DATABASE NAME")
		for _, d := range dbs {
			activeMarker := ""
			if session.Config.Database == d {
				activeMarker = "  ★  "
			}
			tbl.AddRow(activeMarker, d)
		}
		fmt.Println(tbl.Render())
		return nil

	case "tables":
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		tables, err := session.Driver.Tables(ctx, session.DB)
		if err != nil {
			return err
		}
		tbl := ui.NewTable("TABLE NAME", "TYPE", "EST. ROWS", "ENGINE")
		for _, t := range tables {
			tbl.AddRow(t.Name, t.Type, fmt.Sprintf("%d", t.RowCount), t.Engine)
		}
		fmt.Println(tbl.Render())
		return nil

	case "describe", "desc":
		if len(parts) < 2 {
			return fmt.Errorf("usage: describe <table-name>")
		}
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		detail, err := session.Driver.DescribeTable(ctx, session.DB, parts[1])
		if err != nil {
			return err
		}
		tbl := ui.NewTable("#", "COLUMN", "TYPE", "NULLABLE", "DEFAULT", "KEY")
		for _, c := range detail.Columns {
			keyStr := ""
			if c.IsPrimaryKey {
				keyStr = "PRI"
			} else if c.IsForeignKey {
				keyStr = "FK"
			} else if c.IsUnique {
				keyStr = "UNI"
			}
			nullStr := "NO"
			if c.IsNullable {
				nullStr = "YES"
			}
			dfltStr := "NULL"
			if c.DefaultValue != nil {
				dfltStr = *c.DefaultValue
			}
			tbl.AddRow(fmt.Sprintf("%d", c.Position), c.Name, c.RawType, nullStr, dfltStr, keyStr)
		}
		fmt.Println(tbl.Render())
		return nil

	case "indexes":
		if len(parts) < 2 {
			return fmt.Errorf("usage: indexes <table-name>")
		}
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		indexes, err := session.Driver.Indexes(ctx, session.DB, parts[1])
		if err != nil {
			return err
		}
		tbl := ui.NewTable("INDEX NAME", "COLUMNS", "UNIQUE", "PRIMARY", "TYPE")
		for _, idx := range indexes {
			tbl.AddRow(idx.Name, strings.Join(idx.Columns, ", "), fmt.Sprintf("%v", idx.IsUnique), fmt.Sprintf("%v", idx.IsPrimary), idx.Type)
		}
		fmt.Println(tbl.Render())
		return nil

	case "relationships":
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		rels, err := session.Driver.Relationships(ctx, session.DB)
		if err != nil {
			return err
		}
		if len(rels) == 0 {
			fmt.Println(ui.Info("No relationships or foreign keys detected."))
			return nil
		}
		tbl := ui.NewTable("SOURCE TABLE", "COLUMN", "TARGET TABLE", "TARGET COL", "TYPE", "ORPHANS")
		for _, r := range rels {
			relType := "Explicit FK"
			if !r.IsExplicit {
				relType = "Inferred (Naming)"
			}
			orphanStr := fmt.Sprintf("%d", r.OrphanCount)
			if r.OrphanCount > 0 {
				orphanStr = ui.WarningBadge + fmt.Sprintf(" %d", r.OrphanCount)
			}
			tbl.AddRow(r.FromTable, strings.Join(r.FromColumns, ","), r.ToTable, strings.Join(r.ToColumns, ","), relType, orphanStr)
		}
		fmt.Println(tbl.Render())
		return nil

	case "analyze":
		if len(parts) < 2 {
			return fmt.Errorf("usage: analyze <sql-query>")
		}
		queryText := strings.TrimPrefix(trimmed, parts[0]+" ")
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		anz := analyzer.NewAnalyzer(session.Driver)
		result, err := anz.Analyze(ctx, session.DB, queryText)
		if err != nil {
			return err
		}
		fmt.Println("Performance Score: " + ui.RenderScoreMeter(result.PerformanceScore))
		tbl := ui.NewTable("METRIC", "VALUE")
		tbl.AddRow("Execution Time", fmt.Sprintf("%.2f ms", result.ExecutionTimeMs))
		tbl.AddRow("Rows Returned", fmt.Sprintf("%d", result.RowsReturned))
		tbl.AddRow("Rows Examined", fmt.Sprintf("%d", result.RowsExamined))
		tbl.AddRow("Full Table Scan", fmt.Sprintf("%v", result.HasFullTableScan))
		fmt.Println(tbl.Render())
		return nil

	case "explain":
		if len(parts) < 2 {
			return fmt.Errorf("usage: explain <sql-query>")
		}
		queryText := strings.TrimPrefix(trimmed, parts[0]+" ")
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		explainRes, err := session.Driver.Explain(ctx, session.DB, queryText, false)
		if err != nil {
			return err
		}
		planAnalyzer := explain.NewPlanAnalyzer()
		summary := planAnalyzer.Analyze(explainRes)
		if summary.FormattedTree != "" {
			fmt.Println(ui.HeaderStyle.Render("Execution Plan Tree:"))
			fmt.Println(summary.FormattedTree)
		}
		fmt.Println(summary.HumanSummary)
		return nil

	case "optimize":
		if len(parts) < 2 {
			return fmt.Errorf("usage: optimize <sql-query>")
		}
		queryText := strings.TrimPrefix(trimmed, parts[0]+" ")
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		opt := optimizer.NewOptimizer(session.Driver)
		res, err := opt.Optimize(ctx, session.DB, queryText)
		if err != nil {
			return err
		}
		if len(res.IndexRecommendations) > 0 {
			for _, idx := range res.IndexRecommendations {
				fmt.Printf("%s Target: %s\n  DDL:  %s\n  Gain: %s\n", ui.SuccessBadge, idx.TableName, idx.SuggestedDDL, idx.EstimatedGain)
			}
		} else {
			fmt.Println(ui.Success("No new indexes required for this query."))
		}
		return nil

	case "doctor":
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		fmt.Println(ui.Info("Running health diagnostics on [%s]...", session.Config.Database))
		tables, err := session.Driver.Tables(ctx, session.DB)
		if err != nil {
			return err
		}
		fmt.Println(ui.Success("Database '%s' is responsive. Total tables: %d", session.Config.Database, len(tables)))
		return nil

	case "ask":
		if len(parts) < 2 {
			return fmt.Errorf("usage: ask <natural language prompt or query request>")
		}
		question := strings.TrimPrefix(trimmed, parts[0]+" ")
		if err := EnsureShellDatabase(ctx, session.DB, session.Driver, session.Config); err != nil {
			return err
		}
		p := GetAIProvider()
		if !p.IsConfigured() {
			fmt.Println(ui.Warning("AI features are unavailable because %s is not configured.", p.ProviderName()))
			fmt.Println("\nConfigure your AI provider outside the shell using:")
			fmt.Println("  sql-doctor config ai")
			return nil
		}
		fmt.Println(ui.Info("Thinking with %s (%s)...", p.ProviderName(), p.Model()))
		tables, _ := session.Driver.Tables(ctx, session.DB)
		details, _ := schema.FetchAllTableDetails(ctx, session.Driver, session.DB)
		schemaContext := aiContext.BuildMinifiedSchema(tables, details)

		lowerQ := strings.ToLower(question)
		isQueryGen := strings.HasPrefix(lowerQ, "write") || strings.HasPrefix(lowerQ, "generate") ||
			strings.HasPrefix(lowerQ, "find") || strings.HasPrefix(lowerQ, "select") ||
			strings.HasPrefix(lowerQ, "get") || strings.Contains(lowerQ, "query to")

		if isQueryGen {
			genSQL, err := p.GenerateSQL(ctx, question, schemaContext)
			if err != nil {
				return err
			}
			fmt.Println()
			fmt.Println(ui.HeaderStyle.Render("Generated SQL Query:"))
			fmt.Println(ui.CardStyle.Render(genSQL.SQL))
			if genSQL.Explanation != "" {
				fmt.Printf("Explanation: %s\n", genSQL.Explanation)
			}
			if genSQL.IsDestructive {
				fmt.Println(ui.CriticalBadge + " " + ui.Error("Warning: This query modifies or deletes data!"))
			}
			fmt.Println()
			fmt.Println(ui.Info("Tip: Copy and paste the query above to execute it."))
			return nil
		}

		ans, err := p.Ask(ctx, question, schemaContext)
		if err != nil {
			return err
		}
		fmt.Println()
		fmt.Println(ans)
		return nil

	case "help", "?":
		printShellHelp(parts[1:]...)
		return nil

	default:
		if isSQLStatement(lower) {
			return executeDirectSQL(ctx, session.DB, session.Driver, session.Config, trimmed)
		}
		return fmt.Errorf("unknown command '%s'. Type 'help' for available commands.", parts[0])
	}
}

func isSQLStatement(lower string) bool {
	prefixes := []string{
		"select", "insert", "update", "delete", "create", "alter", "drop",
		"show", "describe", "explain", "set", "call", "truncate", "begin", "commit", "rollback",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func executeDirectSQL(ctx context.Context, db *sql.DB, driver database.Driver, cfg *database.ConnectionConfig, sqlQuery string) error {
	trimmedQuery := strings.TrimSpace(sqlQuery)
	isVertical := strings.HasSuffix(trimmedQuery, "\\G") || strings.HasSuffix(trimmedQuery, "\\g")
	if isVertical {
		trimmedQuery = strings.TrimSpace(trimmedQuery[:len(trimmedQuery)-2])
	}

	upper := strings.ToUpper(trimmedQuery)

	if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "EXPLAIN") || strings.HasPrefix(upper, "DESCRIBE") || strings.HasPrefix(upper, "DESC ") {

		start := time.Now()
		rows, err := db.QueryContext(ctx, trimmedQuery)
		if err != nil {
			return err
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return err
		}

		tbl := ui.NewTable(cols...)
		var allRows [][]string
		rowCount := 0

		for rows.Next() {
			rowCount++
			vals := make([]interface{}, len(cols))
			valPtrs := make([]interface{}, len(cols))
			for i := range vals {
				valPtrs[i] = &vals[i]
			}

			if err := rows.Scan(valPtrs...); err != nil {
				return err
			}

			rowStrs := make([]string, len(cols))
			for i, v := range vals {
				if v == nil {
					rowStrs[i] = "NULL"
				} else if t, ok := v.(time.Time); ok {
					rowStrs[i] = t.Format("2006-01-02 15:04:05")
				} else if b, ok := v.([]byte); ok {
					rowStrs[i] = string(b)
				} else {
					rowStrs[i] = fmt.Sprintf("%v", v)
				}
			}
			if isVertical {
				allRows = append(allRows, rowStrs)
			} else {
				tbl.AddRow(rowStrs...)
			}
		}

		duration := time.Since(start)
		if rowCount == 0 {
			fmt.Println(ui.Info("Empty set (%.2f ms)", float64(duration.Microseconds())/1000.0))
			return nil
		}

		if isVertical {
			maxColLen := 0
			for _, col := range cols {
				if len(col) > maxColLen {
					maxColLen = len(col)
				}
			}
			for rIdx, r := range allRows {
				fmt.Printf("*************************** %d. row ***************************\n", rIdx+1)
				for cIdx, val := range r {
					fmt.Printf("%*s: %s\n", maxColLen, cols[cIdx], val)
				}
			}
		} else {
			fmt.Println(tbl.Render())
			if tbl.WasTruncated() {
				fmt.Println(ui.Info("Tip: Wide table fitted to terminal width. Use '\\G' (e.g. %s\\G) for full vertical view.", trimmedQuery))
			}
		}

		fmt.Println(ui.Info("%d rows in set (%.2f ms)", rowCount, float64(duration.Microseconds())/1000.0))
		return nil
	}

	start := time.Now()
	res, err := db.ExecContext(ctx, trimmedQuery)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	duration := time.Since(start)
	fmt.Println(ui.Success("Query OK, %d rows affected (%.2f ms)", affected, float64(duration.Microseconds())/1000.0))
	return nil
}

func promptDriverWizard(ctx context.Context, reader *bufio.Reader, savedProfiles []storage.ConnectionRecord, activeSess *storage.ConnectionRecord) (*database.ConnectionConfig, error) {
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("Select Database Driver / Engine:"))
	fmt.Println("  [1] MySQL")
	fmt.Println("  [2] PostgreSQL")
	fmt.Println("  [3] SQLite")
	fmt.Println("  [4] MariaDB")

	choiceMap := make(map[int]interface{})
	choiceMap[1] = database.DialectMySQL
	choiceMap[2] = database.DialectPostgreSQL
	choiceMap[3] = database.DialectSQLite
	choiceMap[4] = database.DialectMariaDB

	nextIdx := 5
	sessIdx := 0
	if activeSess != nil {
		sessIdx = nextIdx
		choiceMap[sessIdx] = activeSess
		sessTarget := activeSess.Host
		if activeSess.FilePath != "" {
			sessTarget = activeSess.FilePath
		}
		dbInfo := ""
		if activeSess.Database != "" {
			dbInfo = " | db: " + activeSess.Database
		}
		fmt.Printf("  [%d] Active Session (%s: %s%s)\n", sessIdx, activeSess.Dialect, sessTarget, dbInfo)
		nextIdx++
	}

	savedStartIdx := 0
	if len(savedProfiles) > 0 {
		savedStartIdx = nextIdx
		fmt.Printf("  [%d] Use Saved Profile (%d available)\n", savedStartIdx, len(savedProfiles))
		nextIdx++
	}

	maxChoice := nextIdx - 1
	defaultChoice := 1
	if sessIdx != 0 {
		defaultChoice = sessIdx
	}

	fmt.Printf("\nEnter choice [1-%d] (default %d): ", maxChoice, defaultChoice)
	choiceInput, _ := reader.ReadString('\n')
	choiceInput = strings.TrimSpace(choiceInput)

	chosenNum := defaultChoice
	if choiceInput != "" {
		if n, err := strconv.Atoi(choiceInput); err == nil && n >= 1 && n <= maxChoice {
			chosenNum = n
		}
	}

	switch chosenNum {
	case 1: // MySQL
		host := promptDefault(reader, "Host", "127.0.0.1")
		portStr := promptDefault(reader, "Port", "3306")
		port, _ := strconv.Atoi(portStr)
		user := promptDefault(reader, "User", "root")
		fmt.Print("Password (leave empty if none): ")
		password := readPasswordInput(reader)
		dbName := promptDefault(reader, "Database name (optional, press Enter to skip)", "")
		return &database.ConnectionConfig{
			Name:     "mysql",
			Dialect:  database.DialectMySQL,
			Host:     host,
			Port:     port,
			User:     user,
			Password: password,
			Database: dbName,
		}, nil

	case 2: // PostgreSQL
		host := promptDefault(reader, "Host", "127.0.0.1")
		portStr := promptDefault(reader, "Port", "5432")
		port, _ := strconv.Atoi(portStr)
		user := promptDefault(reader, "User", "postgres")
		fmt.Print("Password (leave empty if none): ")
		password := readPasswordInput(reader)
		dbName := promptDefault(reader, "Database name (optional, press Enter to skip)", "")
		return &database.ConnectionConfig{
			Name:     "postgres",
			Dialect:  database.DialectPostgreSQL,
			Host:     host,
			Port:     port,
			User:     user,
			Password: password,
			Database: dbName,
		}, nil

	case 3: // SQLite
		path := promptDefault(reader, "Database file path", "./dev.db")
		return &database.ConnectionConfig{
			Name:     "sqlite",
			Dialect:  database.DialectSQLite,
			FilePath: path,
			Database: path,
		}, nil

	case 4: // MariaDB
		host := promptDefault(reader, "Host", "127.0.0.1")
		portStr := promptDefault(reader, "Port", "3306")
		port, _ := strconv.Atoi(portStr)
		user := promptDefault(reader, "User", "root")
		fmt.Print("Password (leave empty if none): ")
		password := readPasswordInput(reader)
		dbName := promptDefault(reader, "Database name (optional, press Enter to skip)", "")
		return &database.ConnectionConfig{
			Name:     "mariadb",
			Dialect:  database.DialectMariaDB,
			Host:     host,
			Port:     port,
			User:     user,
			Password: password,
			Database: dbName,
		}, nil

	default:
		if chosenNum == sessIdx && activeSess != nil {
			return &database.ConnectionConfig{
				Name:     activeSess.Name,
				Dialect:  activeSess.Dialect,
				Host:     activeSess.Host,
				Port:     activeSess.Port,
				User:     activeSess.User,
				Password: activeSess.Password,
				Database: activeSess.Database,
				FilePath: activeSess.FilePath,
				SSLMode:  activeSess.SSLMode,
			}, nil
		}

		if chosenNum == savedStartIdx && len(savedProfiles) > 0 {
			fmt.Println("\nSaved Connection Profiles:")
			for i, p := range savedProfiles {
				tgt := p.Host
				if p.FilePath != "" {
					tgt = p.FilePath
				}
				fmt.Printf("  [%d] %s (%s: %s)\n", i+1, p.Name, p.Dialect, tgt)
			}
			fmt.Printf("Select profile [1-%d]: ", len(savedProfiles))
			pChoice, _ := reader.ReadString('\n')
			pChoice = strings.TrimSpace(pChoice)
			pIdx, err := strconv.Atoi(pChoice)
			if err == nil && pIdx >= 1 && pIdx <= len(savedProfiles) {
				rec := savedProfiles[pIdx-1]
				return &database.ConnectionConfig{
					Name:     rec.Name,
					Dialect:  rec.Dialect,
					Host:     rec.Host,
					Port:     rec.Port,
					User:     rec.User,
					Password: rec.Password,
					Database: rec.Database,
					FilePath: rec.FilePath,
					SSLMode:  rec.SSLMode,
				}, nil
			}
		}

		return nil, fmt.Errorf("invalid choice")
	}
}

func promptDefault(reader *bufio.Reader, label, dflt string) string {
	if dflt != "" {
		fmt.Printf("%s [%s]: ", label, dflt)
	} else {
		fmt.Printf("%s: ", label)
	}
	input, _ := reader.ReadString('\n')
	val := strings.TrimSpace(input)
	if val == "" {
		return dflt
	}
	return val
}

func readPasswordInput(reader *bufio.Reader) string {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		bytePass, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err == nil {
			return strings.TrimRight(string(bytePass), "\r\n")
		}
	}
	pass, _ := reader.ReadString('\n')
	return strings.TrimRight(pass, "\r\n")
}
