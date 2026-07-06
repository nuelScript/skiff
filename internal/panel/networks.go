package panel

import "log"

// Per-team network isolation: each team's apps and databases join a private
// docker network (skiff-t-<team>) instead of the shared "skiff" net, so a
// compromised app can't reach another team's containers by name. New deploys,
// databases, replicas, and jobs go straight onto the team network; existing
// databases are attached to it on startup so an app redeployed onto the team
// net keeps reaching them.

func teamNetwork(team string) string {
	if team == "" {
		return dbNetwork // shared fallback for team-less / legacy apps
	}
	return "skiff-t-" + sanitizeName(team)
}

// reconcileNetworks makes sure every existing database is reachable on its
// team's private network (in addition to wherever it already lives), so an app
// redeployed onto the team net can still find it by name.
func (p *Panel) reconcileNetworks() {
	rows, err := sqlDB.Query(`SELECT team, container FROM databases`)
	if err != nil {
		return
	}
	type dbNet struct{ team, container string }
	var dbs []dbNet
	for rows.Next() {
		var d dbNet
		if rows.Scan(&d.team, &d.container) == nil {
			dbs = append(dbs, d)
		}
	}
	rows.Close()

	for _, d := range dbs {
		if d.team == "" || d.container == "" {
			continue
		}
		net := teamNetwork(d.team)
		if p.eng.EnsureNetwork(net) != nil {
			continue
		}
		_ = p.eng.ConnectNetwork(net, d.container) // already-connected is a harmless error
	}
	if len(dbs) > 0 {
		log.Printf("networks: reconciled %d databases onto team networks", len(dbs))
	}
}
