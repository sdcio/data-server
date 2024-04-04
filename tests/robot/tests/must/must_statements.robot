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

${DATA-SERVER-CONFIG}    ${CURDIR}/data-server.yaml

${DATA-SERVER-IP}    127.0.0.1
${DATA-SERVER-PORT}    56000

${SCHEMA-SERVER-IP}    127.0.0.1
${SCHEMA-SERVER-PORT}    56000

# TARGET
${srlinux1-name}    srl1
${srlinux1-candidate}    default
${srlinux1-schema-name}    srl
${srlinux1-schema-version}    22.11.2
${srlinux1-schema-Vendor}    Nokia
${srlinux1-target-def}    ${CURDIR}/srl1_target.json
${srlinux1-sync-def}    ${CURDIR}/sync.json
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

Set system0 admin-state disable -> Fail
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=system0]/admin-state

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/system0_disable.json
    
    Should Contain    ${result.stderr}    admin-state must be enable
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set system0 admin-state enable -> Pass
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=system0]/admin-state

   ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/system0_enable.json
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set ethernet-1/1 admin-state disable -> Pass
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=system0]/admin-state

   ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/e11_disable.json
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set lag-type without 'interface[name=xyz]/lag/lacp' existence
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=lag1]/lag/lag-type

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lag_lacp_fail.json
    
    Should Contain    ${result.stderr}    lacp container must be configured when lag-type is lacp
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set lag-type with 'interface[name=xyz]/lag/lacp' existence
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=lag1]/lag/lacp/admin-key
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=lag1]/lag/lag-type

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lag_lacp_pass.json
    
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set auto-negotiate on non allowed interface
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-0/1]/ethernet/auto-negotiate

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/autoneg_fail.json

    Should Contain    ${result.stderr}    auto-negotiation not supported on this interface
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set auto-negotiate on allowed interface
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/auto-negotiate

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/autoneg_pass.json
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set auto-negotiation on breakout-mode port
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/auto-negotiate
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/breakout-mode/num-breakout-ports
    
    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/breakout_autoneg.json

    Should Contain    ${result.stderr}    auto-negotiate not configurable when breakout-mode is enabled
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set breakout-port num to 2 and port-speed to 25G
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/breakout-mode/breakout-port-speed
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/breakout-mode/num-breakout-ports

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/breakout_speed.json

    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stderr}    breakout-port-speed must be 100G when num-breakout-ports is 2

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}


Set interface ethernet l2cp-transparency lldp tunnel true
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/l2cp-transparency/lldp/tunnel

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lldp_tun_pass.json

    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set interface ethernet l2cp-transparency lldp tunnel true on lldp true interface
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/l2cp-transparency/lldp/tunnel

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lldp_tun_fail.json
    
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stderr}    this interface must not have lldp enabled

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set bfd for non existing subinterface
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    /bfd/subinterface/id

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/bfd.json

    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stderr}    Must be an existing subinterface name

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Check LAG interface member speed is set
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/aggregate-id

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lag_speed.json

    Should Contain    ${result.stderr}    member-speed must be configured on associated aggregate-id interface
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Check LAG interface vlan-tagging not set on member
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/aggregate-id

    ${result} =    SetIntent    ${srlinux1-name}    ${srlinux1-candidate}    ${owner}    ${priority}        ${CURDIR}/intents/lag_vlan_tagging.json

    Should Contain    ${result.stderr}    vlan-tagging and aggregate-id can not be configured together
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}