package servicescmd

import (
	"bytes"
	"fmt"

	"github.com/quarkloop/supervisor/pkg/api"
)

func formatServiceTable(services []api.ServiceInfo) string {
	var b bytes.Buffer
	if len(services) == 0 {
		return "No services installed.\n"
	}
	fmt.Fprintf(&b, "%-24s %-14s %-10s %-22s %s\n", "NAME", "STATUS", "VERSION", "ENDPOINT", "FUNCTIONS")
	for _, service := range services {
		endpoint := service.Endpoint
		if endpoint == "" {
			endpoint = "-"
		}
		fmt.Fprintf(&b, "%-24s %-14s %-10s %-22s %d\n", service.Name, service.Status, service.Version, endpoint, service.FunctionCount)
	}
	return b.String()
}

func formatServiceInspect(service api.ServiceInfo) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Name:        %s\n", service.Name)
	fmt.Fprintf(&b, "Status:      %s\n", service.Status)
	if service.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", service.Description)
	}
	if service.Version != "" {
		fmt.Fprintf(&b, "Version:     %s\n", service.Version)
	}
	if service.Endpoint != "" {
		fmt.Fprintf(&b, "Endpoint:    %s\n", service.Endpoint)
	}
	if service.AddressEnv != "" {
		fmt.Fprintf(&b, "Address Env: %s\n", service.AddressEnv)
	}
	if service.HealthService != "" {
		fmt.Fprintf(&b, "Health:      %s\n", service.HealthService)
	}
	if service.MinVersion != "" {
		fmt.Fprintf(&b, "Min Version: %s\n", service.MinVersion)
	}
	if len(service.Functions) > 0 {
		fmt.Fprintln(&b, "Functions:")
		for _, fn := range service.Functions {
			fmt.Fprintf(&b, "  %-28s %s/%s\n", fn.Name, fn.Service, fn.Method)
		}
	}
	if len(service.Diagnostics) > 0 {
		fmt.Fprintln(&b, "Diagnostics:")
		for _, diagnostic := range service.Diagnostics {
			fmt.Fprintf(&b, "  - %s\n", diagnostic)
		}
	}
	return b.String()
}

func formatServiceDoctor(resp api.ServiceDoctorResponse) string {
	var b bytes.Buffer
	b.WriteString(formatServiceTable(resp.Services))
	if len(resp.Issues) == 0 {
		b.WriteString("No service issues detected.\n")
		return b.String()
	}
	b.WriteString("Issues:\n")
	for _, issue := range resp.Issues {
		fmt.Fprintf(&b, "  - %s\n", issue)
	}
	return b.String()
}
