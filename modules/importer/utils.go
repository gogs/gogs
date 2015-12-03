// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"reflect"
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/httplib"
)

func fetchURL(api string, baseUrl *url.URL, token string) ([]byte, error) {
	baseUrl.Path = api
	log.Printf("Fetching: %s", baseUrl)
	req := httplib.Get(baseUrl.String()).Header("PRIVATE-TOKEN", token)
	resp, err := req.Response()
	if err != nil {
		log.Fatalf("Cannot connect: %s", err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		log.Fatalf("Unexpected server reponse: %s", resp.Status)
		return nil, errors.New(resp.Status)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Cannot read: %s", err)
		return nil, err
	}

	return body, nil
}

func fetchObject(api string, baseUrl *url.URL, token string, out interface{}) error {
	body, err := fetchURL(api, baseUrl, token)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, out)
	if err != nil {
		log.Fatalf("Cannot decode: %s", err)
		return err
	}
	return nil
}

func fetchObjects(api string, baseUrl *url.URL, token string, out interface{}) error {
	outSliceValue := reflect.ValueOf(out).Elem()
	slicePtrValue := reflect.New(outSliceValue.Type())

	if strings.Contains(api, "?") {
		api += "&"
	} else {
		api += "?"
	}

	for page := 1; ; page++ {
		body, err := fetchURL(fmt.Sprintf("%sper_page=100&page=%d", api, page), baseUrl, token)
		if err != nil {
			return err
		}
		err = json.Unmarshal(body, slicePtrValue.Interface())
		if err != nil {
			log.Fatalf("Cannot decode: %s", err)
			return err
		}
		outSliceValue.Set(reflect.AppendSlice(outSliceValue, slicePtrValue.Elem()))
		if slicePtrValue.Elem().Len() < 100 {
			break
		}
	}

	return nil
}

func getOrgTeam(org *models.User, cached **models.Team, name string, access models.AccessMode) (*models.Team, error) {
	if *cached != nil {
		return *cached, nil
	}
	if team, err := org.GetTeam(name); err != nil && err != models.ErrTeamNotExist {
		log.Fatalf("Cannot get %s team for org: %s, %s", name, org.Name, err)
		return nil, err
	} else if team != nil {
		*cached = team
	} else {
		team := &models.Team{
			OrgID:     org.Id,
			Name:      name,
			Authorize: access,
		}
		if err := models.NewTeam(team); err != nil {
			log.Fatalf("Cannot create %s team for org: %s, %s", name, org.Name, err)
			return nil, err
		}
		*cached = team
	}
	return *cached, nil
}
