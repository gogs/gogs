package database

import (
	"github.com/cockroachdb/errors"
)

var ErrOrgNotExist = errors.New("Organization does not exist")

// OrgUser represents relations of organizations and their members.
type OrgUser struct {
	ID       int64 `gorm:"primaryKey"`
	UID      int64 `gorm:"column:uid;index:IDX_org_user_uid;uniqueIndex:UQE_org_user_s"`
	OrgID    int64 `gorm:"index:IDX_org_user_org_id;uniqueIndex:UQE_org_user_s"`
	IsPublic bool
	IsOwner  bool
	NumTeams int `gorm:"size:32"`
}
