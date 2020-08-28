#!/usr/bin/env sh

# This is very simple, yet effective backup rotation script.
# Using find command, all files that are older than BACKUP_RETENTION_DAYS are accumulated and deleted using rm.
main() {
	BACKUP_PATH="${1:-}"
	FIND_EXPRESSION="${2:-mtime +7}"

	if [ -z "${BACKUP_PATH}" ]; then
		echo "Error: Required argument missing BACKUP_PATH" 1>&2
		exit 1
	fi

	if [ "$(realpath "${BACKUP_PATH}")" = "/" ]; then
		echo "Error: Dangerous BACKUP_PATH: /" 1>&2
		exit 1
	fi

	if [ ! -d "${BACKUP_PATH}" ]; then
	  echo "Error: BACKUP_PATH does't exist or is not a directory" 1>&2
		exit 1
	fi

	# shellcheck disable=SC2086
	find "${BACKUP_PATH}/" -type f -name "gogs-backup-*.zip" -${FIND_EXPRESSION} -print -exec rm "{}" +
}

main "$@"
