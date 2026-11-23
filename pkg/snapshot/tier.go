package snapshot

// Tier is a tiered performance mode chosen from the source issue count (bv-9thm).
// TierUnknown means normal behavior applies.
type Tier int

const (
	TierUnknown Tier = iota
	TierSmall
	TierMedium
	TierLarge
	TierHuge
)

// TierForIssueCount maps a total issue count to its performance tier.
func TierForIssueCount(total int) Tier {
	switch {
	case total <= 0:
		return TierUnknown
	case total < 1000:
		return TierSmall
	case total < 5000:
		return TierMedium
	case total < 20000:
		return TierLarge
	default:
		return TierHuge
	}
}

func (t Tier) String() string {
	switch t {
	case TierSmall:
		return "small"
	case TierMedium:
		return "medium"
	case TierLarge:
		return "large"
	case TierHuge:
		return "huge"
	default:
		return "unknown"
	}
}

// Config selects which expensive parts of a snapshot Build precomputes.
type Config struct {
	PrecomputeTriage   bool
	PrecomputeTree     bool
	PrecomputeInsights bool
	// SkipPhase2 computes only Phase 1 metrics and never schedules the
	// expensive Phase 2 work.
	SkipPhase2 bool
}

// DefaultConfig precomputes everything and runs Phase 2.
func DefaultConfig() Config {
	return Config{
		PrecomputeTriage:   true,
		PrecomputeTree:     true,
		PrecomputeInsights: true,
		SkipPhase2:         false,
	}
}

// ConfigForTier returns the build configuration for a dataset tier.
func ConfigForTier(t Tier) Config {
	cfg := DefaultConfig()
	switch t {
	case TierLarge:
		cfg.PrecomputeTriage = false
		cfg.PrecomputeTree = false
		cfg.PrecomputeInsights = false
	case TierHuge:
		cfg.PrecomputeTriage = false
		cfg.PrecomputeTree = false
		cfg.PrecomputeInsights = false
		cfg.SkipPhase2 = true
	}
	return cfg
}
