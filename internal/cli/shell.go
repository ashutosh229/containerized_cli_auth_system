package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"containerized_cli_auth_system/internal/auth"

	"github.com/chzyer/readline"
)

type Shell struct {
	in             io.Reader
	out            io.Writer
	printer        *Printer
	auth           *auth.Service
	sessionID      string
	username       string
	completer      *readline.PrefixCompleter
	guestCompleter *readline.PrefixCompleter
	authCompleter  *readline.PrefixCompleter
	commandSet     map[string]command
	rl             *readline.Instance
}

type command struct {
	description string
	handler     func(context.Context, []string) error
	authOnly    bool
	guestOnly   bool
}

func NewShell(in io.Reader, out io.Writer, service *auth.Service) *Shell {
	s := &Shell{
		in:      in,
		out:     out,
		auth:    service,
		printer: NewPrinter(out),
	}
	s.commandSet = map[string]command{
		"register":   {description: "create a new user", handler: s.register, guestOnly: true},
		"login":      {description: "login with username/password and optional TOTP", handler: s.login, guestOnly: true},
		"whoami":     {description: "show current user details", handler: s.whoami, authOnly: true},
		"enable-2fa": {description: "enable Google Authenticator compatible MFA", handler: s.enable2FA, authOnly: true},
		"disable-2fa": {description: "disable MFA after password and TOTP verification",
			handler: s.disable2FA, authOnly: true},
		"logout": {description: "end current session", handler: s.logout, authOnly: true},
		"help":   {description: "show available commands", handler: s.help},
		"exit":   {description: "quit program", handler: s.exit, guestOnly: true},
		"clear":  {description: "clear the terminal screen", handler: s.clear},
	}
	s.guestCompleter = readline.NewPrefixCompleter(
		readline.PcItem("register"),
		readline.PcItem("login"),
		readline.PcItem("help"),
		readline.PcItem("exit"),
		readline.PcItem("clear"),
	)
	s.authCompleter = readline.NewPrefixCompleter(
		readline.PcItem("whoami"),
		readline.PcItem("enable-2fa"),
		readline.PcItem("disable-2fa"),
		readline.PcItem("logout"),
		readline.PcItem("help"),
		readline.PcItem("clear"),
	)
	s.completer = s.guestCompleter
	return s
}

func (s *Shell) Run(ctx context.Context) error {
	cfg := &readline.Config{
		Prompt:          s.prompt(),
		HistoryFile:     historyFile(),
		AutoComplete:    s.completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stdout:          s.out,
		Stderr:          s.out,
	}
	if stdin, ok := s.in.(io.ReadCloser); ok {
		cfg.Stdin = stdin
	}
	var err error
	s.rl, err = readline.NewEx(cfg)
	if err != nil {
		return err
	}
	defer s.rl.Close()

	s.printBanner()
	for {
		s.rl.SetPrompt(s.prompt())
		line, err := s.rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.dispatch(ctx, strings.Fields(line)); err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			s.printer.Error(err.Error())
		}
	}
}

func (s *Shell) dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return nil
	}
	cmd, ok := s.commandSet[args[0]]
	if !ok {
		return fmt.Errorf("unknown command %q; type help", args[0])
	}
	loggedIn := s.sessionID != ""
	if loggedIn {
		if _, err := s.auth.Current(s.sessionID); err != nil {
			s.sessionID = ""
			s.username = ""
			s.updateCompleter()
			s.printer.Warning("Session expired. Please login again.")
			loggedIn = false
		}
	}
	if cmd.authOnly && !loggedIn {
		return errors.New("Please login first")
	}
	if cmd.guestOnly && loggedIn {
		if args[0] == "exit" {
			return errors.New("Please logout before exiting the application")
		}
		return errors.New("Logout before using this command")
	}
	return cmd.handler(ctx, args[1:])
}

func (s *Shell) prompt() string {
	if s.sessionID == "" {
		return "guest > "
	}

	return fmt.Sprintf("%s > ", s.username)
}

func (s *Shell) register(ctx context.Context, _ []string) error {
	username, err := readLine("Username: ")
	if err != nil {
		return err
	}
	password, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	if _, err := s.auth.Register(ctx, username, password); err != nil {
		return err
	}
	s.printer.Success("Registration successful. You can now login.")
	return nil
}

