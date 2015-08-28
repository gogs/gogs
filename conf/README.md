Execute following command in ROOT directory when anything is changed:

$ go-bindata -o=modules/bindata/bindata.go -ignore="\\.DS_Store|README.md" -pkg=bindata conf/...

Add -debug flag to make life easier in development(somehow isn't working):

$ go-bindata -debug -o=modules/bindata/bindata.go -ignore="\\.DS_Store|README.md" -pkg=bindata conf/...