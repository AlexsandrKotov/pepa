#!/usr/bin/env python3
"""Bulk replace log.Printf/Println with slog calls in Go files."""
import re
import os
import sys

def convert_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()
    
    original = content
    
    # Track if file uses log.Fatal (needs os.Exit replacement)
    has_fatal = 'log.Fatal(' in content
    
    # Replace import "log" with "log/slog" (but not if already has log/slog)
    if '"log"' in content and '"log/slog"' not in content:
        content = content.replace('"log"', '"log/slog"')
    elif '"log"' in content and '"log/slog"' in content:
        # Has both, remove "log"
        content = content.replace('\t"log"\n', '')
    
    # Replace log.Println("message") with slog.Info("message")
    content = re.sub(
        r'log\.Println\("([^"]*)"\)',
        r'slog.Info("\1")',
        content
    )
    
    # Replace log.Printf with slog calls based on pattern
    # Pattern: log.Printf("Warning: ...%v", err) -> slog.Warn("...", "error", err)
    # Pattern: log.Printf("...%s...%v", name, err) -> slog.Info("...", "key", name, "error", err)
    
    def convert_printf(match):
        full = match.group(0)
        fmt_str = match.group(1)
        args = match.group(2) if match.group(2) else ""
        
        # Determine log level
        level = "Info"
        msg = fmt_str
        if fmt_str.startswith("Warning: "):
            level = "Warn"
            msg = fmt_str[len("Warning: "):]
        elif fmt_str.startswith("[SECURITY]"):
            level = "Warn"
            msg = fmt_str
        
        # Remove prefixes like [plugin-manager], [SECURITY]
        msg = re.sub(r'\[.*?\]\s*', '', msg).strip()
        
        # If no format verbs, simple replacement
        if '%' not in fmt_str:
            return f'slog.{level}("{msg}")'
        
        # Extract format verbs and build key-value pairs
        verbs = re.findall(r'%[svdfqtTwxXbp]', fmt_str)
        if not verbs:
            return f'slog.{level}("{msg}")'
        
        # Clean message: remove format verbs, make it a static string
        clean_msg = msg
        clean_msg = re.sub(r'%[svdfqtTwxXbp]', '{}', clean_msg)
        
        # Build key-value pairs from args
        arg_list = [a.strip() for a in args.split(',') if a.strip()] if args else []
        
        kv_pairs = []
        for i, verb in enumerate(verbs):
            if i < len(arg_list):
                arg = arg_list[i]
                # Guess key name from context
                key = guess_key_name(arg, i, clean_msg)
                kv_pairs.append(f'"{key}", {arg}')
        
        # Replace {} placeholders with %s for readability in the message
        # Actually for slog, the message should be static
        # Remove the {} from message
        clean_msg = re.sub(r'\{\}', '', clean_msg).strip()
        clean_msg = re.sub(r'\s+', ' ', clean_msg).strip()
        # Remove trailing punctuation
        clean_msg = clean_msg.rstrip(':.,')
        
        if kv_pairs:
            return f'slog.{level}("{clean_msg}", {", ".join(kv_pairs)})'
        return f'slog.{level}("{clean_msg}")'
    
    def guess_key_name(arg, idx, msg):
        """Guess a meaningful key name from the argument and context."""
        arg_lower = arg.lower()
        if 'err' in arg_lower:
            return 'error'
        if 'name' in arg_lower:
            return 'name'
        if 'id' in arg_lower:
            return 'id'
        if 'count' in arg_lower or 'len(' in arg_lower:
            return 'count'
        if 'addr' in arg_lower or 'host' in arg_lower:
            return 'addr'
        if 'version' in arg_lower:
            return 'version'
        if 'type' in arg_lower:
            return 'type'
        if 'dir' in arg_lower or 'path' in arg_lower:
            return 'path'
        if 'duration' in arg_lower or 'since' in arg_lower:
            return 'duration'
        # Default keys based on position
        defaults = ['arg1', 'arg2', 'arg3', 'arg4', 'arg5']
        if idx < len(defaults):
            return defaults[idx]
        return f'arg{idx+1}'
    
    # Match log.Printf("...", args...)
    content = re.sub(
        r'log\.Printf\("((?:[^"\\]|\\.)*)"(?:,\s*(.+?))?\)',
        convert_printf,
        content
    )
    
    # Replace log.Fatal with slog.Error + os.Exit
    if has_fatal:
        content = re.sub(
            r'log\.Fatal\("([^"]*)"\)',
            r'slog.Error("\1")\n\t\tos.Exit(1)',
            content
        )
        content = re.sub(
            r'log\.Fatalf\("((?:[^"\\]|\\.)*)"(?:,\s*(.+?))?\)',
            r'slog.Error("\1")\n\t\tos.Exit(1)',
            content
        )
        # Ensure "os" is imported if we added os.Exit
        if 'os.Exit' in content and '"os"' not in content:
            content = content.replace('"log/slog"', '"log/slog"\n\t"os"')
    
    if content != original:
        with open(filepath, 'w') as f:
            f.write(content)
        return True
    return False

def main():
    files = [
        "./internal/ai/agent.go",
        "./internal/ai/agent_tools.go",
        "./internal/ai/http_provider.go",
        "./internal/ai/intent_router.go",
        "./internal/ai/manager.go",
        "./internal/api/rest/ai_handlers.go",
        "./internal/api/rest/audit_handlers.go",
        "./internal/api/rest/auth_handlers.go",
        "./internal/api/rest/cluster_handlers.go",
        "./internal/api/rest/connection_handlers.go",
        "./internal/api/rest/deployment_handlers.go",
        "./internal/api/rest/discovery_handlers.go",
        "./internal/api/rest/docker_host_handlers.go",
        "./internal/api/rest/gitops_handlers.go",
        "./internal/api/rest/helm_repo_handlers.go",
        "./internal/api/rest/helpers.go",
        "./internal/api/rest/marketplace_handlers.go",
        "./internal/api/rest/observability_handlers.go",
        "./internal/api/rest/organization_handlers.go",
        "./internal/api/rest/plugin_handlers.go",
        "./internal/api/rest/s3browser_handlers.go",
        "./internal/api/rest/service_handlers.go",
        "./internal/api/rest/storage_handlers.go",
        "./internal/api/rest/team_handlers.go",
        "./internal/api/rest/user_credential_handlers.go",
        "./internal/api/rest/vault_handlers.go",
        "./internal/api/rest/workflow_handlers.go",
        "./internal/crypto/crypto.go",
        "./internal/events/bus.go",
        "./internal/gitops/repository.go",
        "./internal/gitops/scanner.go",
        "./internal/gitops/tracker.go",
        "./internal/k8s/deployer.go",
        "./internal/k8s/helm_deployer.go",
        "./internal/plugin/sdk-go/sdk.go",
        "./internal/queue/queue.go",
        "./internal/repository/scorecard_repo.go",
        "./internal/repository/service_repo.go",
        "./internal/service/deployment_service.go",
        "./internal/service/service_deployment_service.go",
        "./internal/storage/local.go",
        "./internal/storage/s3.go",
        "./internal/workflow/engine.go",
        "./plugins/argocd/main.go",
        "./plugins/examples/example_plugin.go",
        "./plugins/premium/jira/main.go",
    ]
    
    changed = 0
    for f in files:
        if os.path.exists(f):
            if convert_file(f):
                changed += 1
                print(f"  updated: {f}")
        else:
            print(f"  missing: {f}")
    
    print(f"\n{changed}/{len(files)} files updated")

if __name__ == '__main__':
    main()
