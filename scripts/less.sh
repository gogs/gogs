#!/bin/sh
echo "compiling LESS Files"
lessc ../public/ng/less/gogs.less ../public/ng/css/gogs.css
lessc ../public/ng/less/ui.less ../public/ng/css/ui.css
echo "done"
