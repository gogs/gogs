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

// func getService(pkgName string) Service {
// 	for _, service := range services {
// 		if service.HasPkg(pkgName) {
// 			return service
// 		}
// 	}
// 	return nil
// }

// type Service interface {
// 	PkgUrl(pkg *Pkg) string
// 	HasPkg(pkgName string) bool
// 	PkgExt() string
// }

//type Pkg struct {
//	Name  string
//	Ver   string
//	VerId string
//}

// func NewPkg(pkgName string, ver string) *Pkg {
// 	vers := strings.Split(ver, ":")
// 	if len(vers) > 2 {
// 		return nil
// 	}

// 	var verId string
// 	if len(vers) == 2 {
// 		verId = vers[1]
// 	}

// 	if len(vers) == 1 {
// 		vers[0] = TRUNK
// 	}

// 	service := getService(pkgName)
// 	if service == nil {
// 		return nil
// 	}

// 	return &Pkg{service, pkgName, vers[0], verId}
// }

// func (p *Pkg) VerSimpleString() string {
// 	if p.VerId != "" {
// 		return p.VerId
// 	}
// 	return p.Ver
// }

// func (p *Pkg) Url() string {
// 	return p.Service.PkgUrl(p)
// }

// func (p *Pkg) FileName() string {
// 	return fmt.Sprintf("%v.%v", p.VerSimpleString(), p.Service.PkgExt())
// }

// // github repository
// type GithubService struct {
// }

// func (s *GithubService) PkgUrl(pkg *Pkg) string {
// 	var verPath string
// 	if pkg.Ver == TRUNK {
// 		verPath = "master"
// 	} else {
// 		verPath = pkg.VerId
// 	}
// 	return fmt.Sprintf("https://%v/archive/%v.zip", pkg.Name, verPath)
// }

// func (s *GithubService) HasPkg(pkgName string) bool {
// 	return strings.HasPrefix(pkgName, "github.com")
// }

// func (s *GithubService) PkgExt() string {
// 	return "zip"
// }

// // git osc repos
// type GitOscService struct {
// }

// func (s *GitOscService) PkgUrl(pkg *Pkg) string {
// 	var verPath string
// 	if pkg.Ver == TRUNK {
// 		verPath = "master"
// 	} else {
// 		verPath = pkg.VerId
// 	}
// 	return fmt.Sprintf("https://%v/repository/archive?ref=%v", pkg.Name, verPath)
// }

// func (s *GitOscService) HasPkg(pkgName string) bool {
// 	return strings.HasPrefix(pkgName, "git.oschina.net")
// }

// func (s *GitOscService) PkgExt() string {
// 	return "zip"
// }

// // bitbucket.org
// type BitBucketService struct {
// }

// func (s *BitBucketService) PkgUrl(pkg *Pkg) string {
// 	var verPath string
// 	if pkg.Ver == TRUNK {
// 		verPath = "default"
// 	} else {
// 		verPath = pkg.VerId
// 	}

// 	return fmt.Sprintf("https://%v/get/%v.zip", pkg.Name, verPath)
// }

// func (s *BitBucketService) HasPkg(pkgName string) bool {
// 	return strings.HasPrefix(pkgName, "bitbucket.org")
// }

// func (s *BitBucketService) PkgExt() string {
// 	return "zip"
// }

// type GitCafeService struct {
// }

// func (s *GitCafeService) PkgUrl(pkg *Pkg) string {
// 	var verPath string
// 	if pkg.Ver == TRUNK {
// 		verPath = "master"
// 	} else {
// 		verPath = pkg.VerId
// 	}

// 	return fmt.Sprintf("https://%v/tarball/%v", pkg.Name, verPath)
// }

// func (s *GitCafeService) HasPkg(pkgName string) bool {
// 	return strings.HasPrefix(pkgName, "gitcafe.com")
// }

// func (s *GitCafeService) PkgExt() string {
// 	return "tar.gz"
// }

// // git lab repos, not completed
// type GitLabService struct {
// 	DomainOrIp string
// 	Username   string
// 	Passwd     string
// 	PrivateKey string
// }

// func (s *GitLabService) PkgUrl(pkg *Pkg) string {
// 	var verPath string
// 	if pkg.Ver == TRUNK {
// 		verPath = "master"
// 	} else {
// 		verPath = pkg.VerId
// 	}

// 	return fmt.Sprintf("https://%v/repository/archive/%v", pkg.Name, verPath)
// }

// func (s *GitLabService) HasPkg(pkgName string) bool {
// 	return strings.HasPrefix(pkgName, s.DomainOrIp)
// }

// func (s *GitLabService) PkgExt() string {
// 	return "tar.gz"
// }

// // code.csdn.net
// type CodeCSDNService struct {
// }

// func (s *CodeCSDNService) PkgUrl(pkg *Pkg) string {
// 	var verPath string
// 	if pkg.Ver == TRUNK {
// 		verPath = "master"
// 	} else {
// 		verPath = pkg.VerId
// 	}

// 	return fmt.Sprintf("https://%v/repository/archive?ref=%v", pkg.Name, verPath)
// }

// func (s *CodeCSDNService) HasPkg(pkgName string) bool {
// 	return strings.HasPrefix(pkgName, "code.csdn.net")
// }

// func (s *CodeCSDNService) PkgExt() string {
// 	return "zip"
// }
