#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

VERSION=$1
REPO=https://github.com/nokia/7x50_YangModels.git
BRANCH="sros_"$VERSION

git clone -b $BRANCH $REPO $SCRIPTPATH/yang/$BRANCH

rm -rf $SCRIPTPATH/yang/$BRANCH/.git
rm -rf $SCRIPTPATH/yang/$BRANCH/.gitignore
rm -rf $SCRIPTPATH/yang/$BRANCH/*.md