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
	return dispatch(pushUsage, []subcommand{
		{"keys", func(args []string) error { return pushKeys(os.Stdout, args) }},
	}, args)
}

// pushKeys prints a fresh pair. It touches no database and stores nothing:
// where the private key goes is the operator's call.
func pushKeys(w io.Writer, args []string) error {
	fs := newFlagSet("push keys")
	subject := fs.String("subject", "", "a mailto: or https: URL the push service can reach the site's operator at")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	// empty prints the variable for the operator to fill; anything else must
	// pass the check send makes, here rather than on the first send
	if *subject != "" && !engine.ValidVAPIDSubject(*subject) {
		return fmt.Errorf("--subject must be a mailto: or https: URL, such as mailto:ops@example.com")
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
