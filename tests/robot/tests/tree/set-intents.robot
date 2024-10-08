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
${srlinux1-schema-version}     23.10.1
${srlinux1-schema-vendor}    Nokia
${srlinux1-target-def}    ${CURDIR}/../colocated/robot_srl1.json
${srlinux1-sync-def}    ${CURDIR}/../colocated/sync.json

# TARGET
${srlinux1-1-name}    srl1-1
${srlinux1-1-candidate}    default
${srlinux1-1-schema-name}    srl
${srlinux1-1-schema-version}    24.3.2
${srlinux1-1-schema-vendor}    Nokia
${srlinux1-1-target-def}    ${CURDIR}/../colocated/robot_srl1.json
${srlinux1-1-sync-def}    ${CURDIR}/../colocated/sync.json

${sros-name}    sros
${sros-candidate}    default
${sros-schema-name}    sros
${sros-schema-version}    23.7
${sros-schema-vendor}    Nokia
${sros-target-def}    ${CURDIR}/../colocated/robot_sros.json
${sros-sync-def}    ${CURDIR}/../colocated/sync_sros.json

# internal vars
${data-server-process-alias}    dsa
${data-server-stderr}    /tmp/ds-out



*** Test Cases ***
Check Server State
    CheckServerState Colocated    ${data-server-process-alias}

Create SRL1-1 Target
    ${result} =    CreateDataStore    ${srlinux1-1-name}    ${srlinux1-1-target-def}    ${srlinux1-1-sync-def}    ${srlinux1-1-schema-name}    ${srlinux1-1-schema-Vendor}     ${srlinux1-1-schema-version}
    Should Be Equal As Integers    ${result.rc}    0

Create SRL1 Target
    ${result} =    CreateDataStore    ${srlinux1-name}    ${srlinux1-target-def}    ${srlinux1-sync-def}    ${srlinux1-schema-name}    ${srlinux1-schema-Vendor}     ${srlinux1-schema-version}
    Should Be Equal As Integers    ${result.rc}    0


Create SROS Target
    ${result} =    CreateDataStore    ${sros-name}    ${sros-target-def}    ${sros-sync-def}    ${sros-schema-name}    ${sros-schema-Vendor}     ${sros-schema-version}
    Should Be Equal As Integers    ${result.rc}    0


One
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    test    100        ${CURDIR}/intents/one.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

Two
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    other    50        ${CURDIR}/intents/two.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

Three
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    bla    5        ${CURDIR}/intents/three.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

Four
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    four    120        ${CURDIR}/intents/four.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

Five
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    four    4        ${CURDIR}/intents/three.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

Six
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    four    4        ${CURDIR}/intents/six.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}


Seven
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    seven    200        ${CURDIR}/intents/seven.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}


Seven-2
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    seven    2        ${CURDIR}/intents/seven.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}


Eight
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    seven    2        ${CURDIR}/intents/three.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

9_1 - Add Double Key
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    nine    10        ${CURDIR}/intents/9-1_double_key_add.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}


9_2 - Double Key, Remove single entry
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    nine    10        ${CURDIR}/intents/9-2_double_key_remove_single_entry.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

10 - Delete three.json
    ${result} =    DeleteIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    seven    2
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

11 - Presence - Accept
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    eleven    10        ${CURDIR}/intents/acl_accept.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

11 - Presence - Accept with Subelement
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    eleven    10        ${CURDIR}/intents/acl_accept_with_subelements.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

11 - Presence - Drop
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    eleven    10        ${CURDIR}/intents/acl_drop.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

11 - Presence - Drop with Subelement
    ${result} =    SetIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    eleven    10        ${CURDIR}/intents/acl_drop_with_subelements.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

11 - Presence - Delete intent
    ${result} =    DeleteIntent    ${srlinux1-1-name}    ${srlinux1-1-candidate}    eleven    10
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-1-name}    ${srlinux1-1-candidate}

12 - Choices - Accept - Low Precedence
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    twelve-1    10        ${CURDIR}/intents/acl_accept_with_subelements.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

12 - Choices - Accept - High Precedence
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    twelve-1    5        ${CURDIR}/intents/acl_accept.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

12 - Choices - Drop - Low Precedence
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    twelve-2    10        ${CURDIR}/intents/acl_drop.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

12 - Choices - Drop - High Precedence
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    twelve-2    5        ${CURDIR}/intents/acl_drop.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

12 - Choices - Delete All
    ${result} =    DeleteIntent    ${srlinux1-name}    ${srlinux1-candidate}    twelve-1    10
    Should Be Equal As Integers    ${result.rc}    0
    ${result} =    DeleteIntent    ${srlinux1-name}    ${srlinux1-candidate}    twelve-2    5
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

14.1 - Union int
    ${result} =     SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    fourteen    5        ${CURDIR}/intents/union_int.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

14.1 - Union String
    ${result} =     SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    fourteen    5        ${CURDIR}/intents/union_string.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

15.1 - Integration1
    ${result} =     SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    fifteen    5        ${CURDIR}/intents/integration1.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

15.2 - Integration1
    ${result} =     SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    fifteen    5        ${CURDIR}/intents/integration2.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

16 - Wim Issue
    ${result} =     SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    sixteen    5        ${CURDIR}/intents/wim_issue.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

17 - Leaf-List as String
    ${result} =     SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    seventeen    5        ${CURDIR}/intents/leaf-list-string.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

18-1 - SROS Add LeafList
    ${result} =     SetIntent    ${sros-name}    ${sros-candidate}    eighteen    5        ${CURDIR}/intents/18-sros-leaflist.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

18-2 - SROS Delete LeafList
    ${result} =    DeleteIntent    ${sros-name}    ${sros-candidate}    eighteen    5
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${sros-name}    ${sros-candidate}





19-1 - SROS Add List
    ${result} =     SetIntent    ${sros-name}    ${sros-candidate}    nineteen    5        ${CURDIR}/intents/19-sros-lists.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

19-2 - SROS Add List
    ${result} =     SetIntent    ${sros-name}    ${sros-candidate}    nineteen    5        ${CURDIR}/intents/19-sros-lists-entry-removed.json
    Should Be Equal As Integers    ${result.rc}    0
    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

19-3 - SROS Delete List
    ${result} =    DeleteIntent    ${sros-name}    ${sros-candidate}    nineteen    5
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${sros-name}    ${sros-candidate}
