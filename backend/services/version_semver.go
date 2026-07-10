package services

import (
	"fmt"
	"strconv"
	"strings"
)

type SemVersion struct {
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease []string
}

func ParseSemVersion(raw string) (SemVersion, error) {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	if value == "" {
		return SemVersion{}, fmt.Errorf("invalid semantic version")
	}

	versionAndBuild := strings.Split(value, "+")
	if len(versionAndBuild) > 2 || versionAndBuild[0] == "" {
		return SemVersion{}, fmt.Errorf("invalid semantic version %q", raw)
	}
	if len(versionAndBuild) == 2 {
		if err := validateSemVersionIdentifiers(versionAndBuild[1], false); err != nil {
			return SemVersion{}, fmt.Errorf("invalid semantic version %q: %w", raw, err)
		}
	}

	coreAndPrerelease := strings.SplitN(versionAndBuild[0], "-", 2)
	core := strings.Split(coreAndPrerelease[0], ".")
	if len(core) != 3 {
		return SemVersion{}, fmt.Errorf("invalid semantic version %q", raw)
	}

	numbers := make([]uint64, 3)
	for i, part := range core {
		if !isNumericIdentifier(part) || (len(part) > 1 && part[0] == '0') {
			return SemVersion{}, fmt.Errorf("invalid semantic version %q", raw)
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return SemVersion{}, fmt.Errorf("invalid semantic version %q: %w", raw, err)
		}
		numbers[i] = parsed
	}

	result := SemVersion{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}
	if len(coreAndPrerelease) == 2 {
		if err := validateSemVersionIdentifiers(coreAndPrerelease[1], true); err != nil {
			return SemVersion{}, fmt.Errorf("invalid semantic version %q: %w", raw, err)
		}
		result.Prerelease = strings.Split(coreAndPrerelease[1], ".")
	}
	return result, nil
}

func CompareSemVersion(left, right SemVersion) int {
	for _, values := range [][2]uint64{
		{left.Major, right.Major},
		{left.Minor, right.Minor},
		{left.Patch, right.Patch},
	} {
		if values[0] < values[1] {
			return -1
		}
		if values[0] > values[1] {
			return 1
		}
	}

	if len(left.Prerelease) == 0 && len(right.Prerelease) == 0 {
		return 0
	}
	if len(left.Prerelease) == 0 {
		return 1
	}
	if len(right.Prerelease) == 0 {
		return -1
	}

	limit := len(left.Prerelease)
	if len(right.Prerelease) < limit {
		limit = len(right.Prerelease)
	}
	for i := 0; i < limit; i++ {
		leftPart := left.Prerelease[i]
		rightPart := right.Prerelease[i]
		leftNumeric := isNumericIdentifier(leftPart)
		rightNumeric := isNumericIdentifier(rightPart)

		switch {
		case leftNumeric && rightNumeric:
			leftValue, _ := strconv.ParseUint(leftPart, 10, 64)
			rightValue, _ := strconv.ParseUint(rightPart, 10, 64)
			if leftValue < rightValue {
				return -1
			}
			if leftValue > rightValue {
				return 1
			}
		case leftNumeric:
			return -1
		case rightNumeric:
			return 1
		default:
			if leftPart < rightPart {
				return -1
			}
			if leftPart > rightPart {
				return 1
			}
		}
	}

	if len(left.Prerelease) < len(right.Prerelease) {
		return -1
	}
	if len(left.Prerelease) > len(right.Prerelease) {
		return 1
	}
	return 0
}

func NormalizeUpgradeChannel(raw string) (string, error) {
	channel := strings.ToLower(strings.TrimSpace(raw))
	switch channel {
	case "stable", "prerelease", "nightly":
		return channel, nil
	default:
		return "", fmt.Errorf("unsupported agent release channel %q", raw)
	}
}

func validateSemVersionIdentifiers(value string, rejectNumericLeadingZero bool) error {
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("empty identifier")
		}
		for _, char := range part {
			if (char >= '0' && char <= '9') ||
				(char >= 'A' && char <= 'Z') ||
				(char >= 'a' && char <= 'z') || char == '-' {
				continue
			}
			return fmt.Errorf("invalid identifier %q", part)
		}
		if rejectNumericLeadingZero && isNumericIdentifier(part) && len(part) > 1 && part[0] == '0' {
			return fmt.Errorf("numeric identifier %q has a leading zero", part)
		}
	}
	return nil
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
