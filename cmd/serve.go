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

import (
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/gpmgo/gopm/doc"
)

var (
	dbDir = "~/.gopm/db"
)

const (
	STOP = iota
	LOCALRUN
	RUNNING
)

var CmdServe = &Command{
	UsageLine: "serve [:port]",
	Short:     "serve for package search",
	Long: `
	serve provide a web service to search packages, download packages 

The serve flags are:

	-l
		only service for localhost ip
`,
}

func init() {
	CmdServe.Run = runServe
	CmdServe.Flags = map[string]bool{
		"-l": false,
	}
}

func printServePrompt(flag string) {
	switch flag {
	case "-l":
		doc.ColorLog("[INFO] You enabled start a service only localhost.\n")
	}
}

// Not implemented
func autoPort() string {
	return "8991"
}

// search packages
func runServe(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, args, printServePrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	var listen string
	var port string
	if cmd.Flags["-l"] {
		listen += "127.0.0.1:"
		port = autoPort()
	} else {
		listen += "0.0.0.0:"
		port = "8991"
	}

	// Check length of arguments.
	if len(args) >= 1 {
		port = args[0]
	}

	startService(listen + port)
}

func splitWord(word string, res *map[string]bool) {
	for i, _ := range word {
		for j, _ := range word[i:] {
			w := word[i : i+j+1]
			(*res)[w] = true
		}
	}
	return
}

func splitPkgName(pkgName string) (res map[string]bool) {
	//var src string
	ps := strings.Split(pkgName, "/")
	if len(ps) > 1 {
		ps = ps[1:]
	}

	res = make(map[string]bool, 0)
	res[strings.Join(ps, "/")] = true
	for _, w := range ps {
		splitWord(w, &res)
	}
	return
}

var (
	ro *opt.ReadOptions  = &opt.ReadOptions{}
	wo *opt.WriteOptions = &opt.WriteOptions{}
)

func dbGet(key string) (string, error) {
	v, err := db.Get([]byte(key), ro)
	return string(v), err
}

func dbPut(key string, value string) error {
	fmt.Println("put ", key, ": ", value)
	return db.Put([]byte(key), []byte(value), wo)
}

func batchPut(batch *leveldb.Batch, key string, value string) error {
	fmt.Println("put ", key, ": ", value)
	batch.Put([]byte(key), []byte(value))
	return nil
}

func addNode(nod *doc.Node) error {
	batch := new(leveldb.Batch)
	strLastId, err := dbGet("lastId")
	if err != nil {
		if err == errors.ErrNotFound {
			strLastId = "0"
			err = batchPut(batch, "lastId", strLastId)
		} else {
			return err
		}
	}
	if err != nil {
		return err
	}

	fmt.Println("last id is ", strLastId)

	lastId, err := strconv.ParseInt(strLastId, 0, 64)
	if err != nil {
		return err
	}

	nodKey := fmt.Sprintf("index:%v", nod.ImportPath)

	id, err := dbGet(nodKey)
	if err != nil {
		if err == errors.ErrNotFound {
			id = fmt.Sprintf("%v", lastId+1)
			fmt.Println(id)
			err = batchPut(batch, "lastId", id)
			if err == nil {
				err = batchPut(batch, nodKey, id)
			}
			if err == nil {
				err = batchPut(batch, "pkg:"+id, nod.ImportPath)
			}
			total, err := dbGet("total")
			if err != nil {
				if err == errors.ErrNotFound {
					total = "1"
				} else {
					return err
				}
			} else {
				totalInt, err := strconv.ParseInt(total, 0, 64)
				if err != nil {
					return err
				}
				totalInt = totalInt + 1
				total = fmt.Sprintf("%v", totalInt)
			}

			err = batchPut(batch, "total", total)
		} else {
			return err
		}
	}

	if err != nil {
		return err
	}

	vers, err := dbGet("ver:" + id)
	needSplit := (err == errors.ErrNotFound)
	if err != nil {
		if err != errors.ErrNotFound {
			return err
		}
	} else {
		return nil
	}

	if vers == "" {
		fmt.Println(nod)
		vers = nod.VerString()
	} else {
		if !strings.Contains(vers, nod.VerString()) {
			vers = vers + "," + nod.VerString()
		} else {
			return nil
		}
	}

	err = batchPut(batch, "ver:"+id, vers)
	if err != nil {
		return err
	}

	if !needSplit {
		return nil
	}

	keys := splitPkgName(nod.ImportPath)

	for key, _ := range keys {
		err = batchPut(batch, fmt.Sprintf("key:%v:%v", key, id), "")
		if err != nil {
			return err
		}
	}

	return db.Write(batch, wo)
}

