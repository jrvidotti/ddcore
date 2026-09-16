package js

import (
	"encoding/json"
	"fmt"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// Notification describes a registered rule; its functions stay in the runtime.
type Notification struct {
	Name       string             `json:"name"`
	Doctype    string             `json:"doctype"`
	Event      string             `json:"event,omitempty"`
	Date       *NotificationDate  `json:"date,omitempty"`
	Desk       bool               `json:"desk"`
	Email      *NotificationEmail `json:"email,omitempty"`
	App        string             `json:"app"`
	SourceFile string             `json:"sourceFile"`
}
type NotificationDate struct {
	Field string `json:"field"`
	Days  int    `json:"days"`
}
type NotificationEmail struct {
	Template string `json:"template"`
}
type NotificationEvaluation struct {
	Matches    bool     `json:"matches"`
	Recipients []string `json:"recipients"`
}
type NotificationContent struct {
	Title     string          `json:"title"`
	Message   string          `json:"message"`
	EmailArgs json.RawMessage `json:"emailArgs"`
}

// ValidateTarget runs after extensions have been merged into the registry.
func (n Notification) ValidateTarget(reg *meta.Registry, hasTemplate func(string) bool) error {
	d, ok := reg.DocTypes[n.Doctype]
	if !ok {
		return fmt.Errorf("notification %s: unknown DocType %s", n.Name, n.Doctype)
	}
	switch n.Doctype {
	case "Webhook", "Webhook Delivery", "Audit Event", "Email Delivery", "Version", "Error Log":
		return fmt.Errorf("notification %s: internal delivery and audit DocTypes are not supported", n.Name)
	}
	if d.IsChild {
		return fmt.Errorf("notification %s: child DocTypes are not supported", n.Name)
	}
	if d.IsSingle {
		return fmt.Errorf("notification %s: Single DocTypes are not supported", n.Name)
	}
	if n.Date != nil {
		f := d.Field(n.Date.Field)
		if f == nil || (f.Fieldtype != "Date" && f.Fieldtype != "Datetime") {
			return fmt.Errorf("notification %s: date field must be Date or Datetime", n.Name)
		}
	}
	if n.Email != nil && !hasTemplate(n.Email.Template) {
		return fmt.Errorf("notification %s: mail template %s does not exist", n.Name, n.Email.Template)
	}
	return nil
}

func (rt *Runtime) EvaluateNotification(name string, doc, before json.RawMessage) (*NotificationEvaluation, error) {
	s, err := rt.callReg("evaluateNotification", name, string(doc), string(before))
	if err != nil {
		return nil, err
	}
	var result NotificationEvaluation
	err = json.Unmarshal([]byte(s), &result)
	return &result, err
}
func (rt *Runtime) RenderNotification(name string, doc, before json.RawMessage) (*NotificationContent, error) {
	s, err := rt.callReg("renderNotification", name, string(doc), string(before))
	if err != nil {
		return nil, err
	}
	var result NotificationContent
	err = json.Unmarshal([]byte(s), &result)
	return &result, err
}
