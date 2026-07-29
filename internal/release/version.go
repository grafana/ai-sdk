package release

import (
	"fmt"
	"regexp"
	"strconv"
)

var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z][0-9A-Za-z-]*)\.([1-9][0-9]*))?$`)

// Version is the semantic version subset supported by the release tool.
type Version struct {
	Major      int    `json:"major"`
	Minor      int    `json:"minor"`
	Patch      int    `json:"patch"`
	Prerelease string `json:"prerelease,omitempty"`
	Sequence   int    `json:"sequence,omitempty"`
}

// ParseVersion parses the strict version format supported by the release tool.
func ParseVersion(value string) (Version, error) {
	match := versionPattern.FindStringSubmatch(value)
	if match == nil {
		return Version{}, fmt.Errorf("release: invalid version %q", value)
	}

	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	sequence := 0
	if match[5] != "" {
		sequence, _ = strconv.Atoi(match[5])
	}
	return Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: match[4],
		Sequence:   sequence,
	}, nil
}

// String returns the canonical v-prefixed semantic version.
func (v Version) String() string {
	base := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease == "" {
		return base
	}
	return fmt.Sprintf("%s-%s.%d", base, v.Prerelease, v.Sequence)
}

// Compare orders versions according to the supported semantic version subset.
func (v Version) Compare(other Version) int {
	for _, pair := range [][2]int{
		{v.Major, other.Major},
		{v.Minor, other.Minor},
		{v.Patch, other.Patch},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease < other.Prerelease {
		return -1
	}
	if v.Prerelease > other.Prerelease {
		return 1
	}
	if v.Sequence < other.Sequence {
		return -1
	}
	if v.Sequence > other.Sequence {
		return 1
	}
	return 0
}

// Bump is one of the supported semantic version increments.
type Bump string

const (
	// BumpPatch increments a patch or continues a prerelease channel.
	BumpPatch Bump = "patch"
	// BumpMinor increments the minor version and resets the patch.
	BumpMinor Bump = "minor"
	// BumpMajor increments the major version and resets minor and patch.
	BumpMajor Bump = "major"
)

// ParseBump validates a semantic version bump name.
func ParseBump(value string) (Bump, error) {
	switch Bump(value) {
	case BumpPatch, BumpMinor, BumpMajor:
		return Bump(value), nil
	default:
		return "", fmt.Errorf("release: invalid bump %q", value)
	}
}

// HigherBump returns the larger of two semantic version bumps.
func HigherBump(left, right Bump) Bump {
	rank := map[Bump]int{BumpPatch: 1, BumpMinor: 2, BumpMajor: 3}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

// NextVersion calculates a normal release, prerelease iteration, or promotion.
func NextVersion(current Version, bump Bump, prerelease string, stable bool) (Version, error) {
	if stable {
		if current.Prerelease == "" {
			return Version{}, fmt.Errorf("release: %s is already stable", current)
		}
		current.Prerelease = ""
		current.Sequence = 0
		return current, nil
	}

	if current.Prerelease != "" && (prerelease == "" || prerelease == current.Prerelease) && bump == BumpPatch {
		current.Sequence++
		return current, nil
	}

	if prerelease == "" && current.Prerelease != "" {
		prerelease = current.Prerelease
	}
	switch bump {
	case BumpPatch:
		if current.Prerelease == "" {
			current.Patch++
		}
	case BumpMinor:
		current.Minor++
		current.Patch = 0
	case BumpMajor:
		current.Major++
		current.Minor = 0
		current.Patch = 0
	default:
		return Version{}, fmt.Errorf("release: invalid bump %q", bump)
	}

	current.Prerelease = prerelease
	if prerelease == "" {
		current.Sequence = 0
	} else {
		current.Sequence = 1
	}
	return current, nil
}
