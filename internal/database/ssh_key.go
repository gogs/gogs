package database

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
)

const (
	tplPublicKey = `command="%s serv key-%d --config='%s'",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty %s` + "\n"
)

var sshOpLocker sync.Mutex

type KeyType int

const (
	KeyTypeUser = iota + 1
	KeyTypeDeploy
)

// PublicKey represents a user or deploy SSH public key.
type PublicKey struct {
	ID          int64      `gorm:"primaryKey"`
	OwnerID     int64      `gorm:"not null;index:IDX_public_key_owner_id"`
	Name        string     `gorm:"size:255;not null"`
	Fingerprint string     `gorm:"size:255;not null"`
	Content     string     `gorm:"type:TEXT;not null"`
	Mode        AccessMode `gorm:"size:32;not null;default:2"`
	Type        KeyType    `gorm:"size:32;not null;default:1"`

	Created           time.Time `json:"-" gorm:"-"`
	CreatedUnix       int64
	Updated           time.Time `json:"-" gorm:"-"`
	UpdatedUnix       int64
	HasRecentActivity bool `json:"-" gorm:"-"`
	HasUsed           bool `json:"-" gorm:"-"`
}

// BeforeCreate implements the GORM create hook.
func (k *PublicKey) BeforeCreate(tx *gorm.DB) error {
	if k.CreatedUnix == 0 {
		k.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (k *PublicKey) BeforeUpdate(tx *gorm.DB) error {
	k.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (k *PublicKey) AfterFind(_ *gorm.DB) error {
	k.Created = time.Unix(k.CreatedUnix, 0).Local()
	k.Updated = time.Unix(k.UpdatedUnix, 0).Local()
	k.HasUsed = k.Updated.After(k.Created)
	k.HasRecentActivity = k.Updated.Add(7 * 24 * time.Hour).After(time.Now())
	return nil
}

// OmitEmail returns content of public key without email address.
func (k *PublicKey) OmitEmail() string {
	return strings.Join(strings.Split(k.Content, " ")[:2], " ")
}

// AuthorizedString returns formatted public key string for authorized_keys file.
func (k *PublicKey) AuthorizedString() string {
	return fmt.Sprintf(tplPublicKey, conf.AppPath(), k.ID, conf.CustomConf, k.Content)
}

// IsDeployKey returns true if the public key is used as deploy key.
func (k *PublicKey) IsDeployKey() bool {
	return k.Type == KeyTypeDeploy
}

func extractTypeFromBase64Key(key string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(b) < 4 {
		return "", errors.Newf("invalid key format: %v", err)
	}

	keyLength := int(binary.BigEndian.Uint32(b))
	if len(b) < 4+keyLength {
		return "", errors.Newf("invalid key format: not enough length %d", keyLength)
	}

	return string(b[4 : 4+keyLength]), nil
}

// parseKeyString parses any key string in OpenSSH or SSH2 format to clean OpenSSH string (RFC4253).
func parseKeyString(content string) (string, error) {
	// Transform all legal line endings to a single "\n".
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// Remove ending new lines as they may cause unwanted behaviour (an extra
	// line means it is not a single-line OpenSSH key).
	content = strings.TrimRight(content, "\n")

	lines := strings.Split(content, "\n")

	var keyType, keyContent, keyComment string

	if len(lines) == 1 {
		// Parse OpenSSH format.
		parts := strings.SplitN(lines[0], " ", 3)
		switch len(parts) {
		case 0:
			return "", errors.New("empty key")
		case 1:
			keyContent = parts[0]
		case 2:
			keyType = parts[0]
			keyContent = parts[1]
		default:
			keyType = parts[0]
			keyContent = parts[1]
			keyComment = parts[2]
		}

		// If keyType is not given, extract it from content. If given, validate it.
		t, err := extractTypeFromBase64Key(keyContent)
		if err != nil {
			return "", errors.Newf("extractTypeFromBase64Key: %v", err)
		}
		if keyType == "" {
			keyType = t
		} else if keyType != t {
			return "", errors.Newf("key type and content does not match: %s - %s", keyType, t)
		}
	} else {
		// Parse SSH2 file format.
		continuationLine := false

		for _, line := range lines {
			// Skip lines that:
			// 1) are a continuation of the previous line,
			// 2) contain ":" as those are comment lines, or
			// 3) contain "-" as those are begin and end tags.
			if continuationLine || strings.ContainsAny(line, ":-") {
				continuationLine = strings.HasSuffix(line, "\\")
			} else {
				keyContent = keyContent + line
			}
		}

		t, err := extractTypeFromBase64Key(keyContent)
		if err != nil {
			return "", errors.Newf("extractTypeFromBase64Key: %v", err)
		}
		keyType = t
	}
	return keyType + " " + keyContent + " " + keyComment, nil
}

// writeTmpKeyFile writes key content to a temporary file
// and returns the name of that file, along with any possible errors.
func writeTmpKeyFile(content, keyTestPath string) (string, error) {
	tmpFile, err := os.CreateTemp(keyTestPath, "gogs_keytest")
	if err != nil {
		return "", errors.Newf("TempFile: %v", err)
	}
	defer tmpFile.Close()

	if _, err = tmpFile.WriteString(content); err != nil {
		return "", errors.Newf("WriteString: %v", err)
	}
	return tmpFile.Name(), nil
}

// sshKeygenFingerprint runs "ssh-keygen -lf" on the public key file at the path,
// and stops it after a minute.
func sshKeygenFingerprint(keygenPath, path string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var outBuf, errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, keygenPath, "-lf", path)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// SSHKeygenParsePublicKey extracts key type and length using ssh-keygen.
func SSHKeygenParsePublicKey(key, keyTestPath, keygenPath string) (string, int, error) {
	tmpName, err := writeTmpKeyFile(key, keyTestPath)
	if err != nil {
		return "", 0, errors.Newf("writeTmpKeyFile: %v", err)
	}
	defer os.Remove(tmpName)

	stdout, stderr, err := sshKeygenFingerprint(keygenPath, tmpName)
	if err != nil {
		return "", 0, errors.Newf("fail to parse public key: %s - %s", err, stderr)
	}
	if strings.Contains(stdout, "is not a public key file") {
		return "", 0, ErrKeyUnableVerify{stdout}
	}

	fields := strings.Split(stdout, " ")
	if len(fields) < 4 {
		return "", 0, errors.Newf("invalid public key line: %s", stdout)
	}

	keyType := strings.Trim(fields[len(fields)-1], "()\r\n")
	length, _ := strconv.Atoi(fields[0])
	return strings.ToLower(keyType), length, nil
}

// SSHNativeParsePublicKey extracts the key type and length using the Go SSH library.
func SSHNativeParsePublicKey(keyLine string) (string, int, error) {
	fields := strings.Fields(keyLine)
	if len(fields) < 2 {
		return "", 0, errors.Newf("not enough fields in public key line: %s", keyLine)
	}

	raw, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "", 0, err
	}

	pkey, err := ssh.ParsePublicKey(raw)
	if err != nil {
		if strings.Contains(err.Error(), "ssh: unknown key algorithm") {
			return "", 0, ErrKeyUnableVerify{err.Error()}
		}
		return "", 0, errors.Newf("ParsePublicKey: %v", err)
	}

	// The ssh library can parse the key, so next we find out what key exactly we have.
	switch pkey.Type() {
	case ssh.KeyAlgoRSA:
		rawPub := struct {
			Name string
			E    *big.Int
			N    *big.Int
		}{}
		if err := ssh.Unmarshal(pkey.Marshal(), &rawPub); err != nil {
			return "", 0, err
		}
		return "rsa", rawPub.N.BitLen(), nil // Use N as per crypto/rsa/rsa.go (is bits).
	case ssh.KeyAlgoECDSA256:
		return "ecdsa", 256, nil
	case ssh.KeyAlgoECDSA384:
		return "ecdsa", 384, nil
	case ssh.KeyAlgoECDSA521:
		return "ecdsa", 521, nil
	case ssh.KeyAlgoED25519:
		return "ed25519", 256, nil
	}
	return "", 0, errors.Newf("unsupported key length detection for type: %s", pkey.Type())
}

// CheckPublicKeyString checks if the given public key string is recognized by SSH.
// It returns the actual public key line on success.
func CheckPublicKeyString(logger *logx.Logger, content string) (_ string, err error) {
	if conf.SSH.Disabled {
		return "", errors.New("SSH is disabled")
	}

	content, err = parseKeyString(content)
	if err != nil {
		return "", err
	}

	content = strings.TrimRight(content, "\n\r")
	if strings.ContainsAny(content, "\n\r") {
		return "", errors.New("only a single line with a single key please")
	}

	// Remove any unnecessary whitespace.
	content = strings.TrimSpace(content)

	if !conf.SSH.MinimumKeySizeCheck {
		return content, nil
	}

	var (
		fnName  string
		keyType string
		length  int
	)
	if conf.SSH.StartBuiltinServer {
		fnName = "SSHNativeParsePublicKey"
		keyType, length, err = SSHNativeParsePublicKey(content)
	} else {
		fnName = "SSHKeygenParsePublicKey"
		keyType, length, err = SSHKeygenParsePublicKey(content, conf.SSH.KeyTestPath, conf.SSH.KeygenPath)
	}
	if err != nil {
		return "", errors.Newf("%s: %v", fnName, err)
	}
	logger.Debug("Parsed public key", "builtinServer", conf.SSH.StartBuiltinServer, "keyType", keyType, "length", length)

	if minLen, found := conf.SSH.MinimumKeySizes[keyType]; found && length >= minLen {
		return content, nil
	} else if found && length < minLen {
		return "", errors.Newf("key length is not enough: got %d, needs %d", length, minLen)
	}
	return "", errors.Newf("key type is not allowed: %s", keyType)
}

// appendAuthorizedKeysToFile appends new SSH keys' content to authorized_keys file.
func appendAuthorizedKeysToFile(logger *logx.Logger, keys ...*PublicKey) error {
	// The builtin SSH server does not read the file.
	if conf.SSH.StartBuiltinServer {
		return nil
	}

	sshOpLocker.Lock()
	defer sshOpLocker.Unlock()

	fpath := filepath.Join(conf.SSH.RootPath, "authorized_keys")
	f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	// Note: Windows does not support chmod.
	if !conf.IsWindowsRuntime() {
		fi, err := f.Stat()
		if err != nil {
			return err
		}

		// .ssh directory should have mode 700, and authorized_keys file should have mode 600.
		if fi.Mode().Perm() > 0o600 {
			logger.Error("Resetting unusual permission flags of authorized_keys file", "mode", fi.Mode().Perm().String(), "newMode", "-rw-------")
			if err = f.Chmod(0o600); err != nil {
				return err
			}
		}
	}

	for _, key := range keys {
		if _, err = f.WriteString(key.AuthorizedString()); err != nil {
			return err
		}
	}
	return nil
}

// checkKeyContent only checks if key content has been used as a user public
// key. It is OK to use same key as deploy key for multiple repositories.
func checkKeyContent(tx *gorm.DB, content string) error {
	var count int64
	err := tx.Model(new(PublicKey)).Where("content = ? AND type = ?", content, KeyTypeUser).Count(&count).Error
	if err != nil {
		return err
	} else if count > 0 {
		return ErrKeyAlreadyExist{0, content}
	}
	return nil
}

// addKey adds the key to the database. Callers add it to the authorized_keys
// file with appendAuthorizedKeysToFile once the transaction commits, so that a
// key rolled back leaves no line whose key ID a later key may get.
func addKey(tx *gorm.DB, key *PublicKey) (err error) {
	// Calculate fingerprint.
	tmpPath := strings.ReplaceAll(path.Join(os.TempDir(), fmt.Sprintf("%d", time.Now().Nanosecond()), "id_rsa.pub"), "\\", "/")
	_ = os.MkdirAll(path.Dir(tmpPath), os.ModePerm)
	if err = os.WriteFile(tmpPath, []byte(key.Content), 0o644); err != nil {
		return err
	}

	stdout, stderr, err := sshKeygenFingerprint(conf.SSH.KeygenPath, tmpPath)
	if err != nil {
		return errors.Newf("fail to parse public key: %s - %s", err, stderr)
	} else if len(stdout) < 2 {
		return errors.New("not enough output for calculating fingerprint: " + stdout)
	}
	key.Fingerprint = strings.Split(stdout, " ")[1]

	return tx.Create(key).Error
}

// deletePublicKeys does the actual key deletion but does not update authorized_keys file.
func deletePublicKeys(tx *gorm.DB, keyIDs ...int64) error {
	if len(keyIDs) == 0 {
		return nil
	}

	return tx.Where("id IN ?", keyIDs).Delete(new(PublicKey)).Error
}

// DeployKey represents deploy key information and its relation with repository.
type DeployKey struct {
	ID          int64
	KeyID       int64  `gorm:"uniqueIndex:UQE_deploy_key_s;index:IDX_deploy_key_key_id"`
	RepoID      int64  `gorm:"uniqueIndex:UQE_deploy_key_s;index:IDX_deploy_key_repo_id"`
	Name        string `gorm:"size:255"`
	Fingerprint string `gorm:"size:255"`
	Content     string `json:"-" gorm:"-"`

	Created           time.Time `json:"-" gorm:"-"`
	CreatedUnix       int64
	Updated           time.Time `json:"-" gorm:"-"`
	UpdatedUnix       int64
	HasRecentActivity bool `json:"-" gorm:"-"`
	HasUsed           bool `json:"-" gorm:"-"`
}

// BeforeCreate implements the GORM create hook.
func (k *DeployKey) BeforeCreate(tx *gorm.DB) error {
	if k.CreatedUnix == 0 {
		k.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (k *DeployKey) BeforeUpdate(tx *gorm.DB) error {
	k.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (k *DeployKey) AfterFind(_ *gorm.DB) error {
	k.Created = time.Unix(k.CreatedUnix, 0).Local()
	k.Updated = time.Unix(k.UpdatedUnix, 0).Local()
	k.HasUsed = k.Updated.After(k.Created)
	k.HasRecentActivity = k.Updated.Add(7 * 24 * time.Hour).After(time.Now())
	return nil
}

func checkDeployKey(tx *gorm.DB, keyID, repoID int64, name string) error {
	// Note: We want error detail, not just true or false here.
	var count int64
	err := tx.Model(new(DeployKey)).Where("key_id = ? AND repo_id = ?", keyID, repoID).Count(&count).Error
	if err != nil {
		return err
	} else if count > 0 {
		return ErrDeployKeyAlreadyExist{keyID, repoID}
	}

	err = tx.Model(new(DeployKey)).Where("repo_id = ? AND name = ?", repoID, name).Count(&count).Error
	if err != nil {
		return err
	} else if count > 0 {
		return ErrDeployKeyNameAlreadyUsed{repoID, name}
	}

	return nil
}

// addDeployKey adds new key-repo relation.
func addDeployKey(tx *gorm.DB, keyID, repoID int64, name, fingerprint string) (*DeployKey, error) {
	if err := checkDeployKey(tx, keyID, repoID, name); err != nil {
		return nil, err
	}

	key := &DeployKey{
		KeyID:       keyID,
		RepoID:      repoID,
		Name:        name,
		Fingerprint: fingerprint,
	}
	return key, tx.Create(key).Error
}

var _ errx.NotFound = (*ErrDeployKeyNotExist)(nil)

type ErrDeployKeyNotExist struct {
	args map[string]any
}

func IsErrDeployKeyNotExist(err error) bool {
	_, ok := err.(ErrDeployKeyNotExist)
	return ok
}

func (err ErrDeployKeyNotExist) Error() string {
	return fmt.Sprintf("deploy key does not exist: %v", err.args)
}

func (ErrDeployKeyNotExist) NotFound() bool {
	return true
}
