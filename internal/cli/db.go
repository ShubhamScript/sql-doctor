package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/schema"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database structure, tables, indexes, relationships, diffs, and snapshots",
}

var dbTablesCmd = &cobra.Command{
	Use:   "tables",
	Short: "List all tables in the connected database",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		tables, err := driver.Tables(ctx, db)
		if err != nil {
			return fmt.Errorf("failed to fetch tables: %w", err)
		}

		dbLabel := cfg.Database
		if dbLabel == "" {
			dbLabel = cfg.Name
		}

		OutputResult(tables, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("Tables in [%s] (%d found)", dbLabel, len(tables))))
			tbl := ui.NewTable("TABLE NAME", "TYPE", "EST. ROWS", "ENGINE")
			for _, t := range tables {
				tbl.AddRow(t.Name, t.Type, fmt.Sprintf("%d", t.RowCount), t.Engine)
			}
			fmt.Println(tbl.Render())
		})
		return nil
	},
}

var dbDescribeCmd = &cobra.Command{
	Use:   "describe <table>",
	Short: "Describe table columns, data types, nullability, and keys",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		tableName := args[0]

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		detail, err := driver.DescribeTable(ctx, db, tableName)
		if err != nil {
			return err
		}

		OutputResult(detail, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("Table: %s (Rows: %d)", detail.Table.Name, detail.Table.RowCount)))
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
		})
		return nil
	},
}

var dbIndexesCmd = &cobra.Command{
	Use:   "indexes <table>",
	Short: "Inspect indexes on a table",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		tableName := args[0]

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		indexes, err := driver.Indexes(ctx, db, tableName)
		if err != nil {
			return err
		}

		OutputResult(indexes, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("Indexes on table '%s' (%d found)", tableName, len(indexes))))
			tbl := ui.NewTable("INDEX NAME", "COLUMNS", "UNIQUE", "PRIMARY", "TYPE")
			for _, idx := range indexes {
				tbl.AddRow(idx.Name, strings.Join(idx.Columns, ", "), fmt.Sprintf("%v", idx.IsUnique), fmt.Sprintf("%v", idx.IsPrimary), idx.Type)
			}
			fmt.Println(tbl.Render())
		})
		return nil
	},
}

var dbRelationshipsCmd = &cobra.Command{
	Use:   "relationships",
	Short: "Show foreign keys, inferred relationships, and orphan records",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		rels, err := driver.Relationships(ctx, db)
		if err != nil {
			return err
		}

		OutputResult(rels, func() {
			fmt.Println(ui.TitleStyle.Render("Table Relationships & Referential Integrity"))
			if len(rels) == 0 {
				fmt.Println(ui.Info("No relationships or foreign keys detected."))
				return
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
		})
		return nil
	},
}

var diffCmd = &cobra.Command{
	Use:   "diff <connA> <connB>",
	Short: "Compare schemas between two database connections or snapshots",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return dbDiffCmd.RunE(cmd, args)
	},
}

