module gogs.io/gogs

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/editorconfig/editorconfig-core-go/v2 v2.4.1
	github.com/fatih/color v1.9.0 // indirect
	github.com/go-macaron/binding v1.1.1
	github.com/go-macaron/cache v0.0.0-20190810181446-10f7c57e2196
	github.com/go-macaron/captcha v0.2.0
	github.com/go-macaron/csrf v0.0.0-20190812063352-946f6d303a4c
	github.com/go-macaron/gzip v0.0.0-20160222043647-cad1c6580a07
	github.com/go-macaron/i18n v0.6.0
	github.com/go-macaron/session v0.0.0-20190805070824-1a3cdc6f5659
	github.com/go-macaron/toolbox v0.0.0-20190813233741-94defb8383c6
	github.com/gogs/chardet v0.0.0-20150115103509-2404f7772561
	github.com/gogs/cron v0.0.0-20171120032916-9f6c956d3e14
	github.com/gogs/git-module v1.1.4
	github.com/gogs/go-gogs-client v0.0.0-20200128182646-c69cb7680fd4
	github.com/gogs/go-libravatar v0.0.0-20191106065024-33a75213d0a0
	github.com/gogs/minwinsvc v0.0.0-20170301035411-95be6356811a
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/issue9/identicon v1.0.1
	github.com/jaytaylor/html2text v0.0.0-20190408195923-01ec452cbe43
	github.com/json-iterator/go v1.1.10
	github.com/klauspost/compress v1.8.6 // indirect
	github.com/klauspost/cpuid v1.2.1 // indirect
	github.com/mattn/go-sqlite3 v2.0.3+incompatible // indirect
	github.com/mcuadros/go-version v0.0.0-20190830083331-035f6764e8d2 // indirect
	github.com/microcosm-cc/bluemonday v1.0.4
	github.com/msteinert/pam v0.0.0-20190215180659-f29b9f28d6f9
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/niklasfasching/go-org v0.1.9
	github.com/olekukonko/tablewriter v0.0.4
	github.com/pkg/errors v0.9.1
	github.com/pquerna/otp v1.2.0
	github.com/prometheus/client_golang v1.9.0
	github.com/russross/blackfriday v1.6.0
	github.com/saintfish/chardet v0.0.0-20120816061221-3af4cd4741ca // indirect
	github.com/satori/go.uuid v1.2.0
	github.com/sergi/go-diff v1.1.0
	github.com/ssor/bom v0.0.0-20170718123548-6386211fdfcf // indirect
	github.com/stretchr/testify v1.7.0
	github.com/unknwon/cae v1.0.2
	github.com/unknwon/com v1.0.1
	github.com/unknwon/i18n v0.0.0-20190805065654-5c6446a380b6
	github.com/unknwon/paginater v0.0.0-20170405233947-45e5d631308e
	github.com/urfave/cli v1.22.5
	golang.org/x/crypto v0.0.0-20201016220609-9e8e0b390897
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	golang.org/x/text v0.3.4
	gopkg.in/DATA-DOG/go-sqlmock.v2 v2.0.0-20180914054222-c19298f520d0
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/asn1-ber.v1 v1.0.0-20181015200546-f715ec2f112d // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/ini.v1 v1.62.0
	gopkg.in/ldap.v2 v2.5.1
	gopkg.in/macaron.v1 v1.4.0
	gorm.io/driver/mysql v1.0.3
	gorm.io/driver/postgres v1.0.5
	gorm.io/driver/sqlite v1.1.4
	gorm.io/driver/sqlserver v1.0.5
	gorm.io/gorm v1.20.8
	unknwon.dev/clog/v2 v2.1.2
	xorm.io/builder v0.3.6
	xorm.io/core v0.7.2
	xorm.io/xorm v0.8.0
)

// +heroku goVersion go1.15
// +heroku install ./
