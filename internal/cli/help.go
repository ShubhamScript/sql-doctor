package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const AsciiLogo = `   _____ ____    __        ____             __            
  / ___// __ \  / /       / __ \____  _____/ /_____  _____
  \__ \/ / / / / /       / / / / __ \/ ___/ __/ __ \/ ___/
 ___/ / /_/ / / /___    / /_/ / /_/ / /__/ /_/ /_/ / /    
/____/\___\_\/_____/   /_____/\____/\___/\__/\____/_/     `

var (
	styleYellow = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	styleGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	styleCyan   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#38BDF8"))
	styleGray   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	styleWhite  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	styleLogo   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6366F1"))
	styleBadge  = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#6366F1")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)
	styleVer    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
)

// CustomHelpFunc renders Laravel / Composer styled help output
func CustomHelpFunc(cmd *cobra.Command, args []string) {
	fmt.Println(renderCommandHelp(cmd))
}

// CustomUsageFunc renders Laravel / Composer styled usage output
func CustomUsageFunc(cmd *cobra.Command) error {
	fmt.Println(renderCommandHelp(cmd))
	return nil
}

func renderCommandHelp(cmd *cobra.Command) string {
	if cmd == RootCmd || !cmd.HasParent() {
		return renderRootHelp()
	}
	return renderSubcommandHelp(cmd)
}

