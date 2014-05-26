package bin

import (
	"fmt"
	"io/ioutil"
	"strings"
)

// bindata_read reads the given file from disk. It returns an error on failure.
func bindata_read(path, name string) ([]byte, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("Error reading asset %s at %s: %v", name, path, err)
	}
	return buf, err
}

// conf_app_ini reads file data from disk. It returns an error on failure.
func conf_app_ini() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/app.ini",
		"conf/app.ini",
	)
}

// conf_content_git_bare_zip reads file data from disk. It returns an error on failure.
func conf_content_git_bare_zip() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/content/git-bare.zip",
		"conf/content/git-bare.zip",
	)
}

// conf_etc_supervisord_conf reads file data from disk. It returns an error on failure.
func conf_etc_supervisord_conf() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/etc/supervisord.conf",
		"conf/etc/supervisord.conf",
	)
}

// conf_gitignore_android reads file data from disk. It returns an error on failure.
func conf_gitignore_android() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/Android",
		"conf/gitignore/Android",
	)
}

// conf_gitignore_c reads file data from disk. It returns an error on failure.
func conf_gitignore_c() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/C",
		"conf/gitignore/C",
	)
}

// conf_gitignore_c_sharp reads file data from disk. It returns an error on failure.
func conf_gitignore_c_sharp() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/C Sharp",
		"conf/gitignore/C Sharp",
	)
}

// conf_gitignore_c_ reads file data from disk. It returns an error on failure.
func conf_gitignore_c_() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/C++",
		"conf/gitignore/C++",
	)
}

// conf_gitignore_google_go reads file data from disk. It returns an error on failure.
func conf_gitignore_google_go() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/Google Go",
		"conf/gitignore/Google Go",
	)
}

// conf_gitignore_java reads file data from disk. It returns an error on failure.
func conf_gitignore_java() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/Java",
		"conf/gitignore/Java",
	)
}

// conf_gitignore_objective_c reads file data from disk. It returns an error on failure.
func conf_gitignore_objective_c() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/Objective-C",
		"conf/gitignore/Objective-C",
	)
}

// conf_gitignore_python reads file data from disk. It returns an error on failure.
func conf_gitignore_python() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/Python",
		"conf/gitignore/Python",
	)
}

// conf_gitignore_ruby reads file data from disk. It returns an error on failure.
func conf_gitignore_ruby() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/gitignore/Ruby",
		"conf/gitignore/Ruby",
	)
}

// conf_license_affero_gpl reads file data from disk. It returns an error on failure.
func conf_license_affero_gpl() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/license/Affero GPL",
		"conf/license/Affero GPL",
	)
}

// conf_license_apache_v2_license reads file data from disk. It returns an error on failure.
func conf_license_apache_v2_license() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/license/Apache v2 License",
		"conf/license/Apache v2 License",
	)
}

// conf_license_artistic_license_2_0 reads file data from disk. It returns an error on failure.
func conf_license_artistic_license_2_0() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/license/Artistic License 2.0",
		"conf/license/Artistic License 2.0",
	)
}

// conf_license_bsd_3_clause_license reads file data from disk. It returns an error on failure.
func conf_license_bsd_3_clause_license() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/license/BSD (3-Clause) License",
		"conf/license/BSD (3-Clause) License",
	)
}

// conf_license_gpl_v2 reads file data from disk. It returns an error on failure.
func conf_license_gpl_v2() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/license/GPL v2",
		"conf/license/GPL v2",
	)
}

// conf_license_mit_license reads file data from disk. It returns an error on failure.
func conf_license_mit_license() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/license/MIT License",
		"conf/license/MIT License",
	)
}

// conf_mysql_sql reads file data from disk. It returns an error on failure.
func conf_mysql_sql() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/mysql.sql",
		"conf/mysql.sql",
	)
}

// conf_supervisor_ini reads file data from disk. It returns an error on failure.
func conf_supervisor_ini() ([]byte, error) {
	return bindata_read(
		"/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/conf/supervisor.ini",
		"conf/supervisor.ini",
	)
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		return f()
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() ([]byte, error){
	"conf/app.ini":                        conf_app_ini,
	"conf/content/git-bare.zip":           conf_content_git_bare_zip,
	"conf/etc/supervisord.conf":           conf_etc_supervisord_conf,
	"conf/gitignore/Android":              conf_gitignore_android,
	"conf/gitignore/C":                    conf_gitignore_c,
	"conf/gitignore/C Sharp":              conf_gitignore_c_sharp,
	"conf/gitignore/C++":                  conf_gitignore_c_,
	"conf/gitignore/Google Go":            conf_gitignore_google_go,
	"conf/gitignore/Java":                 conf_gitignore_java,
	"conf/gitignore/Objective-C":          conf_gitignore_objective_c,
	"conf/gitignore/Python":               conf_gitignore_python,
	"conf/gitignore/Ruby":                 conf_gitignore_ruby,
	"conf/license/Affero GPL":             conf_license_affero_gpl,
	"conf/license/Apache v2 License":      conf_license_apache_v2_license,
	"conf/license/Artistic License 2.0":   conf_license_artistic_license_2_0,
	"conf/license/BSD (3-Clause) License": conf_license_bsd_3_clause_license,
	"conf/license/GPL v2":                 conf_license_gpl_v2,
	"conf/license/MIT License":            conf_license_mit_license,
	"conf/mysql.sql":                      conf_mysql_sql,
	"conf/supervisor.ini":                 conf_supervisor_ini,
}
