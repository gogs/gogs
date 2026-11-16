package migrations

import (
	"time"

	"gorm.io/gorm"
)

// createBackgroundJobsTable creates the table of the database-backed job queue.
func createBackgroundJobsTable(db *gorm.DB) error {
	type backgroundJobs struct {
		ID             int64     `gorm:"primaryKey"`
		Kind           string    `gorm:"type:VARCHAR(32);not null;index:idx_background_jobs_kind_state_priority_run_at;index:idx_background_jobs_kind_unique_key"`
		UniqueKey      string    `gorm:"type:VARCHAR(255);not null;default:'';index:idx_background_jobs_kind_unique_key"`
		Payload        []byte    `gorm:"not null"`
		State          string    `gorm:"type:VARCHAR(16);not null;index:idx_background_jobs_kind_state_priority_run_at"`
		Priority       int       `gorm:"type:SMALLINT;not null;default:100;index:idx_background_jobs_kind_state_priority_run_at"`
		Attempts       int       `gorm:"not null;default:0"`
		MaxAttempts    int       `gorm:"not null"`
		RunAt          time.Time `gorm:"not null;index:idx_background_jobs_kind_state_priority_run_at"`
		LeaseExpiresAt *time.Time
		LastError      string    `gorm:"type:TEXT"`
		CreatedAt      time.Time `gorm:"not null"`
		UpdatedAt      time.Time `gorm:"not null"`
	}
	if db.Migrator().HasTable(&backgroundJobs{}) {
		return errMigrationSkipped
	}
	return db.Migrator().CreateTable(&backgroundJobs{})
}
