package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/freddyrosa16/clibox/internal/app"
)

func main() {
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configPath := flags.String("config", "", "path to clibox config file")
	theme := flags.String("theme", "", "start clibox with a theme: nocturne, ember, or lagoon")
	account := flags.String("account", "", "email account name to read")
	mailbox := flags.String("mailbox", "", "mailbox/folder to read")
	archiveFolder := flags.String("archive-folder", "", "advanced: mailbox/folder to move archived messages into")
	backendBinary := flags.String("backend", "", "advanced: path to the email backend binary")
	mailBackend := flags.String("mail-backend", "", "advanced: mail backend mode: himalaya or native")
	editor := flags.String("editor", "", "editor command for compose and reply drafts")
	himalaya := flags.String("himalaya", "", "deprecated alias for --backend")
	pageSize := flags.Int("page-size", 0, "advanced: envelopes to request per page; 0 uses the backend default (50)")
	confirmDelete := flags.Bool("confirm-delete", true, "ask before moving messages to Trash")
	composeFormat := flags.String("compose-format", "", "compose body format: text or markdown")
	showThemes := flags.Bool("themes", false, "list available themes")
	email := flags.String("email", "", "email address for auth add")
	verbose := flags.Bool("verbose", false, "show detailed doctor output")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [doctor|accounts|sync|auth add|auth login] [flags]\n\n", os.Args[0])
		fmt.Fprintln(flags.Output(), "  -account string")
		fmt.Fprintln(flags.Output(), "    \temail account name to read")
		fmt.Fprintln(flags.Output(), "  -archive-folder string")
		fmt.Fprintln(flags.Output(), "    \tadvanced: mailbox/folder to move archived messages into")
		fmt.Fprintln(flags.Output(), "  -backend string")
		fmt.Fprintln(flags.Output(), "    \tadvanced: path to the email backend binary")
		fmt.Fprintln(flags.Output(), "  -config string")
		fmt.Fprintln(flags.Output(), "    \tpath to clibox config file")
		fmt.Fprintln(flags.Output(), "  -confirm-delete")
		fmt.Fprintln(flags.Output(), "    \task before moving messages to Trash (default true)")
		fmt.Fprintln(flags.Output(), "  -editor string")
		fmt.Fprintln(flags.Output(), "    \teditor command for compose and reply drafts")
		fmt.Fprintln(flags.Output(), "  -mailbox string")
		fmt.Fprintln(flags.Output(), "    \tmailbox/folder to read")
		fmt.Fprintln(flags.Output(), "  -mail-backend string")
		fmt.Fprintln(flags.Output(), "    \tadvanced: mail backend mode: himalaya or native")
		fmt.Fprintln(flags.Output(), "  -page-size int")
		fmt.Fprintln(flags.Output(), "    \tadvanced: envelopes to request per page; 0 uses the backend default (50)")
		fmt.Fprintln(flags.Output(), "  -theme string")
		fmt.Fprintln(flags.Output(), "    \tstart clibox with a theme: nocturne, ember, or lagoon")
		fmt.Fprintln(flags.Output(), "  -themes")
		fmt.Fprintln(flags.Output(), "    \tlist available themes")
		fmt.Fprintln(flags.Output(), "  -email string")
		fmt.Fprintln(flags.Output(), "    \temail address for auth add")
		fmt.Fprintln(flags.Output(), "  -verbose")
		fmt.Fprintln(flags.Output(), "    \tshow detailed doctor output")
	}

	args := os.Args[1:]
	command := ""
	authAction := ""
	if len(args) > 0 {
		switch args[0] {
		case "doctor", "accounts", "sync":
			command = args[0]
			args = args[1:]
		case "auth":
			command = "auth"
			args = args[1:]
			if len(args) > 0 {
				authAction = args[0]
				args = args[1:]
			}
		}
	}
	if command == "auth" && authAction == "" {
		fmt.Fprintln(os.Stderr, "clibox auth needs an action: add or login")
		flags.Usage()
		os.Exit(2)
	}
	if command == "auth" && authAction != "add" && authAction != "login" {
		fmt.Fprintf(os.Stderr, "unknown clibox auth action %q\n", authAction)
		flags.Usage()
		os.Exit(2)
	}

	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	visited := visitedFlags(flags)
	if *showThemes {
		fmt.Fprint(os.Stdout, app.ThemeHelp())
		return
	}

	config, _, err := app.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clibox config failed: %v\n", err)
		os.Exit(2)
	}
	options := app.Options{
		Theme:         config.Theme,
		Account:       config.Account,
		Mailbox:       config.Mailbox,
		ArchiveFolder: config.ArchiveFolder,
		BackendMode:   config.Backend,
		Himalaya:      config.Himalaya,
		Editor:        config.Editor,
		PageSize:      config.PageSize,
		ConfirmDelete: config.ConfirmDelete,
		ComposeFormat: config.ComposeFormat,
		ConfigPath:    *configPath,
		Accounts:      config.Accounts,
		Verbose:       *verbose,
	}
	if visited["theme"] {
		options.Theme = *theme
	}
	if visited["account"] {
		options.Account = *account
	}
	if visited["mailbox"] {
		options.Mailbox = *mailbox
	}
	if visited["archive-folder"] {
		options.ArchiveFolder = *archiveFolder
	}
	if visited["backend"] || visited["himalaya"] {
		options.Himalaya = firstNonEmpty(*backendBinary, *himalaya)
	}
	if visited["mail-backend"] {
		options.BackendMode = *mailBackend
	}
	if visited["editor"] {
		options.Editor = *editor
	}
	if visited["page-size"] {
		options.PageSize = *pageSize
	}
	if visited["confirm-delete"] {
		value := *confirmDelete
		options.ConfirmDelete = &value
	}
	if visited["compose-format"] {
		options.ComposeFormat = *composeFormat
	}
	if command == "" {
		options.RememberSession = true
		if !visited["account"] && !visited["mailbox"] {
			if lastSession, ok, err := app.LoadLastSession(options.StatePath); err == nil && ok {
				options.LastSession = lastSession
				if strings.TrimSpace(lastSession.Account) != "" {
					options.Account = lastSession.Account
				}
				if strings.TrimSpace(lastSession.Mailbox) != "" {
					options.Mailbox = lastSession.Mailbox
				}
			}
		}
	}

	switch command {
	case "doctor":
		report, err := app.Doctor(context.Background(), options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clibox doctor failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, report)
		return
	case "accounts":
		options.BackendMode = "native"
		report, err := app.NativeAccountsReport(context.Background(), options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clibox accounts failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, report)
		return
	case "sync":
		options.BackendMode = "native"
		report, err := app.NativeSync(context.Background(), options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clibox sync failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, report)
		return
	case "auth":
		options.BackendMode = "native"
		switch authAction {
		case "add":
			authEmail := firstNonEmpty(*email, firstArg(flags.Args()))
			report, err := app.NativeAuthAdd(context.Background(), options, authEmail)
			if err != nil {
				fmt.Fprintf(os.Stderr, "clibox auth add failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stdout, report)
			return
		case "login":
			report, err := app.NativeAuthLogin(context.Background(), options)
			if err != nil {
				fmt.Fprintf(os.Stderr, "clibox auth login failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stdout, report)
			return
		}
	}

	program := tea.NewProgram(app.NewWithOptions(options), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "clibox failed: %v\n", err)
		os.Exit(1)
	}
}

func visitedFlags(flags *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	flags.Visit(func(flag *flag.Flag) {
		visited[flag.Name] = true
	})
	return visited
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
