#!/bin/bash

blocks_dir=blocks
docker_dir=docker
template_dir=templates

docker_file=Dockerfile

gogs_config_file=conf.tmp
gogs_config=config
gogs_init_file=$docker_dir/init_gogs.sh

fig_file=fig.yml
fig_config=fig

gogs_init_template=$template_dir/init_gogs.sh.tpl

if [ "$#" == 0 ]; then
    blocks=`ls $blocks_dir`
    if [ -z "$blocks" ]; then
        echo "No Blocks available in $blocks_dir"
    else
        echo "Available Blocks:"
        for block in $blocks; do
            echo "    $block"
        done
    fi
    exit 0
fi

for file in $gogs_config_file $fig_file; do
    if [ -e $file ]; then
        echo "Deleting $file"
        rm $file
    fi
done

for dir in $@; do
    current_dir=$blocks_dir/$dir
    if [ ! -d "$current_dir" ]; then
        echo "$current_dir is not a directory"
        exit 1
    fi

    if [ -e $current_dir/$docker_file ]; then
        echo "Copying $current_dir/$docker_file to $docker_dir/$docker_file"
        cp $current_dir/$docker_file $docker_dir/$docker_file
    fi

    if [ -e $current_dir/$gogs_config ]; then
        echo "Adding $current_dir/$gogs_config to $gogs_config_file"
        cat $current_dir/$gogs_config >> $gogs_config_file
        echo "" >> $gogs_config_file
    fi

    if [ -e $current_dir/$fig_config ]; then
        echo "Adding $current_dir/$fig_config to $fig_file"
        cat $current_dir/fig >> $fig_file
        echo "" >> $fig_file
    fi
done

echo "Creating $gogs_init_file"
sed "/{{ CONFIG }}/{
r $gogs_config_file
d
}" $gogs_init_template > $gogs_init_file

if [ -e $gogs_config_file ]; then
    echo "Removing temporary GoGS config"
    rm $gogs_config_file
fi