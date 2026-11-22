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
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/template"
)

// profileUser is the user or organization whose profile is requested.
type profileUser struct {
	*database.User
}

// withProfileUser maps the user or organization named by the `username` path
// parameter as *profileUser.
func withProfileUser(logger *logx.Logger, c flamego.Context) {
	w := c.ResponseWriter()
	name := c.Param("username")
	u, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("user does not exist"))
			return
		}
		logger.Error("Failed to get user by username", "username", name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get user"))
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
	// Kind is "user", telling individual users apart from organizations.
	Kind string `json:"kind"`
	exploreUser
	FollowerCount     int              `json:"followerCount"`
	FollowingCount    int              `json:"followingCount"`
	Organizations     []userProfileOrg `json:"organizations"`
	ViewerIsSelf      bool             `json:"viewerIsSelf"`
	ViewerIsFollowing bool             `json:"viewerIsFollowing"`
}

// getUserProfile returns the profile of a user, or the home page of an
// organization.
func getUserProfile(logger *logx.Logger, c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if puser.IsOrganization() {
		return getOrgProfile(ctx, logger, viewer, puser)
	}

	orgs, err := database.Handle.Organizations().List(ctx, database.ListOrgsOptions{MemberID: puser.ID, IncludePrivateMembers: puser.canSeePrivate(viewer)})
	if err != nil {
		logger.Error("Failed to get organizations of user", "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get organizations")
	}

	profile := &userProfile{
		Kind:           "user",
		exploreUser:    toExploreUser(puser.User),
		FollowerCount:  puser.NumFollowers,
		FollowingCount: puser.NumFollowing,
		Organizations:  make([]userProfileOrg, 0, len(orgs)),
	}
	profile.AvatarURL = puser.AvatarURLPath()
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

func getUserProfileRepos(logger *logx.Logger, c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp *listPage[exploreRepo], err error) {
	ctx := c.Request().Context()
	page := pageParam(c)
	var (
		repos []*database.Repository
		total int64
	)
	if puser.IsOrganization() && viewer != nil && !viewer.IsAdmin {
		// Authenticated users see the public repositories of the organization and
		// those of their teams.
		repos, total, err = database.Handle.Organizations().ListAccessibleRepos(ctx, puser.ID, viewer.ID, page, conf.UI.User.RepoPagingNum)
	} else {
		showPrivate := puser.canSeePrivate(viewer)
		repos, err = database.GetUserRepositories(&database.UserRepoOptions{
			UserID:   puser.ID,
			Private:  showPrivate,
			Page:     page,
			PageSize: conf.UI.User.RepoPagingNum,
		})
		total = database.CountUserRepositories(logger, puser.ID, showPrivate)
	}
	if err != nil {
		logger.Error("Failed to get repositories of user", "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repositories")
	}
	for _, repo := range repos {
		repo.Owner = puser.User
	}
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
// name.
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
		headline = l.Tr("action::create_repo", "repo_link", repoLink, "repo", repoPath)
	case database.ActionRenameRepo:
		headline = l.Tr("action::rename_repo", "old_name", esc(act.GetContent()), "repo_link", repoLink, "repo", repoPath)
	case database.ActionCommitRepo:
		headline = l.Tr("action::commit_repo", "repo_link", repoLink, "ref_link", refLink, "ref", ref, "repo", repoPath)
	case database.ActionCreateIssue:
		headline = l.Tr("action::create_issue", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionCreatePullRequest:
		headline = l.Tr("action::create_pull_request", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionTransferRepo:
		headline = l.Tr("action::transfer_repo", "old_name", esc(act.GetContent()), "repo_link", repoLink, "repo", repoPath)
	case database.ActionPushTag:
		headline = l.Tr("action::push_tag", "repo_link", repoLink, "ref", ref, "repo", repoPath)
	case database.ActionCommentIssue:
		headline = l.Tr("action::comment_issue", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionMergePullRequest:
		headline = l.Tr("action::merge_pull_request", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionCloseIssue:
		headline = l.Tr("action::close_issue", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionReopenIssue:
		headline = l.Tr("action::reopen_issue", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionClosePullRequest:
		headline = l.Tr("action::close_pull_request", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionReopenPullRequest:
		headline = l.Tr("action::reopen_pull_request", "repo_link", repoLink, "index", index, "repo", repoPath)
	case database.ActionCreateBranch:
		headline = l.Tr("action::create_branch", "repo_link", repoLink, "ref_link", refLink, "ref", ref, "repo", repoPath)
	case database.ActionDeleteBranch:
		headline = l.Tr("action::delete_branch", "repo_link", repoLink, "ref", ref, "repo", repoPath)
	case database.ActionDeleteTag:
		headline = l.Tr("action::delete_tag", "repo_link", repoLink, "ref", ref, "repo", repoPath)
	case database.ActionForkRepo:
		headline = l.Tr("action::fork_repo", "repo_link", repoLink, "repo", repoPath)
	case database.ActionMirrorSyncPush:
		headline = l.Tr("action::mirror_sync_push", "repo_link", repoLink, "ref_link", refLink, "ref", ref, "repo", repoPath)
	case database.ActionMirrorSyncCreate:
		headline = l.Tr("action::mirror_sync_create", "repo_link", repoLink, "ref", ref, "repo", repoPath)
	case database.ActionMirrorSyncDelete:
		headline = l.Tr("action::mirror_sync_delete", "repo_link", repoLink, "ref", ref, "repo", repoPath)
	}
	// Translations are community-maintained, so they are sanitized.
	return markup.Sanitize(headline)
}

// toActivityFeed converts a page of actions as listed from the database. It
// leaves out actions of users that no longer exist.
func toActivityFeed(ctx context.Context, logger *logx.Logger, l i18n.Locale, actions []*database.Action) (*activityFeed, error) {
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
			push := template.ActionContent2Commits(logger, act)
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

func getUserProfileActivity(logger *logx.Logger, c flamego.Context, l i18n.Locale, viewer *database.User, puser *profileUser) (statusCode int, resp *activityFeed, err error) {
	// Organizations have no activity tab.
	if puser.IsOrganization() {
		return http.StatusNotFound, nil, errors.New("user does not exist")
	}

	ctx := c.Request().Context()
	afterID, _ := strconv.ParseInt(c.Query("afterID"), 10, 64)
	actions, err := database.Handle.Actions().ListByUser(ctx, puser.ID, cmp.Or(viewer, &database.User{}).ID, afterID, true)
	if err != nil {
		logger.Error("Failed to list actions of user", "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list actions")
	}
	feed, err := toActivityFeed(ctx, logger, l, actions)
	if err != nil {
		logger.Error("Failed to convert actions of user", "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "convert actions")
	}
	return http.StatusOK, feed, nil
}

type userFollowResponse struct {
	FollowerCount int `json:"followerCount"`
}

func userFollowAction(ctx context.Context, logger *logx.Logger, viewer *database.User, puser *profileUser, following bool) (statusCode int, resp *userFollowResponse, err error) {
	// Organizations cannot be followed.
	if puser.IsOrganization() {
		return http.StatusNotFound, nil, errors.New("user does not exist")
	}
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
		logger.Error("Failed to update follow status", "following", following, "followerID", viewer.ID, "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "follow user")
	}

	updated, err := database.Handle.Users().GetByID(ctx, puser.ID)
	if err != nil {
		logger.Error("Failed to reload user", "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload user")
	}
	return http.StatusOK, &userFollowResponse{FollowerCount: updated.NumFollowers}, nil
}

func postUserFollow(logger *logx.Logger, c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp *userFollowResponse, err error) {
	return userFollowAction(c.Request().Context(), logger, viewer, puser, true)
}

func deleteUserFollow(logger *logx.Logger, c flamego.Context, viewer *database.User, puser *profileUser) (statusCode int, resp *userFollowResponse, err error) {
	return userFollowAction(c.Request().Context(), logger, viewer, puser, false)
}

type orgProfileMember struct {
	Name      string `json:"name"`
	FullName  string `json:"fullName,omitempty"`
	AvatarURL string `json:"avatarURL"`
}

type orgProfileTeam struct {
	Name        string `json:"name"`
	LowerName   string `json:"lowerName"`
	MemberCount int    `json:"memberCount"`
	RepoCount   int    `json:"repoCount"`
}

type orgProfile struct {
	// Kind is "organization", telling organizations apart from individual users.
	Kind string `json:"kind"`
	exploreUser
	ID      int64              `json:"id"`
	Members []orgProfileMember `json:"members"`
	// The member and team counts and the teams are only shown to members.
	MemberCount    int              `json:"memberCount,omitempty"`
	TeamCount      int              `json:"teamCount,omitempty"`
	Teams          []orgProfileTeam `json:"teams,omitempty"`
	ViewerIsMember bool             `json:"viewerIsMember"`
	ViewerIsOwner  bool             `json:"viewerIsOwner"`
}

// orgMembersShown is the number of members listed on the home page of an
// organization.
const orgMembersShown = 12

func getOrgProfile(ctx context.Context, logger *logx.Logger, viewer *database.User, org *profileUser) (statusCode int, resp *orgProfile, err error) {
	profile := &orgProfile{
		Kind:        "organization",
		exploreUser: toExploreUser(org.User),
		ID:          org.ID,
		Members:     []orgProfileMember{},
	}
	profile.Description = org.Description
	profile.AvatarURL = org.AvatarURLPath()
	profile.Website = org.Website
	if viewer != nil {
		// Admins have the access of owners.
		profile.ViewerIsOwner = viewer.IsAdmin || database.Handle.Organizations().IsOwner(ctx, org.ID, viewer.ID)
		profile.ViewerIsMember = profile.ViewerIsOwner || database.Handle.Organizations().IsMember(ctx, org.ID, viewer.ID)
	}

	members, err := database.Handle.Organizations().ListMembers(ctx, org.ID, orgMembersShown)
	if err != nil {
		logger.Error("Failed to get members of organization", "orgID", org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get members")
	}
	for _, m := range members {
		// Members see everyone, others only the public members.
		if !profile.ViewerIsMember && !database.Handle.Organizations().IsPublicMember(ctx, org.ID, m.ID) {
			continue
		}
		member := toExploreUser(m)
		profile.Members = append(profile.Members, orgProfileMember{
			Name:      member.Name,
			FullName:  member.FullName,
			AvatarURL: member.AvatarURL,
		})
	}

	if !profile.ViewerIsMember {
		return http.StatusOK, profile, nil
	}
	profile.MemberCount = org.NumMembers
	profile.TeamCount = org.NumTeams
	// Owners see every team, other members only their own.
	var teams []*database.Team
	if profile.ViewerIsOwner {
		teams, err = database.Handle.Teams().ListByOrg(ctx, org.ID)
	} else {
		teams, err = database.Handle.Teams().ListByMember(ctx, org.ID, viewer.ID)
	}
	if err != nil {
		logger.Error("Failed to get teams of organization", "orgID", org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get teams")
	}
	profile.Teams = make([]orgProfileTeam, 0, len(teams))
	for _, team := range teams {
		profile.Teams = append(profile.Teams, orgProfileTeam{
			Name:        team.Name,
			LowerName:   team.LowerName,
			MemberCount: team.NumMembers,
			RepoCount:   team.NumRepos,
		})
	}
	return http.StatusOK, profile, nil
}
