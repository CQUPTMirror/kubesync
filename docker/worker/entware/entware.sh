#!/bin/sh

createDirectoryTree()
{
    TARGET_DIRECTORY="${1}"
    TEMP_DIRECTORY="${TUNASYNC_WORKING_DIR}/.tmp"
    PRESENT_DIRECTORY_TREE="${TEMP_DIRECTORY}/present_directory_tree"
    LATEST_DIRECTORY_TREE="${TEMP_DIRECTORY}/latest_directory_tree"

    rm -rf "${TEMP_DIRECTORY}"

    mkdir -p "${TEMP_DIRECTORY}"

    wget --timestamping --tries=10 --timeout=100 --continue --no-host-directories --recursive --no-parent --accept "index.html" --directory-prefix="${TEMP_DIRECTORY}" "${TUNASYNC_UPSTREAM_URL}"

    find "${TEMP_DIRECTORY}" -type d | sed -E -e '\#/archive/?[^/]*#d' -e 's#'"${TEMP_DIRECTORY}"'#'"${TUNASYNC_WORKING_DIR}"'#' -e 's#/$##' > "${LATEST_DIRECTORY_TREE}"
    find "${TUNASYNC_WORKING_DIR}" -type d | sed -E -e '\#'"${TEMP_DIRECTORY}"'#d' -e 's#/$##' > "${PRESENT_DIRECTORY_TREE}"

    grep -Fxvf "${PRESENT_DIRECTORY_TREE}" "${LATEST_DIRECTORY_TREE}" | grep -E -e '^'"${TARGET_DIRECTORY}"'' > "${TEMP_DIRECTORY}/list_to_mkdir"
    grep -Fxvf "${LATEST_DIRECTORY_TREE}" "${PRESENT_DIRECTORY_TREE}" | grep -E -e '^'"${TARGET_DIRECTORY}"'' > "${TEMP_DIRECTORY}/list_to_remove"

    LIST_TO_MKDIR=$(cat "${TEMP_DIRECTORY}/list_to_mkdir")
    LIST_TO_REMOVE=$(cat "${TEMP_DIRECTORY}/list_to_remove")

    for ITEM in ${LIST_TO_MKDIR}
    do
        echo "mkdir ${ITEM}"
        mkdir -p "${ITEM}" 2> /dev/null
    done

    for ITEM in ${LIST_TO_REMOVE}
    do
        echo "remove ${ITEM}"
        rm -rf "${ITEM}"
    done

    rm -rf "${TEMP_DIRECTORY}"
}

fileSync()
{
    LOCAL_DIRECTORY="${1}"
    TEMP_DIRECTORY="${LOCAL_DIRECTORY}/.tmp"
    URL_DIRECTORY="$(echo "${LOCAL_DIRECTORY}" | sed -e 's#'"${TUNASYNC_WORKING_DIR}"'/*##')"
    PRESENT_INDEX="${LOCAL_DIRECTORY}/index.html"
    CURRENT_INDEX="${TEMP_DIRECTORY}/current_index.html"
    LATEST_INDEX="${TEMP_DIRECTORY}/latest_index.html"

    rm -rf "${TEMP_DIRECTORY}"

    mkdir -p "${TEMP_DIRECTORY}"

    wget --tries=10 --timeout=100 --continue --no-host-directories --output-document="${LATEST_INDEX}" "${TUNASYNC_UPSTREAM_URL}/${URL_DIRECTORY}/"

    if [ -e "${PRESENT_INDEX}" ]
    then
        cp -f "${PRESENT_INDEX}" "${CURRENT_INDEX}"
    else
        echo "" > "${CURRENT_INDEX}"
    fi

    grep -Fxvf "${CURRENT_INDEX}" "${LATEST_INDEX}" | sed -E -n -e 's#^<a href=\"([^/]+)\".*#\1#p' > "${TEMP_DIRECTORY}/list_to_download"
    grep -Fxvf "${LATEST_INDEX}" "${CURRENT_INDEX}" | sed -E -n -e 's#^<a href=\"([^/]+)\".*#\1#p' > "${TEMP_DIRECTORY}/list_to_remove"

    LIST_TO_DOWNLOAD=$(cat "${TEMP_DIRECTORY}/list_to_download")
    LIST_TO_REMOVE=$(cat "${TEMP_DIRECTORY}/list_to_remove")

    for ITEM in ${LIST_TO_DOWNLOAD}
    do
        echo "download ${URL_DIRECTORY}/${ITEM}}"
        wget --timestamping --tries=10 --timeout=100 --continue --directory-prefix="${LOCAL_DIRECTORY}" "${TUNASYNC_UPSTREAM_URL}/${URL_DIRECTORY}/${ITEM}"
    done

    for ITEM in ${LIST_TO_REMOVE}
    do
        echo "remove ${ITEM}}"
        rm -f "${LOCAL_DIRECTORY}/${ITEM}"
    done

    cp -f "${LATEST_INDEX}" "${PRESENT_INDEX}"

    rm -rf "${TEMP_DIRECTORY}"
}

DIRECTORY_TO_SYNC="${TUNASYNC_WORKING_DIR}/"

mkdir -p "${TUNASYNC_WORKING_DIR}/css/" 2> /dev/null
wget --tries=10 --timeout=100 --continue --no-host-directories --output-document="${TUNASYNC_WORKING_DIR}/css/packages.css" "${TUNASYNC_UPSTREAM_URL}/css/packages.css"

createDirectoryTree "${DIRECTORY_TO_SYNC}"

DIRECTORY_LIST="$(find "${DIRECTORY_TO_SYNC}" -type d | sed -e 's#/$##')"

for ITEM in ${DIRECTORY_LIST}
do
    fileSync "${ITEM}"
done

echo "finished"