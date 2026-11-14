package web

import (
	"cmp"
	"context"
	"html"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/template"
	"gogs.io/gogs/internal/tool"
)

// profileUser is the individual user whose profile is requested.
type profileUser struct {
	*database.User
}

// withProfileUser maps the individual user named by the `username` path
// parameter as *profileUser. Organizations have their own home page, so they
// are not found here.
func withProfileUser(c flamego.Context) {
	w := c.ResponseWriter()
	name := c.Param("username")
	u, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("user does not exist"))
			return
		}
		log.Error("withProfileUser: get user by username %q: %v", name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get user"))
		return
	}
	if u.IsOrganization() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("user does not exist"))
		return
	}
	c.Map(&profileUser{u})
}

// canSeePrivate returns true if the viewer may see the private repositories and
// organization memberships of the profile user.
func (p *profileUser) canSeePrivate(viewer *database.User) bool {
	return viewer != nil && (viewer.IsAdmin || viewer.ID == p.ID)
}

type userProfileOrg struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
}

type userProfile struct {
	exploreUser
	FollowerCount     int              `json:"followerCount"`
	FollowingCount    int              `json:"followingCount"`
	Organizations     []userProfileOrg `json:"organizations"`
	ViewerIsSelf      bool             `json:"viewerIsSelf"`
	ViewerIsFollowing bool             `json:"viewerIsFollowing"`
}

