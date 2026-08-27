package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

var (
	apiURL string
)

func init() {
	apiURL = os.Getenv("PEPA_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8088"
	}
}

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = newRootCmd()

func newRootCmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "pepa",
		Short: "PEPA — Platform Engineering & Pipeline Automator CLI",
		Long: `PEPA CLI — manage entities, workflows, plugins, scorecards, and audit logs
from the command line.

Environment:
  PEPA_API_URL   API server URL (default: http://localhost:8088)
  PEPA_API_USER  API user email (default: admin@pepa.io)`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags = newFlagSet()
	cmd.PersistentFlags.StringVar(&apiURL, "api-url", apiURL, "PEPA API server URL")

	cmd.AddCommand(newEntityCmd())
	cmd.AddCommand(newWorkflowCmd())
	cmd.AddCommand(newPluginCmd())
	cmd.AddCommand(newScorecardCmd())
	cmd.AddCommand(newAuditCmd())
	cmd.AddCommand(newHealthCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newRoleCmd())

	return cmd
}

// ── Minimal cobra-like command framework (self-contained) ─────────────

type cobraCmd struct {
	Use             string
	Short           string
	Long            string
	SilenceUsage    bool
	SilenceErrors   bool
	RunE            func(cmd *cobraCmd, args []string) error
	PersistentFlags *flagSet
	commands        []*cobraCmd
	parent          *cobraCmd
}

type flagSet struct {
	flags map[string]*string
}

func newFlagSet() *flagSet {
	return &flagSet{flags: make(map[string]*string)}
}

func (f *flagSet) StringVar(p *string, name, value, usage string) {
	*p = value
	f.flags[name] = p
}

func (c *cobraCmd) AddCommand(sub *cobraCmd) {
	sub.parent = c
	c.commands = append(c.commands, sub)
}

func (c *cobraCmd) Execute() error {
	args := os.Args[1:]
	return c.execute(args)
}

func (c *cobraCmd) execute(args []string) error {
	// Parse persistent flags
	remaining := c.parseFlags(args)

	// Find subcommand
	if len(remaining) > 0 {
		for _, sub := range c.commands {
			// Extract the command name (first word of Use)
			cmdName := sub.Use
			if idx := strings.IndexByte(sub.Use, ' '); idx >= 0 {
				cmdName = sub.Use[:idx]
			}
			if cmdName == remaining[0] {
				return sub.execute(remaining[1:])
			}
		}
	}

	// If has RunE, execute it
	if c.RunE != nil {
		return c.RunE(c, remaining)
	}

	// Otherwise show help
	c.printHelp()
	return nil
}

func (c *cobraCmd) parseFlags(args []string) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		if len(args[i]) > 2 && args[i][:2] == "--" {
			name := args[i][2:]
			if c.PersistentFlags != nil {
				if ptr, ok := c.PersistentFlags.flags[name]; ok && i+1 < len(args) {
					*ptr = args[i+1]
					i++
					continue
				}
			}
			// Check parent flags
			if c.parent != nil && c.parent.PersistentFlags != nil {
				if ptr, ok := c.parent.PersistentFlags.flags[name]; ok && i+1 < len(args) {
					*ptr = args[i+1]
					i++
					continue
				}
			}
		}
		remaining = append(remaining, args[i])
	}
	return remaining
}

func (c *cobraCmd) printHelp() {
	if c.Long != "" {
		fmt.Println(c.Long)
	} else if c.Short != "" {
		fmt.Println(c.Short)
	}
	fmt.Println()
	fmt.Println("Usage:")
	if len(c.commands) > 0 {
		fmt.Printf("  %s [command]\n\n", c.fullUse())
		fmt.Println("Available Commands:")
		for _, sub := range c.commands {
			fmt.Printf("  %-16s %s\n", sub.Use, sub.Short)
		}
	} else {
		fmt.Printf("  %s\n", c.fullUse())
	}
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Printf("  --api-url string   PEPA API server URL (default \"http://localhost:8088\")\n")
	fmt.Println()
	fmt.Println("Use \"" + c.fullUse() + " [command] --help\" for more information about a command.")
}

func (c *cobraCmd) fullUse() string {
	if c.parent != nil {
		return c.parent.fullUse() + " " + c.Use
	}
	return c.Use
}

// ── HTTP helpers ─────────────────────────────────────────────────────

func doRequest(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, apiURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// In dev mode the API accepts requests without a token (default tenant)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var errBody map[string]interface{}
		json.Unmarshal(data, &errBody)
		if msg, ok := errBody["error"].(string); ok {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	return data, nil
}

func printJSON(data []byte) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(data))
	}
}

func printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, joinTab(headers))
	for _, row := range rows {
		fmt.Fprintln(w, joinTab(row))
	}
	w.Flush()
}

func joinTab(cols []string) string {
	result := ""
	for i, c := range cols {
		if i > 0 {
			result += "\t"
		}
		result += c
	}
	return result
}
