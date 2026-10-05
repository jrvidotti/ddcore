package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const pushUsage = `usage: ddcore push <subcommand>

  keys     print a new VAPID key pair for ddcore.push, as .env lines
           (--subject mailto:you@example.com fills DDCORE_SECRET_VAPID_SUBJECT)`

func cmdPush(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", pushUsage)
	}
	switch args[0] {
	case "keys":
		return pushKeys(os.Stdout, args[1:])
	default:
		return fmt.Errorf("unknown subcommand: %s\n\n%s", args[0], pushUsage)
	}
}

// pushKeys prints a fresh pair. It touches no database and stores nothing:
// where the private key goes is the operator's call.
func pushKeys(w io.Writer, args []string) error {
	fs := newFlagSet("push keys")
	subject := fs.String("subject", "", "a mailto: or https: URL the push service can reach the site's operator at")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	public, private, err := engine.GenerateVAPIDKeys()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, `# VAPID keys for Web Push (ddcore.push). Keep the private key out of the
# repository. Browsers subscribe with the public key, so replacing the pair
# leaves every existing subscription unusable.
%s=%s
%s=%s
# A mailto: or https: URL a push service can reach the site's operator at.
%s=%s
`, engine.SecretEnvName("vapid_public_key"), public,
		engine.SecretEnvName("vapid_private_key"), private,
		engine.SecretEnvName("vapid_subject"), *subject)
	return nil
}
