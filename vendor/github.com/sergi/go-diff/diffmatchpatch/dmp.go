/**
 * dmp.go
 *
 * Go language implementation of Google Diff, Match, and Patch library
 *
 * Original library is Copyright (c) 2006 Google Inc.
 * http://code.google.com/p/google-diff-match-patch/
 *
 * Copyright (c) 2012 Sergi Mansilla <sergi.mansilla@gmail.com>
 * https://github.com/sergi/go-diff
 *
 * See included LICENSE file for license details.
 */

// Package diffmatchpatch offers robust algorithms to perform the
// operations required for synchronizing plain text.
package diffmatchpatch

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The data structure representing a diff is an array of tuples:
// [[DiffDelete, 'Hello'], [DiffInsert, 'Goodbye'], [DiffEqual, ' world.']]
// which means: delete 'Hello', add 'Goodbye' and keep ' world.'

type Operation int8

const (
	DiffDelete Operation = -1
	DiffInsert Operation = 1
	DiffEqual  Operation = 0
)

// unescaper unescapes selected chars for compatibility with JavaScript's encodeURI.
// In speed critical applications this could be dropped since the
// receiving application will certainly decode these fine.
// Note that this function is case-sensitive.  Thus "%3F" would not be
// unescaped.  But this is ok because it is only called with the output of
// HttpUtility.UrlEncode which returns lowercase hex.
//
// Example: "%3f" -> "?", "%24" -> "$", etc.
var unescaper = strings.NewReplacer(
	"%21", "!", "%7E", "~", "%27", "'",
	"%28", "(", "%29", ")", "%3B", ";",
	"%2F", "/", "%3F", "?", "%3A", ":",
	"%40", "@", "%26", "&", "%3D", "=",
	"%2B", "+", "%24", "$", "%2C", ",", "%23", "#", "%2A", "*")

// Define some regex patterns for matching boundaries.
var (
	nonAlphaNumericRegex_ = regexp.MustCompile(`[^a-zA-Z0-9]`)
	whitespaceRegex_      = regexp.MustCompile(`\s`)
	linebreakRegex_       = regexp.MustCompile(`[\r\n]`)
	blanklineEndRegex_    = regexp.MustCompile(`\n\r?\n$`)
	blanklineStartRegex_  = regexp.MustCompile(`^\r?\n\r?\n`)
)

func splice(slice []Diff, index int, amount int, elements ...Diff) []Diff {
	return append(slice[:index], append(elements, slice[index+amount:]...)...)
}

// indexOf returns the first index of pattern in str, starting at str[i].
func indexOf(str string, pattern string, i int) int {
	if i > len(str)-1 {
		return -1
	}
	if i <= 0 {
		return strings.Index(str, pattern)
	}
	ind := strings.Index(str[i:], pattern)
	if ind == -1 {
		return -1
	}
	return ind + i
}

// lastIndexOf returns the last index of pattern in str, starting at str[i].
func lastIndexOf(str string, pattern string, i int) int {
	if i < 0 {
		return -1
	}
	if i >= len(str) {
		return strings.LastIndex(str, pattern)
	}
	_, size := utf8.DecodeRuneInString(str[i:])
	return strings.LastIndex(str[:i+size], pattern)
}

