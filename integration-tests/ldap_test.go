package integrationtests

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"

	goldap "github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/database"
)

// ldapDirectory is a branch of the LDAP directory that the LDAP_* environment
// variables point to, which only one test seeds entries in, so tests that run
// at the same time never find each other's entries.
type ldapDirectory struct {
	t    *testing.T
	conn *goldap.Conn
	host string
	port int
	// base is the DN of the branch.
	base string
}

// newLDAPDirectory creates a branch in the LDAP directory and deletes it when
// the test ends. It skips the test when LDAP_HOST is not set.
func newLDAPDirectory(t *testing.T) *ldapDirectory {
	t.Helper()

	host := os.Getenv("LDAP_HOST")
	if host == "" {
		t.Skip("Skipping LDAP tests, set LDAP_HOST, LDAP_PORT, LDAP_BASE_DN, LDAP_ADMIN_DN and LDAP_ADMIN_PASSWORD to run them")
	}
	port, err := strconv.Atoi(os.Getenv("LDAP_PORT"))
	require.NoError(t, err, "LDAP_PORT")

	conn, err := goldap.DialURL("ldap://" + net.JoinHostPort(host, strconv.Itoa(port)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.Bind(os.Getenv("LDAP_ADMIN_DN"), os.Getenv("LDAP_ADMIN_PASSWORD")))

	ou := UniqueName("test-")
	d := &ldapDirectory{
		t:    t,
		conn: conn,
		host: host,
		port: port,
		base: "ou=" + ou + "," + os.Getenv("LDAP_BASE_DN"),
	}
	d.add(d.base, map[string][]string{
		"objectClass": {"organizationalUnit"},
		"ou":          {ou},
	})
	t.Cleanup(func() {
		children, err := conn.Search(goldap.NewSearchRequest(
			d.base, goldap.ScopeSingleLevel, goldap.NeverDerefAliases, 0, 0, false,
			"(objectClass=*)", []string{"dn"}, nil,
		))
		if err != nil {
			t.Logf("Failed to list LDAP entries under %q: %v", d.base, err)
			return
		}
		for _, e := range append(children.Entries, &goldap.Entry{DN: d.base}) {
			if err = conn.Del(goldap.NewDelRequest(e.DN, nil)); err != nil {
				t.Logf("Failed to delete LDAP entry %q: %v", e.DN, err)
			}
		}
	})
	return d
}

func (d *ldapDirectory) add(dn string, attrs map[string][]string) {
	d.t.Helper()

	req := goldap.NewAddRequest(dn, nil)
	for k, v := range attrs {
		req.Attribute(k, v)
	}
	require.NoError(d.t, d.conn.Add(req), "add %q", dn)
}

// ldapUser is an entry of a person in the LDAP directory.
type ldapUser struct {
	dn       string
	uid      string
	password string
	mail     string
}

// addUser adds a person named "Ada Lovelace" with a unique uid. The employee
// type is set unless it is empty, which the tests filter on, e.g., to find
// admins.
func (d *ldapDirectory) addUser(employeeType string) *ldapUser {
	d.t.Helper()

	uid := UniqueName("ldap-")
	u := &ldapUser{
		dn:       "uid=" + uid + "," + d.base,
		uid:      uid,
		password: UniqueName("password-"),
		mail:     uid + "@example.org",
	}
	attrs := map[string][]string{
		"objectClass":  {"inetOrgPerson"},
		"uid":          {uid},
		"cn":           {"Ada Lovelace"},
		"givenName":    {"Ada"},
		"sn":           {"Lovelace"},
		"mail":         {u.mail},
		"userPassword": {u.password},
	}
	if employeeType != "" {
		attrs["employeeType"] = []string{employeeType}
	}
	d.add(u.dn, attrs)
	return u
}

// addGroup adds a POSIX group of the name that lists the uids of the members.
func (d *ldapDirectory) addGroup(name string, members ...*ldapUser) {
	d.t.Helper()

	uids := make([]string, 0, len(members))
	for _, m := range members {
		uids = append(uids, m.uid)
	}
	d.add("cn="+name+","+d.base, map[string][]string{
		"objectClass": {"posixGroup"},
		"cn":          {name},
		"gidNumber":   {"1000"},
		"memberUid":   uids,
	})
}

// setPassword fails t, which may be a subtest of the one the directory belongs
// to.
func (d *ldapDirectory) setPassword(t *testing.T, u *ldapUser, password string) {
	t.Helper()

	req := goldap.NewModifyRequest(u.dn, nil)
	req.Replace("userPassword", []string{password})
	require.NoError(t, d.conn.Modify(req))
	u.password = password
}

// newSource creates an active authentication source of the type, LDAP via bind
// DN or simple auth, that talks to the directory and maps the attributes of
// the entries addUser adds. The config sets what differs between tests, e.g.,
// filters.
func (d *ldapDirectory) newSource(typ auth.Type, cfg ldap.Config) *database.LoginSource {
	d.t.Helper()

	cfg.Host = d.host
	cfg.Port = d.port
	cfg.AttributeUsername = "uid"
	cfg.AttributeName = "givenName"
	cfg.AttributeSurname = "sn"
	cfg.AttributeMail = "mail"
	source, err := database.Handle.LoginSources().Create(d.t.Context(), database.CreateLoginSourceOptions{
		Type:      typ,
		Name:      UniqueName("ldap-"),
		Activated: true,
		Config:    &cfg,
	})
	require.NoError(d.t, err)
	// A source cannot be deleted while accounts are bound to it, e.g., the ones
	// created by signing in through it.
	d.t.Cleanup(func() {
		ctx := context.Background()
		var userIDs []int64
		err := gormDB.Model(new(database.User)).Where("login_source = ?", source.ID).Pluck("id", &userIDs).Error
		if err != nil {
			d.t.Logf("Failed to list accounts of authentication source %d: %v", source.ID, err)
			return
		}
		for _, id := range userIDs {
			if err = database.Handle.Users().DeleteByID(ctx, id, true); err != nil {
				d.t.Logf("Failed to delete account %d: %v", id, err)
			}
		}
		if err = database.Handle.LoginSources().DeleteByID(ctx, source.ID); err != nil {
			d.t.Logf("Failed to delete authentication source %d: %v", source.ID, err)
		}
	})
	return source
}

// signInWithSource signs in with the login and the password through the
// authentication source, and returns the client with the status code and the
// response body.
func signInWithSource(t *testing.T, source *database.LoginSource, login, password string) (*client, int, []byte) {
	t.Helper()

	c := newClient(t)
	status, body := c.do(http.MethodPost, "/api/web/user/sign-in", map[string]any{
		"username":    login,
		"password":    password,
		"loginSource": source.ID,
	})
	return c, status, body
}

// assertLDAPAccount asserts that the person signed in through the source has an
// account with what the directory has about them.
func assertLDAPAccount(t *testing.T, source *database.LoginSource, u *ldapUser, isAdmin bool) {
	t.Helper()

	account, err := database.Handle.Users().GetByUsername(t.Context(), u.uid)
	require.NoError(t, err)
	assert.Equal(t, source.ID, account.LoginSource)
	assert.Equal(t, u.uid, account.LoginName)
	assert.Equal(t, "Ada Lovelace", account.FullName)
	assert.Equal(t, u.mail, account.Email)
	assert.True(t, account.IsActive)
	assert.Equal(t, isAdmin, account.IsAdmin)
}

func assertNoAccount(t *testing.T, name string) {
	t.Helper()

	_, err := database.Handle.Users().GetByUsername(t.Context(), name)
	assert.True(t, database.IsErrUserNotExist(err), "account %q created: %v", name, err)
}

func TestLDAPBindDN(t *testing.T) {
	t.Parallel()

	d := newLDAPDirectory(t)
	source := d.newSource(auth.LDAP, ldap.Config{
		BindDN:       os.Getenv("LDAP_ADMIN_DN"),
		BindPassword: os.Getenv("LDAP_ADMIN_PASSWORD"),
		UserBase:     d.base,
		Filter:       "(&(objectClass=inetOrgPerson)(uid=%s))",
		AdminFilter:  "(employeeType=admin)",
	})
	person := d.addUser("")
	admin := d.addUser("admin")

	t.Run("first sign-in creates the account", func(t *testing.T) {
		c, status, body := signInWithSource(t, source, person.uid, person.password)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, person.uid, c.username())
		assertLDAPAccount(t, source, person, false)

		c, status, body = signInWithSource(t, source, admin.uid, admin.password)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, admin.uid, c.username())
		assertLDAPAccount(t, source, admin, true)
	})

	t.Run("wrong password or unknown person", func(t *testing.T) {
		c, status, body := signInWithSource(t, source, person.uid, "wrong-"+person.password)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Empty(t, c.username())

		name := UniqueName("ldap-")
		_, status, body = signInWithSource(t, source, name, person.password)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assertNoAccount(t, name)
	})

	t.Run("password changed in the directory", func(t *testing.T) {
		oldPassword := person.password
		d.setPassword(t, person, UniqueName("password-"))

		_, status, body := signInWithSource(t, source, person.uid, oldPassword)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		c, status, body := signInWithSource(t, source, person.uid, person.password)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, person.uid, c.username())

		// Basic auth, e.g., to create an access token, checks the password with the
		// directory too.
		tokensPath := "/api/v1/users/" + person.uid + "/tokens"
		status, body = apiV1BasicClient(t, person.uid, person.password).do(http.MethodGet, tokensPath, nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
		status, body = apiV1BasicClient(t, person.uid, oldPassword).do(http.MethodGet, tokensPath, nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})
}

