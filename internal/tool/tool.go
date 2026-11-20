package tool

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gogs/chardet"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

// ShortSHA1 truncates the commit ID to at most 10 characters.
func ShortSHA1(sha1 string) string {
	if len(sha1) > 10 {
		return sha1[:10]
	}
	return sha1
}

// DetectEncoding returns best guess of encoding of given content.
func DetectEncoding(logger *logx.Logger, content []byte) (string, error) {
	if utf8.Valid(content) {
		logger.Debug("Detected encoding via fast path", "charset", "UTF-8")
		return "UTF-8", nil
	}

	result, err := chardet.NewTextDetector().DetectBest(content)
	if result.Charset != "UTF-8" && len(conf.Repository.ANSICharset) > 0 {
		logger.Debug("Using default ANSI charset", "charset", conf.Repository.ANSICharset)
		return conf.Repository.ANSICharset, err
	}

	logger.Debug("Detected encoding", "charset", result.Charset)
	return result.Charset, err
}

// BasicAuthDecode decodes username and password portions of HTTP Basic Authentication
// from encoded content.
func BasicAuthDecode(encoded string) (string, string, error) {
	s, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}

	auth := strings.SplitN(string(s), ":", 2)
	if len(auth) != 2 {
		return auth[0], "", nil
	}
	return auth[0], auth[1], nil
}

// VerifyTimeLimitCode returns true if the code is a valid time limit code for
// the given data that has not expired.
func VerifyTimeLimitCode(data string, minutes int, code string) bool {
	if len(code) <= 18 {
		return false
	}

	start := code[:12]
	lives, err := strconv.Atoi(code[12:18])
	if err != nil || lives <= 0 {
		return false
	}

	retCode := CreateTimeLimitCode(data, lives, start)
	if !hmac.Equal([]byte(retCode), []byte(code)) {
		return false
	}

	// The code expires by the lifetime it was issued with or the currently
	// configured one, whichever is shorter, so that lowering the configured
	// lifetime also applies to codes already sent.
	before, _ := time.ParseInLocation("200601021504", start, time.Local)
	return before.Add(time.Minute*time.Duration(min(lives, minutes))).Unix() > time.Now().Unix()
}

const TimeLimitCodeLength = 12 + 6 + 64

// CreateTimeLimitCode generates a time limit code based on given input data.
// Format: 12-character date time string + 6-digit minutes string + 64-character
// hex-encoded HMAC-SHA256 string. A nil startInf starts the code now, otherwise
// it is the date time string to start the code from.
func CreateTimeLimitCode(data string, minutes int, startInf any) string {
	format := "200601021504"

	var start, end time.Time
	var startStr, endStr string

	if startInf == nil {
		start = time.Now()
		startStr = start.Format(format)
	} else {
		startStr = startInf.(string)
		start, _ = time.ParseInLocation(format, startStr, time.Local)
		startStr = start.Format(format)
	}

	end = start.Add(time.Minute * time.Duration(minutes))
	endStr = end.Format(format)

	mac := hmac.New(sha256.New, []byte(conf.Security.SecretKey))
	_, _ = mac.Write([]byte(data + startStr + endStr + strconv.Itoa(minutes)))
	encoded := hex.EncodeToString(mac.Sum(nil))

	code := fmt.Sprintf("%s%06d%s", startStr, minutes, encoded)
	return code
}

// Seconds-based time units
const (
	Minute = 60
	Hour   = 60 * Minute
	Day    = 24 * Hour
	Week   = 7 * Day
	Month  = 30 * Day
	Year   = 12 * Month
)

func computeTimeDiff(diff int64) (int64, string) {
	diffStr := ""
	switch {
	case diff <= 0:
		diff = 0
		diffStr = "now"
	case diff < 2:
		diff = 0
		diffStr = "1 second"
	case diff < 1*Minute:
		diffStr = fmt.Sprintf("%d seconds", diff)
		diff = 0

	case diff < 2*Minute:
		diff -= 1 * Minute
		diffStr = "1 minute"
	case diff < 1*Hour:
		diffStr = fmt.Sprintf("%d minutes", diff/Minute)
		diff -= diff / Minute * Minute

	case diff < 2*Hour:
		diff -= 1 * Hour
		diffStr = "1 hour"
	case diff < 1*Day:
		diffStr = fmt.Sprintf("%d hours", diff/Hour)
		diff -= diff / Hour * Hour

	case diff < 2*Day:
		diff -= 1 * Day
		diffStr = "1 day"
	case diff < 1*Week:
		diffStr = fmt.Sprintf("%d days", diff/Day)
		diff -= diff / Day * Day

	case diff < 2*Week:
		diff -= 1 * Week
		diffStr = "1 week"
	case diff < 1*Month:
		diffStr = fmt.Sprintf("%d weeks", diff/Week)
		diff -= diff / Week * Week

	case diff < 2*Month:
		diff -= 1 * Month
		diffStr = "1 month"
	case diff < 1*Year:
		diffStr = fmt.Sprintf("%d months", diff/Month)
		diff -= diff / Month * Month

	case diff < 2*Year:
		diff -= 1 * Year
		diffStr = "1 year"
	default:
		diffStr = fmt.Sprintf("%d years", diff/Year)
		diff = 0
	}
	return diff, diffStr
}

// TimeSincePro calculates the time interval and generates a full user-friendly string.
func TimeSincePro(then time.Time) string {
	now := time.Now()
	diff := now.Unix() - then.Unix()

	if then.After(now) {
		return "future"
	}

	var timeStr, diffStr string
	for diff != 0 {
		diff, diffStr = computeTimeDiff(diff)
		timeStr += ", " + diffStr
	}
	return strings.TrimPrefix(timeStr, ", ")
}

// StringsToInt64s converts a slice of string to a slice of int64.
// Invalid strings are converted to 0 (parse errors are silently ignored).
func StringsToInt64s(strs []string) []int64 {
	ints := make([]int64, len(strs))
	for i := range strs {
		ints[i], _ = strconv.ParseInt(strs[i], 10, 64)
	}
	return ints
}

// Int64sToStrings converts a slice of int64 to a slice of string.
func Int64sToStrings(ints []int64) []string {
	strs := make([]string, len(ints))
	for i := range ints {
		strs[i] = strconv.FormatInt(ints[i], 10)
	}
	return strs
}

// IsLetter reports whether the rune is a letter (category L).
// https://github.com/golang/go/blob/master/src/go/scanner/scanner.go#L257
func IsLetter(ch rune) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_' || ch >= 0x80 && unicode.IsLetter(ch)
}
