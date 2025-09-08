#!/usr/bin/env sh

# This is very simple, yet effective backup rotation script.
# All files that are older than the latest RETENTION_NUM backup files are accumulated and deleted using rm.
main() {
	BACKUP_PATH="${1:-}"
	RETENTION_NUM="${2:-1}"

	if [ -z "${BACKUP_PATH}" ]; then
		echo "Error: Required argument missing BACKUP_PATH" 1>&2
		exit 1
	fi

	if [ "$(realpath "${BACKUP_PATH}")" = "/" ]; then
		echo "Error: Dangerous BACKUP_PATH: /" 1>&2
		exit 1
	fi

	if [ ! -d "${BACKUP_PATH}" ]; then
	  echo "Error: BACKUP_PATH doesn't exist or is not a directory" 1>&2
		exit 1
	fi

	# shellcheck disable=SC2012  # File name is expected
	ls -1t "${BACKUP_PATH}/"gogs-backup-*.zip | tail -n +"$(( RETENTION_NUM + 1 ))" | xargs -n10 --no-run-if-empty rm
}

main "$@"
