#!/bin/bash
UPSTREAM=${TUNASYNC_UPSTREAM_URL}
if [[ -z "$UPSTREAM" ]];then
	echo "Please set the TUNASYNC_UPSTREAM_URL"
	exit 1
fi

function repo_init() {
	git clone "$UPSTREAM" "$TUNASYNC_WORKING_DIR"
}

function update_linux_git() {
	cd "$TUNASYNC_WORKING_DIR"
	echo "==== SYNC $UPSTREAM START ===="
	git remote set-url origin "$UPSTREAM"
	/usr/bin/timeout -s INT 3600 git pull
	echo "==== SYNC $UPSTREAM DONE ===="
}

if [[ ! -d "$TUNASYNC_WORKING_DIR/.git" ]]; then
	echo "Initializing $UPSTREAM mirror"
	repo_init
fi

update_linux_git