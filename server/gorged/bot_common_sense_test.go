package gorged

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/server/host"
	"github.com/adams-shaun/gorge/server/host/httpapi"
)

// TestBotCommonSenseFlagPlumbsToTables pins -bot-common-sense end to end at
// the config layer: the default is false (and a default startup table carries
// it false, so a server that never sets the flag serves exactly today's
// seats), the flag reaches every startup table and the on-demand vs-bot
// table, and the persisted tables.json records it so a restart rebuilds the
// same seats.
func TestBotCommonSenseFlagPlumbsToTables(t *testing.T) {
	fs, c := serveFlags()
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if c.botCommonSense {
		t.Fatal("default -bot-common-sense = true, want false")
	}
	for _, cfg := range c.tableConfigs(nil, nil, view.Omniscient) {
		if cfg.BotCommonSense {
			t.Fatalf("default startup table %s bot_common_sense = true, want false", cfg.ID)
		}
	}
	if err := fs.Parse([]string{"-bot-common-sense=true"}); err != nil {
		t.Fatal(err)
	}
	c.tables, c.seats = 2, 2
	for _, cfg := range c.tableConfigs(nil, nil, view.Omniscient) {
		if !cfg.BotCommonSense {
			t.Fatalf("startup table %s bot_common_sense = false, want true", cfg.ID)
		}
	}

	// The on-demand path: drive the real createGame closure into a persisted
	// registry and read the record it wrote.
	dir := t.TempDir()
	r, err := host.New(host.Options{LoadDeck: gameLoader(t), Sleep: func(time.Duration, <-chan struct{}) {}, Dir: dir, Sync: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	gate := &seatGate{tokenToClaim: map[string]httpapi.SeatClaim{}, claimTokens: map[httpapi.SeatClaim]string{}}
	c.mulligans = 0
	create := c.createGame(r, gate, []string{"a", "b"}, []string{"c", "d"}, view.Public)
	resp, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "tables.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tf struct {
		Tables []struct {
			Config struct {
				ID             string `json:"id"`
				OnDemand       *bool  `json:"on_demand"`
				BotCommonSense *bool  `json:"bot_common_sense"`
			} `json:"config"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(raw, &tf); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rec := range tf.Tables {
		if rec.Config.ID != resp.Table || rec.Config.OnDemand == nil || !*rec.Config.OnDemand {
			continue
		}
		found = true
		if rec.Config.BotCommonSense == nil || !*rec.Config.BotCommonSense {
			t.Fatalf("on-demand table %s persisted bot_common_sense %+v, want true", rec.Config.ID, rec.Config.BotCommonSense)
		}
	}
	if !found {
		t.Fatalf("on-demand table %s not found in tables.json:\n%s", resp.Table, raw)
	}
}

// TestDeployScriptPassesBotCommonSense pins the deploy's default: the script
// carries COMMON_SENSE=${COMMON_SENSE:-false} and passes it as
// -bot-common-sense, and the default the script would serve is false.
func TestDeployScriptPassesBotCommonSense(t *testing.T) {
	if got, want := deployScriptDefault(t, "COMMON_SENSE"), "false"; got != want {
		t.Fatalf("deploy-demo.sh COMMON_SENSE default %q, want %q", got, want)
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "ops", "scripts", "deploy-demo.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), `-bot-common-sense="$COMMON_SENSE"`) {
		t.Fatalf("deploy-demo.sh does not pass -bot-common-sense=\"$COMMON_SENSE\" to the servers:\n%s", script)
	}
}
