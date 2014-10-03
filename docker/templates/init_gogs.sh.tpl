#!/bin/sh

if [ ! -d "$DIRECTORY" ]; then
    mkdir -p $GOGS_CUSTOM_CONF_PATH

echo "
{{ CONFIG }}
" >> $GOGS_CUSTOM_CONF
    
fi

exec "$@"
