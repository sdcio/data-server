#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

VERSION=$1
RELEASE=$2

REPOBRANCH=github.com/Juniper/yang/tree/master

CONFDIR=junos/conf
COMMONDIR=common

DESINATION_BASE=${SCRIPTPATH}/yang/junos-${VERSION}${RELEASE}

DESTINATION_CONF=${DESINATION_BASE}/models
DESTINATION_COMMON=${DESINATION_BASE}/common

# create destination directories
mkdir -p ${DESTINATION_CONF}
mkdir -p ${DESTINATION_COMMON}

# download files 
curl -s https://${REPOBRANCH}/${VERSION}/${VERSION}${RELEASE}/${CONFDIR} | \
    perl -slne '/href=\".*\/(.*\.yang)\"/ && print "https://github.com/Juniper/yang/raw/master/$VERSION/$VERSION${RELEASE}/$CONFDIR/",$1 ' -- -VERSION=$VERSION -RELEASE=$RELEASE -CONFDIR=$CONFDIR | \
    xargs -I{} wget -q --show-progress -P ${DESTINATION_CONF} {}

curl -s https://${REPOBRANCH}/${VERSION}/${VERSION}${RELEASE}/common | \
    perl -slne '/href=\".*\/(.*\.yang)\"/ && print "https://github.com/Juniper/yang/raw/master/$VERSION/$VERSION${RELEASE}/$COMMONDIR/",$1 ' -- -VERSION=$VERSION -RELEASE=$RELEASE -COMMONDIR=$COMMONDIR | \
    xargs -I{} wget -q --show-progress -P ${DESTINATION_COMMON} {}