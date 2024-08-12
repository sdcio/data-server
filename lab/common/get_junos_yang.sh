#!/bin/bash
# Copyright 2024 Nokia
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


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