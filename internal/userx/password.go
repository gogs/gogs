package userx

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

var (
	// ErrPasswordTooShort is returned by CheckPasswordPolicy for a password
	// shorter than [auth] MIN_PASSWORD_LENGTH.
	ErrPasswordTooShort = errors.New("password is too short")
	// ErrPasswordLeaked is returned by CheckPasswordPolicy for a password that
	// appeared in a data breach.
	ErrPasswordLeaked = errors.New("password appeared in a data breach")
)

// pwnedPasswordsURL is the range API of Have I Been Pwned's Pwned Passwords.
var pwnedPasswordsURL = "https://api.pwnedpasswords.com/range/"

// CheckPasswordPolicy checks a new password against the password policy of the
// site: it must be at least [auth] MIN_PASSWORD_LENGTH characters, and with
// [auth] CHECK_LEAKED_PASSWORDS it must not have appeared in a data breach.
// Existing passwords are never checked, so the policy only applies when a
// password is set.
func CheckPasswordPolicy(ctx context.Context, password string) error {
	if utf8.RuneCountInString(password) < conf.Auth.MinPasswordLength {
		return ErrPasswordTooShort
	}
	if !conf.Auth.CheckLeakedPasswords || conf.Server.OfflineMode {
		return nil
	}
	leaked, err := isLeakedPassword(ctx, password)
	if err != nil {
		// The breach list is a second line of defense. Being unable to reach it
		// should not stop anyone from setting a password.
		log.Warn("Failed to check password against the breach list: %v", err)
		return nil
	}
	if leaked {
		return ErrPasswordLeaked
	}
	return nil
}

// isLeakedPassword asks Pwned Passwords whether the password appeared in a
// data breach. Only the first 5 characters of its SHA-1 hash leave the server
// (k-anonymity), and the response is padded so its size does not tell either.
func isLeakedPassword(ctx context.Context, password string) (bool, error) {
	sum := sha1.Sum([]byte(password))
	hash := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := hash[:5], hash[5:]

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pwnedPasswordsURL+prefix, nil)
	if err != nil {
		return false, errors.Wrap(err, "new request")
	}
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "Gogs")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "request")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, errors.Newf("unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		// Each line is the rest of a hash and how often it was seen, e.g.,
		// "0018A45C4D1DEF81644B54AB7F969B88D65:10". Padding lines are seen 0 times.
		gotSuffix, count, ok := strings.Cut(strings.TrimSpace(scanner.Text()), ":")
		if ok && gotSuffix == suffix {
			return count != "0", nil
		}
	}
	return false, errors.Wrap(scanner.Err(), "read response")
}
