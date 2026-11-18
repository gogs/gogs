package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
)

type HookContentType int

const (
	JSON HookContentType = iota + 1
	FORM
)

var hookContentTypes = map[string]HookContentType{
	"json": JSON,
	"form": FORM,
}

// ToHookContentType returns HookContentType by given name.
func ToHookContentType(name string) HookContentType {
	return hookContentTypes[name]
}

func (t HookContentType) Name() string {
	switch t {
	case JSON:
		return "json"
	case FORM:
		return "form"
	}
	return ""
}

// IsValidHookContentType returns true if given name is a valid hook content type.
func IsValidHookContentType(name string) bool {
	_, ok := hookContentTypes[name]
	return ok
}

type HookEvents struct {
	Create       bool `json:"create"`
	Delete       bool `json:"delete"`
	Fork         bool `json:"fork"`
	Push         bool `json:"push"`
	Issues       bool `json:"issues"`
	PullRequest  bool `json:"pull_request"`
	IssueComment bool `json:"issue_comment"`
	Release      bool `json:"release"`
}

// HookEvent represents events that trigger hook deliveries.
type HookEvent struct {
	PushOnly       bool `json:"push_only"`
	SendEverything bool `json:"send_everything"`
	ChooseEvents   bool `json:"choose_events"`

	HookEvents `json:"events"`
}

type HookStatus int

const (
	HookStatusNone = iota
	HookStatusSucceed
	HookStatusFailed
)

// Webhook represents a webhook object.
type Webhook struct {
	ID           int64
	RepoID       int64
	OrgID        int64
	URL          string          `gorm:"type:TEXT"`
	ContentType  HookContentType `gorm:"size:32"`
	Secret       string          `gorm:"type:TEXT"`
	Events       string          `gorm:"type:TEXT"`
	*HookEvent   `gorm:"-"`      // LEGACY [1.0]: Cannot ignore JSON (i.e., json:"-") here, it breaks old backup archive.
	IsSSL        bool
	IsActive     bool
	HookTaskType HookTaskType `gorm:"size:32"`
	Meta         string       `gorm:"type:TEXT"` // Hook-specific attributes.
	LastStatus   HookStatus   `gorm:"size:32"`   // Last delivery status.

	Created     time.Time `json:"-" gorm:"-"`
	CreatedUnix int64
	Updated     time.Time `json:"-" gorm:"-"`
	UpdatedUnix int64
}

// BeforeCreate implements the GORM create hook.
func (w *Webhook) BeforeCreate(tx *gorm.DB) error {
	if w.CreatedUnix == 0 {
		w.CreatedUnix = tx.NowFunc().Unix()
		w.UpdatedUnix = w.CreatedUnix
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (w *Webhook) BeforeUpdate(tx *gorm.DB) error {
	w.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (w *Webhook) AfterFind(_ *gorm.DB) error {
	w.HookEvent = &HookEvent{}
	if err := json.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
		handleLogger().Error("Failed to unmarshal webhook events", "webhookID", w.ID, "error", err)
	}
	w.Created = time.Unix(w.CreatedUnix, 0).Local()
	w.Updated = time.Unix(w.UpdatedUnix, 0).Local()
	return nil
}

func (w *Webhook) SlackMeta() *SlackMeta {
	s := &SlackMeta{}
	if err := json.Unmarshal([]byte(w.Meta), s); err != nil {
		handleLogger().Error("Failed to get Slack meta", "webhookID", w.ID, "error", err)
	}
	return s
}

// UpdateEvent handles conversion from HookEvent to Events.
func (w *Webhook) UpdateEvent() error {
	data, err := json.Marshal(w.HookEvent)
	w.Events = string(data)
	return err
}

// HasCreateEvent returns true if hook enabled create event.
func (w *Webhook) HasCreateEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Create)
}

// HasDeleteEvent returns true if hook enabled delete event.
func (w *Webhook) HasDeleteEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Delete)
}

// HasForkEvent returns true if hook enabled fork event.
func (w *Webhook) HasForkEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Fork)
}

