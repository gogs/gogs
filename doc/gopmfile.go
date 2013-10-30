package doc

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

const (
	Greater     = ">"
	GreaterOrEq = ">="
	Equeal      = "="
	Lesser      = "<"
	LesserOrEq  = "<="
)

var (
	Ops = []string{GreaterOrEq, LesserOrEq, Greater, Equeal, Lesser}
)

const (
	GopmFileName = ".gopmfile"
)

type Depend struct {
	Pkg *Pkg
	Op  string
	Ver string
}

type Section struct {
	Name string
	Deps map[string]*Depend
}

func NewSection() *Section {
	return &Section{Deps: make(map[string]*Depend)}
}

type Gopmfile struct {
	Sections map[string]*Section
}

func NewGopmfile() *Gopmfile {
	return &Gopmfile{Sections: make(map[string]*Section)}
}

func (this *Gopmfile) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var sec *Section
		text := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(text, "[") {
			sec = NewSection()
			if strings.HasSuffix(text, "]") {
				sec.Name = text[1 : len(text)-1]
			} else {
				return errors.New("need section")
			}
			this.Sections[sec.Name] = sec
		} else {
			if sec == nil {
				continue
			}

			var dep *Depend
			for _, op := range Ops {
				if strings.Contains(text, op) {
					ss := strings.Split(text, op)
					pkver := strings.Split(ss[1], ":")
					var tp, value string
					tp = pkver[0]
					if len(pkver) == 2 {
						value = pkver[1]
					}
					dep = &Depend{NewPkg(ss[0], tp, value), ss[1], value}
					break
				}
			}

			if dep == nil {
				dep = &Depend{NewDefaultPkg(text), Equeal, ""}
			}
			sec.Deps[dep.Pkg.ImportPath] = dep
		}
	}

	return nil
}

func (this *Gopmfile) Save(path string) error {
	return nil
}
