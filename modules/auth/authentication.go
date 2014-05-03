package auth

type AuthenticationForm struct {
	Type       int    `form:"type"`
	Name       string `form:"name" binding:"MaxSize(50)"`
	Domain     string `form:"domain"`
	Host       string `form:"host"`
	Port       int    `form:"port"`
	BaseDN     string `form:"base_dn"`
	Attributes string `form:"attributes"`
	Filter     string `form:"filter"`
	MsAdSA     string `form:"ms_ad_sa"`
}