// HasPushEvent returns true if hook enabled push event.
func (w *Webhook) HasPushEvent() bool {
	return w.PushOnly || w.SendEverything ||
		(w.ChooseEvents && w.Push)
}

// HasIssuesEvent returns true if hook enabled issues event.
func (w *Webhook) HasIssuesEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Issues)
}

// HasPullRequestEvent returns true if hook enabled pull request event.
func (w *Webhook) HasPullRequestEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.PullRequest)
}

// HasIssueCommentEvent returns true if hook enabled issue comment event.
func (w *Webhook) HasIssueCommentEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.IssueComment)
}

// HasReleaseEvent returns true if hook enabled release event.
func (w *Webhook) HasReleaseEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Release)
}

type eventChecker struct {
	checker func() bool
	typ     HookEventType
}

func (w *Webhook) EventsArray() []string {
	events := make([]string, 0, 8)
	eventCheckers := []eventChecker{
		{w.HasCreateEvent, HookEventTypeCreate},
		{w.HasDeleteEvent, HookEventTypeDelete},
		{w.HasForkEvent, HookEventTypeFork},
		{w.HasPushEvent, HookEventTypePush},
		{w.HasIssuesEvent, HookEventTypeIssues},
		{w.HasPullRequestEvent, HookEventTypePullRequest},
		{w.HasIssueCommentEvent, HookEventTypeIssueComment},
		{w.HasReleaseEvent, HookEventTypeRelease},
	}
	for _, c := range eventCheckers {
		if c.checker() {
			events = append(events, string(c.typ))
		}
	}
	return events
}

var _ errx.NotFound = (*ErrWebhookNotExist)(nil)

type ErrWebhookNotExist struct {
	args map[string]any
}

func IsErrWebhookNotExist(err error) bool {
	_, ok := err.(ErrWebhookNotExist)
	return ok
}

func (err ErrWebhookNotExist) Error() string {
	return fmt.Sprintf("webhook does not exist: %v", err.args)
}

func (ErrWebhookNotExist) NotFound() bool {
	return true
}

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

// get returns the webhook with given ID whose column of given name, i.e.,
// "repo_id" or "org_id", has given value.
func (s *WebhooksStore) get(ctx context.Context, ownerColumn string, ownerID, id int64) (*Webhook, error) {
	w := new(Webhook)
	err := s.db.WithContext(ctx).Where("id = ? AND "+ownerColumn+" = ?", id, ownerID).First(w).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWebhookNotExist{args: map[string]any{"webhookID": id}}
		}
		return nil, err
	}
	return w, nil
}

// GetOfRepo returns the webhook with given ID of the repository. It returns
// ErrWebhookNotExist when not found.
func (s *WebhooksStore) GetOfRepo(ctx context.Context, repoID, id int64) (*Webhook, error) {
	return s.get(ctx, "repo_id", repoID, id)
}

// GetOfOrg returns the webhook with given ID of the organization. It returns
// ErrWebhookNotExist when not found.
func (s *WebhooksStore) GetOfOrg(ctx context.Context, orgID, id int64) (*Webhook, error) {
	return s.get(ctx, "org_id", orgID, id)
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

// delete deletes the webhook with given ID whose column of given name, i.e.,
// "repo_id" or "org_id", has given value, and its deliveries.
func (s *WebhooksStore) delete(ctx context.Context, ownerColumn string, ownerID, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND "+ownerColumn+" = ?", id, ownerID).Delete(new(Webhook))
		if result.Error != nil {
			return errors.Wrap(result.Error, "delete webhook")
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Where("hook_id = ?", id).Delete(new(HookTask)).Error
	})
}

// DeleteOfRepo deletes the webhook with given ID of the repository and its
// deliveries. Deleting a webhook that does not exist is not an error.
func (s *WebhooksStore) DeleteOfRepo(ctx context.Context, repoID, id int64) error {
	return s.delete(ctx, "repo_id", repoID, id)
}

// DeleteOfOrg deletes the webhook with given ID of the organization and its
// deliveries. Deleting a webhook that does not exist is not an error.
func (s *WebhooksStore) DeleteOfOrg(ctx context.Context, orgID, id int64) error {
	return s.delete(ctx, "org_id", orgID, id)
}