func renderRootHelp() string {
	var b strings.Builder

	// 1. Header (ASCII Art Logo + Laravel Version Badge)
	b.WriteString("\n")
	b.WriteString(styleLogo.Render(AsciiLogo))
	b.WriteString("\n\n")

	badge := styleBadge.Render("SQL DOCTOR")
	ver := styleVer.Render("v0.1.0-beta")
	subtitle := styleGray.Render("— Database Diagnostics & SQL Intelligence CLI")
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n\n", badge, ver, subtitle))

	// 2. Usage
	b.WriteString(styleYellow.Render("Usage:"))
	b.WriteString("\n")
	b.WriteString("  sql-doctor <command> [options] [arguments]\n\n")

	// 3. Options
	b.WriteString(styleYellow.Render("Options:"))
	b.WriteString("\n")
	options := []struct {
		flags string
		desc  string
	}{
		{"-c, --conn <name>", "Saved connection profile name to use"},
		{"-d, --database <db>", "Target database name to use for this command"},
		{"    --db-url <url>", "Direct database URL or SQLite file path"},
		{"    --ai", "Enable optional AI recommendations via Gemini"},
		{"    --json", "Output results as machine-readable JSON"},
		{"-v, --verbose", "Increase the verbosity of messages"},
		{"-h, --help", "Display help for the given command"},
	}
	for _, opt := range options {
		padded := opt.flags
		if len(padded) < 27 {
			padded += strings.Repeat(" ", 27-len(padded))
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", styleGreen.Render(padded), styleWhite.Render(opt.desc)))
	}
	b.WriteString("\n")

	// 4. Available Commands
	b.WriteString(styleYellow.Render("Available commands:"))
	b.WriteString("\n")

	// Root commands
	rootCmds := []struct {
		name string
		desc string
	}{
		{"doctor", "Run full end-to-end database health check"},
		{"shell", "Open interactive query REPL console (MySQL-like)"},
		{"connect", "Create and test a new database connection"},
		{"connections", "List, inspect, and manage saved database connections"},
		{"use <database>", "Select or switch active database for current connection"},
		{"databases", "List all databases on target connection"},
		{"disconnect", "Disconnect and clear ephemeral session memory"},
		{"ping", "Quick connectivity test to target database"},
		{"lint", "Lint SQL query files against anti-pattern rules"},
		{"format", "Format SQL queries with standard indentation"},
		{"ask", "Ask database questions or generate SQL using Gemini AI"},
		{"completion", "Generate shell autocompletion script"},
	}
	for _, c := range rootCmds {
		padded := c.name
		if len(padded) < 27 {
			padded += strings.Repeat(" ", 27-len(padded))
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", styleGreen.Render(padded), styleWhite.Render(c.desc)))
	}

	// Namespaced Command Groups (Laravel Artisan style)
	groups := []struct {
		group string
		cmds  []struct{ name, desc string }
	}{
		{
			group: "db",
			cmds: []struct{ name, desc string }{
				{"db use <database>", "Select or switch active database"},
				{"db databases", "List all databases on target connection"},
				{"db tables", "List all tables in connected database"},
				{"db describe <table>", "Show column types, nullability, keys, and defaults"},
				{"db indexes <table>", "List table indexes, column order, and uniqueness"},
				{"db relationships", "Map foreign key relationships across database"},
				{"db diff <target>", "Compare schemas against another DB or snapshot"},
				{"db snapshot <action>", "Manage point-in-time schema snapshots"},
			},
		},
		{
			group: "query",
			cmds: []struct{ name, desc string }{
				{"query analyze <sql>", "Analyze query performance, execution plan, and smells"},
				{"query explain <sql>", "Visualize database query execution plan tree"},
				{"query optimize <sql>", "Recommend optimal composite indexes and rewrites"},
			},
		},
		{
			group: "schema",
			cmds: []struct{ name, desc string }{
				{"schema analyze", "Analyze schema design smells, missing PKs, unindexed FKs"},
				{"schema datatypes <table>", "Data-aware datatype advisor analyzing row contents"},
			},
		},
		{
			group: "data",
			cmds: []struct{ name, desc string }{
				{"data quality <table>", "Profile columns for null ratios, cardinality, and anomalies"},
			},
		},
		{
			group: "migration",
			cmds: []struct{ name, desc string }{
				{"migration check <file>", "Assess DDL migration safety, risk score, and locks"},
			},
		},
		{
			group: "config",
			cmds: []struct{ name, desc string }{
				{"config ai", "Show Gemini AI configuration status and active model"},
				{"config set-ai-key <key>", "Store Google Gemini API key securely"},
				{"config set-active <name>", "Set default active database connection"},
				{"config get-active", "Show currently active database connection"},
			},
		},
	}

	for _, g := range groups {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(" %s\n", styleYellow.Render(g.group)))
		for _, c := range g.cmds {
			padded := c.name
			if len(padded) < 27 {
				padded += strings.Repeat(" ", 27-len(padded))
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", styleGreen.Render(padded), styleWhite.Render(c.desc)))
		}
	}

	// 5. Footer Tip
	b.WriteString("\n")
	b.WriteString(styleGray.Render("Run 'sql-doctor <command> --help' for more information on a specific command.\n"))

	return b.String()
}

func renderSubcommandHelp(cmd *cobra.Command) string {
	var b strings.Builder

	// Header (ASCII Art Logo + Badge)
	b.WriteString("\n")
	b.WriteString(styleLogo.Render(AsciiLogo))
	b.WriteString("\n\n")

	badge := styleBadge.Render("SQL DOCTOR")
	ver := styleVer.Render("v0.1.0-beta")
	subtitle := styleGray.Render("— Database Diagnostics & SQL Intelligence CLI")
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n\n", badge, ver, subtitle))

	// 1. Usage
	b.WriteString(styleYellow.Render("Usage:"))
	b.WriteString("\n")
	usageLine := cmd.UseLine()
	if cmd.HasSubCommands() {
		usageLine = fmt.Sprintf("%s <command> [options]", cmd.CommandPath())
	} else {
		usageLine = fmt.Sprintf("%s [options]", cmd.CommandPath())
		if cmd.Use != "" && strings.Contains(cmd.Use, " ") {
			usageLine = fmt.Sprintf("sql-doctor %s [options]", cmd.Use)
		}
	}
	b.WriteString(fmt.Sprintf("  %s\n\n", usageLine))

	// 2. Description
	desc := cmd.Long
	if desc == "" {
		desc = cmd.Short
	}
	if desc != "" {
		b.WriteString(styleYellow.Render("Description:"))
		b.WriteString("\n")
		lines := strings.Split(strings.TrimSpace(desc), "\n")
		for _, l := range lines {
			b.WriteString(fmt.Sprintf("  %s\n", styleWhite.Render(strings.TrimSpace(l))))
		}
		b.WriteString("\n")
	}

	// 3. Subcommands (if any)
	if cmd.HasAvailableSubCommands() {
		b.WriteString(styleYellow.Render("Available commands:"))
		b.WriteString("\n")
		for _, sub := range cmd.Commands() {
			if sub.Hidden {
				continue
			}
			padded := sub.Name()
			if len(padded) < 27 {
				padded += strings.Repeat(" ", 27-len(padded))
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", styleGreen.Render(padded), styleWhite.Render(sub.Short)))
		}
		b.WriteString("\n")
	}

	// 4. Options
	flags := cmd.Flags()
	var optLines []string
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		var flagStr string
		if f.Shorthand != "" {
			flagStr = fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
		} else {
			flagStr = fmt.Sprintf("    --%s", f.Name)
		}
		if f.Value.Type() != "bool" {
			switch f.Name {
			case "conn":
				flagStr += " <name>"
			case "db-url":
				flagStr += " <url>"
			default:
				flagStr += " <value>"
			}
		}
		if len(flagStr) < 27 {
			flagStr += strings.Repeat(" ", 27-len(flagStr))
		}

		usage := f.Usage
		if f.Name == "help" {
			usage = "Display help for this command"
		} else if f.Name == "verbose" {
			usage = "Increase the verbosity of messages"
		}

		optLines = append(optLines, fmt.Sprintf("  %s %s", styleGreen.Render(flagStr), styleWhite.Render(usage)))
	})

	if len(optLines) > 0 {
		b.WriteString(styleYellow.Render("Options:"))
		b.WriteString("\n")
		for _, l := range optLines {
			b.WriteString(l + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}
