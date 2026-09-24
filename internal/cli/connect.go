package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/storage"
	"github.com/sql-doctor/sql-doctor/internal/ui"
	"golang.org/x/term"
)

var (
	connFlagType     string
	connFlagHost     string
	connFlagPort     int
	connFlagUser     string
	connFlagPassword string
	connFlagDatabase string
	connFlagFile     string
	connFlagName     string
	connFlagSave     bool
	connFlagShell    bool

	connListUse    string
	connListDelete string
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Create and test a database connection",
	Long: `Test a database connection and optionally save it to your local configuration.

Examples:
  sql-doctor connect --type sqlite --file ./dev.db --name local-dev --save
  sql-doctor connect --type mysql --host 127.0.0.1 --port 3306 --user root --database test --name mysql-local --save
  sql-doctor connect --type postgres --host localhost --port 5432 --user postgres --database test --name pg-local --save`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		dialect := database.Dialect(strings.ToLower(connFlagType))
		if dialect == "" {
			if connFlagFile != "" {
				dialect = database.DialectSQLite
			} else {
				return fmt.Errorf("database type required: --type <mysql|mariadb|postgres|sqlite>")
			}
		}

		password := connFlagPassword
		if password == "__PROMPT__" {
			fmt.Print("Enter password (leave empty if none): ")
			if term.IsTerminal(int(os.Stdin.Fd())) {
				bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err == nil {
					password = strings.TrimRight(string(bytePassword), "\r\n")
				} else {
					password = ""
				}
			} else {
				reader := bufio.NewReader(os.Stdin)
				pass, _ := reader.ReadString('\n')
				password = strings.TrimRight(pass, "\r\n")
			}
		}

		cfg := &database.ConnectionConfig{
			Name:     connFlagName,
			Dialect:  dialect,
			Host:     connFlagHost,
			Port:     connFlagPort,
			User:     connFlagUser,
			Password: password,
			Database: connFlagDatabase,
			FilePath: connFlagFile,
		}

		if cfg.Name == "" {
			if cfg.Database != "" {
				cfg.Name = cfg.Database
			} else if cfg.FilePath != "" {
				cfg.Name = "sqlite-local"
			} else {
				cfg.Name = "default"
			}
		}

		// Test connection
		start := time.Now()
		db, driver, err := database.OpenConnection(ctx, cfg)
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		defer db.Close()

		latency := time.Since(start)
		ver, err := driver.Version(ctx, db)
		if err != nil {
			ver = "Unknown"
		}

		rec := &storage.ConnectionRecord{
			Name:     cfg.Name,
			Dialect:  cfg.Dialect,
			Host:     cfg.Host,
			Port:     cfg.Port,
			User:     cfg.User,
			Password: cfg.Password,
			Database: cfg.Database,
			FilePath: cfg.FilePath,
			IsActive: true,
		}

		if connFlagSave && appStorage != nil {
			if err := appStorage.SaveConnection(ctx, rec); err != nil {
				return fmt.Errorf("failed to save connection profile: %w", err)
			}
			_ = appStorage.SetActiveConnection(ctx, cfg.Name)
			_ = appStorage.ClearSessionConnection(ctx)
		} else if appStorage != nil {
			// Save active session connection so user can continue without saving a permanent profile
			if err := appStorage.SaveSessionConnection(ctx, rec); err != nil {
				return fmt.Errorf("failed to save active session: %w", err)
			}
		}

		if connFlagShell {
			err := RunShell(ctx, db, driver, cfg)
			if !connFlagSave && appStorage != nil {
				_ = appStorage.ClearSessionConnection(ctx)
			}
			return err
		}

		OutputResult(map[string]interface{}{
			"status":   "CONNECTED",
			"name":     cfg.Name,
			"dialect":  cfg.Dialect,
			"version":  ver,
			"latency":  fmt.Sprintf("%v", latency),
			"saved":    connFlagSave,
			"database": cfg.Database,
		}, func() {
			fmt.Println(ui.Success("Connected successfully to %s (%s)", cfg.Name, ver))
			fmt.Printf("  Latency:  %v\n", latency)
			fmt.Printf("  Dialect:  %s\n", cfg.Dialect)
			if connFlagSave {
				fmt.Printf("  Profile:  Saved as '%s' (set as active connection)\n", cfg.Name)
			} else {
				fmt.Println("  Session:  Active (temporary — use --save to persist as profile)")
			}
			if cfg.Database != "" {
				fmt.Printf("  Database: %s\n", cfg.Database)
			} else if cfg.Dialect != database.DialectSQLite {
				fmt.Println(ui.Info("  Note:     No database selected. Choose one with 'sql-doctor use <db>' or view with 'sql-doctor db databases'."))
			}
			fmt.Println(ui.Info("  Tip:      Type 'sql-doctor shell' to open interactive MySQL-like console."))
		})

		return nil
	},
}

