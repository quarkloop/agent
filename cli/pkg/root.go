package pkg

import (
	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/commands"
)

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "quark [OPTIONS] COMMAND",
		Short:         "Quark — the agent runtime",
		Long:          "Your agents. Your machine. Fully Autonomous.\n\nQuark is a runtime for autonomous agents.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddGroup(&cobra.Group{
		ID:    "space",
		Title: "Commands:",
	})
	root.AddGroup(&cobra.Group{
		ID:    "data",
		Title: "Data Commands:",
	})
	root.AddGroup(&cobra.Group{
		ID:    "management",
		Title: "Management Commands:",
	})

	root.PersistentFlags().String("agent-url", "", "Agent HTTP URL (omit for local mode)")
	root.PersistentFlags().String("space", "", "Space name (or set QUARK_SPACE)")

	commands.RegisterCommands(root)

	return root
}
