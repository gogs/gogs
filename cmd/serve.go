// Copyright 2013 gopm authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package cmd

// import (
// 	"errors"
// 	"fmt"
// 	"github.com/Unknwon/com"
// 	"github.com/gpmgo/gopm/doc"
// 	"github.com/syndtr/goleveldb/leveldb"
// 	"github.com/syndtr/goleveldb/leveldb/opt"
// 	"io/ioutil"
// 	"net/http"
// 	"net/url"
// 	"os"
// 	"os/exec"
// 	"path/filepath"
// 	"strconv"
// 	"strings"
// 	"time"
// )

// var (
// 	dbDir = "~/.gopm/db"
// )

// const (
// 	STOP = iota
// 	LOCALRUN
// 	RUNNING
// )

// var CmdServe = &Command{
// 	UsageLine: "serve [:port]",
// 	Short:     "serve for package search",
// 	Long: `
// 	serve provide a web service to search packages, download packages

// The serve flags are:

// 	-l
// 		only service for localhost ip
// `,
// }

// func init() {
// 	CmdServe.Run = runServe
// 	CmdServe.Flags = map[string]bool{
// 		"-l": false,
// 	}
// }

// func printServePrompt(flag string) {
// 	switch flag {
// 	case "-l":
// 		com.ColorLog("[INFO] You enabled start a service only localhost.\n")
// 	}
// }

// // Not implemented
// func autoPort() string {
// 	return "8991"
// }

// func exePath() (string, error) {
// 	file, err := exec.LookPath(os.Args[0])
// 	if err != nil {
// 		return "", err
// 	}

// 	return filepath.Abs(file)
// }

// // search packages
// func runServe(cmd *Command, args []string) {
// 	// Check flags.
// 	num := checkFlags(cmd.Flags, args, printServePrompt)
// 	if num == -1 {
// 		return
// 	}
// 	args = args[num:]

// 	var listen string
// 	var port string
// 	if cmd.Flags["-l"] {
// 		listen += "127.0.0.1"
// 		port = autoPort()
// 	} else {
// 		listen += "0.0.0.0"
// 		port = "8991"
// 	}

// 	// Check length of arguments.
// 	if len(args) >= 1 {
// 		port = args[0]
// 	}

// 	err := startService(listen, port)
// 	if err != nil {
// 		com.ColorLog("[ERRO] %v\n", err)
// 	}
// }

// func splitWord(word string, res *map[string]bool) {
// 	for i, _ := range word {
// 		for j, _ := range word[i:] {
// 			w := word[i : i+j+1]
// 			(*res)[w] = true
// 		}
// 	}
// 	return
// }

// func splitPkgName(pkgName string) (res map[string]bool) {
// 	//var src string
// 	ps := strings.Split(pkgName, "/")
// 	if len(ps) > 1 {
// 		ps = ps[1:]
// 	}

// 	res = make(map[string]bool)
// 	res[strings.Join(ps, "/")] = true
// 	for _, w := range ps {
// 		splitWord(w, &res)
// 	}
// 	return
// }

// func splitSynopsis(synopsis string) map[string]bool {
// 	res := make(map[string]bool)
// 	ss := strings.Fields(synopsis)
// 	for _, s := range ss {
// 		res[s] = true
// 	}
// 	return res
// }

// var (
// 	ro *opt.ReadOptions  = &opt.ReadOptions{}
// 	wo *opt.WriteOptions = &opt.WriteOptions{}
// )

// func dbGet(key string) (string, error) {
// 	v, err := db.Get([]byte(key), ro)
// 	return string(v), err
// }

// func dbPut(key string, value string) error {
// 	//fmt.Println("put ", key, ": ", value)
// 	return db.Put([]byte(key), []byte(value), wo)
// }

// func batchPut(batch *leveldb.Batch, key string, value string) error {
// 	//fmt.Println("put ", key, ": ", value)
// 	batch.Put([]byte(key), []byte(value))
// 	return nil
// }

// func getServeHost() string {
// 	return "localhost"
// }

// func getServePort() string {
// 	return "8991"
// }

// // for exernal of serve to add node to db
// func saveNode(nod *doc.Node) error {
// 	urlPath := fmt.Sprintf("http://%v:%v/add", getServeHost(), getServePort())
// 	resp, err := http.PostForm(urlPath,
// 		url.Values{"importPath": {nod.ImportPath},
// 			"synopsis":    {nod.Synopsis},
// 			"downloadURL": {nod.DownloadURL},
// 			"isGetDeps":   {strconv.FormatBool(nod.IsGetDeps)},
// 			"type":        {nod.Type},
// 			"value":       {nod.Value}})

// 	if err != nil {
// 		com.ColorLog("[ERRO] Fail to save node[ %s ]\n", err)
// 		return err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode == 200 {
// 		return nil
// 	}
// 	return errors.New("save node failed with " + resp.Status)
// }

// // for inetrnal of serve to add node to db
// func addNode(nod *doc.Node) error {
// 	batch := new(leveldb.Batch)
// 	strLastId, err := dbGet("lastId")
// 	if err != nil {
// 		if err == leveldb.ErrNotFound {
// 			strLastId = "0"
// 			err = batchPut(batch, "lastId", strLastId)
// 		} else {
// 			return err
// 		}
// 	}
// 	if err != nil {
// 		return err
// 	}

