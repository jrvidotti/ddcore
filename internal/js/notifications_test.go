package js

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func notificationRuntime(t *testing.T, definition string) (*Runtime, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "notifications"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notifications/test.notification.ts"), []byte(`import { defineNotification } from "@ddcore/sdk"; `+definition), 0644); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildServer(App{Name: "test", Dir: dir}, false)
	if err != nil {
		return nil, err
	}
	return newRuntime(&fakeHost{}, []*Bundle{bundle}, false)
}
func TestNotificationDiscoveryEvaluation(t *testing.T) {
	rt, err := notificationRuntime(t, `export default defineNotification({name:"Changed",doctype:"Task",event:"on_update",condition:(doc,before)=>doc.status!==before.status,recipients:(doc)=>[doc.owner,doc.owner],desk:{title:(doc)=>doc.status,message:(doc,before)=>before.status+" -> "+doc.status},email:{template:"Update",args:(doc)=>({name:doc.name})}});`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := rt.Meta()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"notifications":{"Changed"`) || !strings.Contains(string(raw), `"desk":true`) {
		t.Fatal(string(raw))
	}
	doc := json.RawMessage(`{"name":"one","owner":"alice","status":"Done"}`)
	before := json.RawMessage(`{"status":"Open"}`)
	eval, err := rt.EvaluateNotification("Changed", doc, before)
	if err != nil || !eval.Matches || len(eval.Recipients) != 1 || eval.Recipients[0] != "alice" {
		t.Fatalf("%+v %v", eval, err)
	}
	content, err := rt.RenderNotification("Changed", doc, before)
	if err != nil || content.Title != "Done" || content.Message != "Open -> Done" || string(content.EmailArgs) != `{"name":"one"}` {
		t.Fatalf("%+v %v", content, err)
	}
	eval, err = rt.EvaluateNotification("Changed", doc, doc)
	if err != nil || eval.Matches || len(eval.Recipients) != 0 {
		t.Fatalf("%+v %v", eval, err)
	}
}
func TestNotificationInvalidDefinitions(t *testing.T) {
	base := `name:"Test",doctype:"Task",event:"on_insert",recipients:()=>["alice"],desk:{title:()=>"Title",message:()=>"Message"}`
	for _, fragment := range []string{`name:""`, `event:"on_trash"`, `date:{field:"due",days:1}`, `event:undefined`, `recipients:async()=>["alice"]`, `condition:async()=>true`, `desk:null`, `email:{template:""}`, `event:undefined,date:{field:"due",days:1.5}`} {
		t.Run(fragment, func(t *testing.T) {
			_, err := notificationRuntime(t, `defineNotification({`+base+`,`+fragment+`});`)
			if err == nil {
				t.Fatal("accepted invalid rule")
			}
		})
	}
	_, err := notificationRuntime(t, `defineNotification({`+base+`});defineNotification({`+base+`});`)
	if err == nil {
		t.Fatal("accepted duplicate")
	}
}
func TestNotificationRejectsPromiseAndInvalidResults(t *testing.T) {
	for _, fragment := range []string{`condition:()=>Promise.resolve(true)`, `condition:()=>1`, `recipients:()=>Promise.resolve(["alice"])`, `recipients:()=>[{}]`} {
		rt, err := notificationRuntime(t, `defineNotification({name:"Test",doctype:"Task",event:"on_insert",recipients:()=>["alice"],desk:{title:()=>"Title",message:()=>"Message"},`+fragment+`});`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = rt.EvaluateNotification("Test", json.RawMessage(`{}`), nil); err == nil {
			t.Fatal("accepted " + fragment)
		}
	}
	rt, err := notificationRuntime(t, `defineNotification({name:"Test",doctype:"Task",event:"on_insert",recipients:()=>[],desk:{title:()=>Promise.resolve("Title"),message:()=>"Message"}});`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rt.RenderNotification("Test", json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("accepted asynchronous title")
	}
}
func TestNotificationTargetValidation(t *testing.T) {
	reg := meta.NewRegistry()
	for _, d := range []*meta.DocType{{Name: "Task", Fields: []*meta.Field{{Fieldname: "due", Fieldtype: "Date"}, {Fieldname: "title", Fieldtype: "Data"}}}, {Name: "Child", IsChild: true}, {Name: "Email Delivery"}, {Name: "Webhook"}} {
		if err := reg.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	for _, rule := range []Notification{{Name: "unknown", Doctype: "Missing"}, {Name: "child", Doctype: "Child"}, {Name: "internal", Doctype: "Email Delivery"}, {Name: "internal webhook", Doctype: "Webhook"}, {Name: "bad date", Doctype: "Task", Date: &NotificationDate{Field: "title"}}, {Name: "missing mail", Doctype: "Task", Email: &NotificationEmail{Template: "Missing"}}} {
		if err := rule.ValidateTarget(reg, func(string) bool { return false }); err == nil {
			t.Fatalf("accepted %+v", rule)
		}
	}
	if err := (Notification{Name: "date", Doctype: "Task", Date: &NotificationDate{Field: "due"}}).ValidateTarget(reg, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
}
