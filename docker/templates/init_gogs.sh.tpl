#!/bin/sh

if [ ! -d "$DIRECTORY" ]; then
    mkdir -p $GOGS_CUSTOM_CONF_PATH

    #~ Either "dev", "prod" or "test", default is "dev"
echo "
{{ CONFIG }}
" >> $GOGS_CUSTOM_CONF
    
fi

exec "$@"
