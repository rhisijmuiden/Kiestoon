package pbx

import (
	"context"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// MobileKey is the setting with the mobile app settings.
const MobileKey = "mobile"

// MobileSettings switch the mobile app on and name the push relay.
type MobileSettings struct {
	Enabled bool `json:"enabled"`
	// RelayURL is the push relay (https://push.rhis.nl for the app RHIS publishes).
	RelayURL string `json:"relayURL"`
}

// LoadMobileSettings reads the mobile app settings.
func LoadMobileSettings(ctx context.Context, r *Repo) MobileSettings {
	st := MobileSettings{RelayURL: "https://push.rhis.nl"}
	_ = store.GetJSON(ctx, r.DB, MobileKey, &st)
	st.RelayURL = strings.TrimRight(strings.TrimSpace(st.RelayURL), "/")
	return st
}

// MobileNumbers returns the extension numbers whose users have linked the mobile app.
func (r *Repo) MobileNumbers(ctx context.Context) (map[string]bool, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT DISTINCT e.number FROM mobile_devices d JOIN users u ON u.id = d.user_id
		JOIN extensions e ON e.id = u.extension_id WHERE u.enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

func (r *Repo) markMobileApps(ctx context.Context, s *Snapshot) error {
	if !LoadMobileSettings(ctx, r).Enabled {
		return nil
	}
	nums, err := r.MobileNumbers(ctx)
	if err != nil {
		return err
	}
	for i := range s.Extensions {
		s.Extensions[i].MobileApp = nums[s.Extensions[i].Number]
	}
	return nil
}
