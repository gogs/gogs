#!/usr/bin/env sh

execute_backup_job() {
	BACKUP_ARG_PATH="${1:-}"
	BACKUP_ARG_CONFIG="${BACKUP_ARG_CONFIG:-}"
	BACKUP_ARG_EXCLUDE_REPOS="${BACKUP_ARG_EXCLUDE_REPOS:-}"
	BACKUP_EXTRA_ARGS="${BACKUP_EXTRA_ARGS:-}"
	cd "/app/gogs" || exit 1

	BACKUP_ARGS="--target=${BACKUP_ARG_PATH}"

	if [ -n "${BACKUP_ARG_CONFIG}" ]; then
		BACKUP_ARGS="${BACKUP_ARGS} --config=${BACKUP_ARG_CONFIG}"
	fi

	if [ -n "${BACKUP_ARG_EXCLUDE_REPOS}" ]; then
		BACKUP_ARGS="${BACKUP_ARGS} --exclude-repos=${BACKUP_ARG_EXCLUDE_REPOS}"
	fi

	if [ -n "${BACKUP_EXTRA_ARGS}" ]; then
		BACKUP_ARGS="${BACKUP_ARGS} ${BACKUP_EXTRA_ARGS}"
	fi

	# NOTE: We actually need word splitting to be able to pass multiple arguments.
	# shellcheck disable=SC2086
	./gogs backup ${BACKUP_ARGS} || echo "Error: Backup job returned non-successful code." && exit 1
}

main() {
	BACKUP_PATH="${1:-}"

	if [ -z "${BACKUP_PATH}" ]; then
		echo "Required argument missing BACKUP_PATH" 1>&2
		exit 1
	fi

	execute_backup_job "${BACKUP_PATH}"
}

main "$@"
