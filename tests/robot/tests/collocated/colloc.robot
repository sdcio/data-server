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
Resource            ../../keywords/server.robot
Resource            ../../keywords/client.robot
Resource            ../../keywords/gnmic.robot
Library             OperatingSystem
Library             String
Library             Process

Suite Setup         DeployLab    ${topology-file}
Suite Teardown      DestroyLab    ${topology-file}
Test Setup          Setupcollocated    True    ${data-server-bin}    ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}
Test Teardown       Teardown


*** Variables ***
${DATA-SERVER-BIN}              ${CURDIR}/../../../../bin/data-server
${SDCTL}                        sdctl
${data-server-config}           ${CURDIR}/collocated_test.yaml
${topology-file}                ${CURDIR}/lab/collocated.clab.yaml

${DATA-SERVER-IP}    127.0.0.1
${DATA-SERVER-PORT}    56000

${SCHEMA-SERVER-IP}    127.0.0.1
${SCHEMA-SERVER-PORT}    56000

# TARGET
${schema-name}                  srl
${schema-version}               23.10.1
${schema-vendor}                Nokia
${candidate-name}               default

${owner}                        test
${priority}                     100

${datastore1}                   srl1
${devicename1}                  clab-collocated-srl1
${target-file1}                 ${CURDIR}/robot_srl1.json

${datastore2}                   srl2
${devicename2}                  clab-collocated-srl2
${target-file2}                 ${CURDIR}/robot_srl2.json

${target-sync-file}             ${CURDIR}/sync.json

${router}                       ""
${gnmic_flags}                  ""

# internal vars
${data-server-process-alias}    dsa
${data-server-stderr}           /tmp/ds-out


*** Test Cases ***
Create and delete a Datastore
    [Tags]    robot:continue-on-failure

    # create first datastore
    ${result} =    CreateDataStoreTarget
    ...    ${datastore1}
    ...    ${target-file1}
    ...    ${target-sync-file}
    ...    ${schema-name}
    ...    ${schema-vendor}
    ...    ${schema-version}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}

    # create second datastore
    ${result} =    CreateDataStoreTarget
    ...    ${datastore2}
    ...    ${target-file2}
    ...    ${target-sync-file}
    ...    ${schema-name}
    ...    ${schema-vendor}
    ...    ${schema-version}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore2}

    Wait Until Datastore connected    ${datastore1}    30s    1s

    ${result} =    DeleteDatastore    ${datastore1}
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    GetDataStore    ${datastore1}
    Should Not Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    unknown datastore ${datastore1}

Create and delete a Datastore and candidate
    [Tags]    robot:continue-on-failure

    # create first datastore
    ${result} =    client.CreateDataStoreTarget
    ...    ${datastore1}
    ...    ${target-file1}
    ...    ${target-sync-file}
    ...    ${schema-name}
    ...    ${schema-vendor}
    ...    ${schema-version}

    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}

    # create second datastore
    ${result} =    client.CreateDataStoreTarget
    ...    ${datastore2}
    ...    ${target-file2}
    ...    ${target-sync-file}
    ...    ${schema-name}
    ...    ${schema-vendor}
    ...    ${schema-version}
    Should Be Equal As Integers    ${result.rc}    0

    Wait Until Datastore connected    ${datastore1}    30s    1s

    ${result} =    client.CreateCandidate    ${datastore1}    ${candidate-name}    ${owner}    ${priority}
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    client.ListDataStores
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}
    Should Contain    ${result.stdout}    ${candidate-name}

    ${result} =    client.GetCandidate    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}
    Should Contain    ${result.stdout}    ${candidate-name}

    ${result} =    client.DeleteCandidate    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}

    ${result} =    client.GetCandidate    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}
    Should Not Contain    ${result.stdout}    ${candidate-name}

Configure Router gNMI - Set / Delete Leaf
    [Tags]    robot:continue-on-failure

    ${result} =    ListSchemas
    Should Be Equal As Integers    ${result.rc}    0

    # delete interface config
    ${result} =    gnmic.Set
    ...    ${devicename1}
    ...    --skip-verify
    ...    -u    admin
    ...    -p    NokiaSrl1!
    ...    --delete    /interface[name=ethernet-1/1]
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    client.CreateDataStoreTarget
    ...    ${datastore1}
    ...    ${target-file1}
    ...    ${target-sync-file}
    ...    ${schema-name}
    ...    ${schema-vendor}
    ...    ${schema-version}
    Should Be Equal As Integers    ${result.rc}    0

    Wait Until Datastore connected    ${datastore1}    30s    1s

    # create candidate
    ${result} =    client.CreateCandidate    ${datastore1}    ${candidate-name}    ${owner}    ${priority}
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    client.GetCandidate    ${datastore1}    ${candidate-name}

    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}
    Should Contain    ${result.stdout}    ${candidate-name}

    # set config into candidate
    ${result} =    client.Set
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    --update    /interface[name=ethernet-1/1]/description:::Desc1
    Should Be Equal As Integers    ${result.rc}    0

    # get config from candidate
    ${result} =    client.GetFromCandidate
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1

    # run diff
    ${result} =    client.Diff    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1

    # commit
    ${result} =    client.Commit    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0

    # query fom intended store
    ${result} =    client.Get
    ...    ${datastore1}
    ...    --intended
    ...    --path     /interface[name=ethernet-1/1]/description

    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1

    # query fom router
    ${result} =    gnmic.Get
    ...    ${devicename1}
    ...    --skip-verify
    ...    -e    JSON_IETF
    ...    -u    admin
    ...    -p    NokiaSrl1!
    ...    --format    flat
    ...    --path
    ...    /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1

    # DELETE
    # create candidate
    ${result} =    client.CreateCandidate    ${datastore1}    ${candidate-name}    ${owner}    ${priority}
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    client.GetCandidate    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}
    Should Contain    ${result.stdout}    ${candidate-name}

    # set config into candidate
    ${result} =    client.Set
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    --delete    /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0

    # get config from candidate
    ${result} =    client.GetFromCandidate
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    Desc1

    # run diff
    ${result} =    client.Diff    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1

    # commit
    ${result} =    client.Commit    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0

    # query fom intended store
    ${result} =    client.Get
    ...    ${datastore1}
    ...    --intended 
    ...    --path     /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    Desc1

    # query fom router
    ${result} =    gnmic.Get
    ...    ${devicename1}
    ...    --skip-verify 
    ...    -e     JSON_IETF 
    ...    -u     admin 
    ...    -p     NokiaSrl1! 
    ...    --format     flat 
    ...    --path     /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    Desc1

