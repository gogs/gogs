// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
)

// MailResendCacheKey returns key used for cache mail resend.
func (u *User) MailResendCacheKey() string {
	return fmt.Sprintf("MailResend_%d", u.ID)
}

// TwoFactorCacheKey returns key used for cache two factor passcode.
// e.g. TwoFactor_1_012664
func (u *User) TwoFactorCacheKey(passcode string) string {
	return fmt.Sprintf("TwoFactor_%d_%s", u.ID, passcode)
}
