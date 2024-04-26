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

*** Settings ***
Resource          ../../keywords/server.robot
Resource          ../../keywords/client.robot
Library           OperatingSystem
Library           String
Library           Process
# Suite Setup       SetupColocated    True    ${DATA-SERVER-BIN}    ${DATA-SERVER-CONFIG}    ${data-server-process-alias}    ${data-server-stderr}
# Suite Teardown    Teardown

*** Variables ***
${DATA-SERVER-BIN}    ${CURDIR}/../../../../bin/data-server
${SDCTL}            sdctl

${DATA-SERVER-CONFIG}    ${CURDIR}/../must/data-server.yaml

${DATA-SERVER-IP}    127.0.0.1
${DATA-SERVER-PORT}    56000

${SCHEMA-SERVER-IP}    127.0.0.1
${SCHEMA-SERVER-PORT}    56000


# TARGET
${srlinux1-name}    srl1
${srlinux1-candidate}    default
${srlinux1-schema-name}    srl
${srlinux1-schema-version}    23.10.1
${srlinux1-schema-vendor}    Nokia
${srlinux1-target-def}    ${CURDIR}/../colocated/robot_srl1.json
${srlinux1-sync-def}    ${CURDIR}/../colocated/sync.json


# internal vars
${data-server-process-alias}    dsa
${data-server-stderr}    /tmp/ds-out



*** Test Cases ***
Check Server State
    CheckServerState Colocated    ${data-server-process-alias}

Create SRL1 Target
    ${result} =    CreateDataStore    ${srlinux1-name}    ${srlinux1-target-def}    ${srlinux1-sync-def}    ${srlinux1-schema-name}    ${srlinux1-schema-Vendor}     ${srlinux1-schema-version}
    Should Be Equal As Integers    ${result.rc}    0

One
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    test    100        ${CURDIR}/intents/one.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Two
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    other    50        ${CURDIR}/intents/two.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Three
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    bla    5        ${CURDIR}/intents/three.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Four
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    four    120        ${CURDIR}/intents/four.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Five
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    four    4        ${CURDIR}/intents/three.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Six
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    four    4        ${CURDIR}/intents/six.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}


Seven
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    seven    200        ${CURDIR}/intents/seven.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}


Seven-2
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    seven    2        ${CURDIR}/intents/seven.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}


Eight
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    seven    2        ${CURDIR}/intents/three.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}