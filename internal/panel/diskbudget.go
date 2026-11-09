package panel

import (
	"cmp"
	"fmt"

	"strconv"
	"sync"

	"github.com/nuelScript/skiff/internal/config"
)

// Disk levels that alert the server's owners, once each on the way up; falling below diskClear starts again.
const (
	diskWarnAt     = 80
	diskCriticalAt = 90
	diskClear      = 75
	diskAlertKey   = "disk_alert_level"
)

// diskAlert decides whether the disk's fullness crosses a new level, and the level to remember.
func diskAlert(pct, alerted int) (level int, raise bool) {
	switch {
	case pct >= diskCriticalAt && alerted < diskCriticalAt:
		return diskCriticalAt, true
	case pct >= diskWarnAt && alerted < diskWarnAt:
		return diskWarnAt, true
	case pct < diskClear && alerted > 0:
		return 0, false
	}
	return alerted, false
}

// checkDisk alerts the team that set up the server as its disk fills, with how long the space should last.
func checkDisk(d resourceUsage) {
	if d.Total == 0 {
		return
	}
	pct := int(d.Used * 100 / d.Total)
	alerted, _ := strconv.Atoi(getSetting(diskAlertKey))
	level, raise := diskAlert(pct, alerted)
	if level != alerted {
		_ = setSetting(diskAlertKey, strconv.Itoa(level))
	}
	team := foundingTeam()
	if !raise || team == "" {
		return
	}
	detail := fmt.Sprintf("%s of %s is used.", humanBytes(d.Used), humanBytes(d.Total))
	if _, days := diskForecast(d.Total - d.Used); days >= 0 {
		detail += fmt.Sprintf(" At the past week's rate it fills in about %d days.", days)
	}
	detail += " Deploys and backups fail when it's full. The Server page shows what's using it and can clear old images and build cache."
	go dispatchAlert(alertEvent{Team: team, Kind: "disk.filling", Title: fmt.Sprintf("Your server's disk is %d%% full", pct), Detail: detail})
}

// volumesOver remembers which volumes have been reported over budget, so each is reported once until it shrinks back.
var volumesOver = struct {
	sync.Mutex
	alerted map[string]bool
}{alerted: map[string]bool{}}

// checkVolumeBudgets tells a team when one of its volumes grows past the budget set for it.
func (p *Panel) checkVolumeBudgets() {
	rows, err := sqlDB.Query(`SELECT app, name, budget, owner FROM volumes WHERE budget > 0`)
	if err != nil {
		return
	}
	type budgeted struct {
		app, name, owner string
		budget           int64
	}
	var list []budgeted
	for rows.Next() {
		var b budgeted
		if rows.Scan(&b.app, &b.name, &b.budget, &b.owner) == nil {
			list = append(list, b)
		}
	}
	rows.Close()
	sizes := p.volumeSizes()
	volumesOver.Lock()
	defer volumesOver.Unlock()
	for _, b := range list {
		key := b.app + "/" + b.name
		// A borrowed volume is its holder's, under the holder's name.
		size := sizes[config.VolumeName(cmp.Or(b.owner, b.app), b.name)]
		switch {
		case size > b.budget && !volumesOver.alerted[key]:
			volumesOver.alerted[key] = true
			if src, ok := getSource(b.app); ok {
				go dispatchAlert(alertEvent{Team: src.Team, Kind: "volume.over_budget", App: b.app,
					Title:  fmt.Sprintf("Volume %s of %s is over its budget", b.name, b.app),
					Detail: fmt.Sprintf("It holds %s, and its budget is %s. Clear out old files, or raise the budget in the project's settings.", humanBytes(size), humanBytes(b.budget))})
			}
		case size <= b.budget:
			delete(volumesOver.alerted, key)
		}
	}
}
