package sshremote

import (
	"context"
	"sort"
	"time"
)

// RecoveryEvidence is sanitized, SSH backend-owned evidence. Unknown means
// unknown, never an implicit failure or proof of remote quiescence.
type RecoveryEvidence struct {
	TaskID      string   `json:"taskId"`
	Phase       string   `json:"phase"`
	State       JobState `json:"state"`
	Activity    string   `json:"activity"`
	Fingerprint string   `json:"fingerprint"`
	Recoverable bool     `json:"recoverable"`
}
type RecoveryReport struct {
	Records     []RecoveryEvidence `json:"records"`
	InspectedAt time.Time          `json:"inspectedAt"`
	Verdict     string             `json:"verdict"`
}

// InspectRecovery observes ONLY execution-fenced v2 task journals. It never
// replays a command, creates a task or fabricates terminal evidence. A corrupt
// record is visible as uncertain and blocks lifecycle.
func (j *Jobs) InspectRecovery(ctx context.Context) (RecoveryReport, error) {
	if j == nil || j.store == nil || j.runner == nil {
		return RecoveryReport{}, ErrJobUnavailable
	}
	ids, err := j.store.list(ctx)
	if err != nil {
		return RecoveryReport{}, ErrJobUnknown
	}
	sort.Strings(ids)
	report := RecoveryReport{Records: make([]RecoveryEvidence, 0, len(ids)), InspectedAt: time.Now().UTC(), Verdict: "quiescent_proved"}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return RecoveryReport{}, err
		}
		evidence, e := j.inspectV2(ctx, id)
		if e != nil {
			return RecoveryReport{}, ErrJobUnknown
		}
		switch evidence.Activity {
		case "uncertain":
			report.Verdict = "uncertain"
		case "active":
			if report.Verdict != "uncertain" {
				report.Verdict = "active"
			}
		}
		report.Records = append(report.Records, evidence)
	}
	return report, nil
}
