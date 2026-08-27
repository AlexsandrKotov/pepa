package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func newEntityCmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "entity",
		Short: "Manage entities (list, get, create, update, delete, graph)",
	}

	cmd.AddCommand(&cobraCmd{
		Use:   "list",
		Short: "List all entities",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/entities", nil)
			if err != nil {
				return err
			}
			var result struct {
				Items []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					TypeKey     string `json:"type_key"`
					Status      string `json:"status"`
					Description string `json:"description"`
				} `json:"items"`
				Total int `json:"total"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return err
			}
			if len(result.Items) == 0 {
				fmt.Println("No entities found.")
				return nil
			}
			headers := []string{"ID", "NAME", "TYPE", "STATUS", "DESCRIPTION"}
			var rows [][]string
			for _, e := range result.Items {
				desc := e.Description
				if len(desc) > 40 {
					desc = desc[:40] + "..."
				}
				rows = append(rows, []string{
					e.ID[:8] + "...",
					e.Name,
					e.TypeKey,
					e.Status,
					desc,
				})
			}
			printTable(headers, rows)
			fmt.Printf("\nTotal: %d entities\n", result.Total)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "get [id]",
		Short: "Get entity details",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa entity get [id]")
			}
			data, err := doRequest("GET", "/api/v1/entities/"+args[0], nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "create",
		Short: "Create a new entity",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 3 {
				return fmt.Errorf("usage: pepa entity create [name] [type_key] [description]")
			}
			body := map[string]interface{}{
				"name":        args[0],
				"type_key":    args[1],
				"description": strings.Join(args[2:], " "),
			}
			data, err := doRequest("POST", "/api/v1/entities", body)
			if err != nil {
				return err
			}
			var result struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			_ = json.Unmarshal(data, &result)
			fmt.Printf("Entity created: %s (ID: %s)\n", result.Name, result.ID)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "delete [id]",
		Short: "Delete an entity",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa entity delete [id]")
			}
			_, err := doRequest("DELETE", "/api/v1/entities/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Printf("Entity %s deleted.\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "graph [id]",
		Short: "Show entity graph",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa entity graph [id]")
			}
			data, err := doRequest("GET", "/api/v1/entities/"+args[0]+"/graph?depth=2", nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "update [id] [json]",
		Short: "Update entity fields",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("usage: pepa entity update [id] '{\"name\":\"new-name\"}'")
			}
			var body map[string]interface{}
			if err := json.Unmarshal([]byte(args[1]), &body); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			data, err := doRequest("PUT", "/api/v1/entities/"+args[0], body)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	return cmd
}

func newWorkflowCmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "workflow",
		Short: "Manage workflows (list, get, create, delete, execute, logs)",
	}

	cmd.AddCommand(&cobraCmd{
		Use:   "list",
		Short: "List all workflows",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/workflows", nil)
			if err != nil {
				return err
			}
			var result struct {
				Workflows []struct {
					ID        string `json:"id"`
					Name      string `json:"name"`
					Version   int    `json:"version"`
					IsEnabled bool   `json:"is_enabled"`
					Source    string `json:"source"`
				} `json:"workflows"`
				Total int `json:"total"`
			}
			_ = json.Unmarshal(data, &result)
			if len(result.Workflows) == 0 {
				fmt.Println("No workflows found.")
				return nil
			}
			headers := []string{"ID", "NAME", "VERSION", "ENABLED", "SOURCE"}
			var rows [][]string
			for _, w := range result.Workflows {
				rows = append(rows, []string{
					w.ID[:8] + "...",
					w.Name,
					fmt.Sprintf("v%d", w.Version),
					fmt.Sprintf("%v", w.IsEnabled),
					w.Source,
				})
			}
			printTable(headers, rows)
			fmt.Printf("\nTotal: %d workflows\n", result.Total)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "get [id]",
		Short: "Get workflow details",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa workflow get [id]")
			}
			data, err := doRequest("GET", "/api/v1/workflows/"+args[0], nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "create [json-file]",
		Short: "Create workflow from JSON file",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa workflow create [json-file]")
			}
			fileData, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("cannot read file: %w", err)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(fileData, &body); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			data, err := doRequest("POST", "/api/v1/workflows", body)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "delete [id]",
		Short: "Delete a workflow",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa workflow delete [id]")
			}
			_, err := doRequest("DELETE", "/api/v1/workflows/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Printf("Workflow %s deleted.\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "execute [id] [params-json]",
		Short: "Execute a workflow",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa workflow execute [id] [params-json]")
			}
			body := map[string]interface{}{}
			if len(args) > 1 {
				var params map[string]interface{}
				if err := json.Unmarshal([]byte(args[1]), &params); err != nil {
					return fmt.Errorf("invalid params JSON: %w", err)
				}
				body["parameters"] = params
			}
			data, err := doRequest("POST", "/api/v1/workflows/"+args[0]+"/execute", body)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "executions [id]",
		Short: "List workflow executions",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa workflow executions [id]")
			}
			data, err := doRequest("GET", "/api/v1/workflows/"+args[0]+"/executions", nil)
			if err != nil {
				return err
			}
			var result struct {
				Executions []struct {
					ID          string `json:"id"`
					Status      string `json:"status"`
					TriggerType string `json:"trigger_type"`
					DurationMs  *int   `json:"duration_ms"`
					CreatedAt   string `json:"created_at"`
				} `json:"executions"`
			}
			_ = json.Unmarshal(data, &result)
			if len(result.Executions) == 0 {
				fmt.Println("No executions found.")
				return nil
			}
			headers := []string{"ID", "STATUS", "TRIGGER", "DURATION", "CREATED"}
			var rows [][]string
			for _, e := range result.Executions {
				dur := "—"
				if e.DurationMs != nil {
					dur = fmt.Sprintf("%dms", *e.DurationMs)
				}
				rows = append(rows, []string{
					e.ID[:8] + "...",
					e.Status,
					e.TriggerType,
					dur,
					e.CreatedAt[:19],
				})
			}
			printTable(headers, rows)
			return nil
		},
	})

	return cmd
}

func newPluginCmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "plugin",
		Short: "Manage plugins (list, get, install, enable, disable)",
	}

	cmd.AddCommand(&cobraCmd{
		Use:   "list",
		Short: "List all plugins",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/plugins", nil)
			if err != nil {
				return err
			}
			var result struct {
				Plugins []struct {
					Name    string `json:"name"`
					Version string `json:"version"`
					Type    string `json:"type"`
					Enabled bool   `json:"enabled"`
					Status  string `json:"status"`
				} `json:"plugins"`
			}
			_ = json.Unmarshal(data, &result)
			if len(result.Plugins) == 0 {
				fmt.Println("No plugins installed.")
				return nil
			}
			headers := []string{"NAME", "VERSION", "TYPE", "ENABLED", "STATUS"}
			var rows [][]string
			for _, p := range result.Plugins {
				rows = append(rows, []string{
					p.Name,
					p.Version,
					p.Type,
					fmt.Sprintf("%v", p.Enabled),
					p.Status,
				})
			}
			printTable(headers, rows)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "get [name]",
		Short: "Get plugin details",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa plugin get [name]")
			}
			data, err := doRequest("GET", "/api/v1/plugins/"+args[0], nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "install [json-file]",
		Short: "Install plugin from JSON file",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa plugin install [json-file]")
			}
			fileData, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("cannot read file: %w", err)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(fileData, &body); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			data, err := doRequest("POST", "/api/v1/plugins/install", body)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "enable [name]",
		Short: "Enable a plugin",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa plugin enable [name]")
			}
			_, err := doRequest("POST", "/api/v1/plugins/"+args[0]+"/enable", nil)
			if err != nil {
				return err
			}
			fmt.Printf("Plugin %s enabled.\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "disable [name]",
		Short: "Disable a plugin",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa plugin disable [name]")
			}
			_, err := doRequest("POST", "/api/v1/plugins/"+args[0]+"/disable", nil)
			if err != nil {
				return err
			}
			fmt.Printf("Plugin %s disabled.\n", args[0])
			return nil
		},
	})

	return cmd
}

func newScorecardCmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "scorecard",
		Short: "Manage scorecards (list, get, create, evaluate, results)",
	}

	cmd.AddCommand(&cobraCmd{
		Use:   "list",
		Short: "List all scorecards",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/scorecards", nil)
			if err != nil {
				return err
			}
			var result struct {
				Scorecards []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Description string `json:"description"`
					Enabled     bool   `json:"enabled"`
				} `json:"scorecards"`
				Total int `json:"total"`
			}
			_ = json.Unmarshal(data, &result)
			if len(result.Scorecards) == 0 {
				fmt.Println("No scorecards found.")
				return nil
			}
			headers := []string{"ID", "NAME", "ENABLED", "DESCRIPTION"}
			var rows [][]string
			for _, s := range result.Scorecards {
				desc := s.Description
				if len(desc) > 40 {
					desc = desc[:40] + "..."
				}
				rows = append(rows, []string{
					s.ID[:8] + "...",
					s.Name,
					fmt.Sprintf("%v", s.Enabled),
					desc,
				})
			}
			printTable(headers, rows)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "get [id]",
		Short: "Get scorecard details",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa scorecard get [id]")
			}
			data, err := doRequest("GET", "/api/v1/scorecards/"+args[0], nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "evaluate [scorecard-id] [entity-id]",
		Short: "Evaluate scorecard for an entity",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("usage: pepa scorecard evaluate [scorecard-id] [entity-id]")
			}
			body := map[string]string{"entity_id": args[1]}
			data, err := doRequest("POST", "/api/v1/scorecards/"+args[0]+"/evaluate", body)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "results [id]",
		Short: "List scorecard evaluation results",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa scorecard results [id]")
			}
			data, err := doRequest("GET", "/api/v1/scorecards/"+args[0]+"/results", nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	return cmd
}

func newAuditCmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "audit",
		Short: "View audit log (list, stats)",
	}

	cmd.AddCommand(&cobraCmd{
		Use:   "list",
		Short: "List audit entries",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/audit?per_page=20", nil)
			if err != nil {
				return err
			}
			var result struct {
				Items []struct {
					ID         string `json:"id"`
					Action     string `json:"action"`
					EntityType string `json:"entity_type"`
					UserID     string `json:"user_id"`
					CreatedAt  string `json:"created_at"`
				} `json:"items"`
				Total int `json:"total"`
			}
			_ = json.Unmarshal(data, &result)
			if len(result.Items) == 0 {
				fmt.Println("No audit entries found.")
				return nil
			}
			headers := []string{"ID", "ACTION", "RESOURCE", "USER", "CREATED"}
			var rows [][]string
			for _, a := range result.Items {
				user := a.UserID
				if user == "" {
					user = "system"
				}
				rows = append(rows, []string{
					a.ID[:8] + "...",
					a.Action,
					a.EntityType,
					user,
					a.CreatedAt[:19],
				})
			}
			printTable(headers, rows)
			fmt.Printf("\nTotal: %d entries\n", result.Total)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "stats",
		Short: "Show audit log statistics",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/audit/stats", nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	return cmd
}

func newHealthCmd() *cobraCmd {
	return &cobraCmd{
		Use:   "health",
		Short: "Check API health",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/healthz", nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	}
}

func newVersionCmd() *cobraCmd {
	return &cobraCmd{
		Use:   "version",
		Short: "Print CLI version",
		RunE: func(c *cobraCmd, args []string) error {
			fmt.Println("PEPA CLI v0.1.0")
			return nil
		},
	}
}

func newRoleCmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "role",
		Short: "Manage roles and permissions (list, create, delete, permissions, assign, check)",
	}

	cmd.AddCommand(&cobraCmd{
		Use:   "list",
		Short: "List all roles",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/roles", nil)
			if err != nil {
				return err
			}
			var result struct {
				Roles []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Slug        string `json:"slug"`
					Description string `json:"description"`
					IsSystem    bool   `json:"is_system"`
					Scope       string `json:"scope"`
				} `json:"roles"`
				Total int `json:"total"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return err
			}
			if len(result.Roles) == 0 {
				fmt.Println("No roles found.")
				return nil
			}
			headers := []string{"ID", "NAME", "SLUG", "SCOPE", "SYSTEM", "DESCRIPTION"}
			var rows [][]string
			for _, r := range result.Roles {
				sys := "no"
				if r.IsSystem {
					sys = "yes"
				}
				desc := r.Description
				if len(desc) > 40 {
					desc = desc[:40] + "..."
				}
				rows = append(rows, []string{
					r.ID[:8] + "...",
					r.Name,
					r.Slug,
					r.Scope,
					sys,
					desc,
				})
			}
			printTable(headers, rows)
			fmt.Printf("\nTotal: %d roles\n", result.Total)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "create [name] [slug] [description]",
		Short: "Create a new role",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("usage: pepa role create <name> <slug> [description]")
			}
			body := map[string]string{
				"name": args[0],
				"slug": args[1],
			}
			if len(args) > 2 {
				body["description"] = strings.Join(args[2:], " ")
			}
			data, err := doRequest("POST", "/api/v1/roles", body)
			if err != nil {
				return err
			}
			fmt.Println("Role created:")
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "delete [id]",
		Short: "Delete a role",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa role delete <role-id>")
			}
			data, err := doRequest("DELETE", "/api/v1/roles/"+args[0], nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "permissions [role-id]",
		Short: "List permissions for a role",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: pepa role permissions <role-id>")
			}
			data, err := doRequest("GET", "/api/v1/roles/"+args[0]+"/permissions", nil)
			if err != nil {
				return err
			}
			var result struct {
				Permissions []struct {
					Resource string `json:"resource"`
					Action   string `json:"action"`
					Effect   string `json:"effect"`
				} `json:"permissions"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return err
			}
			if len(result.Permissions) == 0 {
				fmt.Println("No permissions for this role.")
				return nil
			}
			headers := []string{"RESOURCE", "ACTION", "EFFECT"}
			var rows [][]string
			for _, p := range result.Permissions {
				rows = append(rows, []string{p.Resource, p.Action, p.Effect})
			}
			printTable(headers, rows)
			fmt.Printf("\nTotal: %d permissions\n", len(result.Permissions))
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "grant [role-id] [resource] [action]",
		Short: "Grant a permission to a role",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 3 {
				return fmt.Errorf("usage: pepa role grant <role-id> <resource> <action>")
			}
			body := map[string]string{
				"resource": args[1],
				"action":   args[2],
			}
			data, err := doRequest("POST", "/api/v1/roles/"+args[0]+"/permissions", body)
			if err != nil {
				return err
			}
			fmt.Println("Permission granted:")
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "assign [user-id] [role-id]",
		Short: "Assign a role to a user",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("usage: pepa role assign <user-id> <role-id>")
			}
			body := map[string]string{
				"user_id": args[0],
				"role_id": args[1],
			}
			data, err := doRequest("POST", "/api/v1/role-assignments", body)
			if err != nil {
				return err
			}
			fmt.Println("Role assigned:")
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "check [resource] [action]",
		Short: "Check if current user has a permission",
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("usage: pepa role check <resource> <action>")
			}
			data, err := doRequest("GET", "/api/v1/me/check?resource="+args[0]+"&action="+args[1], nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	cmd.AddCommand(&cobraCmd{
		Use:   "my",
		Short: "Show current user's roles",
		RunE: func(c *cobraCmd, args []string) error {
			data, err := doRequest("GET", "/api/v1/me/roles", nil)
			if err != nil {
				return err
			}
			printJSON(data)
			return nil
		},
	})

	return cmd
}
