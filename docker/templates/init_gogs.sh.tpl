#!/bin/sh

if [ ! -d "$GOGS_CUSTOM_CONF_PATH" ]; then
    mkdir -p $GOGS_CUSTOM_CONF_PATH

echo "
{{ CONFIG }}
" >> $GOGS_CUSTOM_CONF
    
fi

exec "$@"
