package systemsvc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

type approvalPlanner struct{}

func (approvalPlanner) KillProcess(pid int64, reason string) *systemv1.MutationPlan {
	return mutationPlan("system.kill_process", fmt.Sprint(pid), reason, "process.kill")
}

func (approvalPlanner) RestartService(name, manager, reason string) *systemv1.MutationPlan {
	return mutationPlan("system.restart_service", firstNonBlank(manager, "systemd")+":"+name, reason, "service.restart")
}

func mutationPlan(action, target, reason, risk string) *systemv1.MutationPlan {
	sum := sha256.Sum256([]byte(action + "|" + target + "|" + reason + "|" + risk))
	return &systemv1.MutationPlan{
		Id: hex.EncodeToString(sum[:8]), Action: action, Target: target,
		Reason: strings.TrimSpace(reason), ApprovalRequired: true, Risks: []string{risk},
	}
}

func keyValueFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
			if !ok {
				continue
			}
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return out
}

func keyValueUnits(path string) map[string]uint64 {
	data := keyValueFile(path)
	out := map[string]uint64{}
	for key, raw := range data {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		value, _ := strconv.ParseUint(fields[0], 10, 64)
		if len(fields) > 1 && strings.EqualFold(fields[1], "kb") {
			value *= 1024
		}
		out[key] = value
	}
	return out
}

func charsToString(chars []int8) string {
	b := make([]byte, 0, len(chars))
	for _, ch := range chars {
		if ch == 0 {
			break
		}
		b = append(b, byte(ch))
	}
	return string(b)
}

func limitOrDefault(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func observationError(err error) error {
	if err == nil {
		return nil
	}
	return serviceerrors.Unavailable(err.Error())
}
