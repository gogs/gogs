package database

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

type ErrWikiAlreadyExist struct {
	Title string
}

func IsErrWikiAlreadyExist(err error) bool {
	_, ok := err.(ErrWikiAlreadyExist)
	return ok
}

func (err ErrWikiAlreadyExist) Error() string {
	return fmt.Sprintf("wiki page already exists [title: %s]", err.Title)
}

type ErrKeyUnableVerify struct {
	Result string
}

func IsErrKeyUnableVerify(err error) bool {
	return errors.As(err, new(ErrKeyUnableVerify))
}

func (err ErrKeyUnableVerify) Error() string {
	return fmt.Sprintf("Unable to verify key content [result: %s]", err.Result)
}

// ErrSSHDisabled is returned when adding a public key while SSH is disabled.
var ErrSSHDisabled = errors.New("SSH is disabled")

// ErrKeyCheckFailed is returned when the server fails to check a public key,
// e.g., because it cannot run ssh-keygen, rather than because the key is
// invalid. Its details are for the server log, not for the user.
type ErrKeyCheckFailed struct {
	Err error
}

func IsErrKeyCheckFailed(err error) bool {
	return errors.As(err, new(ErrKeyCheckFailed))
}

func (err ErrKeyCheckFailed) Error() string {
	return err.Err.Error()
}

func (err ErrKeyCheckFailed) Unwrap() error {
	return err.Err
}

type ErrKeyNotExist struct {
	ID int64
}

func IsErrKeyNotExist(err error) bool {
	_, ok := err.(ErrKeyNotExist)
	return ok
}

func (err ErrKeyNotExist) Error() string {
	return fmt.Sprintf("public key does not exist [id: %d]", err.ID)
}

func (ErrKeyNotExist) NotFound() bool {
	return true
}

type ErrKeyAlreadyExist struct {
	OwnerID int64
	Content string
}

func IsErrKeyAlreadyExist(err error) bool {
	_, ok := err.(ErrKeyAlreadyExist)
	return ok
}

func (err ErrKeyAlreadyExist) Error() string {
	return fmt.Sprintf("public key already exists [owner_id: %d, content: %s]", err.OwnerID, err.Content)
}

type ErrKeyNameAlreadyUsed struct {
	OwnerID int64
	Name    string
}

func IsErrKeyNameAlreadyUsed(err error) bool {
	_, ok := err.(ErrKeyNameAlreadyUsed)
	return ok
}

func (err ErrKeyNameAlreadyUsed) Error() string {
	return fmt.Sprintf("public key already exists [owner_id: %d, name: %s]", err.OwnerID, err.Name)
}

type ErrKeyAccessDenied struct {
	UserID int64
	KeyID  int64
	Note   string
}

func IsErrKeyAccessDenied(err error) bool {
	_, ok := err.(ErrKeyAccessDenied)
	return ok
}

func (err ErrKeyAccessDenied) Error() string {
	return fmt.Sprintf("user does not have access to the key [user_id: %d, key_id: %d, note: %s]",
		err.UserID, err.KeyID, err.Note)
}

type ErrDeployKeyAlreadyExist struct {
	KeyID  int64
	RepoID int64
}

func IsErrDeployKeyAlreadyExist(err error) bool {
	_, ok := err.(ErrDeployKeyAlreadyExist)
	return ok
}

func (err ErrDeployKeyAlreadyExist) Error() string {
	return fmt.Sprintf("public key already exists [key_id: %d, repo_id: %d]", err.KeyID, err.RepoID)
}

type ErrDeployKeyNameAlreadyUsed struct {
	RepoID int64
	Name   string
}

func IsErrDeployKeyNameAlreadyUsed(err error) bool {
	_, ok := err.(ErrDeployKeyNameAlreadyUsed)
	return ok
}

func (err ErrDeployKeyNameAlreadyUsed) Error() string {
	return fmt.Sprintf("public key already exists [repo_id: %d, name: %s]", err.RepoID, err.Name)
}

type ErrLastOrgOwner struct {
	UID int64
}

func IsErrLastOrgOwner(err error) bool {
	_, ok := err.(ErrLastOrgOwner)
	return ok
}

func (err ErrLastOrgOwner) Error() string {
	return fmt.Sprintf("user is the last member of owner team [uid: %d]", err.UID)
}

type ErrInvalidCloneAddr struct {
	IsURLError            bool
	IsInvalidPath         bool
	IsPermissionDenied    bool
	IsBlockedLocalAddress bool
	IsSSHAddress          bool
}

func IsErrInvalidCloneAddr(err error) bool {
	_, ok := err.(ErrInvalidCloneAddr)
	return ok
}

func (err ErrInvalidCloneAddr) Error() string {
	return fmt.Sprintf("invalid clone address [is_url_error: %v, is_invalid_path: %v, is_permission_denied: %v, is_blocked_local_address: %v, is_ssh_address: %v]",
		err.IsURLError, err.IsInvalidPath, err.IsPermissionDenied, err.IsBlockedLocalAddress, err.IsSSHAddress)
}

type ErrUpdateTaskNotExist struct {
	UUID string
}

func (err ErrUpdateTaskNotExist) Error() string {
	return fmt.Sprintf("update task does not exist [uuid: %s]", err.UUID)
}

type ErrReleaseAlreadyExist struct {
	TagName string
}

func IsErrReleaseAlreadyExist(err error) bool {
	_, ok := err.(ErrReleaseAlreadyExist)
	return ok
}

func (err ErrReleaseAlreadyExist) Error() string {
	return fmt.Sprintf("release tag already exist [tag_name: %s]", err.TagName)
}

type ErrInvalidTagName struct {
	TagName string
}

func IsErrInvalidTagName(err error) bool {
	_, ok := err.(ErrInvalidTagName)
	return ok
}

func (err ErrInvalidTagName) Error() string {
	return fmt.Sprintf("release tag name is not valid [tag_name: %s]", err.TagName)
}

type ErrRepoFileAlreadyExist struct {
	FileName string
}

func IsErrRepoFileAlreadyExist(err error) bool {
	_, ok := err.(ErrRepoFileAlreadyExist)
	return ok
}

func (err ErrRepoFileAlreadyExist) Error() string {
	return fmt.Sprintf("repository file already exists [file_name: %s]", err.FileName)
}

type ErrTeamAlreadyExist struct {
	ID    int64
	OrgID int64
	Name  string
}

func IsErrTeamAlreadyExist(err error) bool {
	_, ok := err.(ErrTeamAlreadyExist)
	return ok
}

func (err ErrTeamAlreadyExist) Error() string {
	return fmt.Sprintf("team already exists [id: %d, org_id: %d, name: %s]", err.ID, err.OrgID, err.Name)
}

type ErrUploadNotExist struct {
	ID   int64
	UUID string
}

func IsErrUploadNotExist(err error) bool {
	_, ok := err.(ErrUploadNotExist)
	return ok
}

func (err ErrUploadNotExist) Error() string {
	return fmt.Sprintf("upload does not exist [id: %d, uuid: %s]", err.ID, err.UUID)
}