func TestLDAPSimpleAuth(t *testing.T) {
	t.Parallel()

	d := newLDAPDirectory(t)
	source := d.newSource(auth.DLDAP, ldap.Config{
		UserDN:      "uid=%s," + d.base,
		Filter:      "(&(objectClass=inetOrgPerson)(uid=%s)(!(employeeType=blocked)))",
		AdminFilter: "(employeeType=admin)",
	})
	person := d.addUser("")
	admin := d.addUser("admin")
	blocked := d.addUser("blocked")

	c, status, body := signInWithSource(t, source, person.uid, person.password)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Equal(t, person.uid, c.username())
	assertLDAPAccount(t, source, person, false)

	_, status, body = signInWithSource(t, source, admin.uid, admin.password)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assertLDAPAccount(t, source, admin, true)

	_, status, body = signInWithSource(t, source, person.uid, "wrong-"+person.password)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)

	// The person binds with the right password, but the filter rules them out.
	_, status, body = signInWithSource(t, source, blocked.uid, blocked.password)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	assertNoAccount(t, blocked.uid)
}

func TestLDAPGroupMembership(t *testing.T) {
	t.Parallel()

	d := newLDAPDirectory(t)
	member := d.addUser("")
	nonMember := d.addUser("")
	group := UniqueName("gogs-users-")
	d.addGroup(group, member)
	// Another group lists the person who is not a member, which the group
	// filter leaves out.
	d.addGroup(UniqueName("others-"), nonMember)
	source := d.newSource(auth.LDAP, ldap.Config{
		BindDN:       os.Getenv("LDAP_ADMIN_DN"),
		BindPassword: os.Getenv("LDAP_ADMIN_PASSWORD"),
		UserBase:     d.base,
		Filter:       "(&(objectClass=inetOrgPerson)(uid=%s))",
		// Groups are searched as the bind DN, which reads them under any access
		// control of the directory.
		AttributesInBind: true,
		GroupEnabled:     true,
		GroupDN:          d.base,
		GroupFilter:      "(&(objectClass=posixGroup)(cn=" + group + "))",
		GroupMemberUID:   "memberUid",
		UserUID:          "uid",
	})

	c, status, body := signInWithSource(t, source, member.uid, member.password)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Equal(t, member.uid, c.username())
	assertLDAPAccount(t, source, member, false)

	_, status, body = signInWithSource(t, source, nonMember.uid, nonMember.password)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	assertNoAccount(t, nonMember.uid)

	// The password is still checked after the attributes are read as the bind
	// DN.
	_, status, body = signInWithSource(t, source, member.uid, "wrong-"+member.password)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
}
