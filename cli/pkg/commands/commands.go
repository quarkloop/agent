package commands

import (
	"github.com/spf13/cobra"

	activitycmd "github.com/quarkloop/cli/pkg/commands/activity"
	auditcmd "github.com/quarkloop/cli/pkg/commands/audit"
	chatcmd "github.com/quarkloop/cli/pkg/commands/chat"
	configcmd "github.com/quarkloop/cli/pkg/commands/config"
	contextcmd "github.com/quarkloop/cli/pkg/commands/context"
	doctorcmd "github.com/quarkloop/cli/pkg/commands/doctor"
	initcmd "github.com/quarkloop/cli/pkg/commands/init"
	plancmd "github.com/quarkloop/cli/pkg/commands/plan"
	plugincmd "github.com/quarkloop/cli/pkg/commands/plugin"
	runtimecmd "github.com/quarkloop/cli/pkg/commands/runtime"
	servicescmd "github.com/quarkloop/cli/pkg/commands/services"
	sessioncmd "github.com/quarkloop/cli/pkg/commands/session"
	versioncmd "github.com/quarkloop/cli/pkg/commands/version"
)

func RegisterCommands(root *cobra.Command) {
	// Space Commands — agent lifecycle + space operations.
	addGroup("space", root,
		runtimecmd.NewInspectCommand(),
		initcmd.NewInitCommand(),
		doctorcmd.NewDoctorCommand(),
		versioncmd.NewVersionCommand(),
	)

	// Data Commands — session, config, plan, activity management.
	addGroup("data", root,
		chatcmd.NewChatCommand(),
		sessioncmd.NewSessionCommand(),
		configcmd.NewConfigCommand(),
		plancmd.NewPlanCommand(),
		activitycmd.NewActivityCommand(),
		contextcmd.NewContextCommand(),
	)

	// Management Commands — plugin manager and validation.
	addGroup("management", root,
		plugincmd.NewPluginCommand(),
		servicescmd.NewServicesCommand(),
		auditcmd.NewAuditCommand(),
	)
}

func addGroup(groupID string, root *cobra.Command, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = groupID
		root.AddCommand(c)
	}
}
