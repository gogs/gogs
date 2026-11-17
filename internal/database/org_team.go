package database

import (
	"fmt"

	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
)

const ownerTeamName = "Owners"

// Team represents a organization team.
type Team struct {
	ID          int64
	OrgID       int64      `gorm:"index:IDX_team_org_id"`
	LowerName   string     `gorm:"size:255"`
	Name        string     `gorm:"size:255"`
	Description string     `gorm:"size:255"`
	Authorize   AccessMode `gorm:"size:32"`
	NumRepos    int        `gorm:"size:32"`
	NumMembers  int        `gorm:"size:32"`
}

// AfterFind implements the GORM query hook.
func (t *Team) AfterFind(_ *gorm.DB) error {
	// LEGACY [1.0]: this is backward compatibility bug fix for https://github.com/gogs/gogs/issues/3671
	if t.NumRepos < 0 {
		t.NumRepos = 0
	}
	return nil
}

// IsOwnerTeam returns true if team is owner team.
func (t *Team) IsOwnerTeam() bool {
	return t.Name == ownerTeamName
}

// HasWriteAccess returns true if team has at least write level access mode.
func (t *Team) HasWriteAccess() bool {
	return t.Authorize >= AccessModeWrite
}

var reservedTeamNames = map[string]struct{}{
	"new": {},
}

// IsUsableTeamName return an error if given name is a reserved name or pattern.
func IsUsableTeamName(name string) error {
	return isNameAllowed(reservedTeamNames, nil, name)
}

var _ errx.NotFound = (*ErrTeamNotExist)(nil)

type ErrTeamNotExist struct {
	args map[string]any
}

func IsErrTeamNotExist(err error) bool {
	_, ok := err.(ErrTeamNotExist)
	return ok
}

func (err ErrTeamNotExist) Error() string {
	return fmt.Sprintf("team does not exist: %v", err.args)
}

func (ErrTeamNotExist) NotFound() bool {
	return true
}

// ___________                    ____ ___
// \__    ___/___ _____    _____ |    |   \______ ___________
//   |    |_/ __ \\__  \  /     \|    |   /  ___// __ \_  __ \
//   |    |\  ___/ / __ \|  Y Y  \    |  /\___ \\  ___/|  | \/
//   |____| \___  >____  /__|_|  /______//____  >\___  >__|
//              \/     \/      \/             \/     \/

// TeamUser represents an team-user relation.
type TeamUser struct {
	ID     int64
	OrgID  int64 `gorm:"index:IDX_team_user_org_id"`
	TeamID int64 `gorm:"uniqueIndex:UQE_team_user_s"`
	UID    int64 `gorm:"uniqueIndex:UQE_team_user_s"`
}

// ___________                  __________
// \__    ___/___ _____    _____\______   \ ____ ______   ____
//   |    |_/ __ \\__  \  /     \|       _// __ \\____ \ /  _ \
//   |    |\  ___/ / __ \|  Y Y  \    |   \  ___/|  |_> >  <_> )
//   |____| \___  >____  /__|_|  /____|_  /\___  >   __/ \____/
//              \/     \/      \/       \/     \/|__|

// TeamRepo represents an team-repository relation.
type TeamRepo struct {
	ID     int64
	OrgID  int64 `gorm:"index:IDX_team_repo_org_id"`
	TeamID int64 `gorm:"uniqueIndex:UQE_team_repo_s"`
	RepoID int64 `gorm:"uniqueIndex:UQE_team_repo_s"`
}
