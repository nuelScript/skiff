package panel

import (
	"net/http"

	"github.com/nuelScript/skiff/internal/docker"
)

type gpuView struct {
	docker.GPU
	App string `json:"app,omitempty"`
}

// handleGPUs lists the server's GPUs, which app has each, and whether containers can be given them. Like the server's other figures, it's for the team that set the server up.
func (p *Panel) handleGPUs(w http.ResponseWriter, r *http.Request) {
	if p.teamID(r) != foundingTeam() {
		writeJSON(w, map[string]any{"gpus": []gpuView{}})
		return
	}
	used := p.eng.GPUsInUse()
	out := []gpuView{}
	for _, g := range p.eng.GPUs() {
		out = append(out, gpuView{GPU: g, App: used[g.UUID]})
	}
	problem := ""
	if len(out) > 0 {
		if err := p.eng.CanUseGPUs(); err != nil {
			problem = err.Error()
		}
	}
	writeJSON(w, map[string]any{"gpus": out, "problem": problem})
}
