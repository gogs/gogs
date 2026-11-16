package migrations

import (
	"gorm.io/gorm"
)

// deleteOrphanedTeamRepos deletes the team-repo relations of deleted teams,
// which deleting a team used to leave behind.
func deleteOrphanedTeamRepos(db *gorm.DB) error {
	return db.Exec("DELETE FROM team_repo WHERE team_id NOT IN (SELECT id FROM team)").Error
}
