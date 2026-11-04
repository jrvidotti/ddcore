package engine

import (
	"encoding/json"
	"fmt"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// CallWhitelisted runs a function exported with whitelisted() as the context's
// user: the contract /api/method/<path> and the MCP call_method tool share. A
// function not whitelisted does not exist for the caller, and one whose roles
// option names none of the user's roles is refused (Admin excepted). Server
// code reaches any function through ddcore.callMethod; the transports do not.
func (c *Ctx) CallWhitelisted(path string, args json.RawMessage) (json.RawMessage, error) {
	opts, ok := c.St.Whitelisted(path)
	if !ok {
		return nil, cerr.NotFound("Method {0} does not exist or is not whitelisted", path)
	}
	if roles, ok := opts["roles"].([]any); ok && len(roles) > 0 && c.User != "Admin" {
		has := false
		for _, ro := range roles {
			if c.HasRole(fmt.Sprint(ro)) {
				has = true
				break
			}
		}
		if !has {
			c.AuditDenied("method."+path, "", "", nil)
			return nil, cerr.Permission("No permission for {0}", path)
		}
	}
	rt, err := c.RT()
	if err != nil {
		return nil, err
	}
	return rt.CallWhitelisted(path, args)
}