Configure Router gNMI - Create / Delete List item
    [Tags]    robot:continue-on-failure

    ${result} =    client.ListSchemas
    Should Be Equal As Integers    ${result.rc}    0

    # delete interface config in case there are some remains
    ${result} =    gnmic.Set
    ...    ${devicename1}
    ...    --skip-verify
    ...    -u    admin
    ...    -p    NokiaSrl1!
    ...    --delete    /interface[name=ethernet-1/1]
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    client.CreateDataStoreTarget
    ...    ${datastore1}
    ...    ${target-file1}
    ...    ${target-sync-file}
    ...    ${schema-name}
    ...    ${schema-vendor}
    ...    ${schema-version}
    Should Be Equal As Integers    ${result.rc}    0
    Wait Until Datastore connected    ${datastore1}    30s    1s

    # create candidate
    ${result} =    client.CreateCandidate    ${datastore1}    ${candidate-name}    ${owner}    ${priority}
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    client.GetCandidate    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}
    Should Contain    ${result.stdout}    ${candidate-name}

    # set config into candidate
    ${result} =    client.Set
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    --update     /interface[name=ethernet-1/1]/description:::Desc1 --update /interface[name=ethernet-1/1]/subinterface[index=0]/admin-state:::enable 
    ...    --update     /interface[name=ethernet-1/1]/subinterface[index=0]/description:::DescSub0
    Should Be Equal As Integers    ${result.rc}    0

    # get config from candidate
    ${result} =    client.GetFromCandidate
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1

    # get config from candidate
    ${result} =    client.GetFromCandidate
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    /interface[name=ethernet-1/1]/subinterface[index=0]
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    DescSub0

    # run diff
    ${result} =    client.Diff    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1
    Should Contain    ${result.stdout}    DescSub0

    # commit
    ${result} =    client.Commit    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0

    # query from router
    ${result} =    gnmic.Get
    ...    ${devicename1}
    ...    --skip-verify
    ...    -e    ASCII
    ...    -u    admin
    ...    -p    NokiaSrl1!
    ...    --format    flat
    ...    --path    /interface[name=ethernet-1/1]/description

    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1

    # query from router
    ${result} =    gnmic.Get
    ...    ${devicename1}
    ...    --skip-verify 
    ...    -e    ASCII 
    ...    -u    admin 
    ...    -p    NokiaSrl1! 
    ...    --format    flat 
    ...    --path    /interface[name=ethernet-1/1]/subinterface[index=0]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    DescSub0

    # DELETE
    # create candidate
    ${result} =    client.CreateCandidate    ${datastore1}    ${candidate-name}    ${owner}    ${priority}
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =    client.GetCandidate    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    ${datastore1}
    Should Contain    ${result.stdout}    ${candidate-name}

    # set config into candidate
    ${result} =    client.Set
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    --delete    /interface[name=ethernet-1/1]/description 
    ...    --delete    /interface[name=ethernet-1/1]/subinterface[index=0]/description
    Should Be Equal As Integers    ${result.rc}    0

    # get config from candidate
    ${result} =    client.GetFromCandidate
    ...    ${datastore1}
    ...    ${candidate-name}
    ...    /interface[name=ethernet-1/1]
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    Desc1
    Should Not Contain    ${result.stdout}    DescSub0

    # run diff
    ${result} =    client.Diff    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Desc1
    Should Contain    ${result.stdout}    DescSub0

    # commit
    ${result} =    client.Commit    ${datastore1}    ${candidate-name}
    Should Be Equal As Integers    ${result.rc}    0

    # query fom intended store
    ${result} =    client.Get
    ...    ${datastore1}
    ...    --intended
    ...    --path    /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    Desc1

    # query from router
    ${result} =    gnmic.Get
    ...    ${devicename1}
    ...    --skip-verify
    ...    -e    ASCII
    ...    -u     admin 
    ...    -p     NokiaSrl1!
    ...    --format    flat
    ...    --path    /interface[name=ethernet-1/1]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    Desc1

    # query fom intended store
    ${result} =    client.Get
    ...    ${datastore1}
    ...    --intended 
    ...    --path    /interface[name=ethernet-1/1]/subinterface[index=0]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    DescSub0

    # query from router
    ${result} =    gnmic.Get
    ...    ${devicename1}
    ...    --skip-verify
    ...    -e    ASCII
    ...    -u    admin
    ...    -p    NokiaSrl1!
    ...    --format    flat
    ...    --path    /interface[name=ethernet-1/1]/subinterface[index=0]/description
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    DescSub0
