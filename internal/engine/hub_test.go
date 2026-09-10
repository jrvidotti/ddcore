package engine

import "testing"

// B20: um evento que nomeia um documento só chega a quem pode lê-lo.
func TestB20_HubFiltersEventsByPermission(t *testing.T) {
	h := NewHub()
	var asked [][2]string
	ana := h.Subscribe("ana@x.com", func(doctype, name string) bool {
		asked = append(asked, [2]string{doctype, name})
		return doctype == "Pedido"
	})
	ze := h.Subscribe("ze@x.com", func(doctype, name string) bool { return false })
	anon := h.Subscribe("Guest", nil)

	h.Publish(Event{Name: "doc_update", Doctype: "Pedido", DocName: "PED-0001"})
	h.Publish(Event{Name: "doc_update", Doctype: "User", DocName: "Administrator"})
	h.Publish(Event{Name: "reload"}) // evento sem documento continua broadcast
	h.Publish(Event{Name: "job_done", User: "ze@x.com", Doctype: "Pedido"})

	drain := func(ch chan Event) []Event {
		var out []Event
		for {
			select {
			case ev := <-ch:
				out = append(out, ev)
			default:
				return out
			}
		}
	}
	got := drain(ana)
	if len(got) != 2 || got[0].Name != "doc_update" || got[0].DocName != "PED-0001" || got[1].Name != "reload" {
		t.Fatalf("ana deveria receber o Pedido e o reload, recebeu %+v", got)
	}
	if got := drain(ze); len(got) != 1 || got[0].Name != "reload" {
		t.Fatalf("zé só deveria receber o reload, recebeu %+v", got)
	}
	if got := drain(anon); len(got) != 1 || got[0].Name != "reload" {
		t.Fatalf("assinante sem autorizador só recebe eventos sem documento, recebeu %+v", got)
	}
	if len(asked) == 0 {
		t.Fatal("o hub não consultou o autorizador")
	}
	h.Unsubscribe(ana)
	h.Unsubscribe(ze)
	h.Unsubscribe(anon)
}
