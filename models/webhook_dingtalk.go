// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"

	api "github.com/gogits/go-gogs-client"
)

//Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkActionCard struct {
	Title          string `json:"title"`
	Text           string `json:"text"`
	HideAvatar     string `json:"hideAvatar"`
	BtnOrientation string `json:"btnOrientation"`
	SingleTitle    string `json:"singleTitle"`
	SingleURL      string `json:"singleURL"`
}

//Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkAtObject struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

//Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkPayload struct {
	MsgType    string             `json:"msgtype"`
	At         DingtalkAtObject   `json:"at"`
	ActionCard DingtalkActionCard `json:"actionCard"`
}

func (p *DingtalkPayload) JSONPayload() ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

//TODO: add content
func GetDingtalkPayload(p api.Payloader, event HookEventType) (payload *DingtalkPayload, err error) {
	return nil, nil
}