func getUserProfile(c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp *userProfile, err error) {
	ctx := c.Request().Context()
	orgs, err := database.GetOrgsByUserID(puser.ID, puser.canSeePrivate(viewer))
	if err != nil {
		log.Error("getUserProfile: get organizations of user %d: %v", puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get organizations")
	}

	profile := &userProfile{
		exploreUser:    toExploreUser(puser.User),
		FollowerCount:  puser.NumFollowers,
		FollowingCount: puser.NumFollowing,
		Organizations:  make([]userProfileOrg, 0, len(orgs)),
	}
	// The avatar fills the sidebar, at most 296px wide, so request it for 2x displays.
	profile.AvatarURL = tool.AppendAvatarSize(puser.AvatarURLPath(), 592)
	profile.Website = puser.Website
	for _, org := range orgs {
		profile.Organizations = append(profile.Organizations, userProfileOrg{
			Name:      org.Name,
			AvatarURL: org.AvatarURLPath(),
		})
	}
	if viewer != nil {
		// Email addresses are only shown to authenticated viewers.
		profile.Email = puser.Email
		profile.ViewerIsSelf = viewer.ID == puser.ID
		profile.ViewerIsFollowing = !profile.ViewerIsSelf && database.Handle.Users().IsFollowing(ctx, viewer.ID, puser.ID)
	}
	return http.StatusOK, profile, nil
}

func getUserProfileRepos(c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp *listPage[exploreRepo], err error) {
	page := pageParam(c)
	showPrivate := puser.canSeePrivate(viewer)
	repos, err := database.GetUserRepositories(&database.UserRepoOptions{
		UserID:   puser.ID,
		Private:  showPrivate,
		Page:     page,
		PageSize: conf.UI.User.RepoPagingNum,
	})
	if err != nil {
		log.Error("getUserProfileRepos: get repositories of user %d: %v", puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repositories")
	}
	for _, repo := range repos {
		repo.Owner = puser.User
	}
	total := database.CountUserRepositories(puser.ID, showPrivate)
	return http.StatusOK, newListPage(toExploreRepos(repos), page, total, conf.UI.User.RepoPagingNum), nil
}

type activityCommit struct {
	SHA       string `json:"sha"`
	Message   string `json:"message"`
	AvatarURL string `json:"avatarURL"`
}

type activityItem struct {
	ID             int64  `json:"id"`
	OpType         int    `json:"opType"`
	ActorName      string `json:"actorName"`
	ActorAvatarURL string `json:"actorAvatarURL"`
	// HeadlineHTML is the localized sentence after the actor's name, sanitized
	// for rendering as HTML.
	HeadlineHTML string `json:"headlineHTML"`
	// Title is the title of the issue or pull request the action is about.
	Title string `json:"title,omitempty"`
	// Detail is the text of a comment or merge.
	Detail string `json:"detail,omitempty"`
	// RepoURL is the URL path of the repository the action is in.
	RepoURL     string           `json:"repoURL"`
	Commits     []activityCommit `json:"commits,omitempty"`
	CommitCount int              `json:"commitCount,omitempty"`
	CompareURL  string           `json:"compareURL,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
}

type activityFeed struct {
	Items []activityItem `json:"items"`
	// NextAfterID is the `afterID` that loads the next page, or zero when this
	// is the last page.
	NextAfterID int64 `json:"nextAfterID,omitempty"`
}

// activityHeadline returns the localized sentence that follows the actor's
// name, in the same form as the server-rendered feeds.
func activityHeadline(l i18n.Locale, act *database.Action) string {
	esc := html.EscapeString
	repoLink := esc(act.GetRepoLink())
	repoPath := esc(act.ShortRepoPath())
	ref := esc(act.GetBranch())
	refLink := esc(template.EscapePound(act.GetBranch()))
	index := esc(act.GetIssueInfos()[0])

	var headline string
	switch act.OpType {
	case database.ActionCreateRepo:
		headline = l.Tr("action.create_repo", repoLink, repoPath)
	case database.ActionRenameRepo:
		headline = l.Tr("action.rename_repo", esc(act.GetContent()), repoLink, repoPath)
	case database.ActionCommitRepo:
		headline = l.Tr("action.commit_repo", repoLink, refLink, ref, repoPath)
	case database.ActionCreateIssue:
		headline = l.Tr("action.create_issue", repoLink, index, repoPath)
	case database.ActionCreatePullRequest:
		headline = l.Tr("action.create_pull_request", repoLink, index, repoPath)
	case database.ActionTransferRepo:
		headline = l.Tr("action.transfer_repo", esc(act.GetContent()), repoLink, repoPath)
	case database.ActionPushTag:
		headline = l.Tr("action.push_tag", repoLink, ref, repoPath)
	case database.ActionCommentIssue:
		headline = l.Tr("action.comment_issue", repoLink, index, repoPath)
	case database.ActionMergePullRequest:
		headline = l.Tr("action.merge_pull_request", repoLink, index, repoPath)
	case database.ActionCloseIssue:
		headline = l.Tr("action.close_issue", repoLink, index, repoPath)
	case database.ActionReopenIssue:
		headline = l.Tr("action.reopen_issue", repoLink, index, repoPath)
	case database.ActionClosePullRequest:
		headline = l.Tr("action.close_pull_request", repoLink, index, repoPath)
	case database.ActionReopenPullRequest:
		headline = l.Tr("action.reopen_pull_request", repoLink, index, repoPath)
	case database.ActionCreateBranch:
		headline = l.Tr("action.create_branch", repoLink, refLink, ref, repoPath)
	case database.ActionDeleteBranch:
		headline = l.Tr("action.delete_branch", repoLink, ref, repoPath)
	case database.ActionDeleteTag:
		headline = l.Tr("action.delete_tag", repoLink, ref, repoPath)
	case database.ActionForkRepo:
		headline = l.Tr("action.fork_repo", repoLink, repoPath)
	case database.ActionMirrorSyncPush:
		headline = l.Tr("action.mirror_sync_push", repoLink, refLink, ref, repoPath)
	case database.ActionMirrorSyncCreate:
		headline = l.Tr("action.mirror_sync_create", repoLink, ref, repoPath)
	case database.ActionMirrorSyncDelete:
		headline = l.Tr("action.mirror_sync_delete", repoLink, ref, repoPath)
	}
	// Translations are community-maintained, so sanitize them like the
	// server-rendered feeds did.
	return markup.Sanitize(headline)
}

// toActivityFeed converts a page of actions as listed from the database. Like
// the server-rendered feeds, it leaves out actions of users that no longer
// exist.
func toActivityFeed(ctx context.Context, l i18n.Locale, actions []*database.Action) (*activityFeed, error) {
	feed := &activityFeed{Items: make([]activityItem, 0, len(actions))}
	// A shorter page than requested is the last one.
	if len(actions) > 0 && len(actions) >= conf.UI.User.NewsFeedPagingNum {
		feed.NextAfterID = actions[len(actions)-1].ID
	}

	actorAvatars := make(map[string]string)
	for _, act := range actions {
		avatarURL, ok := actorAvatars[act.ActUserName]
		if !ok {
			actor, err := database.Handle.Users().GetByUsername(ctx, act.ActUserName)
			if err != nil {
				if database.IsErrUserNotExist(err) {
					actorAvatars[act.ActUserName] = ""
					continue
				}
				return nil, errors.Wrap(err, "get actor")
			}
			avatarURL = actor.AvatarURLPath()
			actorAvatars[act.ActUserName] = avatarURL
		}
		if avatarURL == "" {
			continue
		}

		item := activityItem{
			ID:             act.ID,
			OpType:         int(act.OpType),
			ActorName:      act.ActUserName,
			ActorAvatarURL: avatarURL,
			HeadlineHTML:   activityHeadline(l, act),
			RepoURL:        act.GetRepoLink(),
			CreatedAt:      time.Unix(act.CreatedUnix, 0).UTC(),
		}
		switch act.OpType {
		case database.ActionCommitRepo, database.ActionMirrorSyncPush:
			push := template.ActionContent2Commits(act)
			for _, commit := range push.Commits {
				item.Commits = append(item.Commits, activityCommit{
					SHA:       commit.Sha1,
					Message:   commit.Message,
					AvatarURL: push.AvatarLink(commit.AuthorEmail),
				})
			}
			item.CommitCount = push.Len
			if push.Len > 1 && push.CompareURL != "" {
				item.CompareURL = conf.Server.Subpath + "/" + push.CompareURL
			}
		case database.ActionCreateIssue, database.ActionCreatePullRequest:
			item.Title = issueInfo(act)
		case database.ActionCommentIssue:
			item.Title = act.GetIssueTitle()
			item.Detail = issueInfo(act)
		case database.ActionMergePullRequest:
			item.Detail = issueInfo(act)
		case database.ActionCloseIssue, database.ActionReopenIssue, database.ActionClosePullRequest, database.ActionReopenPullRequest:
			item.Title = act.GetIssueTitle()
		}
		feed.Items = append(feed.Items, item)
	}
	return feed, nil
}

// issueInfo returns the text after the issue index in the content of an issue
// action, e.g., the issue title or comment.
func issueInfo(act *database.Action) string {
	infos := act.GetIssueInfos()
	if len(infos) < 2 {
		return ""
	}
	return infos[1]
}

func getUserProfileActivity(c flamego.Context, l i18n.Locale, viewer *database.User, puser *profileUser) (statusCode int, resp *activityFeed, err error) {
	ctx := c.Request().Context()
	afterID, _ := strconv.ParseInt(c.Query("afterID"), 10, 64)
	actions, err := database.Handle.Actions().ListByUser(ctx, puser.ID, cmp.Or(viewer, &database.User{}).ID, afterID, true)
	if err != nil {
		log.Error("getUserProfileActivity: list actions of user %d: %v", puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list actions")
	}
	feed, err := toActivityFeed(ctx, l, actions)
	if err != nil {
		log.Error("getUserProfileActivity: convert actions of user %d: %v", puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "convert actions")
	}
	return http.StatusOK, feed, nil
}

type userFollowResponse struct {
	FollowerCount int `json:"followerCount"`
}

func userFollowAction(ctx context.Context, viewer *database.User, puser *profileUser, following bool) (statusCode int, resp *userFollowResponse, err error) {
	// Match the authentication requirement of the server-rendered follow form.
	if viewer == nil {
		return http.StatusUnauthorized, nil, errors.New("authentication required")
	}
	if !viewer.IsActive && conf.Auth.RequireEmailConfirmation {
		return http.StatusForbidden, nil, errors.New("account not activated")
	}

	if following {
		err = database.Handle.Users().Follow(ctx, viewer.ID, puser.ID)
	} else {
		err = database.Handle.Users().Unfollow(ctx, viewer.ID, puser.ID)
	}
	if err != nil {
		log.Error("userFollowAction: set following=%t for user %d on user %d: %v", following, viewer.ID, puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "follow user")
	}

	updated, err := database.Handle.Users().GetByID(ctx, puser.ID)
	if err != nil {
		log.Error("userFollowAction: reload user %d: %v", puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload user")
	}
	return http.StatusOK, &userFollowResponse{FollowerCount: updated.NumFollowers}, nil
}

func postUserFollow(c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp *userFollowResponse, err error) {
	return userFollowAction(c.Request().Context(), viewer, puser, true)
}

func deleteUserFollow(c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp *userFollowResponse, err error) {
	return userFollowAction(c.Request().Context(), viewer, puser, false)
}
