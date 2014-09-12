rm -rf output
mkdir output
go build ../gogs.go
chmod +x gogs
mv gogs ./output/
cp -r ../conf/ ./output/conf/
cp -r ../custom/ ./output/custom/
cp -r ./dockerfiles/ ./output/dockerfiles/
cp -r ../public/ ./output/public/
cp -r ../templates/ ./output/templates/
cp ../cert.pem ./output/
cp ../CONTRIBUTING.md ./output/
cp gogs_supervisord.sh ./output/
cp ../key.pem ./output/
cp ../LICENSE ./output/
cp ../README.md ./output/
cp ../README_ZH.md ./output/
cp start.bat ./output/
cp start.sh ./output/
cp ../wercker.yml ./output/
cp mysql.sql ./output/
