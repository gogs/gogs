// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"
	"strings"

	"github.com/unknwon/com"
	"xorm.io/xorm"
	"github.com/json-iterator/go"
)

func ldapUseSSLToSecurityProtocol(x *xorm.Engine) error {
	results, err := x.Query("SELECT `id`,`cfg` FROM `login_source` WHERE `type` = 2 OR `type` = 5")
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return nil
		}
		return fmt.Errorf("select LDAP login sources: %v", err)
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for _, result := range results {
		cfg := map[string]interface{}{}
		if err = jsoniter.Unmarshal(result["cfg"], &cfg); err != nil {
			return fmt.Errorf("unmarshal JSON config: %v", err)
		}
		if com.ToStr(cfg["UseSSL"]) == "true" {
			cfg["SecurityProtocol"] = 1 // LDAPS
		}
		delete(cfg, "UseSSL")

		data, err := jsoniter.Marshal(&cfg)
		if err != nil {
			return fmt.Errorf("marshal JSON config: %v", err)
		}

		if _, err = sess.Exec("UPDATE `login_source` SET `cfg`=? WHERE `id`=?",
			string(data), com.StrTo(result["id"]).MustInt64()); err != nil {
			return fmt.Errorf("update config column: %v", err)
		}
	}
	return sess.Commit()
}
