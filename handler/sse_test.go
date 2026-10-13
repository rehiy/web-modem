package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rehiy/web-modem/service"
)

func TestSseBrokerDeliversTypedEvents(t *testing.T) {
	source := make(chan service.ModemEventMsg, 1)
	broker := newSseBroker(source)
	go broker.run()

	client, unsubscribe := broker.subscribe()
	defer unsubscribe()

	source <- service.ModemEventMsg{Event: service.ModemEventModems, Data: "changed"}
	select {
	case ev := <-client:
		if ev.Event != service.ModemEventModems || ev.Data != "changed" {
			t.Fatalf("收到事件 = %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("未收到广播事件")
	}
}

func TestWriteSseEventNamed(t *testing.T) {
	w := httptest.NewRecorder()
	writeSseEvent(w, service.ModemEventModems, "changed")
	if got, want := w.Body.String(), "event: modems\ndata: changed\n\n"; got != want {
		t.Fatalf("SSE 输出 = %q, want %q", got, want)
	}
}

func TestIsTruthyQuery(t *testing.T) {
	for query, want := range map[string]bool{"rescan=1": true, "rescan=true": true, "rescan=0": false, "": false} {
		r := httptest.NewRequest("GET", "/api/modem/list?"+query, nil)
		if got := isTruthyQuery(r, "rescan"); got != want {
			t.Fatalf("isTruthyQuery(%q) = %v, want %v", query, got, want)
		}
	}
}
