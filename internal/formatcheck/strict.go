package formatcheck

import "fmt"

// ValidateStrict runs full layout validation aligned with LeRobot on-disk requirements.
func ValidateStrict(root string) error {
	rep, err := ValidateWithOptions(root, Options{Strict: true})
	if err != nil {
		return err
	}
	if !rep.OK {
		return fmt.Errorf("%s", rep.Errors[0])
	}
	return nil
}
