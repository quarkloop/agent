package systemsvc

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
)

type commandInventory struct{}

func (commandInventory) DefaultPackageManager() string {
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		return "dpkg"
	}
	if _, err := exec.LookPath("rpm"); err == nil {
		return "rpm"
	}
	return ""
}

func (commandInventory) Packages(ctx context.Context, manager string, limit int) ([]*systemv1.Package, error) {
	var cmd *exec.Cmd
	switch manager {
	case "dpkg":
		cmd = exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${Package}\t${Version}\t${Architecture}\n")
	case "rpm":
		cmd = exec.CommandContext(ctx, "rpm", "-qa", "--qf", "%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\n")
	default:
		return nil, nil
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s package inventory: %w", manager, err)
	}
	return packageLines(string(out), manager, limit), nil
}

func (commandInventory) Services(ctx context.Context, manager, state string, limit int) ([]*systemv1.Service, error) {
	if manager != "systemd" {
		return nil, nil
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--no-pager", "--plain", "--all").Output()
	if err != nil {
		return nil, fmt.Errorf("systemd service inventory: %w", err)
	}
	services := make([]*systemv1.Service, 0)
	state = strings.ToLower(strings.TrimSpace(state))
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || !strings.HasSuffix(fields[0], ".service") || (state != "" && fields[2] != state) {
			continue
		}
		description := ""
		if len(fields) > 4 {
			description = strings.Join(fields[4:], " ")
		}
		services = append(services, &systemv1.Service{Name: fields[0], State: fields[2], Manager: "systemd", Description: description})
		if len(services) >= limit {
			break
		}
	}
	return services, scanner.Err()
}

func packageLines(value, manager string, limit int) []*systemv1.Package {
	packages := make([]*systemv1.Package, 0)
	for _, line := range strings.Split(value, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) >= 3 {
			packages = append(packages, &systemv1.Package{Name: parts[0], Version: parts[1], Architecture: parts[2], Manager: manager})
			if len(packages) >= limit {
				break
			}
		}
	}
	return packages
}
