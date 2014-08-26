rm -rf output_linux_64
mkdir output_linux_64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build
chmod +x gogs
mv gogs ./output_linux_64/
cp -r ./conf/ ./output_linux_64/conf/
cp -r ./custom/ ./output_linux_64/custom/
cp -r ./dockerfiles/ ./output_linux_64/dockerfiles/
cp -r ./public/ ./output_linux_64/public/
cp -r ./templates/ ./output_linux_64/templates/
cp bee.json ./output_linux_64/
cp cert.pem ./output_linux_64/
cp CONTRIBUTING.md ./output_linux_64/
cp gogs_supervisord.sh ./output_linux_64/
cp key.pem ./output_linux_64/
cp LICENSE ./output_linux_64/
cp README.md ./output_linux_64/
cp README_ZH.md ./output_linux_64/
cp rpp.ini ./output_linux_64/
cp start.bat ./output_linux_64/
cp start.sh ./output_linux_64/
cp wercker.yml ./output_linux_64/