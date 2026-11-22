package tracker

import "github.com/Dicklesworthstone/beads_viewer/pkg/model"

// Capabilities says what writes a tracker route supports. It is derived from
// the origin the loader already produced; no process is run.
type Capabilities struct {
	// Writable is true when the origin carries a complete tracker route.
	Writable bool
	// ReadOnlyReason says why the origin is not Writable.
	ReadOnlyReason string
	// Tracker is "bd", "br", or "" when the origin is unknown.
	Tracker string
	// Claim is true when the tracker's update command supports --claim.
	Claim bool
	// Signoff is true when signing off works: natively on a recent bd, else
	// through the label/metadata fallback. It equals Writable.
	Signoff bool
}

// CapabilitiesOf reports the capabilities of origin, which may be nil.
func CapabilitiesOf(origin *model.IssueOrigin) Capabilities {
	var caps Capabilities
	if err := OriginError(origin); err != nil {
		caps.ReadOnlyReason = err.Error()
	} else {
		caps.Writable = true
	}
	caps.Signoff = caps.Writable
	if origin != nil {
		if origin.Tracker == "bd" || origin.Tracker == "br" {
			caps.Tracker = origin.Tracker
		}
		caps.Claim = origin.SupportsClaim
	}
	return caps
}
