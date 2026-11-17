package migrations

import (
	"gorm.io/gorm"
)

// addAttachmentUploaderID adds the column that XORM used to add on startup
// since attachments record who uploaded them.
func addAttachmentUploaderID(db *gorm.DB) error {
	return addMissingColumns(db, "attachment", map[string]map[string]string{
		"uploader_id": {
			"sqlite":   "INTEGER NULL",
			"postgres": "BIGINT NULL",
			"mysql":    "BIGINT(20) NULL",
		},
	})
}
