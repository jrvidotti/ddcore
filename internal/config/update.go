package config

import (
	"fmt"
	"strings"
)

// UpdateCheck is whether this deployment asks GitHub for the newest published
// release when `ddcore doctor` runs.
//
// Environment and not ddcore.json, for the same reason as Webhooks: the answer
// differs by machine and not by site. An air-gapped deployment, a CI runner and
// a build sandbox all run the same site as the laptop that should be told it is
// two releases behind.
type UpdateCheck struct {
	// Off stops the lookup. The zero value checks, so an engine built by hand
	// (tests, embedders) behaves like a default site.
	Off bool
}

func updateCheckFromEnv() (UpdateCheck, error) {
	switch v := strings.ToLower(strings.TrimSpace(env("DDCORE_UPDATE_CHECK", "on"))); v {
	case "on", "":
		return UpdateCheck{}, nil
	case "off":
		return UpdateCheck{Off: true}, nil
	default:
		return UpdateCheck{}, fmt.Errorf("DDCORE_UPDATE_CHECK: %q is neither on nor off", v)
	}
}