// 	lastId, err := strconv.ParseInt(strLastId, 0, 64)
// 	if err != nil {
// 		return err
// 	}

// 	nodKey := fmt.Sprintf("index:%v", nod.ImportPath)

// 	id, err := dbGet(nodKey)
// 	if err != nil {
// 		if err == leveldb.ErrNotFound {
// 			id = fmt.Sprintf("%v", lastId+1)
// 			err = batchPut(batch, "lastId", id)
// 			if err == nil {
// 				err = batchPut(batch, nodKey, id)
// 			}
// 			if err == nil {
// 				err = batchPut(batch, "pkg:"+id, nod.ImportPath)
// 			}
// 			if err == nil {
// 				err = batchPut(batch, "desc:"+id, nod.Synopsis)
// 			}
// 			if err == nil {
// 				err = batchPut(batch, "down:"+id, nod.DownloadURL)
// 			}
// 			if err == nil {
// 				err = batchPut(batch, "deps:"+id, strconv.FormatBool(nod.IsGetDeps))
// 			}

// 			// save totals
// 			total, err := dbGet("total")
// 			if err != nil {
// 				if err == leveldb.ErrNotFound {
// 					total = "1"
// 				} else {
// 					return err
// 				}
// 			} else {
// 				totalInt, err := strconv.ParseInt(total, 0, 64)
// 				if err != nil {
// 					return err
// 				}
// 				totalInt = totalInt + 1
// 				total = fmt.Sprintf("%v", totalInt)
// 			}

// 			err = batchPut(batch, "total", total)
// 		} else {
// 			return err
// 		}
// 	}

// 	if err != nil {
// 		return err
// 	}

// 	// save vers
// 	vers, err := dbGet("ver:" + id)
// 	needSplit := (err == leveldb.ErrNotFound)
// 	if err != nil {
// 		if err != leveldb.ErrNotFound {
// 			return err
// 		}
// 	} else {
// 		return nil
// 	}

// 	if vers == "" {
// 		//fmt.Println(nod)
// 		vers = nod.VerString()
// 	} else {
// 		if !strings.Contains(vers, nod.VerString()) {
// 			vers = vers + "," + nod.VerString()
// 		} else {
// 			return nil
// 		}
// 	}

// 	err = batchPut(batch, "ver:"+id, vers)
// 	if err != nil {
// 		return err
// 	}

// 	if !needSplit {
// 		return nil
// 	}

