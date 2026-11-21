package chardet

// singleByteData is the port of chardetng's SingleByteData.
type singleByteData struct {
	encoding      string
	lower         *[128]uint8
	upper         *[128]uint8
	probabilities []uint8
	ascii         int
	nonASCII      int
}

func computeIndex(x, y, asciiClasses, nonASCIIClasses int) (int, bool) {
	if x == 0 && y == 0 {
		return 0, false
	}
	if x < asciiClasses && y < asciiClasses {
		return 0, false
	}
	if y >= asciiClasses {
		return (asciiClasses * nonASCIIClasses) + (asciiClasses+nonASCIIClasses)*(y-asciiClasses) + x, true
	}
	return y*nonASCIIClasses + x - asciiClasses, true
}

func (d *singleByteData) classify(b byte) uint8 {
	if b>>7 == 0 {
		return d.lower[b&0x7F]
	}
	return d.upper[b&0x7F]
}

func (d *singleByteData) isLatinAlphabetic(caselessClass uint8) bool {
	c := int(caselessClass)
	return c > 0 && c < d.ascii+d.nonASCII
}

func (d *singleByteData) isNonLatinAlphabetic(caselessClass uint8, isWindows1256 bool) bool {
	c := int(caselessClass)
	lowerBound := 1
	if isWindows1256 {
		lowerBound = windows1256Zwnj
	}
	return c > lowerBound && c < d.ascii+d.nonASCII
}

func (d *singleByteData) score(currentClass, previousClass uint8, isWindows1256 bool) int64 {
	current := int(currentClass)
	previous := int(previousClass)
	storedBoundary := d.ascii + d.nonASCII
	if current < storedBoundary {
		if previous < storedBoundary {
			// Both below
			if index, ok := computeIndex(previous, current, d.ascii, d.nonASCII); ok {
				b := d.probabilities[index]
				if b == 255 {
					return implausibilityPenalty
				}
				return int64(b)
			}
			return 0
		}
		// Current below stored, prev above
		if current == 0 || current == asciiDigit || (isWindows1256 && current == windows1256Zwnj) {
			// Current is space-like
			return 0
		}
		// Current is alphabetic
		switch previous - storedBoundary {
		case plausibleNextToAlphabeticOnEitherSide:
			return 0
		case implausibleNextToAlphabeticOnEitherSide:
			return implausibilityPenalty
		case implausibleBeforeAlphabetic:
			return implausibilityPenalty
		case implausibleAfterAlphabetic:
			return 0
		case plausibleNextToNonASCIIAlphabeticOnEitherSide:
			if current < d.ascii {
				return implausibilityPenalty
			}
			return 0
		case plausibleNextToASCIIAlphabeticOnEitherSide:
			if current < d.ascii {
				return 0
			}
			return implausibilityPenalty
		default:
			// previous is ASCII_DIGIT
			return 0
		}
	}
	if previous < storedBoundary {
		// Current above, prev below
		if previous == 0 || previous == asciiDigit || (isWindows1256 && previous == windows1256Zwnj) {
			// Previous is space-like
			return 0
		}
		// Current is alphabetic
		switch current - storedBoundary {
		case plausibleNextToAlphabeticOnEitherSide:
			return 0
		case implausibleNextToAlphabeticOnEitherSide:
			return implausibilityPenalty
		case implausibleBeforeAlphabetic:
			return 0
		case implausibleAfterAlphabetic:
			return implausibilityPenalty
		case plausibleNextToNonASCIIAlphabeticOnEitherSide:
			if previous < d.ascii {
				return implausibilityPenalty
			}
			return 0
		case plausibleNextToASCIIAlphabeticOnEitherSide:
			if previous < d.ascii {
				return 0
			}
			return implausibilityPenalty
		default:
			// current is ASCII_DIGIT
			return 0
		}
	}
	if current == asciiDigit || previous == asciiDigit {
		return 0
	}
	// Both above
	return implausibilityPenalty
}