// Return the index of pattern in target, starting at target[i].
func runesIndexOf(target, pattern []rune, i int) int {
	if i > len(target)-1 {
		return -1
	}
	if i <= 0 {
		return runesIndex(target, pattern)
	}
	ind := runesIndex(target[i:], pattern)
	if ind == -1 {
		return -1
	}
	return ind + i
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func runesEqual(r1, r2 []rune) bool {
	if len(r1) != len(r2) {
		return false
	}
	for i, c := range r1 {
		if c != r2[i] {
			return false
		}
	}
	return true
}

// The equivalent of strings.Index for rune slices.
func runesIndex(r1, r2 []rune) int {
	last := len(r1) - len(r2)
	for i := 0; i <= last; i++ {
		if runesEqual(r1[i:i+len(r2)], r2) {
			return i
		}
	}
	return -1
}

// Diff represents one diff operation
type Diff struct {
	Type Operation
	Text string
}

// Patch represents one patch operation.
type Patch struct {
	diffs   []Diff
	start1  int
	start2  int
	length1 int
	length2 int
}

// String emulates GNU diff's format.
// Header: @@ -382,8 +481,9 @@
// Indicies are printed as 1-based, not 0-based.
func (p *Patch) String() string {
	var coords1, coords2 string

	if p.length1 == 0 {
		coords1 = strconv.Itoa(p.start1) + ",0"
	} else if p.length1 == 1 {
		coords1 = strconv.Itoa(p.start1 + 1)
	} else {
		coords1 = strconv.Itoa(p.start1+1) + "," + strconv.Itoa(p.length1)
	}

	if p.length2 == 0 {
		coords2 = strconv.Itoa(p.start2) + ",0"
	} else if p.length2 == 1 {
		coords2 = strconv.Itoa(p.start2 + 1)
	} else {
		coords2 = strconv.Itoa(p.start2+1) + "," + strconv.Itoa(p.length2)
	}

	var text bytes.Buffer
	text.WriteString("@@ -" + coords1 + " +" + coords2 + " @@\n")

	// Escape the body of the patch with %xx notation.
	for _, aDiff := range p.diffs {
		switch aDiff.Type {
		case DiffInsert:
			text.WriteString("+")
		case DiffDelete:
			text.WriteString("-")
		case DiffEqual:
			text.WriteString(" ")
		}

		text.WriteString(strings.Replace(url.QueryEscape(aDiff.Text), "+", " ", -1))
		text.WriteString("\n")
	}

	return unescaper.Replace(text.String())
}

type DiffMatchPatch struct {
	// Number of seconds to map a diff before giving up (0 for infinity).
	DiffTimeout time.Duration
	// Cost of an empty edit operation in terms of edit characters.
	DiffEditCost int
	// How far to search for a match (0 = exact location, 1000+ = broad match).
	// A match this many characters away from the expected location will add
	// 1.0 to the score (0.0 is a perfect match).
	MatchDistance int
	// When deleting a large block of text (over ~64 characters), how close do
	// the contents have to be to match the expected contents. (0.0 = perfection,
	// 1.0 = very loose).  Note that Match_Threshold controls how closely the
	// end points of a delete need to match.
	PatchDeleteThreshold float64
	// Chunk size for context length.
	PatchMargin int
	// The number of bits in an int.
	MatchMaxBits int
	// At what point is no match declared (0.0 = perfection, 1.0 = very loose).
	MatchThreshold float64
}

// New creates a new DiffMatchPatch object with default parameters.
func New() *DiffMatchPatch {
	// Defaults.
	return &DiffMatchPatch{
		DiffTimeout:          time.Second,
		DiffEditCost:         4,
		MatchThreshold:       0.5,
		MatchDistance:        1000,
		PatchDeleteThreshold: 0.5,
		PatchMargin:          4,
		MatchMaxBits:         32,
	}
}

// DiffMain finds the differences between two texts.
func (dmp *DiffMatchPatch) DiffMain(text1, text2 string, checklines bool) []Diff {
	var deadline time.Time
	if dmp.DiffTimeout <= 0 {
		deadline = time.Now().Add(24 * 365 * time.Hour)
	} else {
		deadline = time.Now().Add(dmp.DiffTimeout)
	}
	return dmp.diffMain(text1, text2, checklines, deadline)
}

func (dmp *DiffMatchPatch) diffMain(text1, text2 string, checklines bool, deadline time.Time) []Diff {
	return dmp.diffMainRunes([]rune(text1), []rune(text2), checklines, deadline)
}

// DiffMainRunes finds the differences between two rune sequences.
func (dmp *DiffMatchPatch) DiffMainRunes(text1, text2 []rune, checklines bool) []Diff {
	var deadline time.Time
	if dmp.DiffTimeout <= 0 {
		deadline = time.Now().Add(24 * 365 * time.Hour)
	} else {
		deadline = time.Now().Add(dmp.DiffTimeout)
	}
	return dmp.diffMainRunes(text1, text2, checklines, deadline)
}

func (dmp *DiffMatchPatch) diffMainRunes(text1, text2 []rune, checklines bool, deadline time.Time) []Diff {
	if runesEqual(text1, text2) {
		var diffs []Diff
		if len(text1) > 0 {
			diffs = append(diffs, Diff{DiffEqual, string(text1)})
		}
		return diffs
	}
	// Trim off common prefix (speedup).
	commonlength := commonPrefixLength(text1, text2)
	commonprefix := text1[:commonlength]
	text1 = text1[commonlength:]
	text2 = text2[commonlength:]

	// Trim off common suffix (speedup).
	commonlength = commonSuffixLength(text1, text2)
	commonsuffix := text1[len(text1)-commonlength:]
	text1 = text1[:len(text1)-commonlength]
	text2 = text2[:len(text2)-commonlength]

	// Compute the diff on the middle block.
	diffs := dmp.diffCompute(text1, text2, checklines, deadline)

	// Restore the prefix and suffix.
	if len(commonprefix) != 0 {
		diffs = append([]Diff{Diff{DiffEqual, string(commonprefix)}}, diffs...)
	}
	if len(commonsuffix) != 0 {
		diffs = append(diffs, Diff{DiffEqual, string(commonsuffix)})
	}

	return dmp.DiffCleanupMerge(diffs)
}

// diffCompute finds the differences between two rune slices.  Assumes that the texts do not
// have any common prefix or suffix.
func (dmp *DiffMatchPatch) diffCompute(text1, text2 []rune, checklines bool, deadline time.Time) []Diff {
	diffs := []Diff{}
	if len(text1) == 0 {
		// Just add some text (speedup).
		return append(diffs, Diff{DiffInsert, string(text2)})
	} else if len(text2) == 0 {
		// Just delete some text (speedup).
		return append(diffs, Diff{DiffDelete, string(text1)})
	}

	var longtext, shorttext []rune
	if len(text1) > len(text2) {
		longtext = text1
		shorttext = text2
	} else {
		longtext = text2
		shorttext = text1
	}

	if i := runesIndex(longtext, shorttext); i != -1 {
		op := DiffInsert
		// Swap insertions for deletions if diff is reversed.
		if len(text1) > len(text2) {
			op = DiffDelete
		}
		// Shorter text is inside the longer text (speedup).
		return []Diff{
			Diff{op, string(longtext[:i])},
			Diff{DiffEqual, string(shorttext)},
			Diff{op, string(longtext[i+len(shorttext):])},
		}
	} else if len(shorttext) == 1 {
		// Single character string.
		// After the previous speedup, the character can't be an equality.
		return []Diff{
			Diff{DiffDelete, string(text1)},
			Diff{DiffInsert, string(text2)},
		}
		// Check to see if the problem can be split in two.
	} else if hm := dmp.diffHalfMatch(text1, text2); hm != nil {
		// A half-match was found, sort out the return data.
		text1_a := hm[0]
		text1_b := hm[1]
		text2_a := hm[2]
		text2_b := hm[3]
		mid_common := hm[4]
		// Send both pairs off for separate processing.
		diffs_a := dmp.diffMainRunes(text1_a, text2_a, checklines, deadline)
		diffs_b := dmp.diffMainRunes(text1_b, text2_b, checklines, deadline)
		// Merge the results.
		return append(diffs_a, append([]Diff{Diff{DiffEqual, string(mid_common)}}, diffs_b...)...)
	} else if checklines && len(text1) > 100 && len(text2) > 100 {
		return dmp.diffLineMode(text1, text2, deadline)
	}
	return dmp.diffBisect(text1, text2, deadline)
}

// diffLineMode does a quick line-level diff on both []runes, then rediff the parts for
// greater accuracy. This speedup can produce non-minimal diffs.
func (dmp *DiffMatchPatch) diffLineMode(text1, text2 []rune, deadline time.Time) []Diff {
	// Scan the text on a line-by-line basis first.
	text1, text2, linearray := dmp.diffLinesToRunes(text1, text2)

	diffs := dmp.diffMainRunes(text1, text2, false, deadline)

	// Convert the diff back to original text.
	diffs = dmp.DiffCharsToLines(diffs, linearray)
	// Eliminate freak matches (e.g. blank lines)
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Rediff any replacement blocks, this time character-by-character.
	// Add a dummy entry at the end.
	diffs = append(diffs, Diff{DiffEqual, ""})

	pointer := 0
	count_delete := 0
	count_insert := 0
	text_delete := ""
	text_insert := ""

	for pointer < len(diffs) {
		switch diffs[pointer].Type {
		case DiffInsert:
			count_insert++
			text_insert += diffs[pointer].Text
		case DiffDelete:
			count_delete++
			text_delete += diffs[pointer].Text
		case DiffEqual:
			// Upon reaching an equality, check for prior redundancies.
			if count_delete >= 1 && count_insert >= 1 {
				// Delete the offending records and add the merged ones.
				diffs = splice(diffs, pointer-count_delete-count_insert,
					count_delete+count_insert)

				pointer = pointer - count_delete - count_insert
				a := dmp.diffMain(text_delete, text_insert, false, deadline)
				for j := len(a) - 1; j >= 0; j-- {
					diffs = splice(diffs, pointer, 0, a[j])
				}
				pointer = pointer + len(a)
			}

			count_insert = 0
			count_delete = 0
			text_delete = ""
			text_insert = ""
		}
		pointer++
	}

	return diffs[:len(diffs)-1] // Remove the dummy entry at the end.
}

// DiffBisect finds the 'middle snake' of a diff, split the problem in two
// and return the recursively constructed diff.
// See Myers 1986 paper: An O(ND) Difference Algorithm and Its Variations.
func (dmp *DiffMatchPatch) DiffBisect(text1, text2 string, deadline time.Time) []Diff {
	// Unused in this code, but retained for interface compatibility.
	return dmp.diffBisect([]rune(text1), []rune(text2), deadline)
}

// diffBisect finds the 'middle snake' of a diff, splits the problem in two
// and returns the recursively constructed diff.
// See Myers's 1986 paper: An O(ND) Difference Algorithm and Its Variations.
func (dmp *DiffMatchPatch) diffBisect(runes1, runes2 []rune, deadline time.Time) []Diff {
	// Cache the text lengths to prevent multiple calls.
	runes1_len, runes2_len := len(runes1), len(runes2)

	max_d := (runes1_len + runes2_len + 1) / 2
	v_offset := max_d
	v_length := 2 * max_d

	v1 := make([]int, v_length)
	v2 := make([]int, v_length)
	for i := range v1 {
		v1[i] = -1
		v2[i] = -1
	}
	v1[v_offset+1] = 0
	v2[v_offset+1] = 0

	delta := runes1_len - runes2_len
	// If the total number of characters is odd, then the front path will collide
	// with the reverse path.
	front := (delta%2 != 0)
	// Offsets for start and end of k loop.
	// Prevents mapping of space beyond the grid.
	k1start := 0
	k1end := 0
	k2start := 0
	k2end := 0
	for d := 0; d < max_d; d++ {
		// Bail out if deadline is reached.
		if time.Now().After(deadline) {
			break
		}

		// Walk the front path one step.
		for k1 := -d + k1start; k1 <= d-k1end; k1 += 2 {
			k1_offset := v_offset + k1
			var x1 int

			if k1 == -d || (k1 != d && v1[k1_offset-1] < v1[k1_offset+1]) {
				x1 = v1[k1_offset+1]
			} else {
				x1 = v1[k1_offset-1] + 1
			}

			y1 := x1 - k1
			for x1 < runes1_len && y1 < runes2_len {
				if runes1[x1] != runes2[y1] {
					break
				}
				x1++
				y1++
			}
			v1[k1_offset] = x1
			if x1 > runes1_len {
				// Ran off the right of the graph.
				k1end += 2
			} else if y1 > runes2_len {
				// Ran off the bottom of the graph.
				k1start += 2
			} else if front {
				k2_offset := v_offset + delta - k1
				if k2_offset >= 0 && k2_offset < v_length && v2[k2_offset] != -1 {
					// Mirror x2 onto top-left coordinate system.
					x2 := runes1_len - v2[k2_offset]
					if x1 >= x2 {
						// Overlap detected.
						return dmp.diffBisectSplit_(runes1, runes2, x1, y1, deadline)
					}
				}
			}
		}
		// Walk the reverse path one step.
		for k2 := -d + k2start; k2 <= d-k2end; k2 += 2 {
			k2_offset := v_offset + k2
			var x2 int
			if k2 == -d || (k2 != d && v2[k2_offset-1] < v2[k2_offset+1]) {
				x2 = v2[k2_offset+1]
			} else {
				x2 = v2[k2_offset-1] + 1
			}
			var y2 = x2 - k2
			for x2 < runes1_len && y2 < runes2_len {
				if runes1[runes1_len-x2-1] != runes2[runes2_len-y2-1] {
					break
				}
				x2++
				y2++
			}
			v2[k2_offset] = x2
			if x2 > runes1_len {
				// Ran off the left of the graph.
				k2end += 2
			} else if y2 > runes2_len {
				// Ran off the top of the graph.
				k2start += 2
			} else if !front {
				k1_offset := v_offset + delta - k2
				if k1_offset >= 0 && k1_offset < v_length && v1[k1_offset] != -1 {
					x1 := v1[k1_offset]
					y1 := v_offset + x1 - k1_offset
					// Mirror x2 onto top-left coordinate system.
					x2 = runes1_len - x2
					if x1 >= x2 {
						// Overlap detected.
						return dmp.diffBisectSplit_(runes1, runes2, x1, y1, deadline)
					}
				}
			}
		}
	}
	// Diff took too long and hit the deadline or
	// number of diffs equals number of characters, no commonality at all.
	return []Diff{
		Diff{DiffDelete, string(runes1)},
		Diff{DiffInsert, string(runes2)},
	}
}

func (dmp *DiffMatchPatch) diffBisectSplit_(runes1, runes2 []rune, x, y int,
	deadline time.Time) []Diff {
	runes1a := runes1[:x]
	runes2a := runes2[:y]
	runes1b := runes1[x:]
	runes2b := runes2[y:]

	// Compute both diffs serially.
	diffs := dmp.diffMainRunes(runes1a, runes2a, false, deadline)
	diffsb := dmp.diffMainRunes(runes1b, runes2b, false, deadline)

	return append(diffs, diffsb...)
}

// DiffLinesToChars split two texts into a list of strings.  Reduces the texts to a string of
// hashes where each Unicode character represents one line.
// It's slightly faster to call DiffLinesToRunes first, followed by DiffMainRunes.
func (dmp *DiffMatchPatch) DiffLinesToChars(text1, text2 string) (string, string, []string) {
	chars1, chars2, lineArray := dmp.DiffLinesToRunes(text1, text2)
	return string(chars1), string(chars2), lineArray
}

// DiffLinesToRunes splits two texts into a list of runes.  Each rune represents one line.
func (dmp *DiffMatchPatch) DiffLinesToRunes(text1, text2 string) ([]rune, []rune, []string) {
	// '\x00' is a valid character, but various debuggers don't like it.
	// So we'll insert a junk entry to avoid generating a null character.
	lineArray := []string{""}    // e.g. lineArray[4] == 'Hello\n'
	lineHash := map[string]int{} // e.g. lineHash['Hello\n'] == 4

	chars1 := dmp.diffLinesToRunesMunge(text1, &lineArray, lineHash)
	chars2 := dmp.diffLinesToRunesMunge(text2, &lineArray, lineHash)

	return chars1, chars2, lineArray
}

func (dmp *DiffMatchPatch) diffLinesToRunes(text1, text2 []rune) ([]rune, []rune, []string) {
	return dmp.DiffLinesToRunes(string(text1), string(text2))
}

// diffLinesToRunesMunge splits a text into an array of strings.  Reduces the
// texts to a []rune where each Unicode character represents one line.
// We use strings instead of []runes as input mainly because you can't use []rune as a map key.
func (dmp *DiffMatchPatch) diffLinesToRunesMunge(text string, lineArray *[]string, lineHash map[string]int) []rune {
	// Walk the text, pulling out a substring for each line.
	// text.split('\n') would would temporarily double our memory footprint.
	// Modifying text would create many large strings to garbage collect.
	lineStart := 0
	lineEnd := -1
	runes := []rune{}

	for lineEnd < len(text)-1 {
		lineEnd = indexOf(text, "\n", lineStart)

		if lineEnd == -1 {
			lineEnd = len(text) - 1
		}

		line := text[lineStart : lineEnd+1]
		lineStart = lineEnd + 1
		lineValue_, ok := lineHash[line]

		if ok {
			runes = append(runes, rune(lineValue_))
		} else {
			*lineArray = append(*lineArray, line)
			lineHash[line] = len(*lineArray) - 1
			runes = append(runes, rune(len(*lineArray)-1))
		}
	}

	return runes
}

// DiffCharsToLines rehydrates the text in a diff from a string of line hashes to real lines of
// text.
func (dmp *DiffMatchPatch) DiffCharsToLines(diffs []Diff, lineArray []string) []Diff {
	hydrated := make([]Diff, 0, len(diffs))
	for _, aDiff := range diffs {
		chars := aDiff.Text
		text := make([]string, len(chars))

		for i, r := range chars {
			text[i] = lineArray[r]
		}

		aDiff.Text = strings.Join(text, "")
		hydrated = append(hydrated, aDiff)
	}
	return hydrated
}

// DiffCommonPrefix determines the common prefix length of two strings.
func (dmp *DiffMatchPatch) DiffCommonPrefix(text1, text2 string) int {
	// Unused in this code, but retained for interface compatibility.
	return commonPrefixLength([]rune(text1), []rune(text2))
}

// DiffCommonSuffix determines the common suffix length of two strings.
func (dmp *DiffMatchPatch) DiffCommonSuffix(text1, text2 string) int {
	// Unused in this code, but retained for interface compatibility.
	return commonSuffixLength([]rune(text1), []rune(text2))
}

// commonPrefixLength returns the length of the common prefix of two rune slices.
func commonPrefixLength(text1, text2 []rune) int {
	short, long := text1, text2
	if len(short) > len(long) {
		short, long = long, short
	}
	for i, r := range short {
		if r != long[i] {
			return i
		}
	}
	return len(short)
}

// commonSuffixLength returns the length of the common suffix of two rune slices.
func commonSuffixLength(text1, text2 []rune) int {
	n := min(len(text1), len(text2))
	for i := 0; i < n; i++ {
		if text1[len(text1)-i-1] != text2[len(text2)-i-1] {
			return i
		}
	}
	return n

	// Binary search.
	// Performance analysis: http://neil.fraser.name/news/2007/10/09/
	/*
	   pointermin := 0
	   pointermax := math.Min(len(text1), len(text2))
	   pointermid := pointermax
	   pointerend := 0
	   for pointermin < pointermid {
	       if text1[len(text1)-pointermid:len(text1)-pointerend] ==
	           text2[len(text2)-pointermid:len(text2)-pointerend] {
	           pointermin = pointermid
	           pointerend = pointermin
	       } else {
	           pointermax = pointermid
	       }
	       pointermid = math.Floor((pointermax-pointermin)/2 + pointermin)
	   }
	   return pointermid
	*/
}

// DiffCommonOverlap determines if the suffix of one string is the prefix of another.
func (dmp *DiffMatchPatch) DiffCommonOverlap(text1 string, text2 string) int {
	// Cache the text lengths to prevent multiple calls.
	text1_length := len(text1)
	text2_length := len(text2)
	// Eliminate the null case.
	if text1_length == 0 || text2_length == 0 {
		return 0
	}
	// Truncate the longer string.
	if text1_length > text2_length {
		text1 = text1[text1_length-text2_length:]
	} else if text1_length < text2_length {
		text2 = text2[0:text1_length]
	}
	text_length := int(math.Min(float64(text1_length), float64(text2_length)))
	// Quick check for the worst case.
	if text1 == text2 {
		return text_length
	}

	// Start by looking for a single character match
	// and increase length until no match is found.
	// Performance analysis: http://neil.fraser.name/news/2010/11/04/
	best := 0
	length := 1
	for {
		pattern := text1[text_length-length:]
		found := strings.Index(text2, pattern)
		if found == -1 {
			return best
		}
		length += found
		if found == 0 || text1[text_length-length:] == text2[0:length] {
			best = length
			length++
		}
	}
	return 0
}

// DiffHalfMatch checks whether the two texts share a substring which is at
// least half the length of the longer text. This speedup can produce non-minimal diffs.
func (dmp *DiffMatchPatch) DiffHalfMatch(text1, text2 string) []string {
	// Unused in this code, but retained for interface compatibility.
	runeSlices := dmp.diffHalfMatch([]rune(text1), []rune(text2))
	if runeSlices == nil {
		return nil
	}

	result := make([]string, len(runeSlices))
	for i, r := range runeSlices {
		result[i] = string(r)
	}
	return result
}

func (dmp *DiffMatchPatch) diffHalfMatch(text1, text2 []rune) [][]rune {
	if dmp.DiffTimeout <= 0 {
		// Don't risk returning a non-optimal diff if we have unlimited time.
		return nil
	}

	var longtext, shorttext []rune
	if len(text1) > len(text2) {
		longtext = text1
		shorttext = text2
	} else {
		longtext = text2
		shorttext = text1
	}

	if len(longtext) < 4 || len(shorttext)*2 < len(longtext) {
		return nil // Pointless.
	}

	// First check if the second quarter is the seed for a half-match.
	hm1 := dmp.diffHalfMatchI(longtext, shorttext, int(float64(len(longtext)+3)/4))

	// Check again based on the third quarter.
	hm2 := dmp.diffHalfMatchI(longtext, shorttext, int(float64(len(longtext)+1)/2))

	hm := [][]rune{}
	if hm1 == nil && hm2 == nil {
		return nil
	} else if hm2 == nil {
		hm = hm1
	} else if hm1 == nil {
		hm = hm2
	} else {
		// Both matched.  Select the longest.
		if len(hm1[4]) > len(hm2[4]) {
			hm = hm1
		} else {
			hm = hm2
		}
	}

	// A half-match was found, sort out the return data.
	if len(text1) > len(text2) {
		return hm
	} else {
		return [][]rune{hm[2], hm[3], hm[0], hm[1], hm[4]}
	}

	return nil
}

/**
 * Does a substring of shorttext exist within longtext such that the substring
 * is at least half the length of longtext?
 * @param {string} longtext Longer string.
 * @param {string} shorttext Shorter string.
 * @param {number} i Start index of quarter length substring within longtext.
 * @return {Array.<string>} Five element Array, containing the prefix of
 *     longtext, the suffix of longtext, the prefix of shorttext, the suffix
 *     of shorttext and the common middle.  Or null if there was no match.
 * @private
 */
func (dmp *DiffMatchPatch) diffHalfMatchI(l, s []rune, i int) [][]rune {
	// Start with a 1/4 length substring at position i as a seed.
	seed := l[i : i+len(l)/4]
	j := -1
	best_common := []rune{}
	best_longtext_a := []rune{}
	best_longtext_b := []rune{}
	best_shorttext_a := []rune{}
	best_shorttext_b := []rune{}

	if j < len(s) {
		j = runesIndexOf(s, seed, j+1)
		for {
			if j == -1 {
				break
			}

			prefixLength := commonPrefixLength(l[i:], s[j:])
			suffixLength := commonSuffixLength(l[:i], s[:j])
			if len(best_common) < suffixLength+prefixLength {
				best_common = concat(s[j-suffixLength:j], s[j:j+prefixLength])
				best_longtext_a = l[:i-suffixLength]
				best_longtext_b = l[i+prefixLength:]
				best_shorttext_a = s[:j-suffixLength]
				best_shorttext_b = s[j+prefixLength:]
			}
			j = runesIndexOf(s, seed, j+1)
		}
	}

	if len(best_common)*2 >= len(l) {
		return [][]rune{
			best_longtext_a,
			best_longtext_b,
			best_shorttext_a,
			best_shorttext_b,
			best_common,
		}
	}
	return nil
}

func concat(r1, r2 []rune) []rune {
	result := make([]rune, len(r1)+len(r2))
	copy(result, r1)
	copy(result[len(r1):], r2)
	return result
}

// Diff_cleanupSemantic reduces the number of edits by eliminating
// semantically trivial equalities.
func (dmp *DiffMatchPatch) DiffCleanupSemantic(diffs []Diff) []Diff {
	changes := false
	equalities := new(Stack) // Stack of indices where equalities are found.

	var lastequality string
	// Always equal to diffs[equalities[equalitiesLength - 1]][1]
	var pointer int // Index of current position.
	// Number of characters that changed prior to the equality.
	var length_insertions1, length_deletions1 int
	// Number of characters that changed after the equality.
	var length_insertions2, length_deletions2 int

	for pointer < len(diffs) {
		if diffs[pointer].Type == DiffEqual { // Equality found.
			equalities.Push(pointer)
			length_insertions1 = length_insertions2
			length_deletions1 = length_deletions2
			length_insertions2 = 0
			length_deletions2 = 0
			lastequality = diffs[pointer].Text
		} else { // An insertion or deletion.
			if diffs[pointer].Type == DiffInsert {
				length_insertions2 += len(diffs[pointer].Text)
			} else {
				length_deletions2 += len(diffs[pointer].Text)
			}
			// Eliminate an equality that is smaller or equal to the edits on both
			// sides of it.
			_difference1 := int(math.Max(float64(length_insertions1), float64(length_deletions1)))
			_difference2 := int(math.Max(float64(length_insertions2), float64(length_deletions2)))
			if len(lastequality) > 0 &&
				(len(lastequality) <= _difference1) &&
				(len(lastequality) <= _difference2) {
				// Duplicate record.
				insPoint := equalities.Peek().(int)
				diffs = append(
					diffs[:insPoint],
					append([]Diff{Diff{DiffDelete, lastequality}}, diffs[insPoint:]...)...)

				// Change second copy to insert.
				diffs[insPoint+1].Type = DiffInsert
				// Throw away the equality we just deleted.
				equalities.Pop()

				if equalities.Len() > 0 {
					equalities.Pop()
					pointer = equalities.Peek().(int)
				} else {
					pointer = -1
				}

				length_insertions1 = 0 // Reset the counters.
				length_deletions1 = 0
				length_insertions2 = 0
				length_deletions2 = 0
				lastequality = ""
				changes = true
			}
		}
		pointer++
	}

	// Normalize the diff.
	if changes {
		diffs = dmp.DiffCleanupMerge(diffs)
	}
	diffs = dmp.DiffCleanupSemanticLossless(diffs)
	// Find any overlaps between deletions and insertions.
	// e.g: <del>abcxxx</del><ins>xxxdef</ins>
	//   -> <del>abc</del>xxx<ins>def</ins>
	// e.g: <del>xxxabc</del><ins>defxxx</ins>
	//   -> <ins>def</ins>xxx<del>abc</del>
	// Only extract an overlap if it is as big as the edit ahead or behind it.
	pointer = 1
	for pointer < len(diffs) {
		if diffs[pointer-1].Type == DiffDelete &&
			diffs[pointer].Type == DiffInsert {
			deletion := diffs[pointer-1].Text
			insertion := diffs[pointer].Text
			overlap_length1 := dmp.DiffCommonOverlap(deletion, insertion)
			overlap_length2 := dmp.DiffCommonOverlap(insertion, deletion)
			if overlap_length1 >= overlap_length2 {
				if float64(overlap_length1) >= float64(len(deletion))/2 ||
					float64(overlap_length1) >= float64(len(insertion))/2 {

					// Overlap found.  Insert an equality and trim the surrounding edits.
					diffs = append(
						diffs[:pointer],
						append([]Diff{Diff{DiffEqual, insertion[:overlap_length1]}}, diffs[pointer:]...)...)
					//diffs.splice(pointer, 0,
					//    [DiffEqual, insertion[0 : overlap_length1)]]
					diffs[pointer-1].Text =
						deletion[0 : len(deletion)-overlap_length1]
					diffs[pointer+1].Text = insertion[overlap_length1:]
					pointer++
				}
			} else {
				if float64(overlap_length2) >= float64(len(deletion))/2 ||
					float64(overlap_length2) >= float64(len(insertion))/2 {
					// Reverse overlap found.
					// Insert an equality and swap and trim the surrounding edits.
					overlap := Diff{DiffEqual, insertion[overlap_length2:]}
					diffs = append(
						diffs[:pointer],
						append([]Diff{overlap}, diffs[pointer:]...)...)
					// diffs.splice(pointer, 0,
					//     [DiffEqual, deletion[0 : overlap_length2)]]
					diffs[pointer-1].Type = DiffInsert
					diffs[pointer-1].Text = insertion[0 : len(insertion)-overlap_length2]
					diffs[pointer+1].Type = DiffDelete
					diffs[pointer+1].Text = deletion[overlap_length2:]
					pointer++
				}
			}
			pointer++
		}
		pointer++
	}

	return diffs
}

// Diff_cleanupSemanticLossless looks for single edits surrounded on both sides by equalities
// which can be shifted sideways to align the edit to a word boundary.
// e.g: The c<ins>at c</ins>ame. -> The <ins>cat </ins>came.
func (dmp *DiffMatchPatch) DiffCleanupSemanticLossless(diffs []Diff) []Diff {

	/**
	 * Given two strings, compute a score representing whether the internal
	 * boundary falls on logical boundaries.
	 * Scores range from 6 (best) to 0 (worst).
	 * Closure, but does not reference any external variables.
	 * @param {string} one First string.
	 * @param {string} two Second string.
	 * @return {number} The score.
	 * @private
	 */
	diffCleanupSemanticScore_ := func(one, two string) int {
		if len(one) == 0 || len(two) == 0 {
			// Edges are the best.
			return 6
		}

		// Each port of this function behaves slightly differently due to
		// subtle differences in each language's definition of things like
		// 'whitespace'.  Since this function's purpose is largely cosmetic,
		// the choice has been made to use each language's native features
		// rather than force total conformity.
		rune1, _ := utf8.DecodeLastRuneInString(one)
		rune2, _ := utf8.DecodeRuneInString(two)
		char1 := string(rune1)
		char2 := string(rune2)

		nonAlphaNumeric1 := nonAlphaNumericRegex_.MatchString(char1)
		nonAlphaNumeric2 := nonAlphaNumericRegex_.MatchString(char2)
		whitespace1 := nonAlphaNumeric1 && whitespaceRegex_.MatchString(char1)
		whitespace2 := nonAlphaNumeric2 && whitespaceRegex_.MatchString(char2)
		lineBreak1 := whitespace1 && linebreakRegex_.MatchString(char1)
		lineBreak2 := whitespace2 && linebreakRegex_.MatchString(char2)
		blankLine1 := lineBreak1 && blanklineEndRegex_.MatchString(one)
		blankLine2 := lineBreak2 && blanklineEndRegex_.MatchString(two)

		if blankLine1 || blankLine2 {
			// Five points for blank lines.
			return 5
		} else if lineBreak1 || lineBreak2 {
			// Four points for line breaks.
			return 4
		} else if nonAlphaNumeric1 && !whitespace1 && whitespace2 {
			// Three points for end of sentences.
			return 3
		} else if whitespace1 || whitespace2 {
			// Two points for whitespace.
			return 2
		} else if nonAlphaNumeric1 || nonAlphaNumeric2 {
			// One point for non-alphanumeric.
			return 1
		}
		return 0
	}

	pointer := 1

	// Intentionally ignore the first and last element (don't need checking).
	for pointer < len(diffs)-1 {
		if diffs[pointer-1].Type == DiffEqual &&
			diffs[pointer+1].Type == DiffEqual {

			// This is a single edit surrounded by equalities.
			equality1 := diffs[pointer-1].Text
			edit := diffs[pointer].Text
			equality2 := diffs[pointer+1].Text

			// First, shift the edit as far left as possible.
			commonOffset := dmp.DiffCommonSuffix(equality1, edit)
			if commonOffset > 0 {
				commonString := edit[len(edit)-commonOffset:]
				equality1 = equality1[0 : len(equality1)-commonOffset]
				edit = commonString + edit[:len(edit)-commonOffset]
				equality2 = commonString + equality2
			}

			// Second, step character by character right, looking for the best fit.
			bestEquality1 := equality1
			bestEdit := edit
			bestEquality2 := equality2
			bestScore := diffCleanupSemanticScore_(equality1, edit) +
				diffCleanupSemanticScore_(edit, equality2)

			for len(edit) != 0 && len(equality2) != 0 {
				_, sz := utf8.DecodeRuneInString(edit)
				if len(equality2) < sz || edit[:sz] != equality2[:sz] {
					break
				}
				equality1 += edit[:sz]
				edit = edit[sz:] + equality2[:sz]
				equality2 = equality2[sz:]
				score := diffCleanupSemanticScore_(equality1, edit) +
					diffCleanupSemanticScore_(edit, equality2)
				// The >= encourages trailing rather than leading whitespace on
				// edits.
				if score >= bestScore {
					bestScore = score
					bestEquality1 = equality1
					bestEdit = edit
					bestEquality2 = equality2
				}
			}

			if diffs[pointer-1].Text != bestEquality1 {
				// We have an improvement, save it back to the diff.
				if len(bestEquality1) != 0 {
					diffs[pointer-1].Text = bestEquality1
				} else {
					diffs = splice(diffs, pointer-1, 1)
					pointer--
				}

				diffs[pointer].Text = bestEdit
				if len(bestEquality2) != 0 {
					diffs[pointer+1].Text = bestEquality2
				} else {
					//splice(diffs, pointer+1, 1)
					diffs = append(diffs[:pointer+1], diffs[pointer+2:]...)
					pointer--
				}
			}
		}
		pointer++
	}

	return diffs
}

// Diff_cleanupEfficiency reduces the number of edits by eliminating
// operationally trivial equalities.
func (dmp *DiffMatchPatch) DiffCleanupEfficiency(diffs []Diff) []Diff {
	changes := false
	// Stack of indices where equalities are found.
	equalities := new(Stack)
	// Always equal to equalities[equalitiesLength-1][1]
	lastequality := ""
	pointer := 0 // Index of current position.
	// Is there an insertion operation before the last equality.
	pre_ins := false
	// Is there a deletion operation before the last equality.
	pre_del := false
	// Is there an insertion operation after the last equality.
	post_ins := false
	// Is there a deletion operation after the last equality.
	post_del := false
	for pointer < len(diffs) {
		if diffs[pointer].Type == DiffEqual { // Equality found.
			if len(diffs[pointer].Text) < dmp.DiffEditCost &&
				(post_ins || post_del) {
				// Candidate found.
				equalities.Push(pointer)
				pre_ins = post_ins
				pre_del = post_del
				lastequality = diffs[pointer].Text
			} else {
				// Not a candidate, and can never become one.
				equalities.Clear()
				lastequality = ""
			}
			post_ins = false
			post_del = false
		} else { // An insertion or deletion.
			if diffs[pointer].Type == DiffDelete {
				post_del = true
			} else {
				post_ins = true
			}
			/*
			 * Five types to be split:
			 * <ins>A</ins><del>B</del>XY<ins>C</ins><del>D</del>
			 * <ins>A</ins>X<ins>C</ins><del>D</del>
			 * <ins>A</ins><del>B</del>X<ins>C</ins>
			 * <ins>A</del>X<ins>C</ins><del>D</del>
			 * <ins>A</ins><del>B</del>X<del>C</del>
			 */
			var sum_pres int
			if pre_ins {
				sum_pres++
			}
			if pre_del {
				sum_pres++
			}
			if post_ins {
				sum_pres++
			}
			if post_del {
				sum_pres++
			}
			if len(lastequality) > 0 &&
				((pre_ins && pre_del && post_ins && post_del) ||
					((len(lastequality) < dmp.DiffEditCost/2) && sum_pres == 3)) {

				// Duplicate record.
				diffs = append(diffs[:equalities.Peek().(int)],
					append([]Diff{Diff{DiffDelete, lastequality}}, diffs[equalities.Peek().(int):]...)...)

				// Change second copy to insert.
				diffs[equalities.Peek().(int)+1].Type = DiffInsert
				equalities.Pop() // Throw away the equality we just deleted.
				lastequality = ""

				if pre_ins && pre_del {
					// No changes made which could affect previous entry, keep going.
					post_ins = true
					post_del = true
					equalities.Clear()
				} else {
					if equalities.Len() > 0 {
						equalities.Pop()
						pointer = equalities.Peek().(int)
					} else {
						pointer = -1
					}
					post_ins = false
					post_del = false
				}
				changes = true
			}
		}
		pointer++
	}

	if changes {
		diffs = dmp.DiffCleanupMerge(diffs)
	}

	return diffs
}

// Diff_cleanupMerge reorders and merges like edit sections.  Merge equalities.
// Any edit section can move as long as it doesn't cross an equality.
func (dmp *DiffMatchPatch) DiffCleanupMerge(diffs []Diff) []Diff {
	// Add a dummy entry at the end.
	diffs = append(diffs, Diff{DiffEqual, ""})
	pointer := 0
	count_delete := 0
	count_insert := 0
	commonlength := 0
	text_delete := ""
	text_insert := ""

	for pointer < len(diffs) {
		switch diffs[pointer].Type {
		case DiffInsert:
			count_insert += 1
			text_insert += diffs[pointer].Text
			pointer += 1
			break
		case DiffDelete:
			count_delete += 1
			text_delete += diffs[pointer].Text
			pointer += 1
			break
		case DiffEqual:
			// Upon reaching an equality, check for prior redundancies.
			if count_delete+count_insert > 1 {
				if count_delete != 0 && count_insert != 0 {
					// Factor out any common prefixies.
					commonlength = dmp.DiffCommonPrefix(text_insert, text_delete)
					if commonlength != 0 {
						x := pointer - count_delete - count_insert
						if x > 0 && diffs[x-1].Type == DiffEqual {
							diffs[x-1].Text += text_insert[:commonlength]
						} else {
							diffs = append([]Diff{Diff{DiffEqual, text_insert[:commonlength]}}, diffs...)
							pointer += 1
						}
						text_insert = text_insert[commonlength:]
						text_delete = text_delete[commonlength:]
					}
					// Factor out any common suffixies.
					commonlength = dmp.DiffCommonSuffix(text_insert, text_delete)
					if commonlength != 0 {
						insert_index := len(text_insert) - commonlength
						delete_index := len(text_delete) - commonlength
						diffs[pointer].Text = text_insert[insert_index:] + diffs[pointer].Text
						text_insert = text_insert[:insert_index]
						text_delete = text_delete[:delete_index]
					}
				}
				// Delete the offending records and add the merged ones.
				if count_delete == 0 {
					diffs = splice(diffs, pointer-count_insert,
						count_delete+count_insert,
						Diff{DiffInsert, text_insert})
				} else if count_insert == 0 {
					diffs = splice(diffs, pointer-count_delete,
						count_delete+count_insert,
						Diff{DiffDelete, text_delete})
				} else {
					diffs = splice(diffs, pointer-count_delete-count_insert,
						count_delete+count_insert,
						Diff{DiffDelete, text_delete},
						Diff{DiffInsert, text_insert})
				}

				pointer = pointer - count_delete - count_insert + 1
				if count_delete != 0 {
					pointer += 1
				}
				if count_insert != 0 {
					pointer += 1
				}
			} else if pointer != 0 && diffs[pointer-1].Type == DiffEqual {
				// Merge this equality with the previous one.
				diffs[pointer-1].Text += diffs[pointer].Text
				diffs = append(diffs[:pointer], diffs[pointer+1:]...)
			} else {
				pointer++
			}
			count_insert = 0
			count_delete = 0
			text_delete = ""
			text_insert = ""
			break
		}
	}

	if len(diffs[len(diffs)-1].Text) == 0 {
		diffs = diffs[0 : len(diffs)-1] // Remove the dummy entry at the end.
	}

	// Second pass: look for single edits surrounded on both sides by
	// equalities which can be shifted sideways to eliminate an equality.
	// e.g: A<ins>BA</ins>C -> <ins>AB</ins>AC
	changes := false
	pointer = 1
	// Intentionally ignore the first and last element (don't need checking).
	for pointer < (len(diffs) - 1) {
		if diffs[pointer-1].Type == DiffEqual &&
			diffs[pointer+1].Type == DiffEqual {
			// This is a single edit surrounded by equalities.
			if strings.HasSuffix(diffs[pointer].Text, diffs[pointer-1].Text) {
				// Shift the edit over the previous equality.
				diffs[pointer].Text = diffs[pointer-1].Text +
					diffs[pointer].Text[:len(diffs[pointer].Text)-len(diffs[pointer-1].Text)]
				diffs[pointer+1].Text = diffs[pointer-1].Text + diffs[pointer+1].Text
				diffs = splice(diffs, pointer-1, 1)
				changes = true
			} else if strings.HasPrefix(diffs[pointer].Text, diffs[pointer+1].Text) {
				// Shift the edit over the next equality.
				diffs[pointer-1].Text += diffs[pointer+1].Text
				diffs[pointer].Text =
					diffs[pointer].Text[len(diffs[pointer+1].Text):] + diffs[pointer+1].Text
				diffs = splice(diffs, pointer+1, 1)
				changes = true
			}
		}
		pointer++
	}

	// If shifts were made, the diff needs reordering and another shift sweep.
	if changes {
		diffs = dmp.DiffCleanupMerge(diffs)
	}

	return diffs
}

// Diff_xIndex. loc is a location in text1, comAdde and return the equivalent location in
// text2.
// e.g. "The cat" vs "The big cat", 1->1, 5->8
func (dmp *DiffMatchPatch) DiffXIndex(diffs []Diff, loc int) int {
	chars1 := 0
	chars2 := 0
	last_chars1 := 0
	last_chars2 := 0
	lastDiff := Diff{}
	for i := 0; i < len(diffs); i++ {
		aDiff := diffs[i]
		if aDiff.Type != DiffInsert {
			// Equality or deletion.
			chars1 += len(aDiff.Text)
		}
		if aDiff.Type != DiffDelete {
			// Equality or insertion.
			chars2 += len(aDiff.Text)
		}
		if chars1 > loc {
			// Overshot the location.
			lastDiff = aDiff
			break
		}
		last_chars1 = chars1
		last_chars2 = chars2
	}
	if lastDiff.Type == DiffDelete {
		// The location was deleted.
		return last_chars2
	}
	// Add the remaining character length.
	return last_chars2 + (loc - last_chars1)
}

// DiffPrettyHtml converts a []Diff into a pretty HTML report.
// It is intended as an example from which to write one's own
// display functions.
func (dmp *DiffMatchPatch) DiffPrettyHtml(diffs []Diff) string {
	var buff bytes.Buffer
	for _, diff := range diffs {
		text := strings.Replace(html.EscapeString(diff.Text), "\n", "&para;<br>", -1)
		switch diff.Type {
		case DiffInsert:
			buff.WriteString("<ins style=\"background:#e6ffe6;\">")
			buff.WriteString(text)
			buff.WriteString("</ins>")
		case DiffDelete:
			buff.WriteString("<del style=\"background:#ffe6e6;\">")
			buff.WriteString(text)
			buff.WriteString("</del>")
		case DiffEqual:
			buff.WriteString("<span>")
			buff.WriteString(text)
			buff.WriteString("</span>")
		}
	}
	return buff.String()
}

// Diff_text1 computes and returns the source text (all equalities and deletions).
func (dmp *DiffMatchPatch) DiffText1(diffs []Diff) string {
	//StringBuilder text = new StringBuilder()
	var text bytes.Buffer

	for _, aDiff := range diffs {
		if aDiff.Type != DiffInsert {
			text.WriteString(aDiff.Text)
		}
	}
	return text.String()
}

// Diff_text2 computes and returns the destination text (all equalities and insertions).
func (dmp *DiffMatchPatch) DiffText2(diffs []Diff) string {
	var text bytes.Buffer

	for _, aDiff := range diffs {
		if aDiff.Type != DiffDelete {
			text.WriteString(aDiff.Text)
		}
	}
	return text.String()
}

// Diff_levenshtein computes the Levenshtein distance; the number of inserted, deleted or
// substituted characters.
func (dmp *DiffMatchPatch) DiffLevenshtein(diffs []Diff) int {
	levenshtein := 0
	insertions := 0
	deletions := 0

	for _, aDiff := range diffs {
		switch aDiff.Type {
		case DiffInsert:
			insertions += len(aDiff.Text)
		case DiffDelete:
			deletions += len(aDiff.Text)
		case DiffEqual:
			// A deletion and an insertion is one substitution.
			levenshtein += max(insertions, deletions)
			insertions = 0
			deletions = 0
		}
	}

	levenshtein += max(insertions, deletions)
	return levenshtein
}

// Diff_toDelta crushes the diff into an encoded string which describes the operations
// required to transform text1 into text2.
// E.g. =3\t-2\t+ing  -> Keep 3 chars, delete 2 chars, insert 'ing'.
// Operations are tab-separated.  Inserted text is escaped using %xx
// notation.
func (dmp *DiffMatchPatch) DiffToDelta(diffs []Diff) string {
	var text bytes.Buffer
	for _, aDiff := range diffs {
		switch aDiff.Type {
		case DiffInsert:
			text.WriteString("+")
			text.WriteString(strings.Replace(url.QueryEscape(aDiff.Text), "+", " ", -1))
			text.WriteString("\t")
			break
		case DiffDelete:
			text.WriteString("-")
			text.WriteString(strconv.Itoa(utf8.RuneCountInString(aDiff.Text)))
			text.WriteString("\t")
			break
		case DiffEqual:
			text.WriteString("=")
			text.WriteString(strconv.Itoa(utf8.RuneCountInString(aDiff.Text)))
			text.WriteString("\t")
			break
		}
	}
	delta := text.String()
	if len(delta) != 0 {
		// Strip off trailing tab character.
		delta = delta[0 : utf8.RuneCountInString(delta)-1]
		delta = unescaper.Replace(delta)
	}
	return delta
}

// Diff_fromDelta. Given the original text1, and an encoded string which describes the
// operations required to transform text1 into text2, comAdde the full diff.
func (dmp *DiffMatchPatch) DiffFromDelta(text1, delta string) (diffs []Diff, err error) {
	diffs = []Diff{}

	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	pointer := 0 // Cursor in text1
	tokens := strings.Split(delta, "\t")

	for _, token := range tokens {
		if len(token) == 0 {
			// Blank tokens are ok (from a trailing \t).
			continue
		}

		// Each token begins with a one character parameter which specifies the
		// operation of this token (delete, insert, equality).
		param := token[1:]

		switch op := token[0]; op {
		case '+':
			// decode would Diff all "+" to " "
			param = strings.Replace(param, "+", "%2b", -1)
			param, err = url.QueryUnescape(param)
			if err != nil {
				return nil, err
			}
			if !utf8.ValidString(param) {
				return nil, fmt.Errorf("invalid UTF-8 token: %q", param)
			}
			diffs = append(diffs, Diff{DiffInsert, param})
		case '=', '-':
			n, err := strconv.ParseInt(param, 10, 0)
			if err != nil {
				return diffs, err
			} else if n < 0 {
				return diffs, errors.New("Negative number in DiffFromDelta: " + param)
			}

			// remember that string slicing is by byte - we want by rune here.
			text := string([]rune(text1)[pointer : pointer+int(n)])
			pointer += int(n)

			if op == '=' {
				diffs = append(diffs, Diff{DiffEqual, text})
			} else {
				diffs = append(diffs, Diff{DiffDelete, text})
			}
		default:
			// Anything else is an error.
			return diffs, errors.New("Invalid diff operation in DiffFromDelta: " + string(token[0]))
		}
	}

	if pointer != len([]rune(text1)) {
		return diffs, fmt.Errorf("Delta length (%v) smaller than source text length (%v)", pointer, len(text1))
	}
	return diffs, err
}

//  MATCH FUNCTIONS

// MatchMain locates the best instance of 'pattern' in 'text' near 'loc'.
// Returns -1 if no match found.
func (dmp *DiffMatchPatch) MatchMain(text, pattern string, loc int) int {
	// Check for null inputs not needed since null can't be passed in C#.

	loc = int(math.Max(0, math.Min(float64(loc), float64(len(text)))))
	if text == pattern {
		// Shortcut (potentially not guaranteed by the algorithm)
		return 0
	} else if len(text) == 0 {
		// Nothing to match.
		return -1
	} else if loc+len(pattern) <= len(text) && text[loc:loc+len(pattern)] == pattern {
		// Perfect match at the perfect spot!  (Includes case of null pattern)
		return loc
	}
	// Do a fuzzy compare.
	return dmp.MatchBitap(text, pattern, loc)
}

// MatchBitap locates the best instance of 'pattern' in 'text' near 'loc' using the
// Bitap algorithm.  Returns -1 if no match found.
func (dmp *DiffMatchPatch) MatchBitap(text, pattern string, loc int) int {
	// Initialise the alphabet.
	s := dmp.MatchAlphabet(pattern)

	// Highest score beyond which we give up.
	var score_threshold float64 = dmp.MatchThreshold
	// Is there a nearby exact match? (speedup)
	best_loc := indexOf(text, pattern, loc)
	if best_loc != -1 {
		score_threshold = math.Min(dmp.matchBitapScore(0, best_loc, loc,
			pattern), score_threshold)
		// What about in the other direction? (speedup)
		best_loc = lastIndexOf(text, pattern, loc+len(pattern))
		if best_loc != -1 {
			score_threshold = math.Min(dmp.matchBitapScore(0, best_loc, loc,
				pattern), score_threshold)
		}
	}

	// Initialise the bit arrays.
	matchmask := 1 << uint((len(pattern) - 1))
	best_loc = -1

	var bin_min, bin_mid int
	bin_max := len(pattern) + len(text)
	last_rd := []int{}
	for d := 0; d < len(pattern); d++ {
		// Scan for the best match; each iteration allows for one more error.
		// Run a binary search to determine how far from 'loc' we can stray at
		// this error level.
		bin_min = 0
		bin_mid = bin_max
		for bin_min < bin_mid {
			if dmp.matchBitapScore(d, loc+bin_mid, loc, pattern) <= score_threshold {
				bin_min = bin_mid
			} else {
				bin_max = bin_mid
			}
			bin_mid = (bin_max-bin_min)/2 + bin_min
		}
		// Use the result from this iteration as the maximum for the next.
		bin_max = bin_mid
		start := int(math.Max(1, float64(loc-bin_mid+1)))
		finish := int(math.Min(float64(loc+bin_mid), float64(len(text))) + float64(len(pattern)))

		rd := make([]int, finish+2)
		rd[finish+1] = (1 << uint(d)) - 1

		for j := finish; j >= start; j-- {
			var charMatch int
			if len(text) <= j-1 {
				// Out of range.
				charMatch = 0
			} else if _, ok := s[text[j-1]]; !ok {
				charMatch = 0
			} else {
				charMatch = s[text[j-1]]
			}

			if d == 0 {
				// First pass: exact match.
				rd[j] = ((rd[j+1] << 1) | 1) & charMatch
			} else {
				// Subsequent passes: fuzzy match.
				rd[j] = ((rd[j+1]<<1)|1)&charMatch | (((last_rd[j+1] | last_rd[j]) << 1) | 1) | last_rd[j+1]
			}
			if (rd[j] & matchmask) != 0 {
				score := dmp.matchBitapScore(d, j-1, loc, pattern)
				// This match will almost certainly be better than any existing
				// match.  But check anyway.
				if score <= score_threshold {
					// Told you so.
					score_threshold = score
					best_loc = j - 1
					if best_loc > loc {
						// When passing loc, don't exceed our current distance from loc.
						start = int(math.Max(1, float64(2*loc-best_loc)))
					} else {
						// Already passed loc, downhill from here on in.
						break
					}
				}
			}
		}
		if dmp.matchBitapScore(d+1, loc, loc, pattern) > score_threshold {
			// No hope for a (better) match at greater error levels.
			break
		}
		last_rd = rd
	}
	return best_loc
}

// matchBitapScore computes and returns the score for a match with e errors and x location.
func (dmp *DiffMatchPatch) matchBitapScore(e, x, loc int, pattern string) float64 {
	var accuracy float64 = float64(e) / float64(len(pattern))
	proximity := math.Abs(float64(loc - x))
	if dmp.MatchDistance == 0 {
		// Dodge divide by zero error.
		if proximity == 0 {
			return accuracy
		} else {
			return 1.0
		}
	}
	return accuracy + (proximity / float64(dmp.MatchDistance))
}

// MatchAlphabet initialises the alphabet for the Bitap algorithm.
func (dmp *DiffMatchPatch) MatchAlphabet(pattern string) map[byte]int {
	s := map[byte]int{}
	char_pattern := []byte(pattern)
	for _, c := range char_pattern {
		_, ok := s[c]
		if !ok {
			s[c] = 0
		}
	}
	i := 0

	for _, c := range char_pattern {
		value := s[c] | int(uint(1)<<uint((len(pattern)-i-1)))
		s[c] = value
		i++
	}
	return s
}

//  PATCH FUNCTIONS

// PatchAddContext increases the context until it is unique,
// but doesn't let the pattern expand beyond MatchMaxBits.
func (dmp *DiffMatchPatch) PatchAddContext(patch Patch, text string) Patch {
	if len(text) == 0 {
		return patch
	}

	pattern := text[patch.start2 : patch.start2+patch.length1]
	padding := 0

	// Look for the first and last matches of pattern in text.  If two
	// different matches are found, increase the pattern length.
	for strings.Index(text, pattern) != strings.LastIndex(text, pattern) &&
		len(pattern) < dmp.MatchMaxBits-2*dmp.PatchMargin {
		padding += dmp.PatchMargin
		maxStart := max(0, patch.start2-padding)
		minEnd := min(len(text), patch.start2+patch.length1+padding)
		pattern = text[maxStart:minEnd]
	}
	// Add one chunk for good luck.
	padding += dmp.PatchMargin

	// Add the prefix.
	prefix := text[max(0, patch.start2-padding):patch.start2]
	if len(prefix) != 0 {
		patch.diffs = append([]Diff{Diff{DiffEqual, prefix}}, patch.diffs...)
	}
	// Add the suffix.
	suffix := text[patch.start2+patch.length1 : min(len(text), patch.start2+patch.length1+padding)]
	if len(suffix) != 0 {
		patch.diffs = append(patch.diffs, Diff{DiffEqual, suffix})
	}

	// Roll back the start points.
	patch.start1 -= len(prefix)
	patch.start2 -= len(prefix)
	// Extend the lengths.
	patch.length1 += len(prefix) + len(suffix)
	patch.length2 += len(prefix) + len(suffix)

	return patch
}

func (dmp *DiffMatchPatch) PatchMake(opt ...interface{}) []Patch {
	if len(opt) == 1 {
		diffs, _ := opt[0].([]Diff)
		text1 := dmp.DiffText1(diffs)
		return dmp.PatchMake(text1, diffs)
	} else if len(opt) == 2 {
		text1 := opt[0].(string)
		switch t := opt[1].(type) {
		case string:
			diffs := dmp.DiffMain(text1, t, true)
			if len(diffs) > 2 {
				diffs = dmp.DiffCleanupSemantic(diffs)
				diffs = dmp.DiffCleanupEfficiency(diffs)
			}
			return dmp.PatchMake(text1, diffs)
		case []Diff:
			return dmp.patchMake2(text1, t)
		}
	} else if len(opt) == 3 {
		return dmp.PatchMake(opt[0], opt[2])
	}
	return []Patch{}
}

// Compute a list of patches to turn text1 into text2.
// text2 is not provided, diffs are the delta between text1 and text2.
func (dmp *DiffMatchPatch) patchMake2(text1 string, diffs []Diff) []Patch {
	// Check for null inputs not needed since null can't be passed in C#.
	patches := []Patch{}
	if len(diffs) == 0 {
		return patches // Get rid of the null case.
	}

	patch := Patch{}
	char_count1 := 0 // Number of characters into the text1 string.
	char_count2 := 0 // Number of characters into the text2 string.
	// Start with text1 (prepatch_text) and apply the diffs until we arrive at
	// text2 (postpatch_text). We recreate the patches one by one to determine
	// context info.
	prepatch_text := text1
	postpatch_text := text1

	for i, aDiff := range diffs {
		if len(patch.diffs) == 0 && aDiff.Type != DiffEqual {
			// A new patch starts here.
			patch.start1 = char_count1
			patch.start2 = char_count2
		}

		switch aDiff.Type {
		case DiffInsert:
			patch.diffs = append(patch.diffs, aDiff)
			patch.length2 += len(aDiff.Text)
			postpatch_text = postpatch_text[:char_count2] +
				aDiff.Text + postpatch_text[char_count2:]
		case DiffDelete:
			patch.length1 += len(aDiff.Text)
			patch.diffs = append(patch.diffs, aDiff)
			postpatch_text = postpatch_text[:char_count2] + postpatch_text[char_count2+len(aDiff.Text):]
		case DiffEqual:
			if len(aDiff.Text) <= 2*dmp.PatchMargin &&
				len(patch.diffs) != 0 && i != len(diffs)-1 {
				// Small equality inside a patch.
				patch.diffs = append(patch.diffs, aDiff)
				patch.length1 += len(aDiff.Text)
				patch.length2 += len(aDiff.Text)
			}
			if len(aDiff.Text) >= 2*dmp.PatchMargin {
				// Time for a new patch.
				if len(patch.diffs) != 0 {
					patch = dmp.PatchAddContext(patch, prepatch_text)
					patches = append(patches, patch)
					patch = Patch{}
					// Unlike Unidiff, our patch lists have a rolling context.
					// http://code.google.com/p/google-diff-match-patch/wiki/Unidiff
					// Update prepatch text & pos to reflect the application of the
					// just completed patch.
					prepatch_text = postpatch_text
					char_count1 = char_count2
				}
			}
		}

		// Update the current character count.
		if aDiff.Type != DiffInsert {
			char_count1 += len(aDiff.Text)
		}
		if aDiff.Type != DiffDelete {
			char_count2 += len(aDiff.Text)
		}
	}

	// Pick up the leftover patch if not empty.
	if len(patch.diffs) != 0 {
		patch = dmp.PatchAddContext(patch, prepatch_text)
		patches = append(patches, patch)
	}

	return patches
}

// PatchDeepCopy returns an array that is identical to a
// given an array of patches.
func (dmp *DiffMatchPatch) PatchDeepCopy(patches []Patch) []Patch {
	patchesCopy := []Patch{}
	for _, aPatch := range patches {
		patchCopy := Patch{}
		for _, aDiff := range aPatch.diffs {
			patchCopy.diffs = append(patchCopy.diffs, Diff{
				aDiff.Type,
				aDiff.Text,
			})
		}
		patchCopy.start1 = aPatch.start1
		patchCopy.start2 = aPatch.start2
		patchCopy.length1 = aPatch.length1
		patchCopy.length2 = aPatch.length2
		patchesCopy = append(patchesCopy, patchCopy)
	}
	return patchesCopy
}

// PatchApply merges a set of patches onto the text.  Returns a patched text, as well
// as an array of true/false values indicating which patches were applied.
func (dmp *DiffMatchPatch) PatchApply(patches []Patch, text string) (string, []bool) {
	if len(patches) == 0 {
		return text, []bool{}
	}

	// Deep copy the patches so that no changes are made to originals.
	patches = dmp.PatchDeepCopy(patches)

	nullPadding := dmp.PatchAddPadding(patches)
	text = nullPadding + text + nullPadding
	patches = dmp.PatchSplitMax(patches)

	x := 0
	// delta keeps track of the offset between the expected and actual
	// location of the previous patch.  If there are patches expected at
	// positions 10 and 20, but the first patch was found at 12, delta is 2
	// and the second patch has an effective expected position of 22.
	delta := 0
	results := make([]bool, len(patches))
	for _, aPatch := range patches {
		expected_loc := aPatch.start2 + delta
		text1 := dmp.DiffText1(aPatch.diffs)
		var start_loc int
		end_loc := -1
		if len(text1) > dmp.MatchMaxBits {
			// PatchSplitMax will only provide an oversized pattern
			// in the case of a monster delete.
			start_loc = dmp.MatchMain(text, text1[:dmp.MatchMaxBits], expected_loc)
			if start_loc != -1 {
				end_loc = dmp.MatchMain(text,
					text1[len(text1)-dmp.MatchMaxBits:], expected_loc+len(text1)-dmp.MatchMaxBits)
				if end_loc == -1 || start_loc >= end_loc {
					// Can't find valid trailing context.  Drop this patch.
					start_loc = -1
				}
			}
		} else {
			start_loc = dmp.MatchMain(text, text1, expected_loc)
		}
		if start_loc == -1 {
			// No match found.  :(
			results[x] = false
			// Subtract the delta for this failed patch from subsequent patches.
			delta -= aPatch.length2 - aPatch.length1
		} else {
			// Found a match.  :)
			results[x] = true
			delta = start_loc - expected_loc
			var text2 string
			if end_loc == -1 {
				text2 = text[start_loc:int(math.Min(float64(start_loc+len(text1)), float64(len(text))))]
			} else {
				text2 = text[start_loc:int(math.Min(float64(end_loc+dmp.MatchMaxBits), float64(len(text))))]
			}
			if text1 == text2 {
				// Perfect match, just shove the Replacement text in.
				text = text[:start_loc] + dmp.DiffText2(aPatch.diffs) + text[start_loc+len(text1):]
			} else {
				// Imperfect match.  Run a diff to get a framework of equivalent
				// indices.
				diffs := dmp.DiffMain(text1, text2, false)
				if len(text1) > dmp.MatchMaxBits && float64(dmp.DiffLevenshtein(diffs))/float64(len(text1)) > dmp.PatchDeleteThreshold {
					// The end points match, but the content is unacceptably bad.
					results[x] = false
				} else {
					diffs = dmp.DiffCleanupSemanticLossless(diffs)
					index1 := 0
					for _, aDiff := range aPatch.diffs {
						if aDiff.Type != DiffEqual {
							index2 := dmp.DiffXIndex(diffs, index1)
							if aDiff.Type == DiffInsert {
								// Insertion
								text = text[:start_loc+index2] + aDiff.Text + text[start_loc+index2:]
							} else if aDiff.Type == DiffDelete {
								// Deletion
								start_index := start_loc + index2
								text = text[:start_index] +
									text[start_index+dmp.DiffXIndex(diffs, index1+len(aDiff.Text))-index2:]
							}
						}
						if aDiff.Type != DiffDelete {
							index1 += len(aDiff.Text)
						}
					}
				}
			}
		}
		x++
	}
	// Strip the padding off.
	text = text[len(nullPadding) : len(nullPadding)+(len(text)-2*len(nullPadding))]
	return text, results
}

// PatchAddPadding adds some padding on text start and end so that edges can match something.
// Intended to be called only from within patch_apply.
func (dmp *DiffMatchPatch) PatchAddPadding(patches []Patch) string {
	paddingLength := dmp.PatchMargin
	nullPadding := ""
	for x := 1; x <= paddingLength; x++ {
		nullPadding += string(x)
	}

	// Bump all the patches forward.
	for i, _ := range patches {
		patches[i].start1 += paddingLength
		patches[i].start2 += paddingLength
	}

	// Add some padding on start of first diff.
	if len(patches[0].diffs) == 0 || patches[0].diffs[0].Type != DiffEqual {
		// Add nullPadding equality.
		patches[0].diffs = append([]Diff{Diff{DiffEqual, nullPadding}}, patches[0].diffs...)
		patches[0].start1 -= paddingLength // Should be 0.
		patches[0].start2 -= paddingLength // Should be 0.
		patches[0].length1 += paddingLength
		patches[0].length2 += paddingLength
	} else if paddingLength > len(patches[0].diffs[0].Text) {
		// Grow first equality.
		extraLength := paddingLength - len(patches[0].diffs[0].Text)
		patches[0].diffs[0].Text = nullPadding[len(patches[0].diffs[0].Text):] + patches[0].diffs[0].Text
		patches[0].start1 -= extraLength
		patches[0].start2 -= extraLength
		patches[0].length1 += extraLength
		patches[0].length2 += extraLength
	}

	// Add some padding on end of last diff.
	last := len(patches) - 1
	if len(patches[last].diffs) == 0 || patches[last].diffs[len(patches[last].diffs)-1].Type != DiffEqual {
		// Add nullPadding equality.
		patches[last].diffs = append(patches[last].diffs, Diff{DiffEqual, nullPadding})
		patches[last].length1 += paddingLength
		patches[last].length2 += paddingLength
	} else if paddingLength > len(patches[last].diffs[len(patches[last].diffs)-1].Text) {
		// Grow last equality.
		lastDiff := patches[last].diffs[len(patches[last].diffs)-1]
		extraLength := paddingLength - len(lastDiff.Text)
		patches[last].diffs[len(patches[last].diffs)-1].Text += nullPadding[:extraLength]
		patches[last].length1 += extraLength
		patches[last].length2 += extraLength
	}

	return nullPadding
}

// PatchSplitMax looks through the patches and breaks up any which are longer than the
// maximum limit of the match algorithm.
// Intended to be called only from within patch_apply.
func (dmp *DiffMatchPatch) PatchSplitMax(patches []Patch) []Patch {
	patch_size := dmp.MatchMaxBits
	for x := 0; x < len(patches); x++ {
		if patches[x].length1 <= patch_size {
			continue
		}
		bigpatch := patches[x]
		// Remove the big old patch.
		patches = append(patches[:x], patches[x+1:]...)
		x -= 1

		start1 := bigpatch.start1
		start2 := bigpatch.start2
		precontext := ""
		for len(bigpatch.diffs) != 0 {
			// Create one of several smaller patches.
			patch := Patch{}
			empty := true
			patch.start1 = start1 - len(precontext)
			patch.start2 = start2 - len(precontext)
			if len(precontext) != 0 {
				patch.length1 = len(precontext)
				patch.length2 = len(precontext)
				patch.diffs = append(patch.diffs, Diff{DiffEqual, precontext})
			}
			for len(bigpatch.diffs) != 0 && patch.length1 < patch_size-dmp.PatchMargin {
				diff_type := bigpatch.diffs[0].Type
				diff_text := bigpatch.diffs[0].Text
				if diff_type == DiffInsert {
					// Insertions are harmless.
					patch.length2 += len(diff_text)
					start2 += len(diff_text)
					patch.diffs = append(patch.diffs, bigpatch.diffs[0])
					bigpatch.diffs = bigpatch.diffs[1:]
					empty = false
				} else if diff_type == DiffDelete && len(patch.diffs) == 1 && patch.diffs[0].Type == DiffEqual && len(diff_text) > 2*patch_size {
					// This is a large deletion.  Let it pass in one chunk.
					patch.length1 += len(diff_text)
					start1 += len(diff_text)
					empty = false
					patch.diffs = append(patch.diffs, Diff{diff_type, diff_text})
					bigpatch.diffs = bigpatch.diffs[1:]
				} else {
					// Deletion or equality.  Only take as much as we can stomach.
					diff_text = diff_text[:min(len(diff_text), patch_size-patch.length1-dmp.PatchMargin)]

					patch.length1 += len(diff_text)
					start1 += len(diff_text)
					if diff_type == DiffEqual {
						patch.length2 += len(diff_text)
						start2 += len(diff_text)
					} else {
						empty = false
					}
					patch.diffs = append(patch.diffs, Diff{diff_type, diff_text})
					if diff_text == bigpatch.diffs[0].Text {
						bigpatch.diffs = bigpatch.diffs[1:]
					} else {
						bigpatch.diffs[0].Text =
							bigpatch.diffs[0].Text[len(diff_text):]
					}
				}
			}
			// Compute the head context for the next patch.
			precontext = dmp.DiffText2(patch.diffs)
			precontext = precontext[max(0, len(precontext)-dmp.PatchMargin):]

			postcontext := ""
			// Append the end context for this patch.
			if len(dmp.DiffText1(bigpatch.diffs)) > dmp.PatchMargin {
				postcontext = dmp.DiffText1(bigpatch.diffs)[:dmp.PatchMargin]
			} else {
				postcontext = dmp.DiffText1(bigpatch.diffs)
			}

			if len(postcontext) != 0 {
				patch.length1 += len(postcontext)
				patch.length2 += len(postcontext)
				if len(patch.diffs) != 0 && patch.diffs[len(patch.diffs)-1].Type == DiffEqual {
					patch.diffs[len(patch.diffs)-1].Text += postcontext
				} else {
					patch.diffs = append(patch.diffs, Diff{DiffEqual, postcontext})
				}
			}
			if !empty {
				x += 1
				patches = append(patches[:x], append([]Patch{patch}, patches[x:]...)...)
			}
		}
	}
	return patches
}

// PatchToText takes a list of patches and returns a textual representation.
func (dmp *DiffMatchPatch) PatchToText(patches []Patch) string {
	var text bytes.Buffer
	for _, aPatch := range patches {
		text.WriteString(aPatch.String())
	}
	return text.String()
}

// PatchFromText parses a textual representation of patches and returns a List of Patch
// objects.
func (dmp *DiffMatchPatch) PatchFromText(textline string) ([]Patch, error) {
	patches := []Patch{}
	if len(textline) == 0 {
		return patches, nil
	}
	text := strings.Split(textline, "\n")
	textPointer := 0
	patchHeader := regexp.MustCompile("^@@ -(\\d+),?(\\d*) \\+(\\d+),?(\\d*) @@$")

	var patch Patch
	var sign uint8
	var line string
	for textPointer < len(text) {

		if !patchHeader.MatchString(text[textPointer]) {
			return patches, errors.New("Invalid patch string: " + text[textPointer])
		}

		patch = Patch{}
		m := patchHeader.FindStringSubmatch(text[textPointer])

		patch.start1, _ = strconv.Atoi(m[1])
		if len(m[2]) == 0 {
			patch.start1--
			patch.length1 = 1
		} else if m[2] == "0" {
			patch.length1 = 0
		} else {
			patch.start1--
			patch.length1, _ = strconv.Atoi(m[2])
		}

		patch.start2, _ = strconv.Atoi(m[3])

		if len(m[4]) == 0 {
			patch.start2--
			patch.length2 = 1
		} else if m[4] == "0" {
			patch.length2 = 0
		} else {
			patch.start2--
			patch.length2, _ = strconv.Atoi(m[4])
		}
		textPointer++

		for textPointer < len(text) {
			if len(text[textPointer]) > 0 {
				sign = text[textPointer][0]
			} else {
				textPointer++
				continue
			}

			line = text[textPointer][1:]
			line = strings.Replace(line, "+", "%2b", -1)
			line, _ = url.QueryUnescape(line)
			if sign == '-' {
				// Deletion.
				patch.diffs = append(patch.diffs, Diff{DiffDelete, line})
			} else if sign == '+' {
				// Insertion.
				patch.diffs = append(patch.diffs, Diff{DiffInsert, line})
			} else if sign == ' ' {
				// Minor equality.
				patch.diffs = append(patch.diffs, Diff{DiffEqual, line})
			} else if sign == '@' {
				// Start of next patch.
				break
			} else {
				// WTF?
				return patches, errors.New("Invalid patch mode '" + string(sign) + "' in: " + string(line))
			}
			textPointer++
		}

		patches = append(patches, patch)
	}
	return patches, nil
}
