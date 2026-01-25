package form

import (
	"github.com/flamego/binding"
)
type AdminCrateUser struct {
	LoginType  string `binding:"Required"`
	LoginName  string
	UserName   string `binding:"Required;AlphaDashDot;MaxSize(35)"`
	Email      string `binding:"Required;Email;MaxSize(254)"`
	Password   string `binding:"MaxSize(255)"`
	SendNotify bool
}
func (f *AdminCrateUser) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
	return validate(errs, map[string]interface{}{}, f, req.Context().Value("locale"))
type AdminEditUser struct {
	LoginType        string `binding:"Required"`
	LoginName        string
	FullName         string `binding:"MaxSize(100)"`
	Email            string `binding:"Required;Email;MaxSize(254)"`
	Password         string `binding:"MaxSize(255)"`
	Website          string `binding:"MaxSize(50)"`
	Location         string `binding:"MaxSize(50)"`
	MaxRepoCreation  int
	Active           bool
	Admin            bool
	AllowGitHook     bool
	AllowImportLocal bool
	ProhibitLogin    bool
func (f *AdminEditUser) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
