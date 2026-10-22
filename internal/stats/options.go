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
	// ExternalVideo skips statistics for video features: the caller computes
	// them and writes them into the merged metadata itself. Decoding every
	// episode MP4 for stats otherwise dominates finalize time.
	ExternalVideo bool
}

func (o Options) normalized() Options {
	return o
}

// skips reports whether stats for a feature of dtype are left to the caller.
func (o Options) skips(dtype string) bool {
	return o.ExternalVideo && dtype == "video"
}

// OwnedFeatures drops features whose statistics the caller owns, so coverage
// checks only require keys this package computes.
func (o Options) OwnedFeatures(features map[string]FeatureDesc) map[string]FeatureDesc {
	if !o.ExternalVideo {
		return features
	}
	out := make(map[string]FeatureDesc, len(features))
	for k, v := range features {
		if !o.skips(v.DType) {
			out[k] = v
		}
	}
	return out
}