var connectionsCmd = &cobra.Command{
	Use:   "connections",
	Short: "List, activate, or delete saved connection profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if appStorage == nil {
			return fmt.Errorf("local storage unavailable")
		}

		if connListUse != "" {
			if err := appStorage.SetActiveConnection(ctx, connListUse); err != nil {
				return err
			}
			_ = appStorage.ClearSessionConnection(ctx)
			fmt.Println(ui.Success("Active connection switched to '%s'", connListUse))
			return nil
		}

		if connListDelete != "" {
			if err := appStorage.DeleteConnection(ctx, connListDelete); err != nil {
				return err
			}
			fmt.Println(ui.Success("Connection '%s' deleted", connListDelete))
			return nil
		}

		list, err := appStorage.ListConnections(ctx)
		if err != nil {
			return err
		}

		sess, _ := appStorage.GetSessionConnection(ctx)

		OutputResult(map[string]interface{}{
			"connections": list,
			"session":     sess,
		}, func() {
			if sess != nil {
				sessHost := sess.Host
				if sess.FilePath != "" {
					sessHost = sess.FilePath
				}
				dbStr := sess.Database
				if dbStr == "" {
					dbStr = "(none selected)"
				}
				fmt.Println(ui.InfoBadge + " " + ui.HeaderStyle.Render("Active Session (not saved as permanent profile):"))
				fmt.Printf("  Name: %s | Dialect: %s | Host: %s | Database: %s | User: %s\n",
					sess.Name, sess.Dialect, sessHost, dbStr, sess.User)
				fmt.Println("  (Tip: Run 'sql-doctor connect ... --save' to persist as a profile)")
				fmt.Println()
			}

			if len(list) == 0 {
				if sess == nil {
					fmt.Println(ui.Info("No saved connections found. Use 'sql-doctor connect ... --save' to add one."))
				}
				return
			}

			tbl := ui.NewTable("ACTIVE", "NAME", "DIALECT", "HOST/FILE", "DATABASE", "USER")
			for _, c := range list {
				activeMarker := ""
				if c.IsActive && sess == nil {
					activeMarker = "  ★  "
				}
				hostFile := c.Host
				if c.FilePath != "" {
					hostFile = c.FilePath
				}
				dbName := c.Database
				if dbName == "" {
					dbName = "(none)"
				}
				tbl.AddRow(activeMarker, c.Name, string(c.Dialect), hostFile, dbName, c.User)
			}
			fmt.Println(tbl.Render())
		})

		return nil
	},
}

