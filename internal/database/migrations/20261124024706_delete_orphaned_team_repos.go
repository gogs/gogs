package migrations

import (
	"gorm.io/gorm"
)

// DeleteOrphanedTeamRepos deletes the team-repo relations of deleted teams,
// which deleting a team or an organization used to leave behind. Restoring a
// backup calls it after importing the team-repo relations.
func DeleteOrphanedTeamRepos(db *gorm.DB) error {
	return db.Exec("DELETE FROM team_repo WHERE team_id NOT IN (SELECT id FROM team)").Error
}
