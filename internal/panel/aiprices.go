package panel

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Model prices are the team's own, in US dollars per million tokens: providers change them too often for Skiff to keep a list. A model takes the price of the longest name it starts with, so "gpt-4o-mini" also prices "gpt-4o-mini-2024-07-18".
type aiPrice struct {
	Model  string  `json:"model"`
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

var priceModelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,99}$`)

func teamPrices(team string) []aiPrice {
	rows, err := sqlDB.Query(`SELECT model, input_per_m, output_per_m FROM ai_prices WHERE team=? ORDER BY model`, team)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []aiPrice{}
	for rows.Next() {
		var p aiPrice
		if rows.Scan(&p.Model, &p.Input, &p.Output) == nil {
			out = append(out, p)
		}
	}
	return out
}

// priceFor finds the price a model uses: the longest listed name it starts with.
func priceFor(prices []aiPrice, model string) (aiPrice, bool) {
	model = strings.ToLower(model)
	var best aiPrice
	found := false
	for _, p := range prices {
		if strings.HasPrefix(model, strings.ToLower(p.Model)) && len(p.Model) > len(best.Model) {
			best, found = p, true
		}
	}
	return best, found
}

func costOf(p aiPrice, in, out int64) float64 {
	return math.Round((float64(in)*p.Input+float64(out)*p.Output)/1e6*1e6) / 1e6
}

func (p *Panel) handleAIPrices(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	if r.Method != http.MethodGet && !p.isOwner(r) {
		http.Error(w, "only a team owner can change model prices", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"prices": teamPrices(team)})
	case http.MethodPut:
		var body aiPrice
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body.Model = strings.TrimSpace(body.Model)
		if !priceModelRe.MatchString(body.Model) {
			http.Error(w, "give the model's name, like gpt-4o-mini or claude-sonnet-4-5", http.StatusBadRequest)
			return
		}
		if body.Input < 0 || body.Output < 0 || body.Input > 10000 || body.Output > 10000 || math.IsNaN(body.Input) || math.IsNaN(body.Output) {
			http.Error(w, "prices are dollars per million tokens, from 0 to 10,000", http.StatusBadRequest)
			return
		}
		if _, err := sqlDB.Exec(`INSERT INTO ai_prices(team, model, input_per_m, output_per_m, updated) VALUES(?,?,?,?,?)
			ON CONFLICT(team, model) DO UPDATE SET input_per_m=excluded.input_per_m, output_per_m=excluded.output_per_m, updated=excluded.updated`,
			team, body.Model, body.Input, body.Output, time.Now().Unix()); err != nil {
			http.Error(w, "couldn't save", http.StatusInternalServerError)
			return
		}
		p.audit(r, "ai.price.save", body.Model, "")
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		model := r.URL.Query().Get("model")
		if _, err := sqlDB.Exec(`DELETE FROM ai_prices WHERE team=? AND model=?`, team, model); err != nil {
			http.Error(w, "couldn't remove", http.StatusInternalServerError)
			return
		}
		p.audit(r, "ai.price.remove", model, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
