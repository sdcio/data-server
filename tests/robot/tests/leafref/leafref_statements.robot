*** Settings ***
Resource          ../../keywords/server.robot
Resource          ../../keywords/client.robot
Library           OperatingSystem
Library           String
Library           Process
Suite Setup       Setup Collocated    True    ${DATA-SERVER-BIN}    ${DATA-SERVER-CONFIG}    ${data-server-process-alias}    ${data-server-stderr}
Suite Teardown    Teardown

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
${srlinux1-schema-version}    22.11.2
${srlinux1-schema-vendor}    Nokia
${srlinux1-target-def}    ${CURDIR}/../must/srl1_target.json
${srlinux1-sync-def}    ${CURDIR}/../must/sync.json

${owner}    test
${priority}    100

# internal vars
${data-server-process-alias}    dsa
${data-server-stderr}    /tmp/ds-out



*** Test Cases ***
Check Server State
    CheckServerState Colocated    ${data-server-process-alias}

Create SRL1 Target
    ${result} =    CreateDataStoreTarget    ${srlinux1-name}    ${srlinux1-target-def}    ${srlinux1-sync-def}    ${srlinux1-schema-name}    ${srlinux1-schema-Vendor}     ${srlinux1-schema-version}
    Should Be Equal As Integers    ${result.rc}    0

BGP export-policy non-existing
    [Documentation]    This tests a LeafRef that contains an absolute path.
    LogLeafRefStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    network-instance[name=default]/protocols/bgp/group

    ${result} =    GetSchema    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    network-instance[name=default]/protocols/bgp/group
    Log    ${result.stdout}
    Log    ${result.stderr}

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/bgp_policy_fail.json

    Should Contain    ${result.stderr}    missing leaf reference
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

BGP export-policy existing
    [Documentation]    This tests a LeafRef that contains an absolute path.
    LogLeafRefStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    network-instance[name=default]/protocols/bgp/group

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/bgp_policy_pass.json
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

BGP dynamic-neighbor peer-group - fail
    [Documentation]    This is a LeafRef that contains a relative path.
    LogLeafRefStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    network-instance/protocols/bgp/dynamic-neighbors/accept/match/peer-group

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lref_relative_path_fail.json

    Should Contain    ${result.stderr}    missing leaf reference
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

BGP dynamic-neighbor peer-group - pass
    [Documentation]    This is a LeafRef that contains a relative path.
    LogLeafRefStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    network-instance/protocols/bgp/dynamic-neighbors/accept/match/peer-group

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lref_relative_path_pass.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

BGP danymic-neighbor interface - fail
    [Documentation]    This is a LeafRef that contains an absolute path with a key that utilizes the current() reference.
    LogLeafRefStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    network-instance/protocols/bgp/dynamic-neighbors/interface/interface-name

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lref_abs_current_fail.json

    Should Contain    ${result.stderr}    missing leaf reference
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

BGP danymic-neighbor interface - pass
    [Documentation]    This is a LeafRef that contains an absolute path with a key that utilizes the current() reference.
    LogLeafRefStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    network-instance/protocols/bgp/dynamic-neighbors/interface/interface-name

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lref_abs_current_pass.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

