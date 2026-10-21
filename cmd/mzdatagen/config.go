package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Config is the mage fork's MageZero Config: the keys and defaults of its
// config.yml.
type Config struct {
	GoesFirst string // goes_first: player_a, player_b, random
	GameMode  string // game_mode: normal (commander is not supported)
	PlayerA   PlayerConfig
	PlayerB   PlayerConfig
	Training  TrainingConfig
	Server    ServerConfig
	Logging   LoggingConfig
}

type PlayerConfig struct {
	DeckPath   string
	Type       string // mcts, minimax
	OutputFile string

	Priority, Target, Binary, Opponent bool // priors
	PriorTemperature, PriorBonus       float64

	NoiseEnabled         bool
	DirichletNoise       float64
	SelectionTemperature float64

	SearchBudget         int
	TimeoutMs            int
	TDDiscount           float64
	BackpropDiscount     float64
	OfflineMode          bool
	PruneDuplicateStates bool

	Mulligans bool
	ManualTap bool

	OpponentHand bool
}

type TrainingConfig struct {
	Games, GamesPerFile, MaxTurns, MaxMinutes, Threads int
	TrainOpponentHead                                  bool
}

type ServerConfig struct {
	Host               string
	Port, OpponentPort int
}

type LoggingConfig struct {
	LogFeatureHash, WriteFinalWR, ShowWR bool
}

// parseConfig reads config.yml as Config's constructor does.
func parseConfig(r io.Reader) (Config, error) {
	raw, err := parseYAML(r)
	if err != nil {
		return Config{}, err
	}
	m := yamlMap(raw)
	c := Config{
		GoesFirst: m.str("goes_first", "random"),
		GameMode:  m.str("game_mode", "normal"),
		PlayerA:   playerConfig(m.sub("player_a")),
		PlayerB:   playerConfig(m.sub("player_b")),
	}
	t := m.sub("training")
	c.Training = TrainingConfig{
		Games: t.int("games", 1000), GamesPerFile: t.int("games_per_file", 200), MaxTurns: t.int("max_turns", 50),
		MaxMinutes: t.int("max_minutes", 20), Threads: t.int("threads", 2), TrainOpponentHead: t.bool("train_opponent_head", false),
	}
	s := m.sub("server")
	c.Server = ServerConfig{Host: s.str("host", "localhost"), Port: s.int("port", 8080), OpponentPort: s.int("opponent_port", 8081)}
	l := m.sub("logging")
	c.Logging = LoggingConfig{LogFeatureHash: l.bool("log_feature_hash", false), WriteFinalWR: l.bool("save_final_wr", true), ShowWR: l.bool("show_wr", true)}
	return c, nil
}

func playerConfig(m yamlMap) PlayerConfig {
	p, n, mc, g := m.sub("priors"), m.sub("noise"), m.sub("mcts"), m.sub("gameplay")
	return PlayerConfig{
		DeckPath: m.str("deckPath", ""), Type: m.str("type", "mcts"), OutputFile: m.str("output_file", ""),
		Priority: p.bool("priority", false), Target: p.bool("target", false), Binary: p.bool("binary", false), Opponent: p.bool("opponent", false),
		PriorTemperature: p.float("prior_temperature", 1.5), PriorBonus: p.float("prior_bonus", 0.1),
		NoiseEnabled: n.bool("enabled", false), DirichletNoise: n.float("dirichlet_noise", 0.15), SelectionTemperature: n.float("selection_temperature", 2.0),
		SearchBudget: mc.int("search_budget", 300), TimeoutMs: mc.int("timeout_ms", 4000), TDDiscount: mc.float("td_discount", 0.95),
		BackpropDiscount: mc.float("backprop_discount", 0.99), OfflineMode: mc.bool("offline_mode", false), PruneDuplicateStates: mc.bool("prune_duplicate_states", false),
		Mulligans: g.bool("mulligans_enabled", true), ManualTap: g.bool("manual_tapping", false),
		// The fork reads this block as "hiddenInfo" while its config.yml
		// writes "hidden_info", so see_opponent_hand keeps its default.
		OpponentHand: m.sub("hiddenInfo").bool("see_opponent_hand", true),
	}
}

// yamlMap reads typed values, each with its default when absent.
type yamlMap map[string]any

func (m yamlMap) sub(k string) yamlMap {
	if s, ok := m[k].(map[string]any); ok {
		return s
	}
	return yamlMap{}
}

func (m yamlMap) str(k, def string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return def
}

func (m yamlMap) int(k string, def int) int {
	switch v := m[k].(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return def
}

func (m yamlMap) float(k string, def float64) float64 {
	switch v := m[k].(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return def
}

func (m yamlMap) bool(k string, def bool) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return def
}

// parseYAML reads the YAML subset config.yml is written in: nested maps by
// indentation, scalar values (strings, numbers, booleans), and comments.
func parseYAML(r io.Reader) (map[string]any, error) {
	root := map[string]any{}
	type level struct {
		indent int
		m      map[string]any
	}
	stack := []level{{-1, root}}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := stripComment(sc.Text())
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key, val, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			return nil, fmt.Errorf("config line %d: want key: value", n)
		}
		for indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		m := stack[len(stack)-1].m
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if val == "" {
			sub := map[string]any{}
			m[key] = sub
			stack = append(stack, level{indent, sub})
			continue
		}
		m[key] = scalar(val)
	}
	return root, sc.Err()
}

func stripComment(line string) string {
	quoted := false
	for i, c := range line {
		switch {
		case c == '"' || c == '\'':
			quoted = !quoted
		case c == '#' && !quoted && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			return line[:i]
		}
	}
	return line
}

func scalar(v string) any {
	switch v {
	case "true", "True":
		return true
	case "false", "False":
		return false
	}
	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return strings.Trim(v, `"'`)
}
