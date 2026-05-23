package startup

import "fmt"

func ResolveChannels(channels []string) ([]string, error) {
	active := make(map[string]bool)
	for _, ch := range channels {
		if ch == "all" {
			active["nats"] = true
			continue
		}
		active[ch] = true
	}
	if len(active) == 0 {
		return nil, fmt.Errorf("no channels specified to start")
	}
	valid := make([]string, 0, len(active))
	for ch := range active {
		switch ch {
		case "nats":
			valid = append(valid, ch)
		case "web", "telegram":
			return nil, fmt.Errorf("%s channel is removed from the NATS-native runtime start path", ch)
		default:
			return nil, fmt.Errorf("unknown channel requested: %q", ch)
		}
	}
	return valid, nil
}
