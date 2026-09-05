package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/storage"
	"github.com/sql-doctor/sql-doctor/internal/ui"
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

		cfg := &database.ConnectionConfig{
			Name:     connFlagName,
			Dialect:  dialect,
			Host:     connFlagHost,
			Port:     connFlagPort,
			User:     connFlagUser,
			Password: connFlagPassword,
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

		if connFlagSave && appStorage != nil {
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
			if err := appStorage.SaveConnection(ctx, rec); err != nil {
				return fmt.Errorf("failed to save connection profile: %w", err)
			}
			_ = appStorage.SetActiveConnection(ctx, cfg.Name)
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
			}
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

		OutputResult(list, func() {
			if len(list) == 0 {
				fmt.Println(ui.Info("No saved connections found. Use 'sql-doctor connect ... --save' to add one."))
				return
			}

			tbl := ui.NewTable("ACTIVE", "NAME", "DIALECT", "HOST/FILE", "DATABASE", "USER")
			for _, c := range list {
				activeMarker := ""
				if c.IsActive {
					activeMarker = "  ★  "
				}
				hostFile := c.Host
				if c.FilePath != "" {
					hostFile = c.FilePath
				}
				tbl.AddRow(activeMarker, c.Name, string(c.Dialect), hostFile, c.Database, c.User)
			}
			fmt.Println(tbl.Render())
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
	connectCmd.Flags().StringVarP(&connFlagPassword, "password", "p", "", "Database password")
	connectCmd.Flags().StringVarP(&connFlagDatabase, "database", "d", "", "Database name")
	connectCmd.Flags().StringVarP(&connFlagFile, "file", "f", "", "SQLite database file path")
	connectCmd.Flags().StringVar(&connFlagName, "name", "", "Connection profile name")
	connectCmd.Flags().BoolVar(&connFlagSave, "save", false, "Save this connection profile")

	connectionsCmd.Flags().StringVar(&connListUse, "use", "", "Set connection profile as active")
	connectionsCmd.Flags().StringVar(&connListDelete, "delete", "", "Delete connection profile")
}
