# Fish completion for git-manager
# Install by copying this file to ~/.config/fish/completions/ or
# add this to your ~/.config/fish/config.fish:
#   source /path/to/git-manager.fish

function __git_manager_using_subcommand --description 'Test if git-manager is currently using a subcommand'
	set -l tokens (commandline -opc)
	if test (count $tokens) -gt 1
		return 0
	end
	return 1
end

function __git_manager_current_subcommand --description 'Get the current subcommand'
	set -l tokens (commandline -opc)
	if test (count $tokens) -gt 1
		echo $tokens[2]
	end
end

# Main subcommands
complete -c git-manager -f
complete -c git-manager -n "not __git_manager_using_subcommand" -a "sync" -d "Synchronize repos with configured state"
complete -c git-manager -n "not __git_manager_using_subcommand" -a "status" -d "Report repo status without changes"
complete -c git-manager -n "not __git_manager_using_subcommand" -a "add" -d "Scaffold a config entry from existing checkout"
complete -c git-manager -n "not __git_manager_using_subcommand" -a "install" -d "Install scheduler (systemd/launchd)"
complete -c git-manager -n "not __git_manager_using_subcommand" -a "uninstall" -d "Uninstall scheduler (systemd/launchd)"

# Global flags
complete -c git-manager -l help -d "Show help message"
complete -c git-manager -l version -d "Show version"

# Flags for 'sync' subcommand
complete -c git-manager -n "__fish_seen_subcommand_from sync" -l config -d "Path to config file (required)" -r
complete -c git-manager -n "__fish_seen_subcommand_from sync" -l dry-run -d "Report changes without applying"
complete -c git-manager -n "__fish_seen_subcommand_from sync" -l json -d "Output as JSON"
complete -c git-manager -n "__fish_seen_subcommand_from sync" -l parallel -d "Number of repos to sync in parallel" -r
complete -c git-manager -n "__fish_seen_subcommand_from sync" -l overwrite -d "Remove undeclared remotes"
complete -c git-manager -n "__fish_seen_subcommand_from sync" -l prune -d "Remove undeclared remotes"

# Flags for 'status' subcommand
complete -c git-manager -n "__fish_seen_subcommand_from status" -l config -d "Path to config file (required)" -r
complete -c git-manager -n "__fish_seen_subcommand_from status" -l json -d "Output as JSON"
complete -c git-manager -n "__fish_seen_subcommand_from status" -l parallel -d "Number of repos to check in parallel" -r

# Flags for 'add' subcommand
complete -c git-manager -n "__fish_seen_subcommand_from add" -l config -d "Path to config file (required)" -r
complete -c git-manager -n "__fish_seen_subcommand_from add" -l group -d "Group name (required)" -r
complete -c git-manager -n "__fish_seen_subcommand_from add" -l name -d "Repo name (required)" -r
complete -c git-manager -n "__fish_seen_subcommand_from add" -l dry-run -d "Print generated entry without writing"

# Flags for 'install' subcommand
complete -c git-manager -n "__fish_seen_subcommand_from install" -l dry-run -d "Report changes without applying"

# Flags for 'uninstall' subcommand
complete -c git-manager -n "__fish_seen_subcommand_from uninstall" -l dry-run -d "Report changes without applying"
