package aggregate

import (
	"fmt"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func validateCompatible(infos []meta.DatasetInfo) error {
	if len(infos) < 2 {
		return fmt.Errorf("need at least two datasets")
	}
	base := infos[0]
	for i := 1; i < len(infos); i++ {
		other := infos[i]
		if base.FPS != other.FPS {
			return fmt.Errorf("fps mismatch: %d vs %d", base.FPS, other.FPS)
		}
		if ptrStr(base.RobotType) != ptrStr(other.RobotType) {
			return fmt.Errorf("robot_type mismatch: %v vs %v", base.RobotType, other.RobotType)
		}
		if !featuresEqual(base.Features, other.Features) {
			return fmt.Errorf("features mismatch between dataset 0 and %d", i)
		}
	}
	return nil
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func featuresEqual(a, b map[string]meta.FeatureSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for k, fa := range a {
		fb, ok := b[k]
		if !ok || !fa.Equal(fb) {
			return false
		}
	}
	return true
}
