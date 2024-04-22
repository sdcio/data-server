*** Comments ***
# Copyright 2024 Nokia
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

*** Comments ***
This Test Suite creates a target in the dataserver that contains two sync configurations.

*** Settings ***
Resource   variables.robot
Resource   setup.robot
Resource    ${LibraryPath}/client.robot
Resource    ${LibraryPath}/server.robot
Resource    ${LibraryPath}/gnmic.robot
Library     OperatingSystem
Library     String
Library     Process

Suite Setup    SuiteSetup
Suite Teardown    SuiteTeardown


*** Test Cases ***
Create SRL1 Target
    ${result} =    CreateDataStore
    ...    ${srlinux1-name}
    ...    ${srlinux1-target-def}
    ...    ${srlinux1-sync-def-02}
    ...    ${srlinux1-schema-name}
    ...    ${srlinux1-schema-Vendor}
    ...    ${srlinux1-schema-version}
    Should Be Equal As Integers    ${result.rc}    0

    Wait Until Datastore connected    ${srlinux1-name}    30s    1s

Check Mgmt0 Description not present
    Wait Until Datastore Data Contains
    ...    ${srlinux1-name}
    ...    ${interface-mgmt0-description-path}
    ...    20s
    ...    1s
    ...    num notifications: 0

Check Ethernet-1/1 Description not present
    Wait Until Datastore Data Contains
    ...    ${srlinux1-name}
    ...    ${interface-eth1-1-description-path}
    ...    20s
    ...    1s
    ...    num notifications: 0
    
Set Mgmt0 interface description via gnmic
    ${result} =    gnmic.Set
    ...    ${srlinux1-hostname}
    ...    --skip-verify
    ...    -u    ${srl-user}
    ...    -p    ${srl-password}
    ...    --update-path    ${interface-mgmt0-description-path}
    ...    --update-value    ${interface-mgmt0-description-value}
    Should Be Equal As Integers    ${result.rc}    0

Set Ethernet-1/1 interface description via gnmic
    ${result} =    gnmic.Set
    ...    ${srlinux1-hostname}
    ...    --skip-verify
    ...    -u    ${srl-user}
    ...    -p    ${srl-password}
    ...    --update-path    ${interface-eth1-1-description-path}
    ...    --update-value    ${interface-eth1-1-description-value}
    Should Be Equal As Integers    ${result.rc}    0

Check Mgmt0 description appears in dataserver
    Wait Until Datastore Data Contains
    ...    ${srlinux1-name}
    ...    ${interface-mgmt0-description-path}
    ...    20s
    ...    1s
    ...    num notifications: 1
    ...    ${interface-mgmt0-description-value}

Remove Mgmt0 interface description via gnmic
    ${result} =    gnmic.Set
    ...    ${srlinux1-hostname}
    ...    --skip-verify
    ...    -u    ${srl-user}
    ...    -p    ${srl-password}
    ...    --delete    ${interface-mgmt0-description-path}
    Should Be Equal As Integers    ${result.rc}    0

Check Mgmt0 Description not present after gnmic based delete
    Wait Until Datastore Data Contains
    ...    ${srlinux1-name}
    ...    ${interface-mgmt0-description-path}
    ...    20s
    ...    1s
    ...    num notifications: 0

Check Ethernet-1/1 description appears in dataserver
    Wait Until Datastore Data Contains
    ...    ${srlinux1-name}
    ...    ${interface-eth1-1-description-path}
    ...    20s
    ...    1s
    ...    num notifications: 1
    ...    ${interface-eth1-1-description-value}

Remove Ethernet-1/1 interface description via gnmic
    ${result} =    gnmic.Set
    ...    ${srlinux1-hostname}
    ...    --skip-verify
    ...    -u    ${srl-user}
    ...    -p    ${srl-password}
    ...    --delete    ${interface-eth1-1-description-path}
    Should Be Equal As Integers    ${result.rc}    0

Check Ethernet-1/1 Description not present after gnmic based delete
    Wait Until Datastore Data Contains
    ...    ${srlinux1-name}
    ...    ${interface-eth1-1-description-path}
    ...    20s
    ...    1s
    ...    num notifications: 0

Delete Datastore Target
    ${result} =    DeleteDatastore
    ...    ${srlinux1-name}
    Should Be Equal As Integers    ${result.rc}    0

Check Datastore is gone
    ${result} =    ListDataStores
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    ${srlinux1-name}