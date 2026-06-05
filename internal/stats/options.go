package stats

// Mode controls how image/video statistics are computed.
type Mode int

const (
	// ModeSampled matches official compute_episode_stats (estimate_num_samples heuristic).
	ModeSampled Mode = iota
	// ModeFull uses every frame path when computing image/video statistics.
	ModeFull
)

type Options struct {
	Mode Mode
}

func (o Options) normalized() Options {
	return o
}
