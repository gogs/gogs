package migrations

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/repox"
)

type repositoryV22 struct {
	ID            int64
	OwnerID       int64
	LowerName     string
	DefaultBranch string
}

func (*repositoryV22) TableName() string {
	return "repository"
}

type userV22 struct {
	ID   int64
	Name string
}

func (*userV22) TableName() string {
	return "user"
}

// backfillRepositoryDefaultBranch sets the default branch of repositories that
// have none saved, which happened for repositories that were empty when migrated.
func backfillRepositoryDefaultBranch(db *gorm.DB) error {
	var repos []*repositoryV22
	err := db.Where("default_branch = ? OR default_branch IS NULL", "").Find(&repos).Error
	if err != nil {
		return errors.Wrap(err, "list repositories")
	}
	if len(repos) == 0 {
		return errMigrationSkipped
	}

	for _, repo := range repos {
		defaultBranch := conf.Repository.DefaultBranch
		branch, err := headBranch(db, repo)
		if err != nil {
			log.Warn("Failed to get HEAD branch of repository [id: %d], falling back to %q: %v", repo.ID, defaultBranch, err)
		} else {
			defaultBranch = branch
		}

		err = db.Model(&repositoryV22{}).Where("id = ?", repo.ID).Update("default_branch", defaultBranch).Error
		if err != nil {
			return errors.Wrapf(err, "update default branch of repository [id: %d]", repo.ID)
		}
	}
	return nil
}

func headBranch(db *gorm.DB, repo *repositoryV22) (string, error) {
	var owner userV22
	err := db.Where("id = ?", repo.OwnerID).First(&owner).Error
	if err != nil {
		return "", errors.Wrap(err, "get owner")
	}

	ref, err := git.RepoSymbolicRef(context.TODO(), repox.RepositoryPath(owner.Name, repo.LowerName))
	if err != nil {
		return "", errors.Wrap(err, "get HEAD")
	}
	return git.RefShortName(ref), nil
}
