package ldap

// import (
// 	"fmt"
// 	"testing"
// )

// var ldapServer = "ldap.itd.umich.edu"
// var ldapPort = 389
// var baseDN = "dc=umich,dc=edu"
// var filter = []string{
// 	"(cn=cis-fac)",
// 	"(&(objectclass=rfc822mailgroup)(cn=*Computer*))",
// 	"(&(objectclass=rfc822mailgroup)(cn=*Mathematics*))"}
// var attributes = []string{
// 	"cn",
// 	"description"}
// var msadsaformat = ""

// func TestLDAP(t *testing.T) {
// 	AddSource("test", ldapServer, ldapPort, baseDN, attributes, filter, msadsaformat)
// 	user, err := LoginUserLdap("xiaolunwen", "")
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}

// 	fmt.Println(user)
// }
