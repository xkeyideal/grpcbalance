package label

import ss "github.com/Masterminds/semver/v3"

type Version = ss.Version

func NewVersion(version string) (*Version, error) {
	return ss.NewVersion(version)
}

func Satisfy(constraint, version string) (bool, error) {
	v, err := ss.NewVersion(version)
	if err != nil {
		return false, err
	}
	c, err := ss.NewConstraint(constraint)
	if err != nil {
		return false, err
	}
	return c.Check(v), nil
}

func IsValidConstraint(constraint string) bool {
	_, err := ss.NewConstraint(constraint)
	return err == nil
}

func IsValidVersion(version string) bool {
	_, err := ss.NewVersion(version)
	return err == nil
}
