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
	initLong = `Scaffold a new Quarkfile and register the space with the supervisor.

The <name> argument is used as the space identifier. A local Quarkfile is created
in the working directory, and the space is registered with the supervisor.

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
		Short:   "Scaffold a Quarkfile and register the space with the supervisor",
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

	data := spacemodel.DefaultQuarkfile(name)

	info, err := control.CreateSpace(cmd.Context(), clientcontract.CreateSpaceRequest{
		Name:       name,
		Quarkfile:  data,
		WorkingDir: abs,
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
	fmt.Printf("Quarkfile written to: %s\n", filepath.Join(abs, "Quarkfile"))
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit Quarkfile")
	fmt.Println("  2. quark plugin install <ref>")
	fmt.Println("  3. Start runtime and services through deploy/compose or systemd")

	return nil
}