var dbDiffCmd = &cobra.Command{
	Use:   "diff <connA> <connB>",
	Short: "Compare schemas between two database connections or snapshots",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		connA, connB := args[0], args[1]

		if appStorage == nil {
			return fmt.Errorf("local storage unavailable")
		}

		// Connect to A
		recA, err := appStorage.GetConnection(ctx, connA)
		if err != nil {
			return fmt.Errorf("failed to get connection A '%s': %w", connA, err)
		}
		dbA, driverA, err := database.OpenConnection(ctx, &database.ConnectionConfig{
			Dialect:  recA.Dialect,
			Host:     recA.Host,
			Port:     recA.Port,
			User:     recA.User,
			Password: recA.Password,
			Database: recA.Database,
			FilePath: recA.FilePath,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to A: %w", err)
		}
		defer dbA.Close()

		// Connect to B
		recB, err := appStorage.GetConnection(ctx, connB)
		if err != nil {
			return fmt.Errorf("failed to get connection B '%s': %w", connB, err)
		}
		dbB, driverB, err := database.OpenConnection(ctx, &database.ConnectionConfig{
			Dialect:  recB.Dialect,
			Host:     recB.Host,
			Port:     recB.Port,
			User:     recB.User,
			Password: recB.Password,
			Database: recB.Database,
			FilePath: recB.FilePath,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to B: %w", err)
		}
		defer dbB.Close()

		detailsA, err := schema.FetchAllTableDetails(ctx, driverA, dbA)
		if err != nil {
			return err
		}
		detailsB, err := schema.FetchAllTableDetails(ctx, driverB, dbB)
		if err != nil {
			return err
		}

		diffEngine := schema.NewDiffEngine()
		diff := diffEngine.Compare(detailsA, detailsB)
		diff.SourceSchema = connA
		diff.TargetSchema = connB

		OutputResult(diff, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("Schema Diff: [%s] vs [%s]", connA, connB)))
			if len(diff.Differences) == 0 {
				fmt.Println(ui.Success("Schemas are identical! No differences found."))
				return
			}

			fmt.Printf("Total Differences: %d\n\n", diff.TotalDiffs)
			tbl := ui.NewTable("ACTION", "TYPE", "OBJECT", "DETAILS")
			for _, item := range diff.Differences {
				actionBadge := item.Action
				switch item.Action {
				case "ADDED":
					actionBadge = ui.Success("+ ADDED")
				case "REMOVED":
					actionBadge = ui.Error("- REMOVED")
				case "MODIFIED":
					actionBadge = ui.Warning("~ MODIFIED")
				}
				tbl.AddRow(actionBadge, item.Type, item.ObjectName, item.Details)
			}
			fmt.Println(tbl.Render())

			fmt.Println(ui.HeaderStyle.Render("\nRecommended Migration SQL:"))
			for _, item := range diff.Differences {
				if item.MigrationSQL != "" {
					fmt.Println(item.MigrationSQL)
				}
			}
		})
		return nil
	},
}

var dbSnapshotCmd = &cobra.Command{
	Use:   "snapshot [create <name> | list | show <name>]",
	Short: "Create, view, and list schema snapshots",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if len(args) == 0 || args[0] == "list" {
			if appStorage == nil {
				return fmt.Errorf("local storage unavailable")
			}
			snaps, err := appStorage.ListSnapshots(ctx)
			if err != nil {
				return err
			}
			OutputResult(snaps, func() {
				fmt.Println(ui.TitleStyle.Render("Saved Schema Snapshots"))
				if len(snaps) == 0 {
					fmt.Println(ui.Info("No snapshots saved. Create one with: sql-doctor db snapshot create <name>"))
					return
				}
				tbl := ui.NewTable("ID", "NAME", "CONNECTION", "DATABASE")
				for _, s := range snaps {
					tbl.AddRow(fmt.Sprintf("%d", s.ID), s.Name, s.ConnectionName, s.DatabaseName)
				}
				fmt.Println(tbl.Render())
			})
			return nil
		}

		action := args[0]
		if action == "create" {
			if len(args) < 2 {
				return fmt.Errorf("snapshot name required: sql-doctor db snapshot create <name>")
			}
			name := args[1]
			db, driver, cfg, err := GetActiveDB(ctx)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
				return err
			}

			mgr := schema.NewSnapshotManager(appStorage, driver)
			snap, err := mgr.CreateSnapshot(ctx, db, name, cfg.Name, cfg.Database)
			if err != nil {
				return err
			}

			OutputResult(snap, func() {
				fmt.Println(ui.Success("Schema snapshot '%s' created successfully (%d tables)", name, snap.TableCount))
			})
			return nil
		}

		return fmt.Errorf("unknown snapshot action: %s", action)
	},
}

func init() {
	dbCmd.AddCommand(useCmd)
	dbCmd.AddCommand(databasesCmd)
	dbCmd.AddCommand(dbTablesCmd)
	dbCmd.AddCommand(dbDescribeCmd)
	dbCmd.AddCommand(dbIndexesCmd)
	dbCmd.AddCommand(dbRelationshipsCmd)
	dbCmd.AddCommand(dbDiffCmd)
	dbCmd.AddCommand(dbSnapshotCmd)
}