var useCmd = &cobra.Command{
	Use:   "use <database>",
	Short: "Select or switch the active database for the current connection",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		targetDB := args[0]

		if appStorage == nil {
			return fmt.Errorf("local storage unavailable")
		}

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if cfg.Dialect == database.DialectSQLite {
			return fmt.Errorf("the 'use' command is for multi-database servers (MySQL, PostgreSQL). For SQLite, connect directly using --file <path>")
		}

		// Verify database exists on server
		dbs, err := driver.Databases(ctx, db)
		if err == nil && len(dbs) > 0 {
			found := false
			for _, d := range dbs {
				if strings.EqualFold(d, targetDB) {
					targetDB = d // preserve exact casing from server
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("database '%s' not found on connection '%s'.\n\nRun 'sql-doctor db databases' to view available databases", targetDB, cfg.Name)
			}
		}

		updated, err := appStorage.UpdateConnectionDatabase(ctx, flagConn, targetDB)
		if err != nil {
			return fmt.Errorf("failed to update active database: %w", err)
		}

		OutputResult(map[string]interface{}{
			"status":     "SWITCHED",
			"database":   targetDB,
			"connection": updated.Name,
		}, func() {
			fmt.Println(ui.Success("Active database set to '%s' (connection: '%s')", targetDB, updated.Name))
			fmt.Println("You can now run queries and database commands without specifying the database.")
		})

		return nil
	},
}

var databasesCmd = &cobra.Command{
	Use:   "databases",
	Short: "List all databases available on the connected server",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		databases, err := driver.Databases(ctx, db)
		if err != nil {
			return fmt.Errorf("failed to fetch databases: %w", err)
		}

		OutputResult(databases, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("Databases on [%s] (%d found)", cfg.Name, len(databases))))
			if len(databases) == 0 {
				fmt.Println(ui.Info("No databases found."))
				return
			}

			tbl := ui.NewTable("ACTIVE", "DATABASE NAME")
			for _, d := range databases {
				activeMarker := ""
				if cfg.Database == d {
					activeMarker = "  ★  "
				}
				tbl.AddRow(activeMarker, d)
			}
			fmt.Println(tbl.Render())
			fmt.Println(ui.Info("Tip: Switch active database with 'sql-doctor use <database>'"))
		})
		return nil
	},
}

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Test active database connection and measure latency",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		start := time.Now()

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := driver.Ping(ctx, db); err != nil {
			return fmt.Errorf("ping failed: %w", err)
		}

		latency := time.Since(start)
		ver, _ := driver.Version(ctx, db)

		OutputResult(map[string]interface{}{
			"status":   "PONG",
			"name":     cfg.Name,
			"dialect":  cfg.Dialect,
			"version":  ver,
			"latency":  fmt.Sprintf("%v", latency),
		}, func() {
			fmt.Println(ui.Success("Ping successful to [%s]", cfg.Name))
			fmt.Printf("  Engine:   %s\n", ver)
			fmt.Printf("  Latency:  %v\n", latency)
		})

		return nil
	},
}

func init() {
	connectCmd.Flags().StringVar(&connFlagType, "type", "", "Database type (mysql, mariadb, postgres, sqlite)")
	connectCmd.Flags().StringVar(&connFlagHost, "host", "127.0.0.1", "Database host")
	connectCmd.Flags().IntVar(&connFlagPort, "port", 0, "Database port")
	connectCmd.Flags().StringVarP(&connFlagUser, "user", "u", "", "Database username")
	connectCmd.Flags().StringVarP(&connFlagPassword, "password", "p", "", "Database password (prompt if passed without value)")
	connectCmd.Flags().Lookup("password").NoOptDefVal = "__PROMPT__"
	connectCmd.Flags().StringVarP(&connFlagDatabase, "database", "d", "", "Database name")
	connectCmd.Flags().StringVarP(&connFlagFile, "file", "f", "", "SQLite database file path")
	connectCmd.Flags().StringVar(&connFlagName, "name", "", "Connection profile name")
	connectCmd.Flags().BoolVar(&connFlagSave, "save", false, "Save this connection profile")
	connectCmd.Flags().BoolVarP(&connFlagShell, "shell", "i", false, "Open interactive query shell after connecting")

	connectionsCmd.Flags().StringVar(&connListUse, "use", "", "Set connection profile as active")
	connectionsCmd.Flags().StringVar(&connListDelete, "delete", "", "Delete connection profile")
}