func (s *Shell) login(ctx context.Context, _ []string) error {
	username, err := readLine("Username: ")
	if err != nil {
		return err
	}
	password, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	result, err := s.auth.Login(ctx, username, password, "")
	if errors.Is(err, auth.ErrMFARequired) {
		code, codeErr := readLine("Authenticator code: ")
		if codeErr != nil {
			return codeErr
		}
		result, err = s.auth.Login(ctx, username, password, code)
	}
	if err != nil {
		return err
	}
	s.sessionID = result.Session.ID
	s.username = result.Session.User.Username
	s.updateCompleter()
	s.printWelcomeMessage(result.Session)
	s.printUserDetails(result.Session)
	return nil
}

func (s *Shell) whoami(ctx context.Context, _ []string) error {
	session, err := s.auth.RefreshSession(ctx, s.sessionID)
	if err != nil {
		return err
	}
	s.printUserDetails(session)
	return nil
}

func (s *Shell) enable2FA(ctx context.Context, _ []string) error {
	session, err := s.auth.Current(s.sessionID)
	if err != nil {
		return err
	}
	if session.User.MFAEnabled {
		return errors.New("MFA is already enabled")
	}
	secret, url, err := s.auth.BeginEnableTOTP(session.User.Username)
	if err != nil {
		return err
	}
	fmt.Fprintln(s.out, "Add this secret to Google Authenticator or another TOTP app:")
	fmt.Fprintf(s.out, "Secret: %s\n", secret)
	fmt.Fprintf(s.out, "otpauth URL: %s\n", url)
	code, err := readLine("Enter the current authenticator code to confirm: ")
	if err != nil {
		return err
	}
	if err := s.auth.ConfirmEnableTOTP(ctx, session.User.Username, secret, code); err != nil {
		return err
	}
	if _, err := s.auth.RefreshSession(ctx, s.sessionID); err != nil {
		return err
	}
	s.printer.Success("Two-Factor Authentication enabled successfully.")
	return nil
}

func (s *Shell) disable2FA(ctx context.Context, _ []string) error {
	session, err := s.auth.Current(s.sessionID)
	if err != nil {
		return err
	}
	password, err := readPassword("Password: ")
	if err != nil {
		return err
	}
	code := ""
	if session.User.MFAEnabled {
		code, err = readLine("Authenticator code: ")
		if err != nil {
			return err
		}
	}
	if err := s.auth.DisableTOTP(ctx, session.User.Username, password, code); err != nil {
		return err
	}
	if _, err := s.auth.RefreshSession(ctx, s.sessionID); err != nil {
		return err
	}
	s.printer.Success("Two-Factor Authentication disabled.")
	return nil
}

func (s *Shell) logout(context.Context, []string) error {
	s.auth.Logout(s.sessionID)
	s.sessionID = ""
	s.username = ""
	s.updateCompleter()
	s.printer.Success("Logged out successfully.")
	return nil
}

func (s *Shell) help(context.Context, []string) error {

	loggedIn := s.sessionID != ""

	s.printer.Heading("📖 CLI Help")
	fmt.Fprintln(s.out)

	if !loggedIn {

		fmt.Fprintln(s.out, "You are currently not logged in.")
		fmt.Fprintln(s.out)

		s.printer.PrintTable([]TableRow{
			{
				Icon:  "📝",
				Label: "register",
				Value: "Create a new user account",
			},
			{
				Icon:  "🔑",
				Label: "login",
				Value: "Login using your username and password",
			},
			{
				Icon:  "🧹",
				Label: "clear",
				Value: "Clear the terminal screen",
			},
			{
				Icon:  "📖",
				Label: "help",
				Value: "Display this help menu",
			},
			{
				Icon:  "🚪",
				Label: "exit",
				Value: "Exit the application",
			},
		})

		fmt.Fprintln(s.out)
		s.printer.Info("Register a new account or login to access more features.")

		return nil
	}

	fmt.Fprintln(s.out, "Logged in as:", s.username)
	fmt.Fprintln(s.out)

	s.printer.PrintTable([]TableRow{
		{
			Icon:  "👤",
			Label: "whoami",
			Value: "Display your account information",
		},
		{
			Icon:  "🔐",
			Label: "enable-2fa",
			Value: "Enable Two-Factor Authentication",
		},
		{
			Icon:  "🔓",
			Label: "disable-2fa",
			Value: "Disable Two-Factor Authentication",
		},
		{
			Icon:  "🚪",
			Label: "logout",
			Value: "Logout from the current session",
		},
		{
			Icon:  "🧹",
			Label: "clear",
			Value: "Clear the terminal screen",
		},
		{
			Icon:  "📖",
			Label: "help",
			Value: "Display this help menu",
		},
	})

	fmt.Fprintln(s.out)
	s.printer.Info("Logout first if you wish to exit the application.")

	return nil
}

