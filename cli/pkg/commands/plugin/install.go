package plugincmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/spacecontext"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <ref>",
		Short: "Install a plugin into the current space",
		Long: `Install a plugin into the current space. The supervisor performs the
install and updates the installed catalog; the CLI only sends the request.

  quark plugin install bash                       # hub or registry name
  quark plugin install github.com/user/tool-foo   # git URL
  quark plugin install ./local-plugin/            # local path`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := spacecontext.FromCommand(cmd)
			if err != nil {
				return err
			}
			control, err := connectControl(cmd)
			if err != nil {
				return err
			}
			defer control.Close()
			p, err := control.InstallPlugin(cmd.Context(), name, args[0])
			if err != nil {
				return fmt.Errorf("install failed: %w", err)
			}
			fmt.Printf("Installed %s %s (%s)\n", p.Name, p.Version, p.Type)
			return nil
		},
	}
}
