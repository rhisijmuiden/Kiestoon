package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestQueueOptionsAndCallbacks(t *testing.T) {
	c, s := adminClient(t)
	e := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true}, 200)
	q := mustDo(t, c, "POST", "/api/pbx/queues", map[string]any{"number": "701", "name": "Support", "enabled": true,
		"members": []map[string]any{{"extensionId": e["id"]}},
		"options": map[string]any{"supervision": true, "callback": map[string]any{"enabled": true, "retries": 2}}}, 200)
	opts := q["options"].(map[string]any)
	cb := opts["callback"].(map[string]any)
	if opts["supervision"] != true || cb["offerEvery"].(float64) != 60 || cb["retryDelay"].(float64) != 120 || cb["expire"].(float64) != 240 {
		t.Fatalf("defaults: %v", opts)
	}
	mustDo(t, c, "PUT", fmt.Sprintf("/api/pbx/queues/%v", q["id"]), map[string]any{"number": "701", "name": "Support",
		"options": map[string]any{"callback": map[string]any{"enabled": true, "retries": 9}}}, 400)

	// Callback list: add by hand, cancel, retry.
	added := mustDo(t, c, "POST", "/api/reception/callbacks", map[string]any{"queueId": q["id"], "number": "0612345678", "name": "Jan"}, 200)
	mustDo(t, c, "POST", "/api/reception/callbacks", map[string]any{"queueId": q["id"], "number": "12"}, 400)
	list := mustDoList(t, c, "/api/reception/callbacks")
	if len(list) != 1 || list[0]["status"] != "scheduled" || list[0]["number"] != "0612345678" {
		t.Fatalf("list: %v", list)
	}
	mustDo(t, c, "POST", fmt.Sprintf("/api/reception/callbacks/%v/retry", added["id"]), map[string]any{}, 409)
	mustDo(t, c, "POST", fmt.Sprintf("/api/reception/callbacks/%v/cancel", added["id"]), map[string]any{}, 200)
	mustDo(t, c, "POST", fmt.Sprintf("/api/reception/callbacks/%v/retry", added["id"]), map[string]any{}, 200)

	// A failed answer schedules a retry, until the retries run out.
	id := int64(added["id"].(float64))
	ctx := context.Background()
	if _, err := s.PBX.UpdateCallback(ctx, id, nil, pbx.CallbackCalling, 0, timeZero(), "201", ""); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{pbx.CallbackScheduled, pbx.CallbackScheduled, pbx.CallbackFailed} {
		_, _ = s.PBX.UpdateCallback(ctx, id, nil, pbx.CallbackCalling, 0, timeZero(), "", "")
		s.onCallbackEvent(asterisk.Message{{Key: "Action", Value: "done"}, {Key: "Id", Value: fmt.Sprint(id)}, {Key: "Status", Value: "NOANSWER"}})
		got, _ := s.PBX.Callback(ctx, id)
		if got.Status != want {
			t.Fatalf("attempt %d: status %s, want %s", i+1, got.Status, want)
		}
	}

	// Supervision needs a linked extension and a supervised agent.
	mustDo(t, c, "POST", "/api/reception/supervise", map[string]any{"agent": "201", "mode": "listen"}, 409)
	mustDo(t, c, "POST", "/api/reception/supervise", map[string]any{"agent": "201", "mode": "spy"}, 400)
}

func timeZero() (t time.Time) { return }
