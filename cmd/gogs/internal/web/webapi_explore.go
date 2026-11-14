package web

import (
	"context"
	"html"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
)

type explorePage[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	TotalPages int `json:"totalPages"`
}

func newExplorePage[T any](items []T, page int, total int64) *explorePage[T] {
	// A page size below 1 counts as 1, like the server-rendered pagination did.
	pageSize := int64(max(conf.UI.ExplorePagingNum, 1))
	totalPages := int((total + pageSize - 1) / pageSize)
	return &explorePage[T]{
		Items: items,
		// A page past the end (e.g., a stale link) marks the last page as current,
		// like the server-rendered pagination did.
		Page:       min(page, max(totalPages, 1)),
		TotalPages: totalPages,
	}
}

func explorePageParam(c flamego.Context) int {
	return max(c.QueryInt("page"), 1)
}

type exploreRepo struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
	// DescriptionHTML is the repository description sanitized for rendering as
	// HTML, matching what the legacy templates rendered.
	DescriptionHTML string    `json:"descriptionHTML"`
	AvatarURL       string    `json:"avatarURL,omitempty"`
	IsPrivate       bool      `json:"isPrivate"`
	IsUnlisted      bool      `json:"isUnlisted"`
	IsFork          bool      `json:"isFork"`
	IsMirror        bool      `json:"isMirror"`
	StarCount       int       `json:"starCount"`
	ForkCount       int       `json:"forkCount"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func getExploreRepos(c flamego.Context, user *database.User) (statusCode int, resp *explorePage[exploreRepo], err error) {
	page := explorePageParam(c)
	var viewerID int64
	if user != nil {
		viewerID = user.ID
	}

	repos, total, err := database.SearchRepositoryByName(&database.SearchRepoOptions{
		Keyword:  c.Query("q"),
		UserID:   viewerID,
		OrderBy:  "updated_unix DESC",
		Page:     page,
		PageSize: conf.UI.ExplorePagingNum,
	})
	if err != nil {
		log.Error("getExploreRepos: search repositories: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search repositories")
	}
	if err = database.RepositoryList(repos).LoadAttributes(); err != nil {
		log.Error("getExploreRepos: load repository attributes: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load repository attributes")
	}

	items := make([]exploreRepo, 0, len(repos))
	for _, repo := range repos {
		items = append(items, exploreRepo{
			Owner:           repo.Owner.Name,
			Name:            repo.Name,
			DescriptionHTML: markup.Sanitize(repo.Description),
			AvatarURL:       repo.RelAvatarLink(),
			IsPrivate:       repo.IsPrivate,
			IsUnlisted:      repo.IsUnlisted,
			IsFork:          repo.IsFork,
			IsMirror:        repo.IsMirror,
			StarCount:       repo.NumStars,
			ForkCount:       repo.NumForks,
			UpdatedAt:       repo.Updated.UTC(),
		})
	}
	return http.StatusOK, newExplorePage(items, page, total), nil
}

type exploreUser struct {
	Name      string `json:"name"`
	FullName  string `json:"fullName,omitempty"`
	AvatarURL string `json:"avatarURL"`
	Location  string `json:"location,omitempty"`
	Email     string `json:"email,omitempty"`
	Website   string `json:"website,omitempty"`
	// Description is only set for organizations.
	Description string     `json:"description,omitempty"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
}

func toExploreUser(u *database.User) exploreUser {
	user := exploreUser{
		Name: u.Name,
		// Users loaded through GORM carry an HTML-sanitized full name, while those
		// loaded through xorm carry the raw one. Normalize both to plain text.
		FullName:  html.UnescapeString(markup.Sanitize(u.FullName)),
		AvatarURL: u.AvatarURLPath(),
		Location:  u.Location,
	}
	// Read the column directly because xorm-loaded users leave Created unset.
	// Organizations created before their creation time was recorded have none.
	if u.CreatedUnix > 0 {
		created := time.Unix(u.CreatedUnix, 0).UTC()
		user.CreatedAt = &created
	}
	return user
}

// searchExploreUsers lists users of the given type, or searches them by name
// when the keyword is not empty. It returns the users of the page along with
// the total number of matches.
func searchExploreUsers(ctx context.Context, typ database.UserType, keyword string, page int) ([]*database.User, int64, error) {
	pageSize := conf.UI.ExplorePagingNum
	if keyword != "" {
		if typ == database.UserTypeOrganization {
			return database.Handle.Organizations().SearchByName(ctx, keyword, page, pageSize, "updated_unix DESC")
		}
		return database.Handle.Users().SearchByName(ctx, keyword, page, pageSize, "updated_unix DESC")
	}

	if typ == database.UserTypeOrganization {
		orgs, err := database.Organizations(page, pageSize)
		return orgs, database.CountOrganizations(), err
	}
	users, err := database.Handle.Users().List(ctx, page, pageSize)
	return users, database.Handle.Users().Count(ctx), err
}

func getExploreUsers(c flamego.Context, user *database.User) (statusCode int, resp *explorePage[exploreUser], err error) {
	page := explorePageParam(c)
	users, total, err := searchExploreUsers(c.Request().Context(), database.UserTypeIndividual, c.Query("q"), page)
	if err != nil {
		log.Error("getExploreUsers: search users: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search users")
	}

	items := make([]exploreUser, 0, len(users))
	for _, u := range users {
		item := toExploreUser(u)
		// Email addresses are only shown to signed-in users.
		if user != nil {
			item.Email = u.Email
		}
		items = append(items, item)
	}
	return http.StatusOK, newExplorePage(items, page, total), nil
}

func getExploreOrganizations(c flamego.Context) (statusCode int, resp *explorePage[exploreUser], err error) {
	page := explorePageParam(c)
	orgs, total, err := searchExploreUsers(c.Request().Context(), database.UserTypeOrganization, c.Query("q"), page)
	if err != nil {
		log.Error("getExploreOrganizations: search organizations: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search organizations")
	}

	items := make([]exploreUser, 0, len(orgs))
	for _, org := range orgs {
		item := toExploreUser(org)
		item.Website = org.Website
		item.Description = org.Description
		items = append(items, item)
	}
	return http.StatusOK, newExplorePage(items, page, total), nil
}
