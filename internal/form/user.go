package form

import (
	"mime/multipart"
	"github.com/flamego/binding"
)
//nolint:staticcheck // Reason: needed for legacy code
type Install struct {
	DbType   string `binding:"Required"`
	DbHost   string
	DbUser   string
	DbPasswd string
	DbName   string
	DbSchema string
	SSLMode  string
	DbPath   string
	AppName             string `binding:"Required" locale:"install.app_name"`
	RepoRootPath        string `binding:"Required"`
	RunUser             string `binding:"Required"`
	Domain              string `binding:"Required"`
	SSHPort             int
	UseBuiltinSSHServer bool
	HTTPPort            string `binding:"Required"`
	AppUrl              string `binding:"Required"`
	LogRootPath         string `binding:"Required"`
	EnableConsoleMode   bool
	DefaultBranch       string
	SMTPHost        string
	SMTPFrom        string
	SMTPUser        string `binding:"OmitEmpty;MaxSize(254)" locale:"install.mailer_user"`
	SMTPPasswd      string
	RegisterConfirm bool
	MailNotify      bool
	OfflineMode           bool
	DisableGravatar       bool
	EnableFederatedAvatar bool
	DisableRegistration   bool
	EnableCaptcha         bool
	RequireSignInView     bool
	AdminName          string `binding:"OmitEmpty;AlphaDashDot;MaxSize(30)" locale:"install.admin_name"`
	AdminPasswd        string `binding:"OmitEmpty;MaxSize(255)" locale:"install.admin_password"`
	AdminConfirmPasswd string
	AdminEmail         string `binding:"OmitEmpty;MinSize(3);MaxSize(254);Include(@)" locale:"install.admin_email"`
}
func (f *Install) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
	return validate(errs, map[string]interface{}{}, f, req.Context().Value("locale"))
//    _____   ____ _________________ ___
//   /  _  \ |    |   \__    ___/   |   \
//  /  /_\  \|    |   / |    | /    ~    \
// /    |    \    |  /  |    | \    Y    /
// \____|__  /______/   |____|  \___|_  /
//         \/                         \/
type Register struct {
	UserName string `binding:"Required;AlphaDashDot;MaxSize(35)"`
	Email    string `binding:"Required;Email;MaxSize(254)"`
	Password string `binding:"Required;MaxSize(255)"`
	Retype   string
func (f *Register) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type SignIn struct {
	UserName    string `binding:"Required;MaxSize(254)"`
	Password    string `binding:"Required;MaxSize(255)"`
	LoginSource int64
	Remember    bool
func (f *SignIn) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
//   __________________________________________.___ _______    ________  _________
//  /   _____/\_   _____/\__    ___/\__    ___/|   |\      \  /  _____/ /   _____/
//  \_____  \  |    __)_   |    |     |    |   |   |/   |   \/   \  ___ \_____  \
//  /        \ |        \  |    |     |    |   |   /    |    \    \_\  \/        \
// /_______  //_______  /  |____|     |____|   |___\____|__  /\______  /_______  /
//         \/         \/                                   \/        \/        \/
type UpdateProfile struct {
	Name     string `binding:"Required;AlphaDashDot;MaxSize(35)"`
	FullName string `binding:"MaxSize(100)"`
	Website  string `binding:"Url;MaxSize(100)"`
	Location string `binding:"MaxSize(50)"`
func (f *UpdateProfile) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
const (
	AvatarLocal  string = "local"
	AvatarLookup string = "lookup"
type Avatar struct {
	Source      string
	Avatar      *multipart.FileHeader
	Gravatar    string `binding:"OmitEmpty;Email;MaxSize(254)"`
	Federavatar bool
func (f *Avatar) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type AddEmail struct {
	Email string `binding:"Required;Email;MaxSize(254)"`
func (f *AddEmail) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type ChangePassword struct {
	OldPassword string `binding:"Required;MinSize(1);MaxSize(255)"`
	Retype      string
func (f *ChangePassword) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type AddSSHKey struct {
	Title   string `binding:"Required;MaxSize(50)"`
	Content string `binding:"Required"`
func (f *AddSSHKey) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type NewAccessToken struct {
	Name string `binding:"Required"`
func (f *NewAccessToken) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
