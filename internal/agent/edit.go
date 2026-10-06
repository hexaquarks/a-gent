package agent

import (
	"context"
	"time"
)

// Edit is an observed, successfully completed change to one file.
type Edit struct {
	Filename    string
	CompletedAt time.Time
	Diff        string
	Truncated   bool
}

// Activity is provider-recorded public output or tool activity, not private reasoning.
type Activity struct {
	Label string
	Text  string
	At    time.Time
}

// Preview retains the latest public activity independently of the last file edit.
type Preview struct {
	Edit     *Edit
	Activity *Activity
}

// PreviewSource reads only the selected session. A successful response contains
// the latest retained edit and activity; nil fields mean no matching records.
type PreviewSource interface {
	LatestPreview(context.Context, Session) (Preview, error)
}
