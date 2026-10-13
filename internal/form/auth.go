package form

import (
	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"
)

type Authentication struct {
	ID                             int64
	Type                           int    `binding:"Range(2,7)"`
	Name                           string `binding:"Required;MaxSize(30)"`
	Host                           string
	Port                           int
	BindDN                         string
	BindPassword                   string
	UserBase                       string
	UserDN                         string
	AttributeUsername              string
	AttributeName                  string
	AttributeSurname               string
	AttributeMail                  string
	AttributesInBind               bool
	Filter                         string
	AdminFilter                    string
	GroupEnabled                   bool
	GroupDN                        string
	GroupFilter                    string
	GroupMemberUID                 string
	UserUID                        string
	IsActive                       bool
	IsDefault                      bool
	SMTPAuth                       string
	SMTPHost                       string
	SMTPPort                       int
	AllowedDomains                 string
	SecurityProtocol               int `binding:"Range(0,2)"`
	TLS                            bool
	SkipVerify                     bool
	PAMServiceName                 string
	GitHubAPIEndpoint              string `form:"github_api_endpoint" binding:"Url"`
	SAMLIDPMetadataURL             string `form:"saml_idp_metadata_url"`
	SAMLServiceProviderIssuer      string `form:"saml_service_provider_issuer"`
	SAMLServiceProviderCertificate string `form:"saml_service_provider_certificate"`
	SAMLServiceProviderPrivateKey  string `form:"saml_service_provider_private_key"`
	SAMLLoginAttribute             string `form:"saml_login_attribute"`
	SAMLUsernameAttribute          string `form:"saml_username_attribute"`
	SAMLEmailAttribute             string `form:"saml_email_attribute"`
	SAMLFullNameAttribute          string `form:"saml_full_name_attribute"`
}

func (f *Authentication) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
