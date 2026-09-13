package config

import (
	"fmt"
	"strings"
)

// Webhooks is whether this deployment sends outgoing webhooks at all.
//
// Environment and not ddcore.json, because the answer differs by machine and
// not by site: a migration rehearsal or a restored copy of production must run
// the same site with no outgoing business effects, and it is the deployment
// that knows it is a rehearsal. The subscriptions themselves are rows in
// `Webhook`, for the same reason — their URLs and keys belong to the
// deployment too.
type Webhooks struct {
	// Off stops events from becoming deliveries. Nothing is queued while it is
	// set, so turning it back on does not release a backlog of stale events.
	// The zero value sends, so an engine built by hand (tests, embedders)
	// behaves like a default site.
	Off bool
}

func webhooksFromEnv() (Webhooks, error) {
	switch v := strings.ToLower(strings.TrimSpace(env("DDCORE_WEBHOOKS", "on"))); v {
	case "on", "":
		return Webhooks{}, nil
	case "off":
		return Webhooks{Off: true}, nil
	default:
		return Webhooks{}, fmt.Errorf("DDCORE_WEBHOOKS: %q is neither on nor off", v)
	}
}