func rmPkg(nod *doc.Node) {

}

var db *leveldb.DB

// service should be run
func autoRun() error {
	s, _, _ := runningStatus()
	if s == STOP {
		attr := &os.ProcAttr{
			Files: make([]*os.File, 0),
		}
		_, err := os.StartProcess("./gopm", []string{"serve", "-l"}, attr)
		if err != nil {
			return err
		}

		/*f, err := os.OpenFile("~/.gopm/var/pid", os.O_CREATE, 0700)
		if err != nil {
			return err
		}
		f.WriteString(fmt.Sprintf("%v,%v,%v", RUNNING, , ))

		fmt.Println(p.Pid)*/
	}
	return nil
}

func runningStatus() (int, int, int) {
	contentByte, err := ioutil.ReadFile("~/.gopm/var/pid")
	if err != nil {
		return STOP, 0, 0
	}
	content := string(contentByte)
	if len(content) < 0 || !strings.Contains(content, ",") {
		return STOP, 0, 0
	}
	cs := strings.Split(string(content), ",")
	if len(cs) != 3 {
		return STOP, 0, 0
	}
	status, err := strconv.Atoi(cs[0])
	if err != nil {
		return STOP, 0, 0
	}
	if status < STOP || status > RUNNING {
		return STOP, 0, 0
	}
	pid, err := strconv.Atoi(cs[1])
	if err != nil {
		return STOP, 0, 0
	}

	_, err = os.FindProcess(pid)
	if err != nil {
		return STOP, 0, 0
	}

	port, err := strconv.Atoi(cs[2])
	if err != nil {
		return STOP, 0, 0
	}

	return status, pid, port
}

func startService(listen string) {
	// check the pre serve's type
	curUser, err := user.Current()
	if err != nil {
		fmt.Println(err)
		return
	}

	dbDir = strings.Replace(dbDir, "~", curUser.HomeDir, -1)

	db, err = leveldb.OpenFile(dbDir, &opt.Options{Flag: opt.OFCreateIfMissing})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	// these handlers should only access by localhost
	http.HandleFunc("/add", addHandler)
	http.HandleFunc("/rm", rmHandler)

	// these handlers can be accessed according listen's ip
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/searche", searcheHandler)
	http.ListenAndServe(listen, nil)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ids := make(map[string]bool)
	for key, _ := range r.Form {
		iter := db.NewIterator(ro)
		rkey := fmt.Sprintf("key:%v:", key)
		if iter.Seek([]byte(rkey)) {
			k := iter.Key()
			if !strings.HasPrefix(string(k), rkey) {
				break
			} else {
				ids[string(k)] = true
			}
		}
		for iter.Next() {
			k := iter.Key()
			if !strings.HasPrefix(string(k), rkey) {
				break
			}
			ids[string(k)] = true
		}
	}

	pkgs := make([]string, 0)

	for id, _ := range ids {
		idkeys := strings.SplitN(id, ":", -1)
		rId := idkeys[len(idkeys)-1]
		fmt.Println(rId)
		pkg, err := dbGet(fmt.Sprintf("pkg:%v", rId))
		if err != nil {
			doc.ColorLog(err.Error())
			continue
		}
		pkgs = append(pkgs, pkg)
	}

	w.Write([]byte("[\"" + strings.Join(pkgs, "\", \"") + "\"]"))
}

func searcheHandler(w http.ResponseWriter, r *http.Request) {
	//if r.Method == "POST" {
	r.ParseForm()
	pkgs := make([]string, 0)
	for key, _ := range r.Form {
		_, err := dbGet("index:" + key)

		if err != nil {
			doc.ColorLog(err.Error())
			continue
		}

		pkgs = append(pkgs, key)
	}

	w.Write([]byte("[\"" + strings.Join(pkgs, "\", \"") + "\"]"))
	//}
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	//if r.Method == "POST" {
	r.ParseForm()
	for key, _ := range r.Form {
		fmt.Println(key)
		// pkg := NewPkg(key, "")
		nod := &doc.Node{
			ImportPath:  key,
			DownloadURL: key,
			IsGetDeps:   true,
		}
		if nod != nil {
			err := addNode(nod)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			fmt.Println(key)
		}
	}
	//}
}

func rmHandler(w http.ResponseWriter, r *http.Request) {

}
