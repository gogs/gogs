package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// WebhooksStore is the storage layer for webhooks.
type WebhooksStore struct {
	db *gorm.DB
}

func newWebhooksStore(db *gorm.DB) *WebhooksStore {
	return &WebhooksStore{db: db}
}

// Create creates a new webhook.
func (s *WebhooksStore) Create(ctx context.Context, w *Webhook) error {
	return s.db.WithContext(ctx).Create(w).Error
}

func (s *WebhooksStore) get(ctx context.Context, where *Webhook) (*Webhook, error) {
	w := new(Webhook)
	err := s.db.WithContext(ctx).Where(where).First(w).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWebhookNotExist{args: map[string]any{"webhookID": where.ID}}
		}
		return nil, err
	}
	return w, nil
}

// GetOfRepo returns the webhook with given ID of the repository. It returns
// ErrWebhookNotExist when not found.
func (s *WebhooksStore) GetOfRepo(ctx context.Context, repoID, id int64) (*Webhook, error) {
	return s.get(ctx, &Webhook{ID: id, RepoID: repoID})
}

// GetOfOrg returns the webhook with given ID of the organization. It returns
// ErrWebhookNotExist when not found.
func (s *WebhooksStore) GetOfOrg(ctx context.Context, orgID, id int64) (*Webhook, error) {
	return s.get(ctx, &Webhook{ID: id, OrgID: orgID})
}

// ListByRepo returns all webhooks of the repository.
func (s *WebhooksStore) ListByRepo(ctx context.Context, repoID int64) ([]*Webhook, error) {
	webhooks := make([]*Webhook, 0, 5)
	return webhooks, s.db.WithContext(ctx).Where("repo_id = ?", repoID).Order("id ASC").Find(&webhooks).Error
}

// ListByOrg returns all webhooks of the organization.
func (s *WebhooksStore) ListByOrg(ctx context.Context, orgID int64) ([]*Webhook, error) {
	webhooks := make([]*Webhook, 0, 5)
	return webhooks, s.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id ASC").Find(&webhooks).Error
}

// listActive returns the active webhooks of the repository, followed by those
// of its owner when the owner is an organization.
func (s *WebhooksStore) listActive(ctx context.Context, repo *Repository) ([]*Webhook, error) {
	db := s.db.WithContext(ctx)
	var webhooks []*Webhook
	err := db.Where("repo_id = ? AND is_active = ?", repo.ID, true).Order("id ASC").Find(&webhooks).Error
	if err != nil {
		return nil, errors.Wrap(err, "list webhooks of repository")
	}

	if !repo.mustOwner(s.db).IsOrganization() {
		return webhooks, nil
	}
	var orgWebhooks []*Webhook
	err = db.Where("org_id = ? AND is_active = ?", repo.OwnerID, true).Order("id ASC").Find(&orgWebhooks).Error
	if err != nil {
		return nil, errors.Wrap(err, "list webhooks of organization")
	}
	return append(webhooks, orgWebhooks...), nil
}

// Update saves all fields of the webhook.
func (s *WebhooksStore) Update(ctx context.Context, w *Webhook) error {
	return s.db.WithContext(ctx).Model(w).Select("*").Updates(w).Error
}

// setLastStatus records the status of the last delivery of the webhook with
// given ID.
func (s *WebhooksStore) setLastStatus(ctx context.Context, id int64, status HookStatus) error {
	return s.db.WithContext(ctx).
		Model(new(Webhook)).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_status":  status,
			"updated_unix": s.db.NowFunc().Unix(),
		}).
		Error
}

func (s *WebhooksStore) delete(ctx context.Context, where *Webhook) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(where).Delete(new(Webhook))
		if result.Error != nil {
			return errors.Wrap(result.Error, "delete webhook")
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Where("hook_id = ?", where.ID).Delete(new(HookTask)).Error
	})
}

// DeleteOfRepo deletes the webhook with given ID of the repository and its
// deliveries. Deleting a webhook that does not exist is not an error.
func (s *WebhooksStore) DeleteOfRepo(ctx context.Context, repoID, id int64) error {
	return s.delete(ctx, &Webhook{ID: id, RepoID: repoID})
}

// DeleteOfOrg deletes the webhook with given ID of the organization and its
// deliveries. Deleting a webhook that does not exist is not an error.
func (s *WebhooksStore) DeleteOfOrg(ctx context.Context, orgID, id int64) error {
	return s.delete(ctx, &Webhook{ID: id, OrgID: orgID})
}
