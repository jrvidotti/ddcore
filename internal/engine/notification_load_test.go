package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

func TestNotificationReloadKeepsLastValidState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications", "change.notification.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, target string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`import {defineNotification} from "@ddcore/sdk"; export default defineNotification({name:"`+name+`",doctype:"`+target+`",event:"on_update",recipients:()=>[],desk:{title:()=>"`+name+`",message:()=>"Message"}});`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	appRules := func(rules []js.Notification, app string) []js.Notification {
		var out []js.Notification
		for _, n := range rules {
			if n.App == app {
				out = append(out, n)
			}
		}
		return out
	}
	write("First", "User")
	e, err := New(context.Background(), Config{Apps: []js.App{{Name: "notify", Dir: dir}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { e.Current().Pool.Close() }()
	if notifyRules := appRules(e.Notifications, "notify"); len(notifyRules) != 1 || notifyRules[0].Name != "First" {
		t.Fatalf("%+v", e.Notifications)
	}
	write("Second", "User")
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	if notifyRules := appRules(e.Notifications, "notify"); len(notifyRules) != 1 || notifyRules[0].Name != "Second" {
		t.Fatalf("%+v", e.Notifications)
	}
	for _, target := range []string{"Missing", "Has Role", "Email Delivery", "Webhook Delivery", "Audit Event", "Version", "Error Log"} {
		write("Invalid", target)
		if err := e.Load(); err == nil {
			t.Fatal("accepted invalid target " + target)
		}
		if notifyRules := appRules(e.Current().Notifications, "notify"); len(notifyRules) != 1 || notifyRules[0].Name != "Second" {
			t.Fatal("failed reload replaced active rules")
		}
	}
	rt, err := e.Pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Pool.Release(rt)
	content, err := rt.RenderNotification("Second", json.RawMessage(`{}`), nil)
	if err != nil || content.Title != "Second" {
		t.Fatalf("%+v %v", content, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	if notifyRules := appRules(e.Notifications, "notify"); len(notifyRules) != 0 {
		t.Fatal("removed rule remained active")
	}
}