// 	// indexing package name
// 	keys := splitPkgName(nod.ImportPath)
// 	for key, _ := range keys {
// 		err = batchPut(batch, fmt.Sprintf("key:%v:%v", strings.ToLower(key), id), "")
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	if nod.Synopsis != "" {
// 		fields := splitSynopsis(nod.Synopsis)
// 		for field, _ := range fields {
// 			err = batchPut(batch, fmt.Sprintf("key:%v:%v", strings.ToLower(field), id), "")
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	return db.Write(batch, wo)
// }

// func rmPkg(nod *doc.Node) {

// }

// var db *leveldb.DB

// // service should be run
// func AutoRun() error {
// 	s, _, _ := runningStatus()
// 	if s == STOP {
// 		// current path
// 		curPath, err := os.Getwd()
// 		if err != nil {
// 			return err
// 		}

// 		attr := &os.ProcAttr{
// 			Dir: curPath,
// 			Env: os.Environ(),
// 			//Files: []*os.File{nil, nil, nil},
// 			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
// 		}

// 		p, err := exePath()
// 		if err != nil {
// 			return err
// 		}

// 		//com.ColorLog("[INFO] now is starting search daemon ...\n")
// 		_, err = os.StartProcess(p, []string{"gopm", "serve", "-l"}, attr)
// 		if err != nil {
// 			return err
// 		}
// 		time.Sleep(time.Second)
// 	}
// 	return nil
// }

// func runningStatus() (int, int, int) {
// 	pFile, err := getPidPath()
// 	if err != nil {
// 		return STOP, 0, 0
// 	}

// 	contentByte, err := ioutil.ReadFile(pFile)
// 	if err != nil {
// 		return STOP, 0, 0
// 	}
// 	content := string(contentByte)
// 	if len(content) < 0 || !strings.Contains(content, ",") {
// 		return STOP, 0, 0
// 	}
// 	cs := strings.Split(string(content), ",")
// 	if len(cs) != 3 {
// 		return STOP, 0, 0
// 	}
// 	status, err := strconv.Atoi(cs[0])
// 	if err != nil {
// 		return STOP, 0, 0
// 	}
// 	if status < STOP || status > RUNNING {
// 		return STOP, 0, 0
// 	}
// 	pid, err := strconv.Atoi(cs[1])
// 	if err != nil {
// 		return STOP, 0, 0
// 	}

// 	_, err = os.FindProcess(pid)
// 	if err != nil {
// 		return STOP, 0, 0
// 	}

// 	port, err := strconv.Atoi(cs[2])
// 	if err != nil {
// 		return STOP, 0, 0
// 	}

// 	return status, pid, port
// }

// func getPidPath() (string, error) {
// 	homeDir, err := com.HomeDir()
// 	if err != nil {
// 		return "", err
// 	}

// 	pFile := strings.Replace("~/.gopm/var/", "~", homeDir, -1)
// 	os.MkdirAll(pFile, os.ModePerm)
// 	return pFile + "pid", nil
// }

// func startService(listen, port string) error {
// 	homeDir, err := com.HomeDir()
// 	if err != nil {
// 		return err
// 	}

// 	pFile, err := getPidPath()
// 	if err != nil {
// 		return err
// 	}

// 	f, err := os.OpenFile(pFile, os.O_RDWR|os.O_CREATE, 0700)
// 	if err != nil {
// 		return err
// 	}
// 	defer f.Close()
// 	_, err = f.WriteString(fmt.Sprintf("%v,%v,%v", RUNNING, os.Getpid(), port))
// 	if err != nil {
// 		return err
// 	}

// 	dbDir = strings.Replace(dbDir, "~", homeDir, -1)

// 	db, err = leveldb.OpenFile(dbDir, nil)
// 	if err != nil {
// 		return err
// 	}
// 	defer db.Close()

// 	// these handlers should only access by localhost
// 	http.HandleFunc("/add", addHandler)
// 	http.HandleFunc("/rm", rmHandler)

// 	// these handlers can be accessed according listen's ip
// 	http.HandleFunc("/search", searchHandler)
// 	http.HandleFunc("/searche", searcheHandler)
// 	http.ListenAndServe(listen+":"+port, nil)
// 	return nil
// }

// func searchHandler(w http.ResponseWriter, r *http.Request) {
// 	r.ParseForm()
// 	ids := make(map[string]bool)
// 	for key, _ := range r.Form {
// 		iter := db.NewIterator(ro)
// 		rkey := fmt.Sprintf("key:%v:", strings.ToLower(key))
// 		if iter.Seek([]byte(rkey)) {
// 			k := iter.Key()
// 			if !strings.HasPrefix(string(k), rkey) {
// 				break
// 			} else {
// 				ids[string(k)] = true
// 			}
// 		}
// 		for iter.Next() {
// 			k := iter.Key()
// 			if !strings.HasPrefix(string(k), rkey) {
// 				break
// 			}
// 			ids[string(k)] = true
// 		}
// 	}

// 	pkgs := make([]string, 0)

// 	for id, _ := range ids {
// 		idkeys := strings.SplitN(id, ":", -1)
// 		rId := idkeys[len(idkeys)-1]
// 		//fmt.Println(rId)
// 		pkg, err := dbGet(fmt.Sprintf("pkg:%v", rId))
// 		if err != nil {
// 			com.ColorLog(err.Error())
// 			continue
// 		}
// 		desc, err := dbGet(fmt.Sprintf("desc:%v", rId))
// 		if err != nil {
// 			com.ColorLog(err.Error())
// 			continue
// 		}
// 		pkgs = append(pkgs, fmt.Sprintf(`{"pkg":"%v", "desc":"%v"}`, pkg, desc))
// 	}

// 	w.Write([]byte("[" + strings.Join(pkgs, ", ") + "]"))
// }

// func searcheHandler(w http.ResponseWriter, r *http.Request) {
// 	//if r.Method == "POST" {
// 	r.ParseForm()
// 	pkgs := make([]string, 0)
// 	for key, _ := range r.Form {
// 		rId, err := dbGet("index:" + key)
// 		if err != nil {
// 			com.ColorLog(err.Error())
// 			continue
// 		}

// 		desc, err := dbGet(fmt.Sprintf("desc:%v", rId))
// 		if err != nil {
// 			com.ColorLog(err.Error())
// 			continue
// 		}

// 		pkgs = append(pkgs, fmt.Sprintf(`{"pkg":"%v", "desc":"%v"}`, key, desc))
// 	}

// 	w.Write([]byte("[" + strings.Join(pkgs, ", ") + "]"))
// 	//}
// }

// func addHandler(w http.ResponseWriter, r *http.Request) {
// 	//if r.Method == "POST" {
// 	r.ParseForm()

// 	nod := new(doc.Node)
// 	nod.ImportPath = r.FormValue("importPath")
// 	nod.Synopsis = r.FormValue("synopsis")
// 	nod.DownloadURL = r.FormValue("downloadURL")
// 	isGetDeps, err := strconv.ParseBool(r.FormValue("isGetDeps"))
// 	if err != nil {
// 		com.ColorLog("[ERRO] SEVER: Cannot get deps")
// 	}
// 	nod.IsGetDeps = isGetDeps
// 	nod.Type = r.FormValue("type")
// 	nod.Value = r.FormValue("value")

// 	err = addNode(nod)
// 	if err != nil {
// 		com.ColorLog("[ERRO] SEVER: Cannot add node[ %s ]\n", err)
// 	}
// 	//}
// }

// func rmHandler(w http.ResponseWriter, r *http.Request) {

// }