func (s *Shell) clear(context.Context, []string) error {
	// Clear screen
	fmt.Fprint(s.out, "\033[2J")

	// Move cursor to home position
	fmt.Fprint(s.out, "\033[H")

	// Clear scrollback buffer (supported by many terminals)
	fmt.Fprint(s.out, "\033[3J")

	return nil
}

var errExit = errors.New("exit")

func (s *Shell) exit(context.Context, []string) error {
	return errExit
}

func (s *Shell) printUserDetails(session auth.Session) {

	user := session.User

	status := "🟡 Disabled"
	statusColor := yellow

	if user.MFAEnabled {
		status = "🟢 Enabled"
		statusColor = green
	}

	lastLogin := "Never"

	if user.LastLoginAt != nil {
		lastLogin = formatTime(*user.LastLoginAt)
	}

	s.printer.Heading("👤 Account Information")

	s.printer.PrintTable([]TableRow{
		{
			Icon:  "👤",
			Label: "Username",
			Value: user.Username,
		},
		{
			Icon:  "📅",
			Label: "Registered On",
			Value: formatTime(user.RegisteredAt),
		},
		{
			Icon:  "🔐",
			Label: "Two-Factor Authentication",
			Value: status,
			Color: statusColor,
		},
		{
			Icon:  "⏰",
			Label: "Session Expires",
			Value: formatTime(session.ExpiresAt),
		},
		{
			Icon:  "🕒",
			Label: "Last Login",
			Value: lastLogin,
		},
	})
}

func formatTime(t time.Time) string {
	return t.Local().Format("02 Jan 2006, 03:04:05 PM MST")
}

func (s *Shell) updateCompleter() {
	if s.rl == nil {
		return
	}

	if s.sessionID == "" {
		s.rl.Config.AutoComplete = s.guestCompleter
	} else {
		s.rl.Config.AutoComplete = s.authCompleter
	}
}

func (s *Shell) printBanner() {
	fmt.Fprintln(s.out)
	fmt.Fprintln(s.out, "╭────────────────────────────────────────────────────────────╮")
	fmt.Fprintln(s.out, "│                 🔐 CLI Login System                        │")
	fmt.Fprintln(s.out, "├────────────────────────────────────────────────────────────┤")
	fmt.Fprintln(s.out, "│ Securely manage your account from the command line.        │")
	fmt.Fprintln(s.out, "│                                                            │")
	fmt.Fprintln(s.out, "│  ✓ Register a new account                                  │")
	fmt.Fprintln(s.out, "│  ✓ Login securely                                          │")
	fmt.Fprintln(s.out, "│  ✓ Enable Two-Factor Authentication (2FA)                  │")
	fmt.Fprintln(s.out, "╰────────────────────────────────────────────────────────────╯")
	fmt.Fprintln(s.out)
	s.printer.Heading("Getting Started")
	fmt.Fprintln(s.out, "────────────────")
	fmt.Fprintln(s.out, "  register    Create a new account")
	fmt.Fprintln(s.out, "  login       Sign in to your account")
	fmt.Fprintln(s.out, "  help        Show available commands")
	fmt.Fprintln(s.out)
}

func (s *Shell) printWelcomeMessage(session auth.Session) {

	user := session.User

	// First login after registration
	if user.LastLoginAt == nil {

		s.printer.Success(fmt.Sprintf("Welcome, %s!", user.Username))
		s.printer.Info("Your account has been created successfully.")

	} else {

		s.printer.Success(fmt.Sprintf("Welcome back, %s!", user.Username))
		s.printer.Info("You have successfully logged in.")
	}

	if user.MFAEnabled {
		s.printer.Success("Your account is protected with Two-Factor Authentication.")
	} else {
		s.printer.Warning("Two-Factor Authentication is disabled. Enable it to improve account security.")
	}

	remaining := time.Until(session.ExpiresAt).Round(time.Minute)

	s.printer.Info(fmt.Sprintf("Your session will expire in %v.", remaining))

	fmt.Fprintln(s.out)
}
