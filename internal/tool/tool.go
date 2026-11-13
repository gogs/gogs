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

	log "unknwon.dev/clog/v2"

	"github.com/gogs/chardet"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
)

// ShortSHA1 truncates the commit ID to at most 10 characters.
func ShortSHA1(sha1 string) string {
	if len(sha1) > 10 {
		return sha1[:10]
	}
	return sha1
}

// DetectEncoding returns best guess of encoding of given content.
func DetectEncoding(content []byte) (string, error) {
	if utf8.Valid(content) {
		log.Trace("Detected encoding: UTF-8 (fast)")
		return "UTF-8", nil
	}

	result, err := chardet.NewTextDetector().DetectBest(content)
	if result.Charset != "UTF-8" && len(conf.Repository.ANSICharset) > 0 {
		log.Trace("Using default ANSICharset: %s", conf.Repository.ANSICharset)
		return conf.Repository.ANSICharset, err
	}

	log.Trace("Detected encoding: %s", result.Charset)
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

// verify time limit code
func VerifyTimeLimitCode(data string, minutes int, code string) bool {
	if len(code) <= 18 {
		return false
	}

	// split code
	start := code[:12]
	lives, err := strconv.Atoi(code[12:18])
	if err != nil || lives <= 0 {
		return false
	}

	// right active code
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
// Format: 12 length date time string + 6 minutes string + 64 HMAC-SHA256 encoded
// string.
func CreateTimeLimitCode(data string, minutes int, startInf any) string {
	format := "200601021504"

	var start, end time.Time
	var startStr, endStr string

	if startInf == nil {
		// Use now time create code
		start = time.Now()
		startStr = start.Format(format)
	} else {
		// use start string create code
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

// HashEmail hashes email address to MD5 string.
// https://en.gravatar.com/site/implement/hash/
func HashEmail(email string) string {
	return cryptox.MD5(strings.ToLower(strings.TrimSpace(email)))
}

// AvatarLink returns relative avatar link to the site domain by given email,
// which includes app sub-url as prefix. However, it is possible
// to return full URL if user enables Gravatar-like service.
func AvatarLink(email string) (url string) {
	if conf.Picture.EnableFederatedAvatar && conf.Picture.LibravatarService != nil &&
		strings.Contains(email, "@") {
		var err error
		url, err = conf.Picture.LibravatarService.FromEmail(email)
		if err != nil {
			log.Warn("AvatarLink.LibravatarService.FromEmail [%s]: %v", email, err)
		}
	}
	if url == "" && !conf.Picture.DisableGravatar {
		url = conf.Picture.GravatarSource + HashEmail(email) + "?d=identicon"
	}
	if url == "" {
		url = conf.Server.Subpath + "/img/avatar_default.png"
	}
	return url
}

// AppendAvatarSize appends avatar size query parameter to the URL in the correct format.
func AppendAvatarSize(url string, size int) string {
	if strings.Contains(url, "?") {
		return url + "&s=" + strconv.Itoa(size)
	}
	return url + "?s=" + strconv.Itoa(size)
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

// TimeSincePro calculates the time interval and generate full user-friendly string.
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

// Int64sToMap converts a slice of int64 to a int64 map.
func Int64sToMap(ints []int64) map[int64]bool {
	m := make(map[int64]bool)
	for _, i := range ints {
		m[i] = true
	}
	return m
}

// IsLetter reports whether the rune is a letter (category L).
// https://github.com/golang/go/blob/master/src/go/scanner/scanner.go#L257
func IsLetter(ch rune) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_' || ch >= 0x80 && unicode.IsLetter(ch)
}
