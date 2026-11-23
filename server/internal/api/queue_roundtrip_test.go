package api

import (
	"encoding/json"
	"fmt"
	"testing"
)

// A queue loaded from the queue list saves again unchanged (the edit dialog sends it back as is).
func TestQueueSavesUnchanged(t *testing.T) {
	c, _ := adminClient(t)
	e := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true}, 200)
	mustDo(t, c, "POST", "/api/pbx/queues", map[string]any{"number": "701", "name": "RHIS - Support", "enabled": true, "announcePosition": true,
		"announceFrequency": 60, "joinEmpty": true, "music": "default", "strategy": "rrmemory", "agentTimeout": 15, "retry": 5, "wrapup": 10, "maxWait": 300,
		"members":  []map[string]any{{"extensionId": e["id"], "dynamic": true}},
		"overflow": map[string]string{"type": "hangup", "target": ""}}, 200)
	code, raw := c.raw("GET", "/api/reception/queues")
	if code != 200 {
		t.Fatal(code)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatal(err)
	}
	q := list[0]
	delete(q, "live")
	delete(q, "alerts")
	mustDo(t, c, "PUT", fmt.Sprintf("/api/pbx/queues/%v", q["id"]), q, 200)
}
