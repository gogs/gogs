package form

import (
	"net/url"
	"strings"
	"github.com/flamego/binding"
	"github.com/unknwon/com"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/netutil"
)
// _______________________________________    _________.______________________ _______________.___.
// \______   \_   _____/\______   \_____  \  /   _____/|   \__    ___/\_____  \\______   \__  |   |
//  |       _/|    __)_  |     ___//   |   \ \_____  \ |   | |    |    /   |   \|       _//   |   |
//  |    |   \|        \ |    |   /    |    \/        \|   | |    |   /    |    \    |   \\____   |
//  |____|_  /_______  / |____|   \_______  /_______  /|___| |____|   \_______  /____|_  // ______|
//         \/        \/                   \/        \/                        \/       \/ \/
type CreateRepo struct {
	UserID      int64  `binding:"Required"`
	RepoName    string `binding:"Required;AlphaDashDot;MaxSize(100)"`
	Private     bool
	Unlisted    bool
	Description string `binding:"MaxSize(512)"`
	AutoInit    bool
	Gitignores  string
	License     string
	Readme      string
}
func (f *CreateRepo) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
	return validate(errs, map[string]interface{}{}, f, req.Context().Value("locale"))
type MigrateRepo struct {
	CloneAddr    string `json:"clone_addr" binding:"Required"`
	AuthUsername string `json:"auth_username"`
	AuthPassword string `json:"auth_password"`
	UID          int64  `json:"uid" binding:"Required"`
	RepoName     string `json:"repo_name" binding:"Required;AlphaDashDot;MaxSize(100)"`
	Mirror       bool   `json:"mirror"`
	Private      bool   `json:"private"`
	Unlisted     bool   `json:"unlisted"`
	Description  string `json:"description" binding:"MaxSize(512)"`
func (f *MigrateRepo) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
// ParseRemoteAddr checks if given remote address is valid,
// and returns composed URL with needed username and password.
// It also checks if given user has permission when remote address
// is actually a local path.
func (f MigrateRepo) ParseRemoteAddr(user *database.User) (string, error) {
	remoteAddr := strings.TrimSpace(f.CloneAddr)
	// Remote address can be HTTP/HTTPS/Git URL or local path.
	if strings.HasPrefix(remoteAddr, "http://") ||
		strings.HasPrefix(remoteAddr, "https://") ||
		strings.HasPrefix(remoteAddr, "git://") {
		u, err := url.Parse(remoteAddr)
		if err != nil {
			return "", database.ErrInvalidCloneAddr{IsURLError: true}
		}
		if netutil.IsBlockedLocalHostname(u.Hostname(), conf.Security.LocalNetworkAllowlist) {
			return "", database.ErrInvalidCloneAddr{IsBlockedLocalAddress: true}
		if len(f.AuthUsername)+len(f.AuthPassword) > 0 {
			u.User = url.UserPassword(f.AuthUsername, f.AuthPassword)
		// To prevent CRLF injection in git protocol, see https://github.com/gogs/gogs/issues/6413
		if u.Scheme == "git" && (strings.Contains(remoteAddr, "%0d") || strings.Contains(remoteAddr, "%0a")) {
		remoteAddr = u.String()
	} else if !user.CanImportLocal() {
		return "", database.ErrInvalidCloneAddr{IsPermissionDenied: true}
	} else if !com.IsDir(remoteAddr) {
		return "", database.ErrInvalidCloneAddr{IsInvalidPath: true}
	}
	return remoteAddr, nil
type RepoSetting struct {
	RepoName      string `binding:"Required;AlphaDashDot;MaxSize(100)"`
	Description   string `binding:"MaxSize(512)"`
	Website       string `binding:"Url;MaxSize(100)"`
	Branch        string
	Interval      int
	MirrorAddress string
	Private       bool
	Unlisted      bool
	EnablePrune   bool
	// Advanced settings
	EnableWiki            bool
	AllowPublicWiki       bool
	EnableExternalWiki    bool
	ExternalWikiURL       string
	EnableIssues          bool
	AllowPublicIssues     bool
	EnableExternalTracker bool
	ExternalTrackerURL    string
	TrackerURLFormat      string
	TrackerIssueStyle     string
	EnablePulls           bool
	PullsIgnoreWhitespace bool
	PullsAllowRebase      bool
func (f *RepoSetting) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
// __________                             .__
// \______   \____________    ____   ____ |  |__
//  |    |  _/\_  __ \__  \  /    \_/ ___\|  |  \
//  |    |   \ |  | \// __ \|   |  \  \___|   Y  \
//  |______  / |__|  (____  /___|  /\___  >___|  /
//         \/             \/     \/     \/     \/
type ProtectBranch struct {
	Protected          bool
	RequirePullRequest bool
	EnableWhitelist    bool
	WhitelistUsers     string
	WhitelistTeams     string
func (f *ProtectBranch) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
//  __      __      ___.   .__    .__            __
// /  \    /  \ ____\_ |__ |  |__ |  |__   ____ |  | __
// \   \/\/   // __ \| __ \|  |  \|  |  \ /  _ \|  |/ /
//  \        /\  ___/| \_\ \   Y  \   Y  (  <_> )    <
//   \__/\  /  \___  >___  /___|  /___|  /\____/|__|_ \
//        \/       \/    \/     \/     \/            \/
type Webhook struct {
	Events       string
	Create       bool
	Delete       bool
	Fork         bool
	Push         bool
	Issues       bool
	IssueComment bool
	PullRequest  bool
	Release      bool
	Active       bool
func (f Webhook) PushOnly() bool {
	return f.Events == "push_only"
func (f Webhook) SendEverything() bool {
	return f.Events == "send_everything"
func (f Webhook) ChooseEvents() bool {
	return f.Events == "choose_events"
type NewWebhook struct {
	PayloadURL  string `binding:"Required;Url"`
	ContentType int    `binding:"Required"`
	Secret      string
	Webhook
func (f *NewWebhook) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type NewSlackHook struct {
	PayloadURL string `binding:"Required;Url"`
	Channel    string `binding:"Required"`
	Username   string
	IconURL    string
	Color      string
func (f *NewSlackHook) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type NewDiscordHook struct {
func (f *NewDiscordHook) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type NewDingtalkHook struct {
func (f *NewDingtalkHook) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
// .___
// |   | ______ ________ __   ____
// |   |/  ___//  ___/  |  \_/ __ \
// |   |\___ \ \___ \|  |  /\  ___/
// |___/____  >____  >____/  \___  >
//          \/     \/            \/
type NewIssue struct {
	Title       string `binding:"Required;MaxSize(255)"`
	LabelIDs    string `form:"label_ids"`
	MilestoneID int64
	AssigneeID  int64
	Content     string
	Files       []string
func (f *NewIssue) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type CreateComment struct {
	Content string
	Status  string `binding:"OmitEmpty;In(reopen,close)"`
	Files   []string
func (f *CreateComment) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
//    _____  .__.__                   __
//   /     \ |__|  |   ____   _______/  |_  ____   ____   ____
//  /  \ /  \|  |  | _/ __ \ /  ___/\   __\/  _ \ /    \_/ __ \
// /    Y    \  |  |_\  ___/ \___ \  |  | (  <_> )   |  \  ___/
// \____|__  /__|____/\___  >____  > |__|  \____/|___|  /\___  >
//         \/             \/     \/                   \/     \/
type CreateMilestone struct {
	Title    string `binding:"Required;MaxSize(50)"`
	Content  string
	Deadline string
func (f *CreateMilestone) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
// .____          ___.          .__
// |    |   _____ \_ |__   ____ |  |
// |    |   \__  \ | __ \_/ __ \|  |
// |    |___ / __ \| \_\ \  ___/|  |__
// |_______ (____  /___  /\___  >____/
//         \/    \/    \/     \/
type CreateLabel struct {
	ID    int64
	Title string `binding:"Required;MaxSize(50)" locale:"repo.issues.label_title"`
	Color string `binding:"Required;Size(7)" locale:"repo.issues.label_color"`
func (f *CreateLabel) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type InitializeLabels struct {
	TemplateName string `binding:"Required"`
func (f *InitializeLabels) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
// __________       .__
// \______   \ ____ |  |   ____ _____    ______ ____
//  |       _// __ \|  | _/ __ \\__  \  /  ___// __ \
//  |    |   \  ___/|  |_\  ___/ / __ \_\___ \\  ___/
//  |____|_  /\___  >____/\___  >____  /____  >\___  >
//         \/     \/          \/     \/     \/     \/
type NewRelease struct {
	TagName    string `binding:"Required"`
	Target     string `form:"tag_target" binding:"Required"`
	Title      string `binding:"Required"`
	Content    string
	Draft      string
	Prerelease bool
	Files      []string
func (f *NewRelease) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type EditRelease struct {
func (f *EditRelease) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
//  __      __.__ __   .__
// /  \    /  \__|  | _|__|
// \   \/\/   /  |  |/ /  |
//  \        /|  |    <|  |
//   \__/\  / |__|__|_ \__|
//        \/          \/
type NewWiki struct {
	OldTitle string
	Title    string `binding:"Required"`
	Content  string `binding:"Required"`
	Message  string
// FIXME: use code generation to generate this method.
func (f *NewWiki) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
// ___________    .___.__  __
// \_   _____/  __| _/|__|/  |_
//  |    __)_  / __ | |  \   __\
//  |        \/ /_/ | |  ||  |
// /_______  /\____ | |__||__|
//         \/      \/
type EditRepoFile struct {
	TreePath      string `binding:"Required;MaxSize(500)"`
	Content       string `binding:"Required"`
	CommitSummary string `binding:"MaxSize(100)"`
	CommitMessage string
	CommitChoice  string `binding:"Required;MaxSize(50)"`
	NewBranchName string `binding:"AlphaDashDotSlash;MaxSize(100)"`
	LastCommit    string
func (f *EditRepoFile) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
func (f *EditRepoFile) IsNewBrnach() bool {
	return f.CommitChoice == "commit-to-new-branch"
type EditPreviewDiff struct {
func (f *EditPreviewDiff) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
//  ____ ___        .__                    .___
// |    |   \______ |  |   _________     __| _/
// |    |   /\____ \|  |  /  _ \__  \   / __ |
// |    |  / |  |_> >  |_(  <_> ) __ \_/ /_/ |
// |______/  |   __/|____/\____(____  /\____ |
//           |__|                   \/      \/
//
type UploadRepoFile struct {
	TreePath      string `binding:"MaxSize(500)"`
	NewBranchName string `binding:"AlphaDashDot;MaxSize(100)"`
	Files         []string
func (f *UploadRepoFile) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
func (f *UploadRepoFile) IsNewBrnach() bool {
type RemoveUploadFile struct {
	File string `binding:"Required;MaxSize(50)"`
func (f *RemoveUploadFile) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
// ________         .__          __
// \______ \   ____ |  |   _____/  |_  ____
// |    |  \_/ __ \|  | _/ __ \   __\/ __ \
// |    `   \  ___/|  |_\  ___/|  | \  ___/
// /_______  /\___  >____/\___  >__|  \___  >
//         \/     \/          \/          \/
type DeleteRepoFile struct {
func (f *DeleteRepoFile) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
func (f *DeleteRepoFile) IsNewBrnach() bool {
