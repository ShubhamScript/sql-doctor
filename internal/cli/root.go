package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/ai"
	"github.com/sql-doctor/sql-doctor/internal/ai/gemini"
	"github.com/sql-doctor/sql-doctor/internal/config"
	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/database/mysql"
	"github.com/sql-doctor/sql-doctor/internal/database/postgres"
	"github.com/sql-doctor/sql-doctor/internal/database/sqlite"
	"github.com/sql-doctor/sql-doctor/internal/storage"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var (
	flagJSON     bool
	flagVerbose  bool
	flagConn     string
	flagDBURL    string
	flagAI       bool

	appConfig  *config.Config
	appStorage *storage.Storage
)

var RootCmd = &cobra.Command{
	Use:           "sql-doctor",
	Short:         "SQL Doctor — SQL Intelligence, Database Diagnostics & Query Toolkit",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	// Register custom Laravel / Composer styled help functions
	RootCmd.SetHelpFunc(CustomHelpFunc)
	RootCmd.SetUsageFunc(CustomUsageFunc)

	// Register all database drivers
	database.InitRegistry(
		mysql.NewMySQL(),
		mysql.NewMariaDB(),
		postgres.New(),
		sqlite.New(),
	)

	RootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output results as machine-readable JSON")
	RootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose output and logs")
	RootCmd.PersistentFlags().StringVarP(&flagConn, "conn", "c", "", "Saved connection profile name to use")
	RootCmd.PersistentFlags().StringVar(&flagDBURL, "db-url", "", "Direct database URL or SQLite file path (e.g. postgres://user:pass@localhost:5432/db)")
	RootCmd.PersistentFlags().BoolVar(&flagAI, "ai", false, "Enable optional AI explanations / recommendations via Gemini")

	// Register subcommands
	RootCmd.AddCommand(connectCmd)
	RootCmd.AddCommand(connectionsCmd)
	RootCmd.AddCommand(pingCmd)
	RootCmd.AddCommand(configCmd)
	RootCmd.AddCommand(dbCmd)
	RootCmd.AddCommand(queryCmd)
	RootCmd.AddCommand(schemaCmd)
	RootCmd.AddCommand(dataCmd)
	RootCmd.AddCommand(migrationCmd)
	RootCmd.AddCommand(lintCmd)
	RootCmd.AddCommand(formatCmd)
	RootCmd.AddCommand(askCmd)
	RootCmd.AddCommand(doctorCmd)
}

// Execute runs the root CLI command
func Execute() {
	ctx := context.Background()
	store, err := storage.OpenStorage("")
	if err == nil {
		appStorage = store
		defer store.Close()
	}

	cfg, _ := config.LoadConfig(ctx, appStorage)
	appConfig = cfg

	if err := RootCmd.ExecuteContext(ctx); err != nil {
		if flagJSON {
			out := map[string]string{"error": err.Error()}
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println(ui.Error("%v", err))
		}
		os.Exit(1)
	}
}

// GetActiveDB establishes a connection to the target database
func GetActiveDB(ctx context.Context) (*sql.DB, database.Driver, *database.ConnectionConfig, error) {
	var cfg *database.ConnectionConfig

	// 1. Check direct URL flag
	if flagDBURL != "" {
		c, err := database.ParseURL(flagDBURL)
		if err != nil {
			return nil, nil, nil, err
		}
		cfg = c
	} else {
		// 2. Check --conn flag or active stored connection
		targetName := flagConn
		if targetName == "" && appConfig != nil {
			targetName = appConfig.ActiveConnName
		}

		if targetName != "" && appStorage != nil {
			rec, err := appStorage.GetConnection(ctx, targetName)
			if err != nil {
				return nil, nil, nil, err
			}
			cfg = &database.ConnectionConfig{
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
		}
	}

	if cfg == nil {
		return nil, nil, nil, fmt.Errorf("no database connection specified.\n\nUse --db-url, --conn, or connect using:\n  sql-doctor connect --type <mysql|postgres|sqlite> ...")
	}

	db, driver, err := database.OpenConnection(ctx, cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	return db, driver, cfg, nil
}

// GetAIProvider returns an initialized Gemini provider
func GetAIProvider() ai.AIProvider {
	var key, model string
	if appConfig != nil {
		key = appConfig.GeminiKey
		model = appConfig.GeminiModel
	}
	return gemini.New(key, model)
}

// OutputResult renders data as either JSON or passes to a terminal printer
func OutputResult(data interface{}, textRender func()) {
	if flagJSON {
		bytes, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Printf("{\"error\": %q}\n", err.Error())
			return
		}
		fmt.Println(string(bytes))
		return
	}

	if textRender != nil {
		textRender()
	}
}
