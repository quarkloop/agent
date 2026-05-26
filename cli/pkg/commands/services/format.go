package servicescmd

import (
	"bytes"
	"fmt"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func formatServiceTable(services []clientcontract.ServiceInfo) string {
	var b bytes.Buffer
	if len(services) == 0 {
		return "No services installed.\n"
	}
	fmt.Fprintf(&b, "%-24s %-14s %-10s %-22s %s\n", "NAME", "STATUS", "VERSION", "SUBJECT PREFIX", "FUNCTIONS")
	for _, service := range services {
		subjectPrefix := service.SubjectPrefix
		if subjectPrefix == "" {
			subjectPrefix = "-"
		}
		fmt.Fprintf(&b, "%-24s %-14s %-10s %-22s %d\n", service.Name, service.Status, service.Version, subjectPrefix, service.FunctionCount)
	}
	return b.String()
}

func formatServiceInspect(service clientcontract.ServiceInfo) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Name:        %s\n", service.Name)
	fmt.Fprintf(&b, "Status:      %s\n", service.Status)
	if service.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", service.Description)
	}
	if service.Version != "" {
		fmt.Fprintf(&b, "Version:     %s\n", service.Version)
	}
	if service.SubjectPrefix != "" {
		fmt.Fprintf(&b, "Subjects:    %s.*\n", service.SubjectPrefix)
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

func formatServiceDoctor(resp clientcontract.ServiceDoctorResponse) string {
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
