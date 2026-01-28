package form

import (
	"github.com/flamego/binding"
)
type CreateOrg struct {
	OrgName string `binding:"Required;AlphaDashDot;MaxSize(35)" locale:"org.org_name_holder"`
}
func (f *CreateOrg) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
	return validate(errs, map[string]interface{}{}, f, req.Context().Value("locale"))
type UpdateOrgSetting struct {
	Name            string `binding:"Required;AlphaDashDot;MaxSize(35)" locale:"org.org_name_holder"`
	FullName        string `binding:"MaxSize(100)"`
	Description     string `binding:"MaxSize(255)"`
	Website         string `binding:"Url;MaxSize(100)"`
	Location        string `binding:"MaxSize(50)"`
	MaxRepoCreation int
func (f *UpdateOrgSetting) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
type CreateTeam struct {
	TeamName    string `binding:"Required;AlphaDashDot;MaxSize(30)"`
	Description string `binding:"MaxSize(255)"`
	Permission  string
func (f *CreateTeam) Validate(ctx http.ResponseWriter, req *http.Request, errs binding.Errors) binding.Errors {
