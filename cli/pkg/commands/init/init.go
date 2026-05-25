package initcmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	spacemodel "github.com/quarkloop/pkg/space"
)

var workDir string

const (
	initLong = `Register a new space with the supervisor.

The <name> argument is used as the space identifier. The Space service persists
its authoritative configuration; the working directory is only a referenced
workspace and is not mutated by this command.

The command refuses to run if a space with the same name is already registered.`
	initExample = `  # Create a new space in ./my-space (default)
  quark init my-space

  # Create a space in the current directory
  quark init my-space --work-dir .

  # Create a space in an existing directory
  quark init my-space --work-dir ./projects/existing-dir`
)

// NewInitCommand returns the "init" command.
func NewInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init <name>",
		Short:   "Register a space with the supervisor",
		Long:    initLong,
		Example: initExample,
		Args:    cobra.ExactArgs(1),
		RunE:    runInit,
	}

	cmd.Flags().StringVar(&workDir, "work-dir", "", "Working directory (defaults to ./<name>)")
	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	name := args[0]

	dir := workDir
	if dir == "" {
		dir = name
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	control, err := natsclient.ConnectFromEnv(cmd.Context())
	if err != nil {
		return err
	}
	defer control.Close()

	_, err = control.GetSpace(cmd.Context(), name)
	if err == nil {
		fmt.Printf("Space %q is already registered.\n", name)
		return nil
	}
	if !natsclient.IsNotFound(err) {
		return fmt.Errorf("check space: %w", err)
	}

	data, err := spacemodel.MarshalConfig(spacemodel.NewConfig(name, abs))
	if err != nil {
		return fmt.Errorf("build space config: %w", err)
	}

	info, err := control.CreateSpace(cmd.Context(), clientcontract.CreateSpaceRequest{
		Config: data,
	})
	if err != nil {
		if natsclient.IsConflict(err) {
			fmt.Printf("Space %q is already registered.\n", name)
			return nil
		}
		return fmt.Errorf("register space: %w", err)
	}

	fmt.Printf("Space initialised: %s", info.Name)
	if info.Version != "" {
		fmt.Printf(" (version %s)", info.Version)
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Select this space with --space %s or QUARK_SPACE=%s\n", info.Name, info.Name)
	fmt.Println("  2. quark plugin install <ref>")
	fmt.Println("  3. Start runtime and services through deploy/compose or systemd")

	return nil
}
