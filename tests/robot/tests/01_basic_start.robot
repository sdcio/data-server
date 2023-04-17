*** Settings ***
Resource          ../keywords/server.robot
Resource          ../keywords/client.robot
Library           OperatingSystem
Library           String
Library           Process
Suite Setup       Setup    False    ${server-bin}    ${schema-server-config}    ${schema-server-process-alias}    ${schema-server-stderr}    ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}   
Suite Teardown    Teardown

*** Variables ***
${server-bin}    ./bin/server
${client-bin}    ./bin/client
${schema-server-config}    ./lab/distributed/schema-server.yaml
${data-server-config}    ./lab/distributed/data-server.yaml
${schema-server-ip}    127.0.0.1
${schema-server-port}    55000
${data-server-ip}    127.0.0.1
${data-server-port}    56000

# TARGET
${srlinux1-name}    srl1
${srlinux1-candidate}    default
${srlinux1-schema-name}    srl
${srlinux1-schema-version}    22.11.2
${srlinux1-schema-Vendor}    Nokia


# internal vars
${schema-server-process-alias}    ssa
${schema-server-stderr}    /tmp/ss-out
${data-server-process-alias}    dsa
${data-server-stderr}    /tmp/ds-out


*** Test Cases ***
Check Server State
    CheckServerState    ${schema-server-process-alias}    ${data-server-process-alias}