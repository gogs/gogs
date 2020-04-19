#!/usr/bin/env sh

main() {
	BACKUP_PATH="${1:-}"
	BACKUP_RETENTION_DAYS="${2:-}"

	if [ -z "${BACKUP_PATH}" ]; then
		echo "Required argument missing BACKUP_PATH"
		exit 1
	fi

	find "${BACKUP_PATH}/" -type f -mtime "+${BACKUP_RETENTION_DAYS}" -print -exec rm "{}" +
}

main "$@"
